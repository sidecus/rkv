package raft

// followerIndex manages nextIndex and matchIndex for a follower
type followerIndex struct {
	nodeID     int
	nextIndex  int
	matchIndex int
}

// followerInfo manages next/match indicies for all followers
// This is used by leader to replicate logs
type followerInfo struct {
	followerIndicies map[int]*followerIndex
}

// createFollowerInfo creates the follower info
func createFollowerInfo(nodeID int, nodeIDs []int) *followerInfo {
	info := make(map[int]*followerIndex, len(nodeIDs)-1)

	// Initialize follower info array
	for _, v := range nodeIDs {
		if v != nodeID {
			info[v] = &followerIndex{nodeID: v, nextIndex: 0, matchIndex: -1}
		}
	}

	return &followerInfo{
		followerIndicies: info,
	}
}

func (info *followerInfo) reset(lastLogIndex int) {
	for _, v := range info.followerIndicies {
		v.nextIndex = lastLogIndex + 1
		v.matchIndex = 0
	}
}

func (info *followerInfo) update(nodeID int, aeReplySuccess bool, lastLogIndex int) {
	follower := info.followerIndicies[nodeID]
	if aeReplySuccess {
		follower.nextIndex = lastLogIndex + 1
		follower.matchIndex = lastLogIndex
	} else {
		// only decrement when it's larger than zero
		// nextIndex is meaningless when its less than zero
		if follower.nextIndex > 0 {
			follower.nextIndex--
		}
	}
}

func (info *followerInfo) getFollower(nodeID int) *followerIndex {
	return info.followerIndicies[nodeID]
}
