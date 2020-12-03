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
		nodeID:        100,
		nodeState:     Candidate,
		currentTerm:   50,
		currentLeader: -1,
		followers:     followerIndicies,
		logMgr: &LogManager{
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

func TestTryFollowNewTerm(t *testing.T) {
	n := &node{
		nodeID:        0,
		nodeState:     Leader,
		currentTerm:   0,
		currentLeader: 0,
		votedFor:      0,
	}
	timer := &fakeRaftTimer{
		state: -1,
	}
	n.timer = timer

	if !n.tryFollowNewTerm(1, 1, false) {
		t.Error("tryFollowNewTerm should return true on new term")
	}
	if n.currentLeader != 1 || n.currentTerm != 1 || n.nodeState != Follower {
		t.Error("tryFollowNewTerm doesn't follow upon new term")
	}

	n.nodeState = Candidate
	n.currentLeader = 0
	n.currentTerm = 1
	if !n.tryFollowNewTerm(2, 1, true) {
		t.Error("tryFollowNewTerm should return true on AE calls from the same term")
	}
	if n.currentLeader != 2 || n.currentTerm != 1 || n.nodeState != Follower {
		t.Error("tryFollowNewTerm should follow AE calls from the same term")
	}
	if timer.state != Follower {
		t.Error("tryFollowNewTerm didn't reset timer to follower mode")
	}

	n.nodeState = Candidate
	n.currentLeader = 0
	n.currentTerm = 1
	if n.tryFollowNewTerm(1, 1, false) {
		t.Error("tryFollowNewTerm should not return true on same term when it's not AE call")
	}
	if n.currentLeader != 0 || n.currentTerm != 1 || n.nodeState != Candidate {
		t.Error("tryFollowNewTerm updates node state incorrectly")
	}
}

func TestLeaderCommit(t *testing.T) {
	sm := &testStateMachine{
		lastApplied: -111,
	}
	followers := createTestFollowers(2)
	logMgr := NewLogMgr(sm).(*LogManager)

	followers[0].nextIndex = 2
	followers[0].matchIndex = 1
	followers[1].nextIndex = 3
	followers[1].matchIndex = 1

	for i := 0; i < 5; i++ {
		logMgr.ProcessCmd(StateMachineCmd{
			CmdType: 1,
			Data:    i * 10,
		}, i+1)
	}

	n := &node{
		clusterSize: 3,
		currentTerm: 3,
		logMgr:      logMgr,
		followers:   followers,
	}

	// We only have a match on 1st entry, but it's of a lower term
	n.leaderCommit()
	if logMgr.commitIndex != -1 {
		t.Error("leaderCommit shall not commit entries from previous term")
	}
	if sm.lastApplied != -111 {
		t.Error("leaderCommit shall not trigger apply of entires from previous term")
	}

	// We only have a majority match on 2nd entry in the same term
	followers[1].matchIndex = 2
	n.leaderCommit()
	if logMgr.commitIndex != 2 {
		t.Error("leaderCommit shall commit to the right entry")
	}
	if logMgr.lastApplied != 2 || sm.lastApplied != 20 {
		t.Error("leaderCommit trigger apply to the right entry")
	}
}

func TestReplicateLogsTo(t *testing.T) {
	sm := &testStateMachine{
		lastApplied: -111,
	}
	logMgr := NewLogMgr(sm).(*LogManager)
	for i := 0; i < 5; i++ {
		logMgr.ProcessCmd(StateMachineCmd{
			CmdType: 1,
			Data:    i * 10,
		}, i+1)
	}

	followers := createTestFollowers(1)
	tpm := &testPeerManager{
		lastAENodeID: -1,
		lastAEReq:    nil,
	}
	n := &node{
		nodeID:      1,
		clusterSize: 3,
		currentTerm: 3,
		logMgr:      logMgr,
		followers:   followers,
		peerMgr:     tpm,
	}

	// nextIndex is larger than lastIndex, nothing to replicate
	followers[0].nextIndex = 6
	followers[0].matchIndex = 1
	n.replicateLogsTo(0)
	if tpm.lastAENodeID != -1 || tpm.lastAEReq != nil {
		t.Error("replicateLogsTo should not replicate when nextIndex is high enough")
	}

	followers[0].nextIndex = 2
	followers[0].matchIndex = 1
	n.replicateLogsTo(0)
	if tpm.lastAENodeID != 0 || tpm.lastAEReq == nil {
		t.Error("replicateLogsTo should replicate based on nextIndex")
	}
	if tpm.lastAEReq.LeaderID != n.nodeID || tpm.lastAEReq.Term != 3 ||
		tpm.lastAEReq.PrevLogIndex != 1 || tpm.lastAEReq.PrevLogTerm != 2 {
		t.Error("wrong info are being replicated")
	}
	if len(tpm.lastAEReq.Entries) != 3 || tpm.lastAEReq.Entries[0].Index != 2 {
		t.Error("replicated entries contain bad entries")
	}
}
