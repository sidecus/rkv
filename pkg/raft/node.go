package raft

import (
	"errors"
	"os"
	"sync"

	"github.com/sidecus/raft/pkg/util"
)

const maxAppendEntriesCount = 20

// NodeInfo contains info for a peer node including id and endpoint
type NodeInfo struct {
	NodeID   int
	Endpoint string
}

// NodeState is the state of the node
type NodeState int

const (
	//NodeStateFollower state
	NodeStateFollower = 1
	// NodeStateCandidate state
	NodeStateCandidate = 2
	// NodeStateLeader state
	NodeStateLeader = 3
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

	// timer
	timer IRaftTimer
}

// NewNode creates a new node
func NewNode(nodeID int, peers map[int]NodeInfo, sm IStateMachine, proxyFactory IPeerProxyFactory) INode {
	size := len(peers) + 1
	if size < 3 {
		util.Panicf("To few nodes. Total count: %d", size)
	}

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
		nodeState:     NodeStateFollower,
		currentTerm:   0,
		currentLeader: -1,
		votedFor:      -1,
		votes:         make(map[int]bool, size),
		logMgr:        NewLogMgr(nodeID, sm),
	}

	n.timer = NewRaftTimer(n.onTimer)
	n.peerMgr = NewPeerManager(nodeID, peers, n.replicateData, proxyFactory)

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
	n.peerMgr.Start()
	n.enterFollowerState(n.nodeID, 0)
	n.refreshTimer()
}

// Stop stops a node
func (n *node) Stop() {
	n.mu.Lock()
	defer n.mu.Unlock()

	n.timer.Stop()
	n.peerMgr.Stop()
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

	isLeader := n.nodeState == NodeStateLeader
	leader := n.currentLeader
	success := false

	if isLeader {
		// Process the command
		n.logMgr.ProcessCmd(*cmd, n.currentTerm)

		// Send to followers and wait for response.
		req := n.createAERequest(n.logMgr.LastIndex(), 1)
		aeReplies := n.peerMgr.RunAndWaitAllPeers(func(peer *Peer) interface{} {
			reply, err := peer.AppendEntries(req)
			if err != nil {
				util.WriteVerbose("T%d: Replicating new entry to Node%d failed. %s\n", n.currentTerm, peer.NodeID, err)
				return nil
			}
			util.WriteVerbose("T%d: Replicating new entry to Node%d. Success:%t, lastMatch:%d\n", n.currentTerm, peer.NodeID, reply.Success, reply.LastMatch)
			return reply
		})

		// Update match index based on each reply
		for r := range aeReplies {
			reply := r.(*AppendEntriesReply)
			follower := n.peerMgr.GetPeer(reply.NodeID)
			follower.UpdateMatchIndex(reply.Success, reply.LastMatch)
		}

		// If we have majority matche the last entry, commit
		if n.peerMgr.QuorumReached(n.logMgr.LastIndex()) {
			n.commitTo(n.logMgr.LastIndex())
			success = true
		} else {
			util.WriteWarning("T%d: Node%d (leader) could not get quorum on new entry L%d", n.currentTerm, n.nodeID, n.logMgr.LastIndex())
		}
	}

	n.mu.Unlock()

	if isLeader {
		return &ExecuteReply{NodeID: n.nodeID, Success: success}, nil
	} else if leader == -1 {
		return nil, errorNoLeaderAvailable
	}

	// proxy to leader instead, no lock needed
	// otherwise we might have a deadlock between this and the leader when processing AppendEntries
	return n.peerMgr.GetPeer(leader).Execute(cmd)
}

// AppendEntries handles raft RPC AE calls
func (n *node) AppendEntries(req *AppendEntriesRequest) (*AppendEntriesReply, error) {
	n.mu.Lock()
	defer n.mu.Unlock()

	n.tryFollowNewTerm(req.LeaderID, req.Term, true)

	// After above call, n.currentLeader has been updated accordingly if req.Term is the same or higher
	util.WriteTrace("T%d: Received AE from Leader%d, prevIndex: %d, prevTerm: %d, entryCnt: %d", n.currentTerm, req.LeaderID, req.PrevLogIndex, req.PrevLogTerm, len(req.Entries))
	lastMatchIndex, prevMatch := req.PrevLogIndex, false
	if req.Term >= n.currentTerm {
		prevMatch = n.logMgr.ProcessLogs(req.PrevLogIndex, req.PrevLogTerm, req.Entries)
		if prevMatch {
			// logs are catching up - at least matching up to n.logMgr.lastIndex. Record it.
			// And try to commit based on leader commit
			lastMatchIndex = n.logMgr.LastIndex()
			n.commitTo(util.Min(req.LeaderCommit, n.logMgr.LastIndex()))
		}
	}

	return &AppendEntriesReply{
		Term:      n.currentTerm,
		NodeID:    n.nodeID,
		LeaderID:  n.currentLeader,
		Success:   prevMatch,
		LastMatch: lastMatchIndex,
	}, nil
}

// InstallSnapshot installs a snapshot
func (n *node) InstallSnapshot(req *SnapshotRequest) (*AppendEntriesReply, error) {
	n.mu.Lock()
	defer n.mu.Unlock()

	n.tryFollowNewTerm(req.LeaderID, req.Term, true)

	// After above call, n.currentLeader has been updated accordingly if req.Term is the same or higher

	success := false
	lastMatchIndex := req.SnapshotIndex
	if req.Term >= n.currentTerm {
		if n.logMgr.SnapshotIndex() == req.SnapshotIndex && n.logMgr.SnapshotTerm() == req.SnapshotTerm {
			util.WriteInfo("T%d: Node%d ignoring duplicate T%dL%d snapshot from Node%d\n", n.currentTerm, n.nodeID, req.SnapshotTerm, req.SnapshotIndex, req.LeaderID)
			success = true
		} else {
			// only process logs when term is valid
			util.WriteInfo("T%d: Node%d installing T%dL%d snapshot from Node%d\n", n.currentTerm, n.nodeID, req.SnapshotTerm, req.SnapshotIndex, req.LeaderID)
			err := n.logMgr.InstallSnapshot(req.File, req.SnapshotIndex, req.SnapshotTerm)
			if err == nil {
				success = true
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
		LastMatch: lastMatchIndex,
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
		if n.nodeState == NodeStateLeader {
			n.sendHeartbeat()
		} else {
			n.startElection()
		}
	}
}

// enter follower state and follows new leader (or potential leader)
func (n *node) enterFollowerState(sourceNodeID, newTerm int) {
	oldLeader := n.currentLeader
	n.nodeState = NodeStateFollower
	n.currentLeader = sourceNodeID
	n.setTerm(newTerm)

	if n.nodeID != sourceNodeID && oldLeader != n.currentLeader {
		util.WriteInfo("T%d: Node%d follows Node%d on new Term\n", n.currentTerm, n.nodeID, sourceNodeID)
	}
}

// enter candidate state
func (n *node) enterCandidateState() {
	n.nodeState = NodeStateCandidate
	n.currentLeader = -1
	n.setTerm(n.currentTerm + 1)

	// vote for self first
	n.votedFor = n.nodeID
	n.votes = make(map[int]bool, n.clusterSize)
	n.votes[n.nodeID] = true

	util.WriteInfo("T%d: \u270b Node%d starts election\n", n.currentTerm, n.nodeID)
}

// start an election, caller should acquire write lock
func (n *node) startElection() {
	n.enterCandidateState()

	// Request vote to all nodes and wait for replies
	req := &RequestVoteRequest{
		Term:         n.currentTerm,
		CandidateID:  n.nodeID,
		LastLogIndex: n.logMgr.LastIndex(),
		LastLogTerm:  n.logMgr.LastTerm(),
	}
	rvReplies := n.peerMgr.RunAndWaitAllPeers(func(peer *Peer) interface{} {
		reply, err := peer.RequestVote(req)
		if err != nil {
			return nil
		}
		util.WriteTrace("T%d: Vote received from Node%d, term:%d\n", n.currentTerm, reply.NodeID, reply.VotedTerm)
		return reply
	})

	// reset election timer before processing replies, don't do it after reply processing
	n.refreshTimer()

	// process replies
	n.handleRequestVoteReplies(rvReplies)
}

// handleRequestVoteReplies processes RV replies
func (n *node) handleRequestVoteReplies(replies chan interface{}) {
	for v := range replies {
		reply := v.(*RequestVoteReply)

		if n.tryFollowNewTerm(reply.NodeID, reply.Term, false) {
			return // there is a higher term, no need to continue
		}

		if !reply.VoteGranted {
			// stale vote or denied, ignore
			util.WriteTrace("T%d: Stale or ungranted vote received from Node%d, term:%d, voteGranted:%t\n", n.currentTerm, reply.NodeID, reply.VotedTerm, reply.VoteGranted)
			continue
		}

		// record and count votes
		n.votes[reply.NodeID] = true
	}

	if n.wonElection() {
		n.enterLeaderState()
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
		util.WriteInfo("T%d: Received new term from Node%d, newTerm:%d.\n", n.currentTerm, sourceNodeID, newTerm)
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

// commitTo tries to commit to the target commit index
// Called by both leader (upon AE reply) or follower (upon AE request)
func (n *node) commitTo(targetCommitIndex int) {
	if newCommit, newSnapshot := n.logMgr.CommitAndApply(targetCommitIndex); newCommit {
		util.WriteTrace("T%d: Node%d committed to L%d\n", n.currentTerm, n.nodeID, n.logMgr.CommitIndex())
		if newSnapshot {
			util.WriteInfo("T%d: Node%d created new snapshot L%d_T%d\n", n.currentTerm, n.nodeID, n.logMgr.SnapshotIndex(), n.logMgr.SnapshotTerm())
		}
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
