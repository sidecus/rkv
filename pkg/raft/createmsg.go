package raft

import "github.com/sidecus/raft/pkg/network"

func (node *raftNode) createRequestVoteMessage() *network.Message {
	return &network.Message{
		NodeID:  node.id,
		Term:    node.term,
		MsgType: network.MsgRequestVote,
		Data:    node.id,
	}
}

func (node *raftNode) createStartElectionMessage() *network.Message {
	return &network.Message{
		NodeID:  node.id,
		Term:    node.term,
		MsgType: network.MsgStartElection,
		Data:    node.id,
	}
}

func (node *raftNode) createVoteMessage(electMsg *network.Message) *network.Message {
	return &network.Message{
		NodeID:  node.id,
		Term:    electMsg.Term,
		MsgType: network.MsgVote,
		Data:    electMsg.NodeID,
	}
}

func (node *raftNode) createHeartBeatMessage() *network.Message {
	return &network.Message{
		NodeID:  node.id,
		Term:    node.term,
		MsgType: network.MsgHeartbeat,
		Data:    node.id,
	}
}

func (node *raftNode) createSendHeartBeatMessage() *network.Message {
	return &network.Message{
		NodeID:  node.id,
		Term:    node.term,
		MsgType: network.MsgSendHeartBeat,
		Data:    node.id,
	}
}
