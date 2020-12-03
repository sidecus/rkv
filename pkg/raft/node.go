package raft

import (
	"sync"

	"github.com/sidecus/raft/pkg/util"
)

// NodeState is the state of the node
type NodeState int

const (
	//Follower state
	Follower = 1
	// Candidate state
	Candidate = 2
	// Leader state
	Leader = 3
)

// INode represents one raft node
type INode interface {
	// Raft
	AppendEntries(*AppendEntriesRequest) (*AppendEntriesReply, error)
	RequestVote(*RequestVoteRequest) (*RequestVoteReply, error)
	OnTimer()

	// Data related
	Get(*GetRequest) (*GetReply, error)
	Execute(*StateMachineCmd) (*ExecuteReply, error)

	// Lifecycle
	Start()
	Stop()
}

// node A raft node
type node struct {
	// node lock
	mu sync.RWMutex

	// data for all states
	clusterSize   int
	nodeID        int
	nodeState     NodeState
	currentTerm   int
	currentLeader int
	votedFor      int // resets on term change
	logMgr        ILogManager
	peerMgr       IPeerManager

	// leader only
	followers followerInfo

	// candidate only
	votes map[int]bool

	// timer
	timer IRaftTimer
}

// NewNode creates a new node
func NewNode(nodeID int, peers map[int]PeerInfo, sm IStateMachine, proxyFactory IPeerProxyFactory) INode {
	size := len(peers) + 1

	n := &node{
		mu:            sync.RWMutex{},
		clusterSize:   size,
		nodeID:        nodeID,
		nodeState:     Follower,
		currentTerm:   0,
		currentLeader: -1,
		votedFor:      -1,
		logMgr:        NewLogMgr(sm),
		peerMgr:       NewPeerManager(peers, proxyFactory),
		followers:     createFollowers(nodeID, peers),
		votes:         make(map[int]bool, size),
	}

	n.timer = newRaftTimer(n)

	return n
}

// OnTimer handles a timer event.
// Action is based on node's current state
func (n *node) OnTimer() {
	n.mu.Lock()
	defer n.mu.Unlock()

	if n.nodeState == Follower || n.nodeState == Candidate {
		n.startElection()
	} else if n.nodeState == Leader {
		n.sendHeartbeat()
	}
}

// Start starts the node
func (n *node) Start() {
	n.mu.Lock()
	n.mu.Unlock()

	util.WriteInfo("Node%d starting...", n.nodeID)
	n.enterFollowerState(n.nodeID, 0)
	n.timer.Start()
}

func (n *node) Stop() {
	n.timer.Stop()
}

// enter follower state and follows new leader (or potential leader)
func (n *node) enterFollowerState(sourceNodeID, newTerm int) {
	oldLeader := n.currentLeader
	n.nodeState = Follower
	n.currentLeader = sourceNodeID
	n.setTerm(newTerm)

	if n.nodeID != sourceNodeID && oldLeader != n.currentLeader {
		util.WriteInfo("T%d: Node%d follows Node%d on new Term\n", n.currentTerm, n.nodeID, sourceNodeID)
	}
}

// enter candidate state
func (n *node) enterCandidateState() {
	n.nodeState = Candidate
	n.currentLeader = -1
	n.setTerm(n.currentTerm + 1)

	// vote for self first
	n.votedFor = n.nodeID
	n.votes = make(map[int]bool, n.clusterSize)
	n.votes[n.nodeID] = true

	util.WriteInfo("T%d: \u270b Node%d starts election\n", n.currentTerm, n.nodeID)
}

// enterLeaderState resets leader indicies. Caller should acquire writer lock
func (n *node) enterLeaderState() {
	n.nodeState = Leader
	n.currentLeader = n.nodeID

	// reset all follower's indicies
	n.followers.resetAllIndices(n.logMgr.LastIndex())

	util.WriteInfo("T%d: \U0001f451 Node%d won election\n", n.currentTerm, n.nodeID)
}

func (n *node) setTerm(newTerm int) {
	if newTerm < n.currentTerm {
		util.Panicf("can't set new term %d, which is less than current term %d\n", newTerm, n.currentTerm)
	}

	if newTerm > n.currentTerm {
		// reset vote on higher term
		n.votedFor = -1
	}

	n.currentTerm = newTerm
}

// start an election, caller should acquire write lock
func (n *node) startElection() {
	n.enterCandidateState()

	// create RV request
	req := &RequestVoteRequest{
		Term:         n.currentTerm,
		CandidateID:  n.nodeID,
		LastLogIndex: n.logMgr.LastIndex(),
		LastLogTerm:  n.logMgr.LastTerm(),
	}

	// request vote (on different goroutines), response will be processed there
	n.peerMgr.BroadcastRequestVote(req, func(reply *RequestVoteReply) { n.handleRequestVoteReply(reply) })

	n.refreshTimer()
}

// send heartbeat, caller should acquire at least reader lock
func (n *node) sendHeartbeat() {
	// create empty AE request
	req := n.logMgr.CreateAERequest(n.currentTerm, n.nodeID, n.logMgr.LastIndex()+1)
	util.WriteTrace("T%d: \U0001f493 Node%d sending heartbeat\n", n.currentTerm, n.nodeID)

	// send heart beat (on different goroutines), response will be processed there
	n.peerMgr.BroadcastAppendEntries(
		req,
		func(reply *AppendEntriesReply) {
			n.handleAppendEntriesReply(reply)
		})

	// 5.2 - refresh timer
	n.refreshTimer()
}

// Check peer's term and follow if needed. This will be called upon all RPC request and responses.
// Returns true if we are following the given node and term
func (n *node) tryFollowNewTerm(sourceNodeID, newTerm int, isAppendEntries bool) bool {
	follow := false
	if newTerm > n.currentTerm {
		// Follow newer term right away. sourceNodeID might not be the new leader, but it potentially
		// has better knowledge of the leader than us
		follow = true
	} else if newTerm == n.currentTerm && isAppendEntries {
		// For AE calls, we should follow even term is the same
		follow = true
	}

	if follow {
		n.enterFollowerState(sourceNodeID, newTerm)
		n.refreshTimer()
	}

	return follow
}

// replicateLogsTo replicate logs to follower as needed
// This should be only be called by leader
func (n *node) replicateLogsTo(targetNodeID int) {
	target := n.followers[targetNodeID]

	if target.nextIndex > n.logMgr.LastIndex() {
		// nothing to replicate
		return
	}

	// there are logs to replicate, create AE request and send
	req := n.logMgr.CreateAERequest(n.currentTerm, n.nodeID, target.nextIndex)
	minIdx := req.Entries[0].Index
	maxIdx := req.Entries[len(req.Entries)-1].Index
	util.WriteInfo("T%d: Node%d replicating logs to Node%d (log#%d-log#%d)\n", n.currentTerm, n.nodeID, targetNodeID, minIdx, maxIdx)

	n.peerMgr.AppendEntries(
		target.nodeID,
		req,
		func(reply *AppendEntriesReply) {
			n.handleAppendEntriesReply(reply)
		})
}

// leaderCommit commits logs as needed by checking each follower's match index
// This should only be called by leader
func (n *node) leaderCommit() {
	commitIndex := n.logMgr.CommitIndex()
	for i := n.logMgr.LastIndex(); i > n.logMgr.CommitIndex(); i-- {
		entry := n.logMgr.GetLogEntry(i)

		if entry.Term < n.currentTerm {
			// 5.4.2 Raft doesn't allow committing of previous terms
			// A leader shall only commit entries added by itself, and term is the indication of ownership
			break
		} else if entry.Term > n.currentTerm {
			// This will never happen, adding for safety purpose
			continue
		}

		// If we reach here, we can safely declare sole ownership of the ith entry
		if n.followers.majorityMatch(i) {
			commitIndex = i
			break
		}
	}

	n.commitTo(commitIndex)
}

// Called by both leader (upon AE reply) or follower (upon AE request)
func (n *node) commitTo(targetCommitIndex int) {
	if targetCommitIndex >= 0 && n.logMgr.Commit(targetCommitIndex) {
		util.WriteInfo("T%d: Node%d committed to log%d\n", n.currentTerm, n.nodeID, n.logMgr.CommitIndex())
	}
}

// count votes for current node and term
func (n *node) countVotes() int {
	total := 0
	for _, v := range n.votes {
		if v {
			total++
		}
	}

	return total
}

// Refreshes timer based on current state
func (n *node) refreshTimer() {
	n.timer.Refresh(n.nodeState)
}
