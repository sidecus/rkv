package raft

import (
	"testing"

	"github.com/sidecus/raft/pkg/util"
)

// PeerProxy mock
type PeerProxyMock struct {
	nodeID int
}

func (proxy *PeerProxyMock) AppendEntries(req *AppendEntriesRequest) (*AppendEntriesReply, error) {
	return &AppendEntriesReply{
		NodeID:    proxy.nodeID,
		Term:      req.Term,
		LastMatch: req.PrevLogIndex + len(req.Entries),
		Success:   true,
	}, nil
}
func (proxy *PeerProxyMock) RequestVote(req *RequestVoteRequest) (*RequestVoteReply, error) {
	return nil, nil
}
func (proxy *PeerProxyMock) InstallSnapshot(req *SnapshotRequest) (*AppendEntriesReply, error) {
	return &AppendEntriesReply{
		NodeID:    proxy.nodeID,
		Term:      req.Term,
		LastMatch: req.SnapshotIndex,
		Success:   true,
	}, nil
}
func (proxy *PeerProxyMock) Get(req *GetRequest) (*GetReply, error) {
	return nil, nil
}
func (proxy *PeerProxyMock) Execute(cmd *StateMachineCmd) (*ExecuteReply, error) {
	return nil, nil
}

// PeerFactory mock
type PeerFactoryMock struct{}

func (f *PeerFactoryMock) NewPeerProxy(info NodeInfo) IPeerProxy {
	return &PeerProxyMock{
		nodeID: info.NodeID,
	}
}

func TestNewPeerManager(t *testing.T) {
	size := 5
	peerManager := createTestPeerManager(size).(*PeerManager)

	if len(peerManager.Peers) != size {
		t.Error("PeerManager created with wrong number of peers")
	}

	for i := 0; i < size; i++ {
		p := peerManager.GetPeer(i)
		if p.nextIndex != 0 || p.matchIndex != -1 {
			t.Error("follower indicies are not initialized correctly")
		}

		if p.NodeID != i {
			t.Error("peer node id is not initialized correctly")
		}
	}
}

func TestResetFollowerIndicies(t *testing.T) {
	peerManager := createTestPeerManager(3).(*PeerManager)
	peerManager.Peers[0].nextIndex = 5
	peerManager.Peers[0].matchIndex = 3
	peerManager.Peers[1].nextIndex = 10
	peerManager.Peers[1].matchIndex = 9
	peerManager.Peers[2].nextIndex = 6
	peerManager.Peers[2].matchIndex = -1

	peerManager.ResetFollowerIndicies(20)
	for _, p := range peerManager.Peers {
		if p.nextIndex != 21 || p.matchIndex != -1 {
			t.Fatal("reset doesn't reset on positive last index")
		}
	}

	peerManager.ResetFollowerIndicies(-1)
	for _, p := range peerManager.Peers {
		if p.nextIndex != 0 || p.matchIndex != -1 {
			t.Fatal("reset doesn't reset on -1 as last index")
		}
	}
}

func TestUpdateFollowerMatchIndex(t *testing.T) {
	peerManager := createTestPeerManager(3).(*PeerManager)

	peer0 := peerManager.GetPeer(0)

	// has new match
	peerManager.Peers[0].nextIndex = 5
	peerManager.Peers[0].matchIndex = 3
	peerManager.UpdateFollowerMatchIndex(0, true, -1)
	if peer0.nextIndex != 0 || peer0.matchIndex != -1 {
		t.Error("updateMatchIndex fails with successful match on -1")
	}

	peerManager.Peers[0].nextIndex = 5
	peerManager.Peers[0].matchIndex = 3
	peerManager.UpdateFollowerMatchIndex(0, true, 6)
	if peer0.nextIndex != 7 || peer0.matchIndex != 6 {
		t.Error("updateMatchIndex fails with successful match on 6")
	}

	// no match
	peerManager.Peers[0].nextIndex = 8
	peerManager.Peers[0].matchIndex = 3
	peerManager.UpdateFollowerMatchIndex(0, false, -2)
	if peer0.nextIndex != util.Max(0, 8-nextIndexFallbackStep) || peer0.matchIndex != 3 {
		t.Error("updateMatchIndex doesn't decrease nextIndex correctly upon failed match")
	}

	peerManager.Peers[0].nextIndex = 0
	peerManager.Peers[0].matchIndex = -1
	peerManager.UpdateFollowerMatchIndex(0, false, -2)
	if peer0.nextIndex != 0 || peer0.matchIndex != -1 {
		t.Error("updateMatchIndex unnecessarily decrease nextIndex when it's already 0 upon failure")
	}
}

func TestQuorumReached(t *testing.T) {
	peerManager := createTestPeerManager(2).(*PeerManager)

	peer0 := peerManager.GetPeer(0)
	peer1 := peerManager.GetPeer(1)

	if !peerManager.QuorumReached(-1) {
		t.Error("QuorumReached fails on -1 when should be")
	}

	if peerManager.QuorumReached(0) {
		t.Error("QuorumReached returns true on 0 when it should not")
	}

	peer0.matchIndex = 3
	peer1.matchIndex = 6

	for i := 0; i < 10; i++ {
		expected := i <= 6
		result := peerManager.QuorumReached(i)

		if expected != result {
			t.Errorf("QuorumReached failed on index %d", i)
		}
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
	peers := createTestPeerInfo(size)
	peerMgr := NewPeerManager(size, peers, &PeerFactoryMock{})

	return peerMgr
}
