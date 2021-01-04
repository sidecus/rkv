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
	GetPeer(nodeID int) *Peer
	WaitAll(action func(*Peer, *sync.WaitGroup))

	ResetFollowerIndicies(lastLogIndex int)
	QuorumReached(logIndex int) bool
	TriggerHeartbeats()

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
	replicate func(*Peer) int,
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

	for peerID, info := range peers {
		if peerID != info.NodeID {
			util.Fatalf("peer %d has different id set in NodeInfo %d", peerID, info.NodeID)
		}
		mgr.Peers[peerID] = newPeer(info, replicate, factory.NewPeerProxy(info))
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

// ResetFollowerIndicies resets all follower's indices based on lastLogIndex
func (mgr *PeerManager) ResetFollowerIndicies(lastLogIndex int) {
	for _, p := range mgr.Peers {
		p.resetFollowerIndex(lastLogIndex)
	}
}

// QuorumReached tells whether we have majority of the followers match the given logIndex
func (mgr *PeerManager) QuorumReached(logIndex int) bool {
	// both match count and majority should include the leader itself, which is not part of the peerManager
	matchCnt := 1
	quorum := (len(mgr.Peers) + 1) / 2
	for _, p := range mgr.Peers {
		if p.hasConsensus(logIndex) {
			matchCnt++
			if matchCnt > quorum {
				return true
			}
		}
	}

	return false
}

// TriggerHeartbeats trigger heart beat for each peer
func (mgr *PeerManager) TriggerHeartbeats() {
	for _, p := range mgr.Peers {
		p.tryRequestReplicate(nil)
	}
}

// Start starts a replication goroutine for each follower
func (mgr *PeerManager) Start() {
	for _, p := range mgr.Peers {
		p.start()
	}
}

// Stop stops the replication goroutines
func (mgr *PeerManager) Stop() {
	for _, p := range mgr.Peers {
		p.stop()
	}
}

// WaitAll executes an action against each peer and wait for all to finish
func (mgr *PeerManager) WaitAll(action func(*Peer, *sync.WaitGroup)) {
	var wg sync.WaitGroup
	for _, p := range mgr.Peers {
		wg.Add(1)
		action(p, &wg)
	}
	wg.Wait()
}
