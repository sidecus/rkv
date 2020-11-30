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
	cmds := make([]StateMachineCmd, 10)
	if lm.lastIndex != -1 {
		t.Error("LastIndex is not -1 upon init")
	}

	if lm.commitIndex != -1 {
		t.Error("CommitIndex is not -1 upon init")
	}

	if lm.lastApplied != -1 {
		t.Error("LastApplied is not -1 upon init")
	}

	lm.appendCmds(cmds, 3)
	if lm.lastIndex != len(cmds)-1 {
		t.Error("LastIndex is incorrect")
	}
	if len(lm.logs) != len(cmds) {
		t.Error("append failed")
	}
	for i, v := range lm.logs {
		if v.Index != i {
			t.Error("appended log entry doesn't have correct index")
			t.FailNow()
		}
		if v.Term != 3 {
			t.Error("appended log entry doesn't have correct term")
			t.FailNow()
		}
	}

	start := len(cmds)
	lm.appendCmds(cmds, 4)
	if lm.lastIndex != start+len(cmds)-1 {
		t.Error("LastIndex is incorrect")
	}
	end := len(cmds)
	newlogs := lm.logs[start:end]
	for i, v := range newlogs {
		if v.Index != start+i {
			t.Error("appended log entry doesn't have correct index")
			t.FailNow()
		}
		if v.Term != 4 {
			t.Error("appended log entry doesn't have correct term")
			t.FailNow()
		}
	}
}
