package raft

import (
	"context"
	"errors"
	"sync"

	"github.com/sidecus/raft/pkg/util"
)

var errorInsufficientPeers = errors.New("At least 2 peers required to make a 3 node cluster")
var errorCurrentNodeInPeers = errors.New("current node should not exist as one of its peers")
var errorInvalidPeerNodeID = errors.New("peer node has invalid node ID")
var errorNoLeaderAvailable = errors.New("No leader currently available")

// NodeState is the state of the node
type NodeState int

const (
	//NodeStateFollower state
	NodeStateFollower = NodeState(1)
	// NodeStateCandidate state
	NodeStateCandidate = NodeState(2)
	// NodeStateLeader state
	NodeStateLeader = NodeState(3)
)

// NodeInfo contains info for a peer node including id and endpoint
type NodeInfo struct {
	NodeID   int
	Endpoint string
}

// INodeRPCProvider interface defines the RPC related methods for a node
type INodeRPCProvider interface {
	// AppendEntries appends entries
	AppendEntries(ctx context.Context, req *AppendEntriesRequest) (*AppendEntriesReply, error)

	// RequestVote requests for vote
	RequestVote(ctx context.Context, req *RequestVoteRequest) (*RequestVoteReply, error)

	// InstallSnapshot installs a snapshot.
	InstallSnapshot(ctx context.Context, req *SnapshotRequest) (*AppendEntriesReply, error)

	// Get gets a committed and applied value from state machine
	Get(ctx context.Context, req *GetRequest) (*GetReply, error)

	// Execute runs a write operation
	Execute(ctx context.Context, cmd *StateMachineCmd) (*ExecuteReply, error)
}

// INode represents one raft node
type INode interface {
	// Start starts the node
	Start()

	// Stop stops the node
	Stop()

	// NodeID returns the node's ID
	NodeID() int

	// OnSnapshotPart is invoked when receiving a snapshot part (full snapshot might still be pending)
	OnSnapshotPart(part *SnapshotRequestHeader) bool

	// Node RPC functions
	INodeRPCProvider
}

// node define struct for a raft node implementing INode interface
type node struct {
	mu sync.RWMutex

	clusterSize   int
	nodeID        int
	nodeState     NodeState
	currentTerm   int
	currentLeader int
	votedFor      int          // resets on term change
	votes         map[int]bool // resets when entering candidate state
	logMgr        ILogManager
	peerMgr       IPeerManager
	timer         IRaftTimer
}

// NewNode creates a new node
func NewNode(nodeID int, peers map[int]NodeInfo, sm IStateMachine, proxyFactory IPeerProxyFactory) (INode, error) {
	if err := validateCluster(nodeID, peers); err != nil {
		return nil, err
	}
	size := len(peers) + 1

	n := &node{
		mu:            sync.RWMutex{},
		clusterSize:   size,
		nodeID:        nodeID,
		nodeState:     NodeStateFollower,
		currentTerm:   0,
		currentLeader: -1,
		votedFor:      -1,
		votes:         make(map[int]bool, size),
		logMgr:        newLogMgr(nodeID, sm),
	}

	n.timer = newRaftTimer(n.onTimer)
	n.peerMgr = newPeerManager(peers, n.replicateData, proxyFactory)

	return n, nil
}

// validateCluster validates params for the raft cluster
func validateCluster(nodeID int, peers map[int]NodeInfo) error {
	if len(peers) < 2 {
		return errorInsufficientPeers
	}

	for i, p := range peers {
		if p.NodeID == nodeID {
			return errorCurrentNodeInPeers
		} else if p.NodeID != i {
			return errorInvalidPeerNodeID
		}
	}

	return nil
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
	n.timer.start()
	n.peerMgr.start()

	// Enter follower state
	n.enterFollowerState(n.nodeID, 0)
}

// Stop stops a node
func (n *node) Stop() {
	n.mu.Lock()
	defer n.mu.Unlock()

	n.timer.stop()
	n.peerMgr.stop()
}

// Get gets values from state machine, no need to proxy
func (n *node) Get(ctx context.Context, req *GetRequest) (result *GetReply, err error) {
	n.mu.RLock()
	defer n.mu.RUnlock()

	var ret interface{}
	if ret, err = n.logMgr.Get(req.Params...); err != nil {
		return
	}

	result = &GetReply{
		NodeID: n.nodeID,
		Data:   ret,
	}

	return
}

// Execute runs a command via the raft node
// If current node is the leader, it'll append the cmd to logs
// If current node is not the leader, it'll proxy the request to leader node
func (n *node) Execute(ctx context.Context, cmd *StateMachineCmd) (*ExecuteReply, error) {
	n.mu.RLock()
	state := n.nodeState
	leader := n.currentLeader
	n.mu.RUnlock()

	switch {
	case leader == -1:
		// no leader available now, error out
		return nil, errorNoLeaderAvailable
	case state != NodeStateLeader:
		// We are not the leader, proxy to leader
		return n.peerMgr.getPeer(leader).Execute(ctx, cmd)
	default:
		// we are the leader
		return n.leaderExecute(ctx, cmd)
	}
}

// AppendEntries handles raft RPC AE calls
func (n *node) AppendEntries(ctx context.Context, req *AppendEntriesRequest) (*AppendEntriesReply, error) {
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
func (n *node) InstallSnapshot(ctx context.Context, req *SnapshotRequest) (*AppendEntriesReply, error) {
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
			if err := n.logMgr.InstallSnapshot(req.File, req.SnapshotIndex, req.SnapshotTerm); err != nil {
				util.WriteError("T%d: Install snapshot failed. %s\n", n.currentTerm, err)
			} else {
				success = true
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

// OnSnapshotPart is invoked when a snapshot part is received. Returns false if we don't want to continue (e.g. lower term)
func (n *node) OnSnapshotPart(part *SnapshotRequestHeader) bool {
	n.mu.Lock()
	defer n.mu.Unlock()

	// Teated in the same way as AE request
	return n.tryFollowNewTerm(part.LeaderID, part.Term, true)
}

// RequestVote handles raft RPC RV calls
func (n *node) RequestVote(ctx context.Context, req *RequestVoteRequest) (*RequestVoteReply, error) {
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
func (n *node) onTimer(state NodeState, term int) {
	n.mu.RLock()
	var fn func()
	if state == n.nodeState && term == n.currentTerm {
		if n.nodeState == NodeStateLeader {
			fn = n.sendHeartbeat
		} else {
			fn = n.startElection
		}
	}
	n.mu.RUnlock()

	if fn != nil {
		// fn needs to be concurrency safe
		fn()
	}
}

// enter follower state and follows new leader (or potential leader)
// also resets vote timer
func (n *node) enterFollowerState(sourceNodeID, newTerm int) {
	oldLeader := n.currentLeader
	n.nodeState = NodeStateFollower
	n.currentLeader = sourceNodeID
	n.setTerm(newTerm)

	// refresh timer
	n.refreshTimer()

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

	// reset timer
	n.refreshTimer()

	util.WriteInfo("T%d: \u270b Node%d starts election\n", n.currentTerm, n.nodeID)
}

// start an election
func (n *node) startElection() {
	runCampaign := n.prepareCampaign()
	votes := runCampaign()
	n.countVotes(votes)
}

// prepare election prepares node for election and returns a elect func
func (n *node) prepareCampaign() func() <-chan *RequestVoteReply {
	n.mu.Lock()
	defer n.mu.Unlock()

	n.enterCandidateState()

	req := &RequestVoteRequest{
		Term:         n.currentTerm,
		CandidateID:  n.nodeID,
		LastLogIndex: n.logMgr.LastIndex(),
		LastLogTerm:  n.logMgr.LastTerm(),
	}

	currentTerm := n.currentTerm

	return func() <-chan *RequestVoteReply {
		ctx, cancel := context.WithTimeout(context.Background(), rpcTimeOut)
		defer cancel()

		rvReplies := make(chan *RequestVoteReply, n.clusterSize)
		defer close(rvReplies)

		n.peerMgr.waitAll(func(peer *Peer, wg *sync.WaitGroup) {
			go func() {
				reply, err := peer.RequestVote(ctx, req)
				if err == nil {
					util.WriteInfo("T%d: Vote reply from Node%d, granted:%v\n", currentTerm, reply.NodeID, reply.VoteGranted)
					rvReplies <- reply
				}
				wg.Done()
			}()
		})

		return rvReplies
	}
}

// countVotes processes RV replies
func (n *node) countVotes(replies <-chan *RequestVoteReply) {
	n.mu.Lock()
	defer n.mu.Unlock()

	for reply := range replies {
		if n.tryFollowNewTerm(reply.NodeID, reply.Term, false) {
			return // there is a higher term, no need to continue
		}

		if n.nodeState != NodeStateCandidate || reply.VotedTerm != n.currentTerm || !reply.VoteGranted {
			// stale vote or denied, ignore
			util.WriteTrace("T%d: Stale or ungranted vote received from Node%d, term:%d, voteGranted:%t\n", n.currentTerm, reply.NodeID, reply.VotedTerm, reply.VoteGranted)
			continue
		}

		// record and count votes
		n.votes[reply.NodeID] = true
	}

	// enter leader state if won
	if n.wonElection() {
		n.enterLeaderState()
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
		// For AE calls, we should (re)follow when term is the same
		follow = true
	}

	if follow {
		n.enterFollowerState(sourceNodeID, newTerm)
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
	n.timer.reset(n.nodeState, n.currentTerm)
}
