package raft

import (
	"testing"

	"github.com/sidecus/raft/pkg/util"
)

// PeerProxy mock
type MockPeerProxy struct {
	nodeID int
	aeReq  *AppendEntriesRequest
	isReq  *SnapshotRequest
}

func (proxy *MockPeerProxy) AppendEntries(req *AppendEntriesRequest) (*AppendEntriesReply, error) {
	proxy.aeReq = req
	return &AppendEntriesReply{
		NodeID:    proxy.nodeID,
		Term:      req.Term,
		LastMatch: req.PrevLogIndex + len(req.Entries),
		Success:   true,
	}, nil
}
func (proxy *MockPeerProxy) RequestVote(req *RequestVoteRequest) (*RequestVoteReply, error) {
	return nil, nil
}
func (proxy *MockPeerProxy) InstallSnapshot(req *SnapshotRequest) (*AppendEntriesReply, error) {
	proxy.isReq = req
	return &AppendEntriesReply{
		NodeID:    proxy.nodeID,
		Term:      req.Term,
		LastMatch: req.SnapshotIndex,
		Success:   true,
	}, nil
}
func (proxy *MockPeerProxy) Get(req *GetRequest) (*GetReply, error) {
	return nil, nil
}
func (proxy *MockPeerProxy) Execute(cmd *StateMachineCmd) (*ExecuteReply, error) {
	return nil, nil
}

// PeerFactory mock
type MockPeerFactory struct{}

func (f *MockPeerFactory) NewPeerProxy(info NodeInfo) IPeerProxy {
	return &MockPeerProxy{
		nodeID: info.NodeID,
	}
}

// Create n peers with index from 0 to n-1
func createTestPeerInfo(n int) map[int]NodeInfo {
	peers := make(map[int]NodeInfo)
	for i := 0; i < n; i++ {
		peers[i] = NodeInfo{NodeID: i}
	}

	return peers
}

func createTestPeerManager(size int) IPeerManager {
	replicateFunc := func(int) {}
	peers := createTestPeerInfo(size)
	peerMgr := NewPeerManager(size, peers, replicateFunc, &MockPeerFactory{})

	return peerMgr
}

func TestNewPeerManager(t *testing.T) {
	size := 5
	peerManager := createTestPeerManager(size).(*PeerManager)

	if len(peerManager.Peers) != size {
		t.Error("PeerManager created with wrong number of peers")
	}

	for i := 0; i < size; i++ {
		p := peerManager.GetPeer(i)
		if p.NodeID != i {
			t.Error("peer node id is not initialized correctly")
		}
		if p.nextIndex != 0 || p.matchIndex != -1 {
			t.Error("follower indicies are not initialized correctly")
		}
	}
}

func TestResetFollowerIndicies(t *testing.T) {
	mgr := createTestPeerManager(3).(*PeerManager)
	mgr.GetPeer(0).nextIndex = 5
	mgr.GetPeer(0).matchIndex = 3
	mgr.GetPeer(1).nextIndex = 10
	mgr.GetPeer(1).matchIndex = 9
	mgr.GetPeer(2).nextIndex = 6
	mgr.GetPeer(2).matchIndex = -1

	mgr.ResetFollowerIndicies(20)
	for _, p := range mgr.Peers {
		if p.nextIndex != 21 || p.matchIndex != -1 {
			t.Fatal("reset doesn't reset on positive last index")
		}
	}

	mgr.ResetFollowerIndicies(-1)
	for _, p := range mgr.Peers {
		if p.nextIndex != 0 || p.matchIndex != -1 {
			t.Fatal("reset doesn't reset on -1 as last index")
		}
	}
}

func TestQuorumReached(t *testing.T) {
	mgr := createTestPeerManager(2).(*PeerManager)

	follower0 := mgr.GetPeer(0)
	follower1 := mgr.GetPeer(1)

	if !mgr.QuorumReached(-1) {
		t.Error("QuorumReached fails on -1 when should be")
	}

	if mgr.QuorumReached(0) {
		t.Error("QuorumReached returns true on 0 when it should not")
	}

	follower0.matchIndex = 3
	follower1.matchIndex = 6

	for i := 0; i < 10; i++ {
		expected := i <= 6
		result := mgr.QuorumReached(i)

		if expected != result {
			t.Errorf("QuorumReached failed on index %d", i)
		}
	}
}

func TestUpdateMatchIndex(t *testing.T) {
	mgr := createTestPeerManager(3).(*PeerManager)

	follower0 := mgr.GetPeer(0)

	// has new match
	follower0.nextIndex = 5
	follower0.matchIndex = 3
	follower0.UpdateMatchIndex(true, -1)
	if follower0.nextIndex != 0 || follower0.matchIndex != -1 {
		t.Error("updateMatchIndex fails with successful match on -1")
	}

	follower0.nextIndex = 5
	follower0.matchIndex = 3
	follower0.UpdateMatchIndex(true, 6)
	if follower0.nextIndex != 7 || follower0.matchIndex != 6 {
		t.Error("updateMatchIndex fails with successful match on 6")
	}

	// no match
	follower0.nextIndex = 8
	follower0.matchIndex = 3
	follower0.UpdateMatchIndex(false, -2)
	if follower0.nextIndex != util.Max(0, 8-nextIndexFallbackStep) || follower0.matchIndex != -1 {
		t.Error("updateMatchIndex doesn't decrease nextIndex correctly or set match index to -1 upon failed match")
	}

	follower0.nextIndex = 0
	follower0.matchIndex = -1
	follower0.UpdateMatchIndex(false, -2)
	if follower0.nextIndex != 0 || follower0.matchIndex != -1 {
		t.Error("updateMatchIndex unnecessarily decrease nextIndex when it's already 0 upon failure")
	}
}
