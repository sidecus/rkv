package raft

import "github.com/sidecus/raft/pkg/util"

const nextIndexFallbackStep = 5

// fStatus manages nextIndex and matchIndex for a follower
type fStatus struct {
	nodeID     int
	nextIndex  int
	matchIndex int
}

// followerStatus manages next/match indicies for all followers
// This is used by leader to replicate logs
type followerStatus map[int]*fStatus

// createFollowers creates the follower indicies
func createFollowers(nodeID int, peers map[int]PeerInfo) followerStatus {
	if _, ok := peers[nodeID]; ok {
		util.Panicf("current node %d is listed in peers\n", nodeID)
	}

	ret := make(map[int]*fStatus, len(peers))

	// Initialize follower info array
	for _, v := range peers {
		ret[v.NodeID] = &fStatus{
			nodeID:     v.NodeID,
			nextIndex:  0,
			matchIndex: -1,
		}
	}

	return ret
}

// reset all follower's indices based on lastLogIndex
func (info followerStatus) reset(lastLogIndex int) {
	for _, v := range info {
		v.nextIndex = lastLogIndex + 1
		v.matchIndex = -1
	}
}

// update match index for a given node
func (info followerStatus) updateMatchIndex(nodeID int, match bool, lastMatch int) {
	follower := info[nodeID]
	if follower == nil {
		util.Panicf("Invalid nodeID %d when calling updateMatchIndex\n", nodeID)
	}

	if match {
		follower.nextIndex = lastMatch + 1
		follower.matchIndex = lastMatch
	} else {
		// prev entries don't match. decrement nextIndex.
		// cap it to 0. It is meaningless when less than zero
		follower.nextIndex = util.Max(0, follower.nextIndex-nextIndexFallbackStep)
	}
}

// do we have majority of the followers match the given logIndex
func (info followerStatus) majorityMatch(logIndex int) bool {
	// both match count and majority should include the leader itself, which is not in the followerInfo
	matchCnt := 1
	majority := (len(info) + 1) / 2
	for _, v := range info {
		if v.matchIndex >= logIndex {
			matchCnt++
			if matchCnt > majority {
				return true
			}
		}
	}

	return false
}
