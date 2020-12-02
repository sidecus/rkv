package raft

// followerIndex manages nextIndex and matchIndex for a follower
type followerIndex struct {
	nodeID     int
	nextIndex  int
	matchIndex int
}

// followerInfo manages next/match indicies for all followers
// This is used by leader to replicate logs
type followerInfo map[int]*followerIndex

// createFollowers creates the follower indicies
func createFollowers(nodeID int, peers map[int]PeerInfo) followerInfo {
	if _, ok := peers[nodeID]; ok {
		panic("current node is listed in peers")
	}

	ret := make(map[int]*followerIndex, len(peers))

	// Initialize follower info array
	for _, v := range peers {
		ret[v.NodeID] = &followerIndex{
			nodeID:     v.NodeID,
			nextIndex:  0,
			matchIndex: -1,
		}
	}

	return ret
}

// reset all follower's indices based on lastLogIndex
func (info followerInfo) resetAllIndices(lastLogIndex int) {
	for _, v := range info {
		v.nextIndex = lastLogIndex + 1
		v.matchIndex = -1
	}
}

// update match index for a given node
func (info followerInfo) updateMatchIndex(nodeID int, match bool, lastMatch int) {
	follower := info[nodeID]
	if follower == nil {
		panic("Invalid nodeID when calling updateMatchIndex")
	}

	if match {
		follower.nextIndex = lastMatch + 1
		follower.matchIndex = lastMatch
	} else {
		// only decrement when it's larger than zero
		// nextIndex is meaningless when its less than zero
		if follower.nextIndex > 0 {
			follower.nextIndex--
		}
	}
}

// do we have majority of the followers match the given logIndex
func (info followerInfo) majorityMatch(logIndex int) bool {
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
