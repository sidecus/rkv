package raft

import (
	"errors"
	"os"
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

	// candidate only
	votes map[int]bool

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

	n := &node{
		mu:            sync.RWMutex{},
		clusterSize:   size,
		nodeID:        nodeID,
		nodeState:     Follower,
		currentTerm:   0,
		currentLeader: -1,
		votedFor:      -1,
		logMgr:        NewLogMgr(nodeID, sm, cwd),
		peerMgr:       NewPeerManager(nodeID, peers, proxyFactory),
		votes:         make(map[int]bool, size),
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

	n.timer.Start()

	// Enter follower state
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
		// 5.3, 5.4 states that the leader need to wait for response synchronously and then commit,
		// as well infinite retry upon failure.
		// We take a different approach here - we return eagerly and don't wait for follower's response synchronously
		// Instead commit is done asynchronously after replication (upon AE replies).
		n.logMgr.ProcessCmd(*cmd, n.currentTerm)
		for _, follower := range n.peerMgr.GetAllPeers() {
			n.replicateLogsTo(follower.NodeID)
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
	if newCommit, newSnapshot := n.logMgr.Commit(targetCommitIndex); newCommit {
		util.WriteInfo("T%d: Node%d committed to L%d\n", n.currentTerm, n.nodeID, n.logMgr.CommitIndex())
		if newSnapshot {
			util.WriteInfo("T%d: Node%d created new snapshot L%d_T%d\n", n.currentTerm, n.nodeID, n.logMgr.SnapshotIndex(), n.logMgr.SnapshotTerm())
		}
	}
}

// Refreshes timer based on current state
func (n *node) refreshTimer() {
	n.timer.Refresh(n.nodeState, n.currentTerm)
}
