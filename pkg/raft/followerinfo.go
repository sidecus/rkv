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

// createFollowerInfo creates the follower info
func createFollowerInfo(nodeID int, nodeIDs []int) followerInfo {
	info := make(map[int]*followerIndex, len(nodeIDs)-1)

	// Initialize follower info array
	for _, v := range nodeIDs {
		if v != nodeID {
			info[v] = &followerIndex{nodeID: v, nextIndex: 0, matchIndex: -1}
		}
	}

	return info
}

func (info followerInfo) reset(lastLogIndex int) {
	for _, v := range info {
		v.nextIndex = lastLogIndex + 1
		v.matchIndex = 0
	}
}

func (info followerInfo) update(nodeID int, aeReplySuccess bool, lastLogIndex int) {
	follower := info[nodeID]
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

func (info followerInfo) majorityMatch(index int) bool {
	if index < 0 {
		panic("cannot have majority match on negative index")
	}

	// both match count and majority should include the leader itself, which is not in the followerInfo
	matchCnt := 1
	majority := (len(info) + 1) / 2
	for _, v := range info {
		if v.matchIndex >= index {
			matchCnt++
			if matchCnt > majority {
				return true
			}
		}
	}

	return false
}
