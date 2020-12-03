package raft

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
