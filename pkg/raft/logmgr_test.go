package raft

import (
	"testing"
)

// IStateMachine holds the interface to a statemachine
type dummyStateMachine struct {
}

func (sm *dummyStateMachine) Apply(cmd StateMachineCmd) {

}
func (sm *dummyStateMachine) Get(param ...interface{}) (result interface{}, err error) {
	return "a", nil
}

func TestAppend(t *testing.T) {
	lm := newLogMgr(&dummyStateMachine{})
	cmd := StateMachineCmd{}
	if lm.lastIndex != -1 {
		t.Error("LastIndex is not -1 upon init")
	}

	if lm.commitIndex != -1 {
		t.Error("CommitIndex is not -1 upon init")
	}

	if lm.lastApplied != -1 {
		t.Error("LastApplied is not -1 upon init")
	}

	lm.appendCmd(cmd, 3)
	lm.appendCmd(cmd, 3)
	lm.appendCmd(cmd, 3)
	if lm.lastIndex != 2 {
		t.Error("LastIndex is incorrect")
	}
	if len(lm.logs) != 3 {
		t.Error("append failed")
	}
	for i, v := range lm.logs {
		if v.Index != i {
			t.Fatal("appended log entry doesn't have correct index")
		}
		if v.Term != 3 {
			t.Fatal("appended log entry doesn't have correct term")
		}
	}

	start := lm.lastIndex
	end := start + 20
	for i := start; i < end; i++ {
		lm.appendCmd(cmd, 4)
	}
	if lm.lastIndex != end {
		t.Error("LastIndex is incorrect")
	}
	newlogs := lm.logs[start+1 : end+1]
	for i, v := range newlogs {
		if v.Index != start+1+i {
			t.Fatal("appended log entry doesn't have correct index")
		}
		if v.Term != 4 {
			t.Fatal("appended log entry doesn't have correct term")
		}
	}
}
