package raft

// StateMachineCmd holds one command to the statemachine
type StateMachineCmd struct {
	CmdType int
	Data    interface{}
}

// IStateMachine holds the interface to a statemachine
type IStateMachine interface {
	Apply(cmd StateMachineCmd)
	Init(cmds []StateMachineCmd)

	Get(param ...interface{}) (result interface{}, err error)
}
