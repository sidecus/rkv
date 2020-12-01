package raft

import (
	"testing"
)

func TestNodeSetTerm(t *testing.T) {
	n := &node{
		currentTerm: 0,
		votedFor:    2,
	}

	n.setTerm(1)
	if n.votedFor != -1 {
		t.Error("Set new term doesn't reset votedFor")
	}

	n.votedFor = 2
	n.setTerm(1)
	if n.votedFor != 2 {
		t.Error("Set same term resets votedFor")
	}
}

func TestEnterFollowerState(t *testing.T) {
	n := &node{
		nodeState:     Leader,
		currentTerm:   0,
		currentLeader: 0,
		votedFor:      0,
	}

	n.enterFollowerState(1, 1)

	if n.nodeState != Follower {
		t.Error("enterFollowerState didn't update nodeState to Follower")
	}
	if n.currentLeader != 1 {
		t.Error("enterFollowerState didn't update currentLeader correctly")
	}
	if n.currentTerm != 1 {
		t.Error("enterFollowerState didn't update currentTerm correctly")
	}
	if n.votedFor != -1 {
		t.Error("enterFollowerState didn't reset votedFor on new term")
	}
}

func TestEnterCandidateState(t *testing.T) {
	n := &node{
		nodeID:        100,
		nodeState:     Leader,
		currentTerm:   0,
		currentLeader: 0,
		votedFor:      0,
	}

	n.enterCandidateState()

	if n.nodeState != Candidate {
		t.Error("enterCandidateState didn't update nodeState to Candidate")
	}
	if n.currentLeader != -1 {
		t.Error("enterCandidateState didn't reset currentLeader to -1")
	}
	if n.currentTerm != 1 {
		t.Error("enterCandidateState didn't increase current term")
	}
	if n.votedFor != 100 {
		t.Error("enterCandidateState didn't vote for self")
	}
	if len(n.votes) != 1 || !n.votes[100] {
		t.Error("enterCandidateState didn't reset other votes")
	}
}

func TestEnterLeaderState(t *testing.T) {
	follower99 := &followerIndex{
		nodeID:     99,
		nextIndex:  30,
		matchIndex: 20,
	}
	follower105 := &followerIndex{
		nodeID:     105,
		nextIndex:  100,
		matchIndex: 70,
	}
	followerIndicies := make(map[int]*followerIndex)
	followerIndicies[99] = follower99
	followerIndicies[105] = follower105

	n := &node{
		nodeID:          100,
		nodeState:       Candidate,
		currentTerm:     50,
		currentLeader:   -1,
		followerIndices: followerIndicies,
		logMgr: &logManager{
			lastIndex: 3,
		},
	}

	n.enterLeaderState()

	if n.nodeState != Leader {
		t.Error("enterLeaderState didn't update nodeState to Leader")
	}
	if n.currentLeader != 100 {
		t.Error("enterLeaderState didn't set currentLeader to self")
	}
	if n.currentTerm != 50 {
		t.Error("enterLeaderState changes term by mistake")
	}
	if follower99.nextIndex != 4 || follower105.nextIndex != 4 {
		t.Error("enterLeaderState didn't reset nextIndex for peers")
	}
	if follower99.matchIndex != -1 || follower105.matchIndex != -1 {
		t.Error("enterLeaderState didn't reset matchIndex for peers")
	}
}
