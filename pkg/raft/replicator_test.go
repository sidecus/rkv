package raft

import (
	"testing"

	"github.com/sidecus/raft/pkg/util"
)

func TestNewReplicator(t *testing.T) {
	size := 5
	replicator := createTestReplicator(size).(*Replicator)

	if len(replicator.Followers) != size {
		t.Error("NewReplicator created with wrong number of followers")
	}

	for i, f := range replicator.Followers {
		if f.NodeID != i {
			t.Error("Follower id is not initialized correctly")
		}
		if f.nextIndex != 0 || f.matchIndex != -1 {
			t.Error("follower indicies are not initialized correctly")
		}
	}
}

func TestResetFollowerIndicies(t *testing.T) {
	replicator := createTestReplicator(3).(*Replicator)
	replicator.Followers[0].nextIndex = 5
	replicator.Followers[0].matchIndex = 3
	replicator.Followers[1].nextIndex = 10
	replicator.Followers[1].matchIndex = 9
	replicator.Followers[2].nextIndex = 6
	replicator.Followers[2].matchIndex = -1

	replicator.ResetFollowerIndicies(20)
	for _, p := range replicator.Followers {
		if p.nextIndex != 21 || p.matchIndex != -1 {
			t.Fatal("reset doesn't reset on positive last index")
		}
	}

	replicator.ResetFollowerIndicies(-1)
	for _, p := range replicator.Followers {
		if p.nextIndex != 0 || p.matchIndex != -1 {
			t.Fatal("reset doesn't reset on -1 as last index")
		}
	}
}

func TestUpdateFollowerMatchIndex(t *testing.T) {
	replicator := createTestReplicator(3).(*Replicator)

	follower0 := replicator.Followers[0]

	// has new match
	follower0.nextIndex = 5
	follower0.matchIndex = 3
	replicator.UpdateFollowerMatchIndex(0, true, -1)
	if follower0.nextIndex != 0 || follower0.matchIndex != -1 {
		t.Error("updateMatchIndex fails with successful match on -1")
	}

	follower0.nextIndex = 5
	follower0.matchIndex = 3
	replicator.UpdateFollowerMatchIndex(0, true, 6)
	if follower0.nextIndex != 7 || follower0.matchIndex != 6 {
		t.Error("updateMatchIndex fails with successful match on 6")
	}

	// no match
	follower0.nextIndex = 8
	follower0.matchIndex = 3
	replicator.UpdateFollowerMatchIndex(0, false, -2)
	if follower0.nextIndex != util.Max(0, 8-nextIndexFallbackStep) || follower0.matchIndex != 3 {
		t.Error("updateMatchIndex doesn't decrease nextIndex correctly upon failed match")
	}

	follower0.nextIndex = 0
	follower0.matchIndex = -1
	replicator.UpdateFollowerMatchIndex(0, false, -2)
	if follower0.nextIndex != 0 || follower0.matchIndex != -1 {
		t.Error("updateMatchIndex unnecessarily decrease nextIndex when it's already 0 upon failure")
	}
}

func TestQuorumReached(t *testing.T) {
	replicator := createTestReplicator(2).(*Replicator)

	follower0 := replicator.Followers[0]
	follower1 := replicator.Followers[1]

	if !replicator.QuorumReached(-1) {
		t.Error("QuorumReached fails on -1 when should be")
	}

	if replicator.QuorumReached(0) {
		t.Error("QuorumReached returns true on 0 when it should not")
	}

	follower0.matchIndex = 3
	follower1.matchIndex = 6

	for i := 0; i < 10; i++ {
		expected := i <= 6
		result := replicator.QuorumReached(i)

		if expected != result {
			t.Errorf("QuorumReached failed on index %d", i)
		}
	}
}

func createTestReplicator(size int) IReplicator {
	peers := createTestPeerInfo(size)
	return NewReplicator(peers)
}
