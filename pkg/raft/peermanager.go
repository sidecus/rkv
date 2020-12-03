package raft

import (
	"errors"

	"github.com/sidecus/raft/pkg/util"
)

// PeerInfo contains info for a peer node
type PeerInfo struct {
	NodeID   int
	Endpoint string
}

// IPeerProxyFactory creates a new proxy
type IPeerProxyFactory interface {
	// factory method
	NewPeerProxy(info PeerInfo) IPeerProxy
}

// IPeerProxy wraps the RPC client to call into peer nodes
type IPeerProxy interface {
	// Raft related
	AppendEntries(req *AppendEntriesRequest, callback func(*AppendEntriesReply))
	RequestVote(req *RequestVoteRequest, callback func(*RequestVoteReply))

	// Data related
	Get(req *GetRequest) (*GetReply, error)
	Execute(cmd *StateMachineCmd) (*ExecuteReply, error)
}

// Peer contains information for a raft peer
type Peer struct {
	info  PeerInfo
	proxy IPeerProxy
}

// IPeerManager defines the interface for raft peer management
type IPeerManager interface {
	AppendEntries(nodeID int, req *AppendEntriesRequest, callback func(*AppendEntriesReply))
	BroadcastAppendEntries(req *AppendEntriesRequest, callback func(*AppendEntriesReply))
	RequestVote(nodeID int, req *RequestVoteRequest, callback func(*RequestVoteReply))
	BroadcastRequestVote(req *RequestVoteRequest, callback func(*RequestVoteReply))
	Get(nodeID int, req *GetRequest) (*GetReply, error)
	Execute(nodeID int, cmd *StateMachineCmd) (*ExecuteReply, error)
}

// PeerManager manages communication with peers
type PeerManager struct {
	Peers map[int]Peer
}

var errorNoPeersProvided = errors.New("No raft peers provided")
var errorInvalidNodeID = errors.New("Invalid node id")

// NewPeerManager creates the node proxy for kv store
func NewPeerManager(peers map[int]PeerInfo, factory IPeerProxyFactory) IPeerManager {
	if len(peers) == 0 {
		util.Panicln(errorNoPeersProvided)
	}

	mgr := &PeerManager{
		Peers: make(map[int]Peer),
	}

	for _, info := range peers {
		proxy := factory.NewPeerProxy(info)

		mgr.Peers[info.NodeID] = Peer{
			info:  info,
			proxy: proxy,
		}
	}

	return mgr
}

// AppendEntries sends AE request to a single node
func (mgr *PeerManager) AppendEntries(nodeID int, req *AppendEntriesRequest, callback func(*AppendEntriesReply)) {
	// Send request to the peer node on different go routine
	peer := mgr.getPeer(nodeID)

	go func() {
		peer.proxy.AppendEntries(req, callback)
	}()
}

// BroadcastAppendEntries handles raft RPC AE calls to all peers (heartbeat)
func (mgr *PeerManager) BroadcastAppendEntries(req *AppendEntriesRequest, callback func(*AppendEntriesReply)) {
	// send request to all peers
	for _, peer := range mgr.Peers {
		mgr.AppendEntries(peer.info.NodeID, req, callback)
	}
}

// RequestVote handles raft RPC RV calls to a peer nodes
func (mgr *PeerManager) RequestVote(nodeID int, req *RequestVoteRequest, callback func(*RequestVoteReply)) {
	peer := mgr.getPeer(nodeID)
	go func() {
		peer.proxy.RequestVote(req, callback)
	}()
}

// BroadcastRequestVote handles raft RPC RV calls to all peer nodes
func (mgr *PeerManager) BroadcastRequestVote(req *RequestVoteRequest, callback func(*RequestVoteReply)) {
	for _, peer := range mgr.Peers {
		mgr.RequestVote(peer.info.NodeID, req, callback)
	}
}

// Get gets values from state machine against leader, runs on current goroutine
func (mgr *PeerManager) Get(nodeID int, req *GetRequest) (*GetReply, error) {
	peer := mgr.getPeer(nodeID)
	return peer.proxy.Get(req)
}

// Execute runs a command via the leader, runs on current goroutine
func (mgr *PeerManager) Execute(nodeID int, cmd *StateMachineCmd) (*ExecuteReply, error) {
	peer := mgr.getPeer(nodeID)
	return peer.proxy.Execute(cmd)
}

// get the peer from a given node id
func (mgr *PeerManager) getPeer(nodeID int) Peer {
	peer, ok := mgr.Peers[nodeID]
	if !ok {
		util.Panicln(errorInvalidNodeID)
	}

	return peer
}
