package raft

import (
	"sync"
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
	logMgr        *logManager
	stateMachine  IStateMachine
	peerMgr       *PeerManager

	// leader only
	followerIndicies followerIndicies

	// candidate only
	votes map[int]bool
}

// NewNode creates a new node
func NewNode(nodeID int, peers map[int]PeerInfo, sm IStateMachine, proxyFactory IPeerProxyFactory) INode {
	size := len(peers) + 1

	n := &node{
		mu:               sync.RWMutex{},
		clusterSize:      size,
		nodeID:           nodeID,
		nodeState:        Follower,
		currentTerm:      0,
		currentLeader:    -1,
		votedFor:         -1,
		logMgr:           newLogMgr(sm),
		stateMachine:     sm,
		peerMgr:          NewPeerManager(peers, proxyFactory),
		followerIndicies: createFollowerIndicies(nodeID, peers),
		votes:            make(map[int]bool, size),
	}

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

	writeInfo("Node%d starting...", n.nodeID)
	n.enterFollowerState(n.nodeID, 0)
	startRaftTimer(n)
}

func (n *node) Stop() {
	// We don't wait for the goroutine to finish, just no need
	stopRaftTimer()
}

// Refreshes timer based on current state
func (n *node) refreshTimer() {
	refreshRaftTimer(n.nodeState)
}

// enter follower state and follows new leader (or potential leader)
func (n *node) enterFollowerState(sourceNodeID, newTerm int) {
	oldLeader := n.currentLeader
	n.nodeState = Follower
	n.currentLeader = sourceNodeID
	n.setTerm(newTerm)

	if n.nodeID != sourceNodeID && oldLeader != n.currentLeader {
		writeInfo("T%d: Node%d follows Node%d on new Term\n", n.currentTerm, n.nodeID, sourceNodeID)
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

	writeInfo("T%d: \u270b Node%d starts election\n", n.currentTerm, n.nodeID)
}

// enterLeaderState resets leader indicies. Caller should acquire writer lock
func (n *node) enterLeaderState() {
	n.nodeState = Leader
	n.currentLeader = n.nodeID

	// reset leader indicies
	n.followerIndicies.reset(n.logMgr.lastIndex)

	writeInfo("T%d: \U0001f451 Node%d won election\n", n.currentTerm, n.nodeID)
}

func (n *node) setTerm(newTerm int) {
	if newTerm < n.currentTerm {
		logger.Panicf("can't set new term %d, which is less than current term %d", newTerm, n.currentTerm)
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
		LastLogIndex: n.logMgr.lastIndex,
		LastLogTerm:  n.logMgr.lastTerm,
	}

	// request vote (on different go routines), response will be processed on different go routine
	n.peerMgr.BroadcastRequestVote(req, func(reply *RequestVoteReply) { n.handleRequestVoteReply(reply) })

	n.refreshTimer()
}

// send heartbeat, caller should acquire at least reader lock
func (n *node) sendHeartbeat() {
	// create empty AE request
	req := n.logMgr.createAERequest(n.currentTerm, n.nodeID, n.logMgr.lastIndex+1)
	writeTrace("T%d: \U0001f493 Node%d sending heartbeat\n", n.currentTerm, n.nodeID)

	// send heart beat (on different go routines), response will be processed there
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
		// For AE calls, we should follow as well
		follow = true
	}

	if follow {
		n.enterFollowerState(sourceNodeID, newTerm)
		n.refreshTimer()
	}

	return false
}

// replicateLogsIfAny replicate logs to follower as needed
// This should be only be called by leader
func (n *node) replicateLogsIfAny(targetNodeID int) {
	follower := n.followerIndicies[targetNodeID]

	if follower.nextIndex > n.logMgr.lastIndex {
		// nothing to replicate
		return
	}

	// there are logs to replicate, create AE request and send
	req := n.logMgr.createAERequest(n.currentTerm, n.nodeID, follower.nextIndex)
	minIdx := req.Entries[0].Index
	maxIdx := req.Entries[len(req.Entries)-1].Index
	writeInfo("T%d: Node%d replicating logs to Node%d (log#%d-log#%d)\n", n.currentTerm, n.nodeID, targetNodeID, minIdx, maxIdx)

	n.peerMgr.AppendEntries(
		follower.nodeID,
		req,
		func(reply *AppendEntriesReply) {
			n.handleAppendEntriesReply(reply)
		})
}

// commitIfAny commits logs as needed by checking each follower's match index
// This should only be called by leader
func (n *node) commitIfAny() {
	commitIndex := n.logMgr.commitIndex
	for i := n.logMgr.lastIndex; i > n.logMgr.commitIndex; i-- {
		if n.logMgr.logs[i].Term != n.currentTerm {
			continue
		}

		if n.followerIndicies.majorityMatch(i) {
			commitIndex = i
			break
		}
	}

	n.commitTo(commitIndex)
}

// Called by both leader (upon AE reply) or follower (upon AE request)
func (n *node) commitTo(commitIndex int) {
	if commitIndex >= 0 && n.logMgr.commit(commitIndex) {
		writeInfo("T%d: Node%d committed to log%d\n", n.currentTerm, n.nodeID, commitIndex)
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
