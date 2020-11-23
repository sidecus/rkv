package raft

import (
	"github.com/sidecus/raft/internal/net"
)

func (node *raftNode) createRequestVoteMessage() *net.Message {
	return &net.Message{
		NodeID:  node.id,
		Term:    node.term,
		MsgType: net.MsgRequestVote,
		Data:    node.id,
	}
}

func (node *raftNode) createStartElectionMessage() *net.Message {
	return &net.Message{
		NodeID:  node.id,
		Term:    node.term,
		MsgType: net.MsgStartElection,
		Data:    node.id,
	}
}

func (node *raftNode) createVoteMessage(electMsg *net.Message) *net.Message {
	return &net.Message{
		NodeID:  node.id,
		Term:    electMsg.Term,
		MsgType: net.MsgVote,
		Data:    electMsg.NodeID,
	}
}

func (node *raftNode) createHeartBeatMessage() *net.Message {
	return &net.Message{
		NodeID:  node.id,
		Term:    node.term,
		MsgType: net.MsgHeartbeat,
		Data:    node.id,
	}
}

func (node *raftNode) createSendHeartBeatMessage() *net.Message {
	return &net.Message{
		NodeID:  node.id,
		Term:    node.term,
		MsgType: net.MsgSendHeartBeat,
		Data:    node.id,
	}
}
