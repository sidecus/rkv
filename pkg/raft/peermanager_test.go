package raft

type testPeerProxy struct {
}

// Raft related
func (proxy *testPeerProxy) AppendEntries(req *AppendEntriesRequest, callback func(*AppendEntriesReply)) {

}
func (proxy *testPeerProxy) RequestVote(req *RequestVoteRequest, callback func(*RequestVoteReply)) {

}

// Data related
func (proxy *testPeerProxy) Get(req *GetRequest) (*GetReply, error) {
	return nil, nil
}

func (proxy *testPeerProxy) Execute(cmd *StateMachineCmd) (*ExecuteReply, error) {
	return nil, nil
}

type testPeerFactory struct{}

func (f *testPeerFactory) NewPeerProxy(info PeerInfo) IPeerProxy {
	return &testPeerProxy{}
}

type testPeerManager struct {
	lastAENodeID int
	lastAEReq    *AppendEntriesRequest
}

func (tpm *testPeerManager) AppendEntries(nodeID int, req *AppendEntriesRequest, callback func(*AppendEntriesReply)) {
	tpm.lastAENodeID = nodeID
	tpm.lastAEReq = req
}

func (tpm *testPeerManager) BroadcastAppendEntries(req *AppendEntriesRequest, callback func(*AppendEntriesReply)) {

}
func (tpm *testPeerManager) RequestVote(nodeID int, req *RequestVoteRequest, callback func(*RequestVoteReply)) {

}
func (tpm *testPeerManager) BroadcastRequestVote(req *RequestVoteRequest, callback func(*RequestVoteReply)) {

}
func (tpm *testPeerManager) Get(nodeID int, req *GetRequest) (*GetReply, error) {
	return nil, nil
}

func (tpm *testPeerManager) Execute(nodeID int, cmd *StateMachineCmd) (*ExecuteReply, error) {
	return nil, nil
}
