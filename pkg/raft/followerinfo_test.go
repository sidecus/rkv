package raft

import (
	"testing"

	"github.com/sidecus/raft/pkg/util"
)

func TestCreateFollowers(t *testing.T) {
	size := 5
	followers := createTestFollowers(size)

	if len(followers) != size {
		t.Error("followers created with wrong number of entries")
	}

	for i := 0; i < size; i++ {
		if followers[i].nodeID != i || followers[i].nextIndex != 0 || followers[i].matchIndex != -1 {
			t.Fatal("followers are not initialized with correct index")
		}
	}
}

func TestResetAllIndices(t *testing.T) {
	followers := createTestFollowers(3)
	followers[0].nextIndex = 5
	followers[0].matchIndex = 3
	followers[1].nextIndex = 10
	followers[1].matchIndex = 9
	followers[2].nextIndex = 6
	followers[2].matchIndex = -1

	followers.resetAllIndices(20)
	for i := 0; i < 3; i++ {
		if followers[i].nextIndex != 21 || followers[i].matchIndex != -1 {
			t.Fatal("resetAllIndices doesn't reset on positive last index")
		}
	}

	followers.resetAllIndices(-1)
	for i := 0; i < 3; i++ {
		if followers[i].nextIndex != 0 || followers[i].matchIndex != -1 {
			t.Fatal("resetAllIndices doesn't reset on -1 as last index")
		}
	}
}

func TestUpdateMatchIndex(t *testing.T) {
	followers := createTestFollowers(3)

	// has new match
	followers[0].nextIndex = 5
	followers[0].matchIndex = 3
	followers.updateMatchIndex(0, true, -1)
	if followers[0].nextIndex != 0 || followers[0].matchIndex != -1 {
		t.Error("updateMatchIndex fails with successful match on -1")
	}

	followers[0].nextIndex = 5
	followers[0].matchIndex = 3
	followers.updateMatchIndex(0, true, 6)
	if followers[0].nextIndex != 7 || followers[0].matchIndex != 6 {
		t.Error("updateMatchIndex fails with successful match on 6")
	}

	// no match
	followers[0].nextIndex = 8
	followers[0].matchIndex = 3
	followers.updateMatchIndex(0, false, -2)
	if followers[0].nextIndex != util.Max(0, 8-nextIndexFallbackStep) || followers[0].matchIndex != 3 {
		t.Error("updateMatchIndex doesn't decrease nextIndex correctly upon failed match")
	}

	followers[0].nextIndex = 0
	followers[0].matchIndex = -1
	followers.updateMatchIndex(0, false, -2)
	if followers[0].nextIndex != 0 || followers[0].matchIndex != -1 {
		t.Error("updateMatchIndex unnecessarily decrease nextIndex when it's already 0 upon failure")
	}
}

func TestMajorityMatch(t *testing.T) {
	followers := createTestFollowers(2)
	if !followers.majorityMatch(-1) {
		t.Error("testMajorityMatch fails on -1 when should be")
	}

	if followers.majorityMatch(0) {
		t.Error("testMajorityMatch agrees on 0 when it should not")
	}

	followers[0].matchIndex = 3
	followers[1].matchIndex = 6

	for i := 0; i < 10; i++ {
		shouldMatch := i <= 6
		result := followers.majorityMatch(i)

		if shouldMatch != result {
			t.Errorf("testMajorityMatch failed on index %d", i)
		}
	}
}

func createTestFollowers(size int) followerInfo {
	peers := make(map[int]PeerInfo)
	for i := 0; i < size; i++ {
		peers[i] = PeerInfo{NodeID: i}
	}

	followers := createFollowers(100, peers)

	return followers
}
