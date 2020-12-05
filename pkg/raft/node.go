package raft

import (
	"errors"
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

// INodeLifeCycleProvider defines node lifecycle methods
type INodeLifeCycleProvider interface {
	Start()
	Stop()
}

// INodeRPCProvider defines node functions used by RPC
// These are invoked when an RPC request is received
type INodeRPCProvider interface {
	// Raft
	AppendEntries(*AppendEntriesRequest) (*AppendEntriesReply, error)
	RequestVote(*RequestVoteRequest) (*RequestVoteReply, error)

	// Data related
	Get(*GetRequest) (*GetReply, error)
	Execute(*StateMachineCmd) (*ExecuteReply, error)
}

// INode represents one raft node
type INode interface {
	// RPC
	INodeRPCProvider

	// Lifecycle
	INodeLifeCycleProvider
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

	n.timer = NewRaftTimer(n.onTimer)

	return n
}

// Start starts the node
func (n *node) Start() {
	n.mu.Lock()
	n.mu.Unlock()

	util.WriteInfo("Node%d starting...", n.nodeID)
	n.enterFollowerState(n.nodeID, 0)
	n.timer.Start()
}

// Stop stops a node
func (n *node) Stop() {
	n.timer.Stop()
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

// Get gets values from state machine, no need to proxy
func (n *node) Get(req *GetRequest) (*GetReply, error) {
	n.mu.RLock()
	defer n.mu.RUnlock()

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
		// TODO[sidecus]: low pri 5.3, 5.4 - wait for response and then commit.
		// For now we return eagerly and don't wait for follower's response synchronously
		// Instead commit is done asynchronously after replication (upon AE replies).
		n.logMgr.ProcessCmd(*cmd, n.currentTerm)
		for _, follower := range n.followers {
			n.replicateLogsTo(follower.nodeID)
		}
		success = true
	}

	n.mu.Unlock()

	if success {
		return &ExecuteReply{NodeID: n.nodeID, Success: true}, nil
	} else if leader == -1 {
		return nil, errorNoLeaderAvailable
	}

	// proxy to leader instead, no lock needed
	// otherwise we might have a deadlock between this and the leader when processing AppendEntries
	return n.peerMgr.Execute(leader, cmd)
}

// onTimer handles a timer event.
// Action is based on node's current state.
// This is run on the timer goroutine so we need lock
func (n *node) onTimer() {
	n.mu.Lock()
	defer n.mu.Unlock()

	if n.nodeState == Follower || n.nodeState == Candidate {
		n.startElection()
	} else if n.nodeState == Leader {
		n.sendHeartbeat()
	}
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

// Called by both leader (upon AE reply) or follower (upon AE request)
func (n *node) commitTo(targetCommitIndex int) {
	if n.logMgr.Commit(targetCommitIndex) {
		util.WriteInfo("T%d: Node%d committed to L%d\n", n.currentTerm, n.nodeID, n.logMgr.CommitIndex())
	}
}

// Refreshes timer based on current state
func (n *node) refreshTimer() {
	n.timer.Refresh(n.nodeState)
}
