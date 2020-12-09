package raft

import "io"

// StateMachineCmd holds one command to the statemachine
type StateMachineCmd struct {
	CmdType int
	Data    interface{}
}

// IValueGetter defines an interface to get a value
type IValueGetter interface {
	Get(param ...interface{}) (interface{}, error)
}

// IStateMachine is the interface for the underneath statemachine
// detailed implementation needs to have proper R/W locks
// Writer locks for Apply/Deserialize
// Reader locks for Serialize/Get
type IStateMachine interface {
	Apply(cmd StateMachineCmd)
	Serialize(io.Writer) error
	Deserialize(reader io.Reader) error
	IValueGetter
}
