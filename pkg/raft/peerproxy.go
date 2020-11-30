package raft

// PeerInfo contains info for a peer node
type PeerInfo struct {
	NodeID   int
	Endpoint string
}

// IPeerProxy is used for a node to talk to its peers
type IPeerProxy interface {
	// Raft related, if no nodeid param it's boroadcasting
	AppendEntries(nodeID int, req *AppendEntriesRequest, callback func(*AppendEntriesReply))
	BroadcastAppendEntries(req *AppendEntriesRequest, callback func(*AppendEntriesReply))
	RequestVote(req *RequestVoteRequest, callback func(*RequestVoteReply))

	// Data related, this need leader id
	Get(nodeID int, req *GetRequest) (*GetReply, error)
	Execute(nodeID int, cmd *StateMachineCmd) (bool, error)
}
