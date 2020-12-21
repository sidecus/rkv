package raft

import (
	"testing"
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
		if p.NodeID != i {
			t.Error("peer node id is not initialized correctly")
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
