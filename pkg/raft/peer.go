package raft

import (
	"github.com/sidecus/raft/pkg/util"
)

const nextIndexFallbackStep = 1
const maxAppendEntriesCount = 64

// Peer wraps information for a raft Peer as well as the RPC proxy
type Peer struct {
	NodeInfo
	nextIndex  int
	matchIndex int

	*batchReplicator
	IPeerProxy
}

// hasMatch tells us whether we have found a matching entry for the given follower
func (p *Peer) hasMatch() bool {
	return p.matchIndex+1 == p.nextIndex
}

// get next index and entry count for next replication
func (p *Peer) getReplicationParams() (nextIndex int, entryCount int) {
	nextIndex = p.nextIndex
	entryCount = maxAppendEntriesCount
	if !p.hasMatch() {
		// no need for any payload if we haven't got a match yet
		entryCount = 0
	}
	return
}

// should we send a snapshot
func (p *Peer) shouldSendSnapshot(snapshotIndex int) bool {
	return p.nextIndex <= snapshotIndex
}

// upToDate tells us whether follower is up to date with given index
func (p *Peer) upToDate(lastIndex int) bool {
	return p.matchIndex >= lastIndex
}

// hasConsensus checks whether current peer has consensus upon the given log index
func (p *Peer) hasConsensus(logIndex int) bool {
	return p.matchIndex >= logIndex
}

// reset follower index based on last index
func (p *Peer) resetFollowerIndex(lastLogIndex int) {
	p.nextIndex = lastLogIndex + 1
	p.matchIndex = -1
}

// updateMatchIndex updates match index for a given node
func (p *Peer) updateMatchIndex(match bool, lastMatch int) {
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
