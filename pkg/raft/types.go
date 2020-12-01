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
	NodeID    int
	Term      int
	LeaderID  int
	LastIndex int
	Success   bool
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
	NodeID int
	Data   interface{}
}

// ExecuteReply is used to reply to Execute
type ExecuteReply struct {
	NodeID  int
	Success bool
}
