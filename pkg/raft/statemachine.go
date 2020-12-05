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
type IStateMachine interface {
	Apply(cmd StateMachineCmd)
	Snapshot() (io.Reader, error)
	IValueGetter
}
