package raft

import (
	"github.com/sidecus/raft/pkg/util"
)

const nextIndexFallbackStep = 20

// Set batcher queue size to be the same as max entries per request
// Batcher queue size also controls our max Execute request concurrency
const batcherQueueSize = maxAppendEntriesCount

// Peer wraps information for a raft Peer as well as the RPC proxy
type Peer struct {
	NodeInfo
	nextIndex  int
	matchIndex int

	Batcher
	IPeerProxy
}

// NewPeer creats a new peer
func NewPeer(info NodeInfo, replicate func() int, factory IPeerProxyFactory) *Peer {
	return &Peer{
		NodeInfo:   info,
		nextIndex:  0,
		matchIndex: -1,
		Batcher:    *NewBatcher(replicate, batcherQueueSize),
		IPeerProxy: factory.NewPeerProxy(info),
	}
}

// HasMatch tells us whether we have found a matching entry for the given follower
func (p *Peer) HasMatch() bool {
	return p.matchIndex+1 == p.nextIndex
}

// HasMoreToReplicate tells us whether there are more to replicate for this follower
func (p *Peer) HasMoreToReplicate(lastIndex int) bool {
	return p.matchIndex < lastIndex
}

// UpdateMatchIndex updates match index for a given node
func (p *Peer) UpdateMatchIndex(match bool, lastMatch int) {
	if match {
		if p.matchIndex < lastMatch {
			util.WriteVerbose("Updating Node%d's nextIndex. lastMatch %d", p.NodeID, lastMatch)
			p.nextIndex = lastMatch + 1
			p.matchIndex = lastMatch
		}
	} else {
		util.WriteVerbose("Decreasing Node%d's nextIndex. lastMatch %d", p.NodeID, lastMatch)
		// prev entries don't match. decrement nextIndex.
		// cap it to 0. It is meaningless when less than zero
		p.nextIndex = util.Max(0, p.nextIndex-nextIndexFallbackStep)
		p.matchIndex = -1
	}
}
