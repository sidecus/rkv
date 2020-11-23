package network

// MessageType message type used by raft nodes
type MessageType string

// allowed raftMessageType values
const (
	MsgVote          = "Vote"
	MsgRequestVote   = "RequestVote"
	MsgHeartbeat     = "Heartbeat"
	MsgStartElection = "StartElection" // dummy message to handle new election
	MsgSendHeartBeat = "SendHeartbeat" // dummy message to send heartbeat
)

// Message object used by raft
type Message struct {
	NodeID  int
	Term    int
	MsgType MessageType
	Data    int
}

// BoradcastAddress is a special NodeId representing broadcasting to all other nodes
const BoradcastAddress = -1

// Request request type used by network internally to send Message to nodes
type Request struct {
	Sender   int
	Receiver int
	Message  *Message
}
