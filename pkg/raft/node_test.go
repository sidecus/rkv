package raft

import (
	"os"
	"testing"
)

func TestNewNode(t *testing.T) {
	tempDir := os.TempDir()
	if err := os.Chdir(tempDir); err != nil {
		t.Fatal("Changing working directory to temp dir failed. Test case cannot continue")
	}

	peerCount := 2
	nodeID := peerCount // last node
	peers := createTestPeerInfo(peerCount)
	n := NewNode(nodeID, peers, &testStateMachine{}, &PeerFactoryMock{}).(*node)

	if n.nodeID != nodeID {
		t.Error("Node created with invalid node ID")
	}

	if n.clusterSize != peerCount+1 {
		t.Error("Node created with invalid clustersize")
	}

	if n.nodeState != NodeStateFollower {
		t.Error("Node created with invalid starting state")
	}

	if n.currentTerm != 0 {
		t.Error("Node created with invalid starting term")
	}

	if n.currentLeader != -1 {
		t.Error("Node created with invalid current leader")
	}

	if n.votedFor != -1 {
		t.Error("Node created with invalid votedFor")
	}

	if len(n.peerMgr.GetAllPeers()) != peerCount {
		t.Error("Node created with invalid number of followers")
	}

	if len(n.votes) != 0 {
		t.Error("Node created with invalid votes map")
	}

	if snapshotPath == "" {
		t.Error("NewNode doesn't set snapshot path")
	}
}

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
		nodeState:     NodeStateLeader,
		currentTerm:   0,
		currentLeader: 0,
		votedFor:      0,
	}

	n.enterFollowerState(1, 1)

	if n.nodeState != NodeStateFollower {
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
		nodeState:     NodeStateLeader,
		currentTerm:   0,
		currentLeader: 0,
		votedFor:      0,
	}

	n.enterCandidateState()

	if n.nodeState != NodeStateCandidate {
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
	n := &node{
		nodeID:        100,
		nodeState:     NodeStateCandidate,
		currentTerm:   50,
		currentLeader: -1,
		peerMgr:       createTestPeerManager(2),
		logMgr: &LogManager{
			lastIndex: 3,
		},
		replicator: createTestReplicator(2),
	}

	follower0 := n.replicator.GetFollower(0)
	follower0.nextIndex = 30
	follower0.matchIndex = 20
	follower1 := n.replicator.GetFollower(1)
	follower1.nextIndex = 100
	follower1.matchIndex = 70

	n.enterLeaderState()

	if n.nodeState != NodeStateLeader {
		t.Error("enterLeaderState didn't update nodeState to Leader")
	}
	if n.currentLeader != 100 {
		t.Error("enterLeaderState didn't set currentLeader to self")
	}
	if n.currentTerm != 50 {
		t.Error("enterLeaderState changes term by mistake")
	}
	if follower0.nextIndex != 4 || follower1.nextIndex != 4 {
		t.Error("enterLeaderState didn't reset nextIndex for peers")
	}
	if follower0.matchIndex != -1 || follower1.matchIndex != -1 {
		t.Error("enterLeaderState didn't reset matchIndex for peers")
	}
}

func TestTryFollowNewTerm(t *testing.T) {
	n := &node{
		nodeID:        0,
		nodeState:     NodeStateLeader,
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
	if n.currentLeader != 1 || n.currentTerm != 1 || n.nodeState != NodeStateFollower {
		t.Error("tryFollowNewTerm doesn't follow upon new term")
	}

	n.nodeState = NodeStateCandidate
	n.currentLeader = 0
	n.currentTerm = 1
	if !n.tryFollowNewTerm(2, 1, true) {
		t.Error("tryFollowNewTerm should return true on AE calls from the same term")
	}
	if n.currentLeader != 2 || n.currentTerm != 1 || n.nodeState != NodeStateFollower {
		t.Error("tryFollowNewTerm should follow AE calls from the same term")
	}
	if timer.state != NodeStateFollower {
		t.Error("tryFollowNewTerm didn't reset timer to follower mode")
	}

	n.nodeState = NodeStateCandidate
	n.currentLeader = 0
	n.currentTerm = 1
	if n.tryFollowNewTerm(1, 1, false) {
		t.Error("tryFollowNewTerm should not return true on same term when it's not AE call")
	}
	if n.currentLeader != 0 || n.currentTerm != 1 || n.nodeState != NodeStateCandidate {
		t.Error("tryFollowNewTerm updates node state incorrectly")
	}
}

func TestLeaderCommit(t *testing.T) {
	sm := &testStateMachine{
		lastApplied: -111,
	}
	peerMgr := createTestPeerManager(2)
	replicator := createTestReplicator(2)
	logMgr := NewLogMgr(100, sm).(*LogManager)

	replicator.GetFollower(0).nextIndex = 2
	replicator.GetFollower(0).matchIndex = 1
	replicator.GetFollower(1).nextIndex = 3
	replicator.GetFollower(1).matchIndex = 1

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
		peerMgr:     peerMgr,
		replicator:  replicator,
	}

	// We only have a match on 1st entry, but it's of a lower term
	n.tryCommitUponHeartbeatReplies()
	if logMgr.commitIndex != -1 {
		t.Error("leaderCommit shall not commit entries from previous term")
	}
	if sm.lastApplied != -111 {
		t.Error("leaderCommit shall not trigger apply of entires from previous term")
	}

	// We only have a majority match on 2nd entry in the same term
	replicator.GetFollower(1).matchIndex = 2
	n.tryCommitUponHeartbeatReplies()
	if logMgr.commitIndex != 2 {
		t.Error("leaderCommit shall commit to the right entry")
	}
	if logMgr.lastApplied != 2 || sm.lastApplied != 20 {
		t.Error("leaderCommit trigger apply to the right entry")
	}
}

func TestReplicateData(t *testing.T) {
	logMgr := NewLogMgr(100, &testStateMachine{
		lastApplied: -111,
	}).(*LogManager)
	for i := 0; i < 5; i++ {
		logMgr.ProcessCmd(StateMachineCmd{
			CmdType: 1,
			Data:    i * 10,
		}, i+1)
	}

	peerMgr := &mockPeerManager{}

	n := &node{
		nodeID:      2,
		clusterSize: 3,
		currentTerm: 3,
		logMgr:      logMgr,
		peerMgr:     peerMgr,
	}

	onAEReply := func(*AppendEntriesReply) {}
	follower1 := &Follower{
		NodeID: 1,
	}

	// nextIndex is larger than lastIndex, should send empty request
	peerMgr.reset()
	follower1.nextIndex = logMgr.lastIndex + 1
	follower1.matchIndex = 1
	n.replicateData(follower1, true, onAEReply)
	if peerMgr.aeReq == nil {
		t.Error("replicateData should replicate even when nextIndex is higher than lastIndex")
	}
	if peerMgr.aeReq.LeaderID != n.nodeID || peerMgr.aeReq.Term != n.currentTerm ||
		peerMgr.aeReq.PrevLogIndex != follower1.nextIndex-1 || peerMgr.aeReq.PrevLogTerm != logMgr.logs[peerMgr.aeReq.PrevLogIndex].Term {
		t.Error("wrong info are being replicated")
	}
	if len(peerMgr.aeReq.Entries) != 0 {
		t.Error("wrong payload when nextIndex is higher than lastIndex")
	}

	// nextIndex is smaler than lastIndex
	peerMgr.reset()
	follower1.nextIndex = logMgr.lastIndex - 2
	follower1.matchIndex = 1
	n.replicateData(follower1, true, onAEReply)
	if peerMgr.aeReq == nil {
		t.Error("replicateLogsTo should replicate when nextIndex smaller")
	}
	if peerMgr.aeReq.LeaderID != n.nodeID || peerMgr.aeReq.Term != n.currentTerm ||
		peerMgr.aeReq.PrevLogIndex != follower1.nextIndex-1 || peerMgr.aeReq.PrevLogTerm != logMgr.logs[peerMgr.aeReq.PrevLogIndex].Term {
		t.Error("wrong info are being replicated")
	}
	if len(peerMgr.aeReq.Entries) != 3 || peerMgr.aeReq.Entries[0].Index != logMgr.logs[logMgr.lastIndex-2].Index {
		t.Error("replicated entries contain bad entries")
	}

	// nextIndex is the same as snapshotIndex, none heartbeat should trigger snapshot request
	peerMgr.reset()
	logMgr.snapshotIndex = 3
	logMgr.snapshotTerm = 2
	logMgr.lastSnapshotFile = "snapshot"
	follower1.nextIndex = 3
	n.replicateData(follower1, false, onAEReply)
	if peerMgr.isReq == nil {
		t.Error("replicateLogsTo should replicate snapshot but it didn't (or replicated more than once)")
	}
	if peerMgr.isReq.LeaderID != n.nodeID || peerMgr.isReq.Term != n.currentTerm ||
		peerMgr.isReq.File != logMgr.lastSnapshotFile ||
		peerMgr.isReq.SnapshotIndex != logMgr.snapshotIndex || peerMgr.isReq.SnapshotTerm != logMgr.snapshotTerm {
		t.Error("wrong info in SnapshotRequest")
	}
	// heartbeat should not trigger snapshot
	peerMgr.reset()
	n.replicateData(follower1, true, onAEReply)
	if peerMgr.isReq != nil || peerMgr.aeReq == nil {
		t.Error("replicateData should not replicate snapshot upon heartbeat")
	}
	if peerMgr.aeReq.LeaderID != n.nodeID || peerMgr.aeReq.Term != n.currentTerm ||
		peerMgr.aeReq.PrevLogIndex != logMgr.snapshotIndex || peerMgr.aeReq.PrevLogTerm != logMgr.snapshotTerm {
		t.Error("wrong info are being replicated")
	}
	if len(peerMgr.aeReq.Entries) != 0 {
		t.Error("wrong payload when nextIndex is higher than lastIndex")
	}

	// nextIndex is smaller than snapshotIndex
	peerMgr.reset()
	logMgr.snapshotIndex = 5
	logMgr.snapshotTerm = 3
	logMgr.lastSnapshotFile = "snapshotsmaller"
	follower1.nextIndex = 4
	n.replicateData(follower1, false, n.onHeartbeatReply)
	if peerMgr.isReq == nil {
		t.Error("replicateLogsTo should replicate snapshot but it didn't")
	}
	if peerMgr.isReq.LeaderID != n.nodeID || peerMgr.isReq.Term != n.currentTerm || peerMgr.isReq.File != logMgr.lastSnapshotFile || peerMgr.isReq.SnapshotIndex != logMgr.snapshotIndex || peerMgr.isReq.SnapshotTerm != logMgr.snapshotTerm {
		t.Error("wrong info in SnapshotRequest")
	}
}

func TestWonElection(t *testing.T) {
	n := &node{}
	n.clusterSize = 3
	n.votes = make(map[int]bool)

	n.votes[0] = true
	if n.wonElection() {
		t.Error("wonElection should return false on 1 vote out of 3")
	}

	n.votes[2] = true
	if !n.wonElection() {
		t.Error("wonElection should return true on 2 votes out of 3")
	}

}

type mockPeerManager struct {
	nodeID  int
	aeReq   *AppendEntriesRequest
	rvReq   *RequestVoteRequest
	isReq   *SnapshotRequest
	getReq  *GetRequest
	execReq *StateMachineCmd
	peer    *Peer
}

func (pm *mockPeerManager) reset() {
	pm.nodeID = 0
	pm.aeReq = nil
	pm.rvReq = nil
	pm.isReq = nil
	pm.getReq = nil
	pm.execReq = nil
}

func (pm *mockPeerManager) AppendEntries(nodeID int, req *AppendEntriesRequest, onReply func(*AppendEntriesReply)) {
	pm.nodeID = nodeID
	pm.aeReq = req
}
func (pm *mockPeerManager) RequestVote(nodeID int, req *RequestVoteRequest, onReply func(*RequestVoteReply)) {
	pm.nodeID = nodeID
	pm.rvReq = req
}
func (pm *mockPeerManager) BroadcastRequestVote(req *RequestVoteRequest, onReply func(*RequestVoteReply)) {
	pm.nodeID = -1 //means all
	pm.rvReq = req
}

func (pm *mockPeerManager) InstallSnapshot(nodeID int, req *SnapshotRequest, onReply func(*AppendEntriesReply)) {
	pm.nodeID = nodeID
	pm.isReq = req
}

func (pm *mockPeerManager) Get(nodeID int, req *GetRequest) (*GetReply, error) {
	pm.nodeID = nodeID
	pm.getReq = req

	return nil, nil
}

func (pm *mockPeerManager) Execute(nodeID int, cmd *StateMachineCmd) (*ExecuteReply, error) {
	pm.nodeID = nodeID
	pm.execReq = cmd
	return nil, nil
}

func (pm *mockPeerManager) GetAllPeers() map[int]*Peer {
	return nil
}

func (pm *mockPeerManager) GetPeer(nodeID int) *Peer {
	if pm.peer == nil {
		pm.peer = &Peer{
			NodeInfo: NodeInfo{
				NodeID: nodeID,
			},
		}
	}

	return pm.peer
}

func (pm *mockPeerManager) ResetFollowerIndicies(lastLogIndex int) {

}

func (pm *mockPeerManager) UpdateFollowerMatchIndex(nodeID int, match bool, lastMatch int) {

}
func (pm *mockPeerManager) QuorumReached(logIndex int) bool {
	return false
}
