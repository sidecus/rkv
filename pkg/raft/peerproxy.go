package raft

// PeerInfo contains info for a peer node
type PeerInfo struct {
	NodeID   int
	Endpoint string
}

// IPeerProxy is used for a node to talk to its peers
type IPeerProxy interface {
	// Raft related, these are boroadcasting so no node id required
	AppendEntries(req *AppendEntriesRequest, callback func(*AppendEntriesReply))
	RequestVote(req *RequestVoteRequest, callback func(*RequestVoteReply))

	// Data related, this need leader id
	Get(nodeID int, req *GetRequest) (*GetReply, error)
	Execute(nodeID int, cmd *StateMachineCmd) (bool, error)
}
