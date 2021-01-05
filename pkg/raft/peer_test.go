package raft

import (
	"testing"

	"github.com/sidecus/raft/pkg/util"
)

func TestUpdateMatchIndex(t *testing.T) {
	mgr := createTestPeerManager(3).(*peerManager)

	follower0 := mgr.getPeer(0)

	// has new match
	follower0.nextIndex = 5
	follower0.matchIndex = 3
	follower0.updateMatchIndex(true, 2)
	if follower0.nextIndex != 5 || follower0.matchIndex != 3 {
		t.Error("updateMatchIndex should ignore stale matches")
	}

	follower0.nextIndex = 5
	follower0.matchIndex = 3
	follower0.updateMatchIndex(true, 6)
	if follower0.nextIndex != 7 || follower0.matchIndex != 6 {
		t.Error("updateMatchIndex fails with successful match on 6")
	}

	// no match
	follower0.nextIndex = 8
	follower0.matchIndex = 3
	follower0.updateMatchIndex(false, -2)
	if follower0.nextIndex != util.Max(0, 8-nextIndexFallbackStep) || follower0.matchIndex != -1 {
		t.Error("updateMatchIndex doesn't decrease nextIndex correctly or set match index to -1 upon failed match")
	}

	follower0.nextIndex = 0
	follower0.matchIndex = -1
	follower0.updateMatchIndex(false, -2)
	if follower0.nextIndex != 0 || follower0.matchIndex != -1 {
		t.Error("updateMatchIndex unnecessarily decrease nextIndex when it's already 0 upon failure")
	}
}
