package raft

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
	nodeID  int
	term    int
	msgType MessageType
	data    int
}

func (node *raftNode) createRequestVoteMessage() *Message {
	return &Message{
		nodeID:  node.id,
		term:    node.term,
		msgType: MsgRequestVote,
		data:    node.id,
	}
}

func (node *raftNode) createStartElectionMessage() *Message {
	return &Message{
		nodeID:  node.id,
		term:    node.term,
		msgType: MsgStartElection,
		data:    node.id,
	}
}

func (node *raftNode) createVoteMessage(electMsg *Message) *Message {
	return &Message{
		nodeID:  node.id,
		term:    electMsg.term,
		msgType: MsgVote,
		data:    electMsg.nodeID,
	}
}

func (node *raftNode) createHeartBeatMessage() *Message {
	return &Message{
		nodeID:  node.id,
		term:    node.term,
		msgType: MsgHeartbeat,
		data:    node.id,
	}
}

func (node *raftNode) createSendHeartBeatMessage() *Message {
	return &Message{
		nodeID:  node.id,
		term:    node.term,
		msgType: MsgSendHeartBeat,
		data:    node.id,
	}
}
