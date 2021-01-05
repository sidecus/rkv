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
	getPeer(nodeID int) *Peer
	waitAll(action func(*Peer, *sync.WaitGroup))
	resetFollowerIndicies(lastLogIndex int)
	quorumReached(logIndex int) bool
	tryReplicateAll()

	start()
	stop()
}

// peerManager manages communication with peers
type peerManager struct {
	peers map[int]*Peer
}

// newPeerManager creates the node proxy for kv store
func newPeerManager(peers map[int]NodeInfo, replicate func(*Peer) int, proxyFactory IPeerProxyFactory) IPeerManager {
	if len(peers) == 0 {
		util.Panicf("%s\n", errorNoPeersProvided)
	}

	mgr := &peerManager{
		peers: make(map[int]*Peer),
	}

	for peerID, info := range peers {
		if peerID != info.NodeID {
			util.Panicf("peer %d has different id set in NodeInfo %d\n", peerID, info.NodeID)
		}

		peer := &Peer{
			NodeInfo:   info,
			nextIndex:  0,
			matchIndex: -1,
		}
		peer.IPeerProxy = proxyFactory.NewPeerProxy(info)
		peer.batchReplicator = newBatchReplicator(func() int { return replicate(peer) })
		mgr.peers[peerID] = peer
	}

	return mgr
}

// GetPeer gets the peer for a given node id
func (mgr *peerManager) getPeer(nodeID int) *Peer {
	peer, ok := mgr.peers[nodeID]
	if !ok {
		util.Panicln(errorInvalidNodeID)
	}

	return peer
}

// WaitAll executes an action against each peer and wait for all to finish
func (mgr *peerManager) waitAll(action func(*Peer, *sync.WaitGroup)) {
	var wg sync.WaitGroup
	for _, p := range mgr.peers {
		wg.Add(1)
		action(p, &wg)
	}
	wg.Wait()
}

// resetFollowerIndicies resets all follower's indices based on lastLogIndex
func (mgr *peerManager) resetFollowerIndicies(lastLogIndex int) {
	for _, p := range mgr.peers {
		p.resetFollowerIndex(lastLogIndex)
	}
}

// quorumReached tells whether we have majority of the followers match the given logIndex
func (mgr *peerManager) quorumReached(logIndex int) bool {
	// both match count and majority should include the leader itself, which is not part of the peerManager
	matchCnt := 1
	quorum := (len(mgr.peers) + 1) / 2
	for _, p := range mgr.peers {
		if p.hasConsensus(logIndex) {
			matchCnt++
			if matchCnt > quorum {
				return true
			}
		}
	}

	return false
}

// tryReplicateAll tries to request replication to all peers
func (mgr *peerManager) tryReplicateAll() {
	for _, p := range mgr.peers {
		p.tryRequestReplicate(nil)
	}
}

// Start starts a replication goroutine for each follower
func (mgr *peerManager) start() {
	for _, p := range mgr.peers {
		p.start()
	}
}

// Stop stops the replication goroutines
func (mgr *peerManager) stop() {
	for _, p := range mgr.peers {
		p.stop()
	}
}
