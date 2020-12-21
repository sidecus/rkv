package raft

import (
	"errors"

	"github.com/sidecus/raft/pkg/util"
)

// IPeerProxy defines the RPC client interface for a specific peer nodes
// It's an abstraction layer so that concrete implementation (RPC or REST) can be decoupled from this package
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
	proxy IPeerProxy
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
			proxy:    factory.NewPeerProxy(info),
		}
	}

	return mgr
}

// AppendEntries sends AE request to a single node
func (mgr *PeerManager) AppendEntries(nodeID int, req *AppendEntriesRequest, onReply func(*AppendEntriesReply)) {
	// Send request to the peer node on different go routine
	peer := mgr.GetPeer(nodeID)

	go func() {
		reply, err := peer.proxy.AppendEntries(req)

		if err != nil {
			util.WriteTrace("AppendEntry call failed to Node%d: %s", nodeID, err)
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
			util.WriteTrace("InstallSnapshot call failed to Node%d: %s", nodeID, err)
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
			util.WriteTrace("RequestVote call failed to Node%d: %s", nodeID, err)
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
