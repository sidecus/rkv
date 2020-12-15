package raft

import (
	"errors"

	"github.com/sidecus/raft/pkg/util"
)

const nextIndexFallbackStep = 5

// followerStatus manages nextIndex and matchIndex for a follower
type followerStatus struct {
	nextIndex  int
	matchIndex int
}

// NodeInfo contains info for a peer node including id and endpoint
type NodeInfo struct {
	NodeID   int
	Endpoint string
}

// IPeerProxy defines the RPC client interface for a specific peer nodes
// It's an abstraction layer so that concrete implementation (RPC or REST) is detached
type IPeerProxy interface {
	// AppendEntries calls a peer node to append entries.
	// interface implementation needs to ensure onReply is called regardless of whether the called failed or not. On failure, call onReply with nil
	AppendEntries(req *AppendEntriesRequest) (*AppendEntriesReply, error)

	// RequestVote calls a peer node to vote.
	// interface implementation needs to ensure onReply is called regardless of whether the called failed or not. On failure, call onReply with nil
	RequestVote(req *RequestVoteRequest) (*RequestVoteReply, error)

	// InstallSnapshot calls a peer node to install a snapshot.
	// interface implementation needs to ensure onReply is called regardless of whether the called failed or not. On failure, call onReply with nil
	InstallSnapshot(req *SnapshotRequest) (*AppendEntriesReply, error)

	// Get invokes a peer node to get values
	Get(req *GetRequest) (*GetReply, error)

	// Execute invokes a node (usually the leader) to do set or delete operations
	Execute(cmd *StateMachineCmd) (*ExecuteReply, error)
}

// IPeerProxyFactory creates a new proxy
type IPeerProxyFactory interface {
	// factory method
	NewPeerProxy(info NodeInfo) IPeerProxy
}

// Peer wraps information for a raft Peer as well as the RPC proxy
type Peer struct {
	NodeInfo
	followerStatus
	proxy IPeerProxy
}

// IFollowerStatusManager defines interfaces to manage follower status
// Used by leader only
type IFollowerStatusManager interface {
	ResetFollowerIndicies(lastLogIndex int)
	UpdateFollowerMatchIndex(nodeID int, match bool, lastMatch int)
	MajorityMatch(logIndex int) bool
}

// IPeerManager defines raft peer manager interface.
// A peer manager tracks peers' status as well as communicate with them
type IPeerManager interface {
	AppendEntries(nodeID int, req *AppendEntriesRequest, onReply func(*AppendEntriesReply))
	RequestVote(nodeID int, req *RequestVoteRequest, onReply func(*RequestVoteReply))
	BroadcastRequestVote(req *RequestVoteRequest, onReply func(*RequestVoteReply))
	InstallSnapshot(nodeID int, req *SnapshotRequest, onReply func(*AppendEntriesReply))
	Get(nodeID int, req *GetRequest) (*GetReply, error)
	Execute(nodeID int, cmd *StateMachineCmd) (*ExecuteReply, error)

	GetAllPeers() map[int]*Peer
	GetPeer(nodeID int) *Peer

	// PeerManager also manages follower status
	IFollowerStatusManager
}

// PeerManager manages communication with peers
type PeerManager struct {
	Peers map[int]*Peer
}

var errorNoPeersProvided = errors.New("No raft peers provided")
var errorInvalidNodeID = errors.New("Invalid node id")

// NewPeerManager creates the node proxy for kv store
func NewPeerManager(nodeID int, peers map[int]NodeInfo, factory IPeerProxyFactory) IPeerManager {
	if len(peers) == 0 {
		util.Panicln(errorNoPeersProvided)
	}

	if _, ok := peers[nodeID]; ok {
		util.Panicf("current node %d is listed in peers\n", nodeID)
	}

	mgr := &PeerManager{
		Peers: make(map[int]*Peer),
	}

	for _, info := range peers {
		mgr.Peers[info.NodeID] = &Peer{
			NodeInfo: info,
			followerStatus: followerStatus{
				nextIndex:  0,
				matchIndex: -1,
			},
			proxy: factory.NewPeerProxy(info),
		}
	}

	return mgr
}

// ResetFollowerIndicies resets all follower's indices based on lastLogIndex
func (mgr *PeerManager) ResetFollowerIndicies(lastLogIndex int) {
	for _, p := range mgr.Peers {
		p.nextIndex = lastLogIndex + 1
		p.matchIndex = -1
	}
}

// UpdateFollowerMatchIndex updates match index for a given node
func (mgr *PeerManager) UpdateFollowerMatchIndex(nodeID int, matched bool, lastMatch int) {
	peer := mgr.GetPeer(nodeID)

	if matched {
		util.WriteTrace("Updating Node%d nextIndex. lastMatch %d", nodeID, lastMatch)
		peer.nextIndex = lastMatch + 1
		peer.matchIndex = lastMatch
	} else {
		util.WriteTrace("Decreasing Node%d nextIndex.", nodeID)
		// prev entries don't match. decrement nextIndex.
		// cap it to 0. It is meaningless when less than zero
		peer.nextIndex = util.Max(0, peer.nextIndex-nextIndexFallbackStep)
	}
}

// MajorityMatch tells whether we have majority of the followers match the given logIndex
func (mgr *PeerManager) MajorityMatch(logIndex int) bool {
	// both match count and majority should include the leader itself, which is not part of the peerManager
	matchCnt := 1
	majority := (len(mgr.Peers) + 1) / 2
	for _, p := range mgr.Peers {
		if p.matchIndex >= logIndex {
			matchCnt++
			if matchCnt > majority {
				return true
			}
		}
	}

	return false
}

// AppendEntries sends AE request to a single node
func (mgr *PeerManager) AppendEntries(nodeID int, req *AppendEntriesRequest, onReply func(*AppendEntriesReply)) {
	// Send request to the peer node on different go routine
	peer := mgr.GetPeer(nodeID)

	go func() {
		reply, err := peer.proxy.AppendEntries(req)

		if err != nil {
			util.WriteTrace("AppendEntry call failed to Node%d", nodeID)
			reply = nil
		}

		onReply(reply)
	}()
}

// InstallSnapshot installs a snapshot on the target node
func (mgr *PeerManager) InstallSnapshot(nodeID int, req *SnapshotRequest, onReply func(*AppendEntriesReply)) {
	peer := mgr.GetPeer(nodeID)

	go func() {
		reply, err := peer.proxy.InstallSnapshot(req)

		if err != nil {
			util.WriteTrace("InstallSnapshot call failed to Node%d", nodeID)
			reply = nil
		}

		onReply(reply)
	}()
}

// RequestVote handles raft RPC RV calls to a peer nodes
func (mgr *PeerManager) RequestVote(nodeID int, req *RequestVoteRequest, onReply func(*RequestVoteReply)) {
	peer := mgr.GetPeer(nodeID)
	go func() {
		reply, err := peer.proxy.RequestVote(req)

		if err != nil {
			util.WriteTrace("RequestVote call failed to Node%d", nodeID)
			reply = nil
		}

		onReply(reply)
	}()
}

// BroadcastRequestVote handles raft RPC RV calls to all peer nodes
func (mgr *PeerManager) BroadcastRequestVote(req *RequestVoteRequest, onReply func(*RequestVoteReply)) {
	for _, peer := range mgr.Peers {
		mgr.RequestVote(peer.NodeID, req, onReply)
	}
}

// Get gets values from state machine against leader, runs on current goroutine
func (mgr *PeerManager) Get(nodeID int, req *GetRequest) (*GetReply, error) {
	peer := mgr.GetPeer(nodeID)
	return peer.proxy.Get(req)
}

// Execute runs a command via the leader, runs on current goroutine
func (mgr *PeerManager) Execute(nodeID int, cmd *StateMachineCmd) (*ExecuteReply, error) {
	peer := mgr.GetPeer(nodeID)
	return peer.proxy.Execute(cmd)
}

// GetPeer gets the peer for a given node id
func (mgr *PeerManager) GetPeer(nodeID int) *Peer {
	peer, ok := mgr.Peers[nodeID]
	if !ok {
		util.Panicln(errorInvalidNodeID)
	}

	return peer
}

// GetAllPeers returns all the peers
func (mgr *PeerManager) GetAllPeers() map[int]*Peer {
	return mgr.Peers
}
