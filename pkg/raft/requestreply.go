package raft

// AppendEntriesRequest event payload type for AE calls
type AppendEntriesRequest struct {
	Term         int
	LeaderID     int
	PrevLogIndex int
	PrevLogTerm  int
	Entries      []LogEntry
	LeaderCommit int
}

// AppendEntriesReply reply type for AE calls
type AppendEntriesReply struct {
	NodeID   int
	Term     int
	LeaderID int
	Success  bool
}

// RequestVoteRequest request type for RV calls
type RequestVoteRequest struct {
	Term         int
	CandidateID  int
	LastLogIndex int
	LastLogTerm  int
}

// RequestVoteReply reply type for RV calls
type RequestVoteReply struct {
	NodeID      int
	Term        int
	VotedTerm   int
	VoteGranted bool
}

// GetRequest is used for an get operation
type GetRequest struct {
	Params []interface{}
}

// GetReply is used to reply to GetRequest
type GetReply struct {
	Data interface{}
}

// NewAppendEntriesRequest creates a new AppendEntriesRequest
func NewAppendEntriesRequest(term, leaderID, prevLogIndex, prevLogTerm, leaderCommit int, entries []LogEntry) *AppendEntriesRequest {
	req := &AppendEntriesRequest{
		Term:         term,
		LeaderID:     leaderID,
		PrevLogIndex: prevLogIndex,
		PrevLogTerm:  prevLogTerm,
		LeaderCommit: leaderCommit,
		Entries:      entries,
	}

	return req
}

// NewAppendEntriesReply creates an append entries reply
func NewAppendEntriesReply(nodeID, term, leaderID int, success bool) *AppendEntriesReply {
	req := &AppendEntriesReply{
		NodeID:   nodeID,
		LeaderID: leaderID,
		Term:     term,
		Success:  success,
	}

	return req
}

// NewRequestVoteRequest creates a new request vote
func NewRequestVoteRequest(term, candidateID, lastLogIndex, lastLogTerm int) *RequestVoteRequest {
	return &RequestVoteRequest{
		Term:         term,
		CandidateID:  candidateID,
		LastLogIndex: lastLogIndex,
		LastLogTerm:  lastLogTerm,
	}
}

// NewRequestVoteReply creates a reply for RV requests from the node
func NewRequestVoteReply(nodeID, term, votedTerm int, voteGranted bool) *RequestVoteReply {
	return &RequestVoteReply{
		NodeID:      nodeID,
		Term:        term,
		VotedTerm:   votedTerm,
		VoteGranted: voteGranted,
	}
}

// NewGetRequest creates a new get event
func NewGetRequest(params ...interface{}) *GetRequest {
	req := &GetRequest{
		Params: params,
	}

	return req
}

// NewGetReply creates a get reply
func NewGetReply(data interface{}) *GetReply {
	return &GetReply{
		Data: data,
	}
}
