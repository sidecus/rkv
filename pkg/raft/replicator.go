package raft

import "github.com/sidecus/raft/pkg/util"

const nextIndexFallbackStep = 5

// Follower manages nextIndex and matchIndex for a follower
type Follower struct {
	NodeID     int
	nextIndex  int
	matchIndex int
}

// IReplicator defines interfaces to manage follower status and data replication.
// Used by leader only
type IReplicator interface {
	ResetFollowerIndicies(lastLogIndex int)
	UpdateFollowerMatchIndex(nodeID int, match bool, lastMatch int)
	QuorumReached(logIndex int) bool

	GetFollower(nodeID int) *Follower
	GetAllFollowers() []*Follower
}

// Replicator manages follower status and data replication to them
type Replicator struct {
	Followers map[int]*Follower
}

// NewReplicator creates a new replicator
func NewReplicator(peers map[int]NodeInfo) *Replicator {
	followers := make(map[int]*Follower, len(peers))
	for _, peer := range peers {
		followers[peer.NodeID] = &Follower{
			NodeID:     peer.NodeID,
			nextIndex:  0,
			matchIndex: -1,
		}
	}

	return &Replicator{
		Followers: followers,
	}
}

// GetFollower gets the follower info
func (replicator *Replicator) GetFollower(nodeID int) *Follower {
	return replicator.Followers[nodeID]
}

// GetAllFollowers returns all followers
func (replicator *Replicator) GetAllFollowers() []*Follower {
	followers := make([]*Follower, len(replicator.Followers))
	i := 0
	for _, f := range replicator.Followers {
		followers[i] = f
		i++
	}

	return followers
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
		util.WriteVerbose("Decreasing Node%d's nextIndex.", nodeID)
		// prev entries don't match. decrement nextIndex.
		// cap it to 0. It is meaningless when less than zero
		follower.nextIndex = util.Max(0, follower.nextIndex-nextIndexFallbackStep)
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
