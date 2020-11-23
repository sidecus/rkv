package core

// RaftMessageType message type used by raft nodes
type RaftMessageType string

// allowed raftMessageType values
const (
	MsgVote          = "Vote"
	MsgRequestVote   = "RequestVote"
	MsgHeartbeat     = "Heartbeat"
	MsgStartElection = "StartElection" // dummy message to handle new election
	MsgSendHeartBeat = "SendHeartbeat" // dummy message to send heartbeat
)

// RaftMessage object used by raft
type RaftMessage struct {
	nodeID  int
	term    int
	msgType RaftMessageType
	data    int
}

func (node *raftNode) createRequestVoteMessage() *RaftMessage {
	return &RaftMessage{
		nodeID:  node.id,
		term:    node.term,
		msgType: MsgRequestVote,
		data:    node.id,
	}
}

func (node *raftNode) createStartElectionMessage() *RaftMessage {
	return &RaftMessage{
		nodeID:  node.id,
		term:    node.term,
		msgType: MsgStartElection,
		data:    node.id,
	}
}

func (node *raftNode) createVoteMessage(electMsg *RaftMessage) *RaftMessage {
	return &RaftMessage{
		nodeID:  node.id,
		term:    electMsg.term,
		msgType: MsgVote,
		data:    electMsg.nodeID,
	}
}

func (node *raftNode) createHeartBeatMessage() *RaftMessage {
	return &RaftMessage{
		nodeID:  node.id,
		term:    node.term,
		msgType: MsgHeartbeat,
		data:    node.id,
	}
}

func (node *raftNode) createSendHeartBeatMessage() *RaftMessage {
	return &RaftMessage{
		nodeID:  node.id,
		term:    node.term,
		msgType: MsgSendHeartBeat,
		data:    node.id,
	}
}
