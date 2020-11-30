package raft

// StateMachineCmd holds one command to the statemachine
type StateMachineCmd struct {
	CmdType int
	Data    interface{}
}

// IStateMachine holds the interface to a statemachine
type IStateMachine interface {
	Apply(cmd StateMachineCmd)
	Get(param ...interface{}) (result interface{}, err error)
}

// NodeState is the state of the node
type NodeState int

const (
	//Follower state
	Follower = 1
	// Candidate state
	Candidate = 2
	// Leader state
	Leader = 3
)

// INode represents one raft node
type INode interface {
	// Raft
	AppendEntries(*AppendEntriesRequest) (*AppendEntriesReply, error)
	RequestVote(*RequestVoteRequest) (*RequestVoteReply, error)
	OnTimer()

	// Data related
	Get(*GetRequest) (*GetReply, error)
	Execute(*StateMachineCmd) (bool, error)

	// Lifecycle
	Start()
	Stop()
}

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
