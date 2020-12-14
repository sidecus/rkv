package raft

import (
	"errors"
	"os"
	"sync"

	"github.com/sidecus/raft/pkg/util"
)

const maxAppendEntriesCount = 20

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

// INodeRPCProvider defines node functions used by RPC
// These are invoked when an RPC request is received
type INodeRPCProvider interface {
	// Gets the node's id
	NodeID() int

	// Raft
	AppendEntries(*AppendEntriesRequest) (*AppendEntriesReply, error)
	RequestVote(*RequestVoteRequest) (*RequestVoteReply, error)
	InstallSnapshot(req *SnapshotRequest) (*AppendEntriesReply, error)

	// Data related
	Get(*GetRequest) (*GetReply, error)
	Execute(*StateMachineCmd) (*ExecuteReply, error)
}

// INode represents one raft node
type INode interface {
	// RPC
	INodeRPCProvider

	// Lifecycle
	Start()
	Stop()
}

// errorNoLeaderAvailable means there is no leader elected yet (or at least not known to current node)
var errorNoLeaderAvailable = errors.New("No leader currently available")

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
	votedFor      int          // resets on term change
	votes         map[int]bool // resets when entering candidate state
	logMgr        ILogManager
	peerMgr       IPeerManager

	// candidate only

	// timer
	timer IRaftTimer
}

// NewNode creates a new node
func NewNode(nodeID int, peers map[int]NodeInfo, sm IStateMachine, proxyFactory IPeerProxyFactory) INode {
	size := len(peers) + 1

	// TODO[sidecus]: Allow passing snapshot path as parameter instead of using current working directory
	cwd, err := os.Getwd()
	if err != nil {
		util.Panicf("Failed to get current working directory for snapshot. %s", err)
	}
	SetSnapshotPath(cwd)

	n := &node{
		mu:            sync.RWMutex{},
		clusterSize:   size,
		nodeID:        nodeID,
		nodeState:     Follower,
		currentTerm:   0,
		currentLeader: -1,
		votedFor:      -1,
		votes:         make(map[int]bool, size),
		logMgr:        NewLogMgr(nodeID, sm),
		peerMgr:       NewPeerManager(nodeID, peers, proxyFactory),
	}

	n.timer = NewRaftTimer(n.onTimer)

	return n
}

// NodeID gets the node's id
func (n *node) NodeID() int {
	return n.nodeID
}

// Start starts the node
func (n *node) Start() {
	n.mu.Lock()
	defer n.mu.Unlock()

	util.WriteInfo("Node%d starting...", n.nodeID)

	// Enter follower state
	n.timer.Start()
	n.enterFollowerState(n.nodeID, 0)
	n.refreshTimer()
}

// Stop stops a node
func (n *node) Stop() {
	n.mu.Lock()
	defer n.mu.Unlock()

	n.timer.Stop()
}

// Get gets values from state machine, no need to proxy
func (n *node) Get(req *GetRequest) (*GetReply, error) {
	// We don't need lock on the node since we are only accessing its logMgr property which never changes after startup.
	// However, this means the downstream component (statemachine) must implement proper locking. Our KVStore for exmaple, does it.
	// n.mu.RLock()
	// defer n.mu.RUnlock()

	var result *GetReply = nil

	ret, err := n.logMgr.Get(req.Params...)
	if err == nil {
		result = &GetReply{
			NodeID: n.nodeID,
			Data:   ret,
		}
	}

	return result, err
}

// Execute runs a command via the raft node
// If current node is the leader, it'll append the cmd to logs
// If current node is not the leader, it'll proxy the request to leader node
func (n *node) Execute(cmd *StateMachineCmd) (*ExecuteReply, error) {
	n.mu.Lock()

	leader := n.currentLeader
	success := false

	if n.nodeState == Leader {
		// Process the command, then replicate to followers
		n.logMgr.ProcessCmd(*cmd, n.currentTerm)
		n.replicateAndWait()
		success = true
	}

	n.mu.Unlock()

	if success {
		return &ExecuteReply{NodeID: n.nodeID, Success: true}, nil
	}

	// proxy to leader instead, no lock needed
	// otherwise we might have a deadlock between this and the leader when processing AppendEntries
	if leader == -1 {
		return nil, errorNoLeaderAvailable
	}
	return n.peerMgr.Execute(leader, cmd)
}

// AppendEntries handles raft RPC AE calls
func (n *node) AppendEntries(req *AppendEntriesRequest) (*AppendEntriesReply, error) {
	n.mu.Lock()
	defer n.mu.Unlock()

	n.tryFollowNewTerm(req.LeaderID, req.Term, true)

	// After above call, n.currentLeader has been updated accordingly if req.Term is the same or higher

	lastMatchIndex, prevMatch := -1, false
	if req.Term >= n.currentTerm {
		// only process logs when term is valid
		prevMatch = n.logMgr.ProcessLogs(req.PrevLogIndex, req.PrevLogTerm, req.Entries)
		if prevMatch {
			// logs are catching up - at least matching up to n.logMgr.lastIndex. record it and try to commit
			lastMatchIndex = n.logMgr.LastIndex()
			n.commitTo(req.LeaderCommit)
		}
	}

	return &AppendEntriesReply{
		Term:      n.currentTerm,
		NodeID:    n.nodeID,
		LeaderID:  n.currentLeader,
		Success:   prevMatch,
		LastMatch: lastMatchIndex, // this is only meaningful when Success is true
	}, nil
}

// InstallSnapshot installs a snapshot
func (n *node) InstallSnapshot(req *SnapshotRequest) (*AppendEntriesReply, error) {
	n.mu.Lock()
	defer n.mu.Unlock()

	n.tryFollowNewTerm(req.LeaderID, req.Term, true)

	// After above call, n.currentLeader has been updated accordingly if req.Term is the same or higher

	success := false
	lastMatchIndex := -1
	if req.Term >= n.currentTerm {
		// TODO[sidecus]: test case for the if branch
		if n.logMgr.SnapshotIndex() == req.SnapshotIndex && n.logMgr.SnapshotTerm() == req.SnapshotTerm {
			util.WriteInfo("T%d: Node%d ignoring already installed snapshot from Node%d upto L%d\n", n.currentTerm, n.nodeID, req.LeaderID, req.SnapshotIndex)
			success = true
			lastMatchIndex = req.SnapshotIndex
		} else {
			// only process logs when term is valid
			util.WriteInfo("T%d: Node%d installing snapshot from Node%d upto L%d\n", n.currentTerm, n.nodeID, req.LeaderID, req.SnapshotIndex)
			err := n.logMgr.InstallSnapshot(req.File, req.SnapshotIndex, req.SnapshotTerm)
			if err == nil {
				success = true
				lastMatchIndex = req.SnapshotIndex
				util.WriteInfo("T%d: Snapshot installed.\n", n.currentTerm)
			} else {
				util.WriteError("T%d: Install snapshot failed. %s\n", n.currentTerm, err)
			}
		}
	}

	return &AppendEntriesReply{
		Term:      n.currentTerm,
		NodeID:    n.nodeID,
		LeaderID:  n.currentLeader,
		Success:   success,
		LastMatch: lastMatchIndex, // this is only meaningful when Success is true
	}, nil

}

// RequestVote handles raft RPC RV calls
func (n *node) RequestVote(req *RequestVoteRequest) (*RequestVoteReply, error) {
	n.mu.Lock()
	defer n.mu.Unlock()

	// Here is what the paper says (5.2 and 5.4):
	// 1. if req.Term > currentTerm, convert to follower state (reset votedFor)
	// 2. if req.Term < currentTerm deny vote
	// 3. if req.Term >= currentTerm, and if votedFor is null or candidateId, and logs are up to date, grant vote
	// The req.Term == currentTerm situation AFAIK can only happen when we receive a duplicate RV request
	n.tryFollowNewTerm(req.CandidateID, req.Term, false)
	voteGranted := false
	if req.Term >= n.currentTerm && (n.votedFor == -1 || n.votedFor == req.CandidateID) {
		if req.LastLogIndex >= n.logMgr.LastIndex() && req.LastLogTerm >= n.logMgr.LastTerm() {
			n.votedFor = req.CandidateID
			voteGranted = true
			util.WriteInfo("T%d: \U0001f4e7 Node%d voted for Node%d\n", req.Term, n.nodeID, req.CandidateID)
		}
	}

	return &RequestVoteReply{
		Term:        n.currentTerm,
		NodeID:      n.nodeID,
		VotedTerm:   req.Term,
		VoteGranted: voteGranted,
	}, nil
}

// onTimer handles a timer event. Action is based on node's current state.
// This is run on the timer goroutine so we need to lock first
func (n *node) onTimer(state NodeState, term int) {
	n.mu.Lock()
	defer n.mu.Unlock()

	if state == n.nodeState && term == n.currentTerm {
		if n.nodeState == Follower || n.nodeState == Candidate {
			n.startElection()
		} else if n.nodeState == Leader {
			n.sendHeartbeat()
		}
	}
}

// enterLeaderState resets leader indicies. Caller should acquire writer lock
func (n *node) enterLeaderState() {
	n.nodeState = Leader
	n.currentLeader = n.nodeID

	// reset all follower's indicies
	n.peerMgr.ResetFollowerIndicies(n.logMgr.LastIndex())

	util.WriteInfo("T%d: \U0001f451 Node%d won election\n", n.currentTerm, n.nodeID)
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

// callback to be invoked when reply is received (on different goroutine so we need to acquire lock)
func (n *node) handleRequestVoteReply(reply *RequestVoteReply) {
	if reply == nil {
		return
	}

	n.mu.Lock()
	defer n.mu.Unlock()

	if n.tryFollowNewTerm(reply.NodeID, reply.Term, false) {
		// there is a higher term, no need to continue
		return
	}

	if reply.VotedTerm != n.currentTerm || n.nodeState != Candidate || !reply.VoteGranted {
		// stale vote or denied, ignore
		return
	}

	// record and count votes
	n.votes[reply.NodeID] = true
	if n.wonElection() {
		n.enterLeaderState()
		n.sendHeartbeat()
	}
}

// send heartbeat, caller should acquire at least reader lock
func (n *node) sendHeartbeat() {
	for _, peer := range n.peerMgr.GetAllPeers() {
		req := n.createAERequest(peer.nextIndex, 0)
		util.WriteTrace("T%d: \U0001f493 Node%d sending heartbeat to Node%d, prevIndex %d\n", n.currentTerm, n.nodeID, peer.NodeID, req.PrevLogIndex)

		// send heart beat (on different goroutines), response will be processed there
		n.peerMgr.AppendEntries(peer.NodeID, req, n.handleAppendEntriesReply)
	}

	// 5.2 - refresh timer
	n.refreshTimer()
}

// replicateAndWait replicates logs to all followers and wait for their response
func (n *node) replicateAndWait() {
	// TODO[sidecus]: We still need to wait for majority to agree, it's key for raft.
	// Right now we don't wait for response
	// 5.3, 5.4 states that the leader need to wait for response synchronously and then commit,
	// as well infinite retry upon failure.
	for _, follower := range n.peerMgr.GetAllPeers() {
		n.replicateToFollower(follower.NodeID)
	}
}

// replicateToFollower replicate logs to follower as needed
// This should be only be called by leader
func (n *node) replicateToFollower(targetNodeID int) bool {
	follower := n.peerMgr.GetPeer(targetNodeID)

	if follower.nextIndex > n.logMgr.LastIndex() {
		// nothing to replicate
		return false
	}

	if follower.nextIndex <= n.logMgr.SnapshotIndex() {
		// Send snapshot
		req := n.createSnapshotRequest()

		util.WriteInfo("T%d: Node%d sending snapshot to Node%d (L%d)\n", n.currentTerm, n.nodeID, targetNodeID, n.logMgr.SnapshotIndex())
		n.peerMgr.InstallSnapshot(follower.NodeID, req, n.handleAppendEntriesReply)
	} else {
		// there are logs to replicate, create AE request and send
		req := n.createAERequest(follower.nextIndex, maxAppendEntriesCount)
		minIdx := req.Entries[0].Index
		maxIdx := req.Entries[len(req.Entries)-1].Index

		util.WriteInfo("T%d: Node%d replicating logs to Node%d (L%d-L%d)\n", n.currentTerm, n.nodeID, targetNodeID, minIdx, maxIdx)
		n.peerMgr.AppendEntries(follower.NodeID, req, n.handleAppendEntriesReply)
	}

	return true
}

// handleAppendEntriesReply handles append entries reply. Need locking since this will be
// running on different goroutine for reply from each node
func (n *node) handleAppendEntriesReply(reply *AppendEntriesReply) {
	if reply == nil {
		return
	}

	n.mu.Lock()
	defer n.mu.Unlock()

	// If there is a higher term, follow and stop processing
	if n.tryFollowNewTerm(reply.LeaderID, reply.Term, false) {
		return
	}

	// Different from the paper, we don't wait for AE call to finish
	// so there is a chance that we are no longer the leader
	// In that case no need to continue
	if n.nodeState != Leader || n.currentTerm != reply.Term {
		return
	}

	followerID := reply.NodeID

	// 5.3 update leader indicies.
	// Kindly note: since we proces this asynchronously, we cannot use n.logMgr.lastIndex
	// to update follower indicies (it might be different from when the AE request is sent and when the reply is received).
	// Here we added a LastMatch field on AppendEntries reply. And it's used instead.
	n.peerMgr.UpdateFollowerMatchIndex(followerID, reply.Success, reply.LastMatch)

	// Check whether there are logs to commit and then replicate
	n.leaderCommit()
	n.replicateToFollower(followerID)
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
		if n.peerMgr.MajorityMatch(i) {
			commitIndex = i
			break
		}
	}

	n.commitTo(commitIndex)
}

// Called by both leader (upon AE reply) or follower (upon AE request)
func (n *node) commitTo(targetCommitIndex int) {
	if newCommit, newSnapshot := n.logMgr.Commit(targetCommitIndex); newCommit {
		util.WriteInfo("T%d: Node%d committed to L%d\n", n.currentTerm, n.nodeID, n.logMgr.CommitIndex())
		if newSnapshot {
			util.WriteInfo("T%d: Node%d created new snapshot L%d_T%d\n", n.currentTerm, n.nodeID, n.logMgr.SnapshotIndex(), n.logMgr.SnapshotTerm())
		}
	}
}

// createAERequest creates an AppendEntriesRequest with proper log payload
func (n *node) createAERequest(nextIdx int, count int) *AppendEntriesRequest {
	// make sure nextIdx is larger than n.logMgr.SnapshotIndex()
	// nextIdx <= n.logMgr.SnapshotIndex() will cause panic on log entry retrieval.
	// That scenario will only happen for heartbeats - and it's ok to change it to point to the first entry in the logs
	nextIdx = util.Max(nextIdx, n.logMgr.SnapshotIndex()+1)
	entris, prevIdx, prevTerm := n.logMgr.GetLogEntries(nextIdx, nextIdx+count)

	req := &AppendEntriesRequest{
		Term:         n.currentTerm,
		LeaderID:     n.nodeID,
		PrevLogIndex: prevIdx,
		PrevLogTerm:  prevTerm,
		Entries:      entris,
		LeaderCommit: n.logMgr.CommitIndex(),
	}

	return req
}

// createSnapshotRequest creates a snapshot request to send to follower
func (n *node) createSnapshotRequest() *SnapshotRequest {
	return &SnapshotRequest{
		Term:          n.currentTerm,
		LeaderID:      n.nodeID,
		SnapshotIndex: n.logMgr.SnapshotIndex(),
		SnapshotTerm:  n.logMgr.SnapshotTerm(),
		File:          n.logMgr.SnapshotFile(),
	}
}

// count votes for current node and term and return true if we won
func (n *node) wonElection() bool {
	total := 0
	for _, v := range n.votes {
		if v {
			total++
		}
	}
	return total > n.clusterSize/2
}

// setTerm sets a new term
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

// Refreshes timer based on current state
func (n *node) refreshTimer() {
	n.timer.Refresh(n.nodeState, n.currentTerm)
}
