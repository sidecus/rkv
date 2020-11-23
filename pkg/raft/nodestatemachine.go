package raft

// timerAction is the action we want to take on the given timer
type timerAction int

const (
	timerActionNoop  = 0
	timerActionStop  = 1
	timerActionReset = 2
)

// raftMsgHandler defines a message handler struct
type raftMsgHandler struct {
	handle               func(INode, *Message) bool
	nextState            nodeState
	electTimerAction     timerAction
	heartbeatTimerAction timerAction
}

//raftMsgHandlerMap defines map from message type to handler
type raftMsgHandlerMap map[MessageType]raftMsgHandler

// raftStateMachine defines map from state to MsgHandlerMap
type raftStateMachine map[nodeState]raftMsgHandlerMap

// processMessage runs a message through the node state machine
// if message is handled and state change is required, it'll perform other needed work
// including advancing node state and stoping/reseting related timers
func (sm raftStateMachine) processMessage(node INode, msg *Message) {
	handlerMap, valid := sm[node.getState()]
	if !valid {
		panic("Invalid state for node %d")
	}

	entry, ok := handlerMap[msg.msgType]
	if ok && entry.handle(node, msg) {
		// set new state
		node.setState(entry.nextState)

		// update election timer
		switch entry.electTimerAction {
		case timerActionStop:
			node.stopElectionTimer()
		case timerActionReset:
			node.resetElectionTimer()
		}

		// update heartbreat timer
		switch entry.heartbeatTimerAction {
		case timerActionStop:
			node.stopHeartbeatTimer()
		case timerActionReset:
			node.resetHeartbeatTimer()
		}
	}
}

func handleStartElection(node INode, msg *Message) bool {
	return node.startElection()
}

func handleSendHearbeat(node INode, msg *Message) bool {
	return node.sendHeartbeat()
}

func handleHeartbeat(node INode, msg *Message) bool {
	return node.ackHeartbeat(msg)
}

func handleVoteMsg(node INode, msg *Message) bool {
	return node.countVotes(msg)
}

func handleRequestVoteMsg(node INode, msg *Message) bool {
	return node.vote(msg)
}

// raftSM is the predefined node state machine, it manages raft node state transition
var raftSM = raftStateMachine{
	follower: {
		MsgStartElection: {
			handle:               handleStartElection,
			nextState:            candidate,
			electTimerAction:     timerActionReset,
			heartbeatTimerAction: timerActionStop,
		},
		MsgHeartbeat: {
			handle:               handleHeartbeat,
			nextState:            follower,
			electTimerAction:     timerActionReset,
			heartbeatTimerAction: timerActionStop,
		},
		MsgRequestVote: {
			handle:               handleRequestVoteMsg,
			nextState:            follower,
			electTimerAction:     timerActionNoop,
			heartbeatTimerAction: timerActionNoop,
		},
	},
	candidate: {
		MsgStartElection: {
			handle:               handleStartElection,
			nextState:            candidate,
			electTimerAction:     timerActionReset,
			heartbeatTimerAction: timerActionStop,
		},
		MsgHeartbeat: {
			handle:               handleHeartbeat,
			nextState:            follower,
			electTimerAction:     timerActionReset,
			heartbeatTimerAction: timerActionStop,
		},
		MsgRequestVote: {
			handle:               handleRequestVoteMsg,
			nextState:            follower,
			electTimerAction:     timerActionNoop,
			heartbeatTimerAction: timerActionNoop,
		},
		MsgVote: {
			handle:               handleVoteMsg,
			nextState:            leader,
			electTimerAction:     timerActionStop,
			heartbeatTimerAction: timerActionReset,
		},
	},
	leader: {
		MsgSendHeartBeat: {
			handle:               handleSendHearbeat,
			nextState:            leader,
			electTimerAction:     timerActionStop,
			heartbeatTimerAction: timerActionReset,
		},
		MsgHeartbeat: {
			handle:               handleHeartbeat,
			nextState:            follower,
			electTimerAction:     timerActionReset,
			heartbeatTimerAction: timerActionStop,
		},
		MsgRequestVote: {
			handle:               handleRequestVoteMsg,
			nextState:            follower,
			electTimerAction:     timerActionNoop,
			heartbeatTimerAction: timerActionNoop,
		},
	},
}
