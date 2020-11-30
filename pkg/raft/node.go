package raft

import (
	"errors"
	"sync"
)

// ErrorNoLeaderAvailable means there is no leader elected yet (or at least not known to current node)
var ErrorNoLeaderAvailable = errors.New("No leader currently available")

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
	votedFor      int
	logMgr        *logManager
	stateMachine  IStateMachine
	proxy         IPeerProxy

	// leader only
	followers followerInfo

	// candidate only
	votes map[int]bool
}

// NewNode creates a new node
func NewNode(id int, nodeIDs []int, sm IStateMachine, proxy IPeerProxy) INode {
	size := len(nodeIDs)
	n := &node{
		mu:            sync.RWMutex{},
		clusterSize:   size,
		nodeID:        id,
		nodeState:     Follower,
		currentTerm:   0,
		currentLeader: -1,
		votedFor:      -1,
		logMgr:        newLogMgr(sm),
		stateMachine:  sm,
		proxy:         proxy,
		followers:     createFollowerInfo(id, nodeIDs),
		votes:         make(map[int]bool, size),
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
	n.nodeState = Follower
	n.currentTerm = newTerm
	n.currentLeader = sourceNodeID

	n.votedFor = -1

	if n.nodeID != sourceNodeID {
		writeInfo("T%d: Node%d follows Node%d on new Term\n", n.currentTerm, n.nodeID, sourceNodeID)
	}
}

// enter candidate state
func (n *node) enterCandidateState() {
	n.nodeState = Candidate
	n.currentTerm++
	n.currentLeader = -1

	// vote for self first
	n.votedFor = n.nodeID
	n.votes = make(map[int]bool, n.clusterSize)
	n.votes[n.nodeID] = true

	writeTrace("T%d: \u270b Node%d starts election\n", n.currentTerm, n.nodeID)
}

// enterLeaderState resets leader indicies. Caller should acquire writer lock
func (n *node) enterLeaderState() {
	n.nodeState = Leader
	n.currentLeader = n.nodeID
	// reset leader indicies
	n.followers.reset(n.logMgr.lastIndex)

	writeInfo("T%d: \U0001f451 Node%d won election\n", n.currentTerm, n.nodeID)
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
	n.proxy.RequestVote(req, func(reply *RequestVoteReply) { n.handleRequestVoteReply(reply) })

	n.refreshTimer()
}

// send heartbeat, caller should acquire at least reader lock
func (n *node) sendHeartbeat() {
	// create empty AE request
	req := n.logMgr.createAERequest(n.currentTerm, n.nodeID, n.logMgr.lastIndex+1)
	writeTrace("T%d: \U0001f493 Node%d sending heartbeat\n", n.currentTerm, n.nodeID)

	// send heart beat (on different go routines), response will be processed there
	n.proxy.BroadcastAppendEntries(
		req,
		func(reply *AppendEntriesReply) {
			n.handleAppendEntriesReply(reply)
		})

	// 5.2 - refresh timer
	n.refreshTimer()
}

// Check peer's term and follow if it's higher. This will be called upon all RPC request and responses.
// Returns true if we are following a higher term
func (n *node) tryFollowHigherTerm(sourceNodeID, newTerm int, refreshTimerOnSameTerm bool) bool {
	ret := false
	if newTerm > n.currentTerm {
		// Treat the source node as the potential leader for the higher term
		// Till real heartbeat for that term arrives, the source node potentially knows better about the leader than us
		n.enterFollowerState(sourceNodeID, newTerm)
		n.refreshTimer()
		ret = true
	} else if newTerm == n.currentTerm && refreshTimerOnSameTerm {
		n.refreshTimer()
	}

	return ret
}

// replicateLogsIfAny replicate logs to follower as needed
// This should be only be called by leader
func (n *node) replicateLogsIfAny(targetNodeID int) {
	follower := n.followers[targetNodeID]

	if follower.nextIndex > n.logMgr.lastIndex {
		// nothing to replicate
		return
	}

	// there are logs to replicate, create AE request and send
	req := n.logMgr.createAERequest(n.currentTerm, n.nodeID, follower.nextIndex)
	minIdx := req.Entries[0].Index
	maxIdx := req.Entries[len(req.Entries)-1].Index
	writeInfo("T%d: Node%d replicating logs to Node%d (log%d-log%d)\n", n.currentTerm, n.nodeID, targetNodeID, minIdx, maxIdx)

	n.proxy.AppendEntries(
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

		if n.followers.majorityMatch(i) {
			commitIndex = i
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
