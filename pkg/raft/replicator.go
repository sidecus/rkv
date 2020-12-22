package raft

import (
	"sync"

	"github.com/sidecus/raft/pkg/util"
)

const nextIndexFallbackStep = 5

// Follower manages nextIndex and matchIndex for a follower
type Follower struct {
	NodeID     int
	nextIndex  int
	matchIndex int
}

// HasMatch tells whether we have found a match index for the follower
func (follower *Follower) HasMatch() bool {
	return follower.matchIndex+1 == follower.nextIndex
}

// IReplicator defines interfaces to manage follower status and data replication.
// Used by leader only
type IReplicator interface {
	Start()
	Stop()

	GetFollower(nodeID int) *Follower
	GetFollowers() map[int]*Follower

	Synchronize(nodeID int)

	ResetFollowerIndicies(lastLogIndex int)
	UpdateFollowerMatchIndex(nodeID int, match bool, lastMatch int)
	QuorumReached(logIndex int) bool
}

// Replicator manages follower status and data replication to them
type Replicator struct {
	Followers     map[int]*Follower
	Signals       map[int]chan bool
	ReplicateFunc func(followerID int)
	wg            sync.WaitGroup
}

// NewReplicator creates a new replicator
func NewReplicator(peers map[int]NodeInfo, replicateFunc func(followerID int)) *Replicator {
	followers := make(map[int]*Follower, len(peers))
	signals := make(map[int]chan bool, len(peers))
	for _, peer := range peers {
		followers[peer.NodeID] = &Follower{
			NodeID:     peer.NodeID,
			nextIndex:  0,
			matchIndex: -1,
		}
		signals[peer.NodeID] = make(chan bool, 10)
	}

	return &Replicator{
		Followers:     followers,
		Signals:       signals,
		ReplicateFunc: replicateFunc,
	}
}

// Start starts a replication goroutine for each follower
func (replicator *Replicator) Start() {
	replicator.wg.Add(len(replicator.Followers))
	for _, follower := range replicator.Followers {
		go func(followerID int) {
			for range replicator.Signals[followerID] {
				replicator.ReplicateFunc(followerID)
			}
			replicator.wg.Done()
		}(follower.NodeID)
	}
}

// Stop stops the replication goroutines
func (replicator *Replicator) Stop() {
	for _, signal := range replicator.Signals {
		close(signal)
	}
	replicator.wg.Wait()
}

// GetFollower gets the follower info
func (replicator *Replicator) GetFollower(nodeID int) *Follower {
	return replicator.Followers[nodeID]
}

// GetFollowers returns all followers
func (replicator *Replicator) GetFollowers() map[int]*Follower {
	return replicator.Followers
}

// Synchronize trigger a replication
func (replicator *Replicator) Synchronize(nodeID int) {
	replicator.Signals[nodeID] <- true
}

// ResetFollowerIndicies resets all follower's indices based on lastLogIndex
func (replicator *Replicator) ResetFollowerIndicies(lastLogIndex int) {
	for _, p := range replicator.Followers {
		p.nextIndex = lastLogIndex + 1
		p.matchIndex = -1
	}
}

// UpdateFollowerMatchIndex updates match index for a given node
func (replicator *Replicator) UpdateFollowerMatchIndex(nodeID int, matched bool, lastMatch int) {
	follower := replicator.Followers[nodeID]

	if matched {
		util.WriteVerbose("Updating Node%d's nextIndex. lastMatch %d", nodeID, lastMatch)
		follower.nextIndex = lastMatch + 1
		follower.matchIndex = lastMatch
	} else {
		util.WriteVerbose("Decreasing Node%d's nextIndex. lastMatch %d", nodeID, lastMatch)
		// prev entries don't match. decrement nextIndex.
		// cap it to 0. It is meaningless when less than zero
		follower.nextIndex = util.Max(0, follower.nextIndex-nextIndexFallbackStep)
		follower.matchIndex = -1
	}
}

// QuorumReached tells whether we have majority of the followers match the given logIndex
func (replicator *Replicator) QuorumReached(logIndex int) bool {
	// both match count and majority should include the leader itself, which is not part of the peerManager
	matchCnt := 1
	quorum := (len(replicator.Followers) + 1) / 2
	for _, p := range replicator.Followers {
		if p.matchIndex >= logIndex {
			matchCnt++
			if matchCnt > quorum {
				return true
			}
		}
	}

	return false
}
