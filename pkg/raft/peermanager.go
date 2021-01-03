package raft

import (
	"errors"
	"sync"

	"github.com/sidecus/raft/pkg/util"
)

var errorNoPeersProvided = errors.New("No raft peers provided")
var errorInvalidNodeID = errors.New("Invalid node id")

// IPeerProxy defines the RPC client interface for a specific peer nodes
// It's an abstraction layer so that concrete implementation (RPC or REST) can be decoupled from this package
type IPeerProxy interface {
	INodeRPCProvider
}

// IPeerProxyFactory creates a new proxy
type IPeerProxyFactory interface {
	// factory method
	NewPeerProxy(info NodeInfo) IPeerProxy
}

// IPeerManager defines raft peer manager interface.
type IPeerManager interface {
	GetPeers() map[int]*Peer
	GetPeer(nodeID int) *Peer
	WaitAllPeers(action func(*Peer, *sync.WaitGroup))

	ResetFollowerIndicies(lastLogIndex int)
	QuorumReached(logIndex int) bool

	Start()
	Stop()
}

// PeerManager manages communication with peers
type PeerManager struct {
	Peers map[int]*Peer
}

// NewPeerManager creates the node proxy for kv store
func NewPeerManager(
	nodeID int,
	peers map[int]NodeInfo,
	replicate func(followerID int) int,
	factory IPeerProxyFactory,
) IPeerManager {
	if len(peers) == 0 {
		util.Fatalf("%s\n", errorNoPeersProvided)
	}

	if _, ok := peers[nodeID]; ok {
		util.Fatalf("current node %d is listed in peers\n", nodeID)
	}

	mgr := &PeerManager{
		Peers: make(map[int]*Peer),
	}

	for _, info := range peers {
		peerID := info.NodeID
		mgr.Peers[peerID] = NewPeer(
			info,
			func() int { return replicate(peerID) },
			factory,
		)
	}

	return mgr
}

// GetPeer gets the peer for a given node id
func (mgr *PeerManager) GetPeer(nodeID int) *Peer {
	peer, ok := mgr.Peers[nodeID]
	if !ok {
		util.Panicln(errorInvalidNodeID)
	}

	return peer
}

// GetPeers returns all the peers
func (mgr *PeerManager) GetPeers() map[int]*Peer {
	return mgr.Peers
}

// ResetFollowerIndicies resets all follower's indices based on lastLogIndex
func (mgr *PeerManager) ResetFollowerIndicies(lastLogIndex int) {
	for _, p := range mgr.Peers {
		p.nextIndex = lastLogIndex + 1
		p.matchIndex = -1
	}
}

// QuorumReached tells whether we have majority of the followers match the given logIndex
func (mgr *PeerManager) QuorumReached(logIndex int) bool {
	// both match count and majority should include the leader itself, which is not part of the peerManager
	matchCnt := 1
	quorum := (len(mgr.Peers) + 1) / 2
	for _, p := range mgr.Peers {
		if p.matchIndex >= logIndex {
			matchCnt++
			if matchCnt > quorum {
				return true
			}
		}
	}

	return false
}

// Start starts a replication goroutine for each follower
func (mgr *PeerManager) Start() {
	for _, p := range mgr.Peers {
		p.Start()
	}
}

// Stop stops the replication goroutines
func (mgr *PeerManager) Stop() {
	for _, p := range mgr.Peers {
		p.Stop()
	}
}

// WaitAllPeers executes an action against each peer and wait for all to finish
func (mgr *PeerManager) WaitAllPeers(action func(*Peer, *sync.WaitGroup)) {
	peers := mgr.GetPeers()
	count := len(peers)

	var wg sync.WaitGroup
	wg.Add(count)
	for _, p := range peers {
		action(p, &wg)
	}
	wg.Wait()
}
