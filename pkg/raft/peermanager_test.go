package raft

import (
	"context"
	"testing"
)

// PeerProxy mock
type MockPeerProxy struct {
	nodeID int
	aeReq  *AppendEntriesRequest
	isReq  *SnapshotRequest
}

func (proxy *MockPeerProxy) AppendEntries(ctx context.Context, req *AppendEntriesRequest) (*AppendEntriesReply, error) {
	proxy.aeReq = req
	return &AppendEntriesReply{
		NodeID:    proxy.nodeID,
		Term:      req.Term,
		LastMatch: req.PrevLogIndex + len(req.Entries),
		Success:   true,
	}, nil
}
func (proxy *MockPeerProxy) RequestVote(ctx context.Context, req *RequestVoteRequest) (*RequestVoteReply, error) {
	return nil, nil
}
func (proxy *MockPeerProxy) InstallSnapshot(ctx context.Context, req *SnapshotRequest) (*AppendEntriesReply, error) {
	proxy.isReq = req
	return &AppendEntriesReply{
		NodeID:    proxy.nodeID,
		Term:      req.Term,
		LastMatch: req.SnapshotIndex,
		Success:   true,
	}, nil
}
func (proxy *MockPeerProxy) Get(ctx context.Context, req *GetRequest) (*GetReply, error) {
	return nil, nil
}
func (proxy *MockPeerProxy) Execute(ctx context.Context, cmd *StateMachineCmd) (*ExecuteReply, error) {
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
	replicateFunc := func(p *Peer) int { return 3 }
	peers := createTestPeerInfo(size)
	peerMgr := newPeerManager(peers, replicateFunc, &MockPeerFactory{})

	return peerMgr
}

func TestNewPeerManager(t *testing.T) {
	size := 5
	peerMgr := createTestPeerManager(size).(*peerManager)

	if len(peerMgr.peers) != size {
		t.Error("PeerManager created with wrong number of peers")
	}

	for i := 0; i < size; i++ {
		p := peerMgr.getPeer(i)
		if p.NodeID != i {
			t.Error("peer node id is not initialized correctly")
		}
		if p.nextIndex != 0 || p.matchIndex != -1 {
			t.Error("follower indicies are not initialized correctly")
		}
	}
}

func TestResetFollowerIndicies(t *testing.T) {
	mgr := createTestPeerManager(3).(*peerManager)
	mgr.getPeer(0).nextIndex = 5
	mgr.getPeer(0).matchIndex = 3
	mgr.getPeer(1).nextIndex = 10
	mgr.getPeer(1).matchIndex = 9
	mgr.getPeer(2).nextIndex = 6
	mgr.getPeer(2).matchIndex = -1

	mgr.resetFollowerIndicies(20)
	for _, p := range mgr.peers {
		if p.nextIndex != 21 || p.matchIndex != -1 {
			t.Fatal("reset doesn't reset on positive last index")
		}
	}

	mgr.resetFollowerIndicies(-1)
	for _, p := range mgr.peers {
		if p.nextIndex != 0 || p.matchIndex != -1 {
			t.Fatal("reset doesn't reset on -1 as last index")
		}
	}
}

func TestQuorumReached(t *testing.T) {
	mgr := createTestPeerManager(2)

	follower0 := mgr.getPeer(0)
	follower1 := mgr.getPeer(1)

	if !mgr.quorumReached(-1) {
		t.Error("QuorumReached fails on -1 when should be")
	}

	if mgr.quorumReached(0) {
		t.Error("QuorumReached returns true on 0 when it should not")
	}

	follower0.matchIndex = 3
	follower1.matchIndex = 6

	for i := 0; i < 10; i++ {
		expected := i <= 6
		result := mgr.quorumReached(i)

		if expected != result {
			t.Errorf("QuorumReached failed on index %d", i)
		}
	}
}
