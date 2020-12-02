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

func TestAppendCmd(t *testing.T) {
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

func TestHasMatchingPrevEntry(t *testing.T) {
	lm := logManager{
		logs:      make([]LogEntry, 100),
		lastIndex: 10,
	}
	lm.logs[9].Term = 4
	lm.logs[10].Term = 5

	if !lm.hasMatchingPrevEntry(-1, -1) {
		t.Error("hasMatchingPrevEntry should return true on -1, -1")
	}

	if lm.hasMatchingPrevEntry(11, 5) {
		t.Error("hasMatchingPrevEntry should return false when prevLogIndex is larger than lastIndex")
	}

	if lm.hasMatchingPrevEntry(9, 5) {
		t.Error("hasMatchingPrevEntry should return false when entry doesn't match")
	}

	if !lm.hasMatchingPrevEntry(10, 5) {
		t.Error("hasMatchingPrevEntry should return true when prev entry matches")
	}
}

func TestAppendLogs(t *testing.T) {
	lm := &logManager{
		logs:      make([]LogEntry, 5),
		lastIndex: 4,
		lastTerm:  3,
	}
	lm.logs[0].Term = 1
	lm.logs[1].Term = 1
	lm.logs[2].Term = 2
	lm.logs[3].Term = 2
	lm.logs[4].Term = 3

	// empty entries
	if lm.appendLogs(6, 5, make([]LogEntry, 0)) {
		t.Error("appendLogs should return false on nonmatching prevIndex/prevTerm")
	}
	if lm.lastIndex != 4 {
		t.Error("appendLogs should not modify lastIndex on nonmatching prev entry")
	}

	if !lm.appendLogs(4, 3, make([]LogEntry, 0)) {
		t.Error("appendLogs should return true on matching prevIndex/prevTerm")
	}
	if lm.lastIndex != 4 {
		t.Error("appendLogs should not modify lastIndex on empty entries")
	}

	// entries are much newer than logs we have
	entries := generateTestEntries(5, 3)
	if lm.appendLogs(5, 3, entries) {
		t.Error("appendLogs should return false on nonmatching prevIndex/prevTerm when entries is non empty")
	}
	if lm.lastIndex != 4 {
		t.Error("appendLogs should not modify logs for much newer logs")
	}

	// simple append
	entries = generateTestEntries(4, 10)
	if !lm.appendLogs(4, 3, entries) {
		t.Error("appendLogs should return true on correct new logs")
	}
	if lm.lastIndex != 6 || lm.lastTerm != 10 {
		t.Error("appendLogs should append correct new logs")
	}

	// 1 overlapping bad entry
	entries = generateTestEntries(3, 10)
	if !lm.appendLogs(3, 2, entries) {
		t.Error("appendLogs should return true by skiping non matching entries")
	}
	if lm.lastIndex != 5 || lm.lastTerm != 10 {
		t.Error("appendLogs skip bad entries and append rest good ones")
	}

	// all entries are overlapping and non matching
	entries = generateTestEntries(2, 10)
	if !lm.appendLogs(2, 2, entries) {
		t.Error("appendLogs should return true by skiping non matching entries")
	}
	if lm.lastIndex != 4 || lm.lastTerm != 10 || len(lm.logs) != lm.lastIndex+1 {
		t.Error("appendLogs append all new good entries")
	}

	// empty logs scenario
	lm.lastIndex = -1
	lm.lastTerm = -1
	entries = generateTestEntries(-1, 10)
	if !lm.appendLogs(-1, -1, entries) {
		t.Error("appendLogs should append new entries when it's empty")
	}
	if lm.lastIndex != 1 || lm.lastTerm != 10 || len(lm.logs) != lm.lastIndex+1 {
		t.Error("appendLogs append all new good entries when it's empty")
	}
}

func TestAppend(t *testing.T) {
	lm := &logManager{
		logs:      make([]LogEntry, 5),
		lastIndex: 4,
		lastTerm:  3,
	}
	lm.logs[4].Term = 3

	entries := make([]LogEntry, 0)
	lm.append(entries)
	if lm.lastIndex != 4 || lm.lastTerm != 3 {
		t.Error("append doesn't update lastIndex/lastTerm correctly on empty input")
	}

	entries = generateTestEntries(4, 20)
	lm.append(entries)
	if lm.lastIndex != 6 || lm.lastTerm != 20 || len(lm.logs) != 7 {
		t.Error("append doesn't update lastIndex/lastTerm correctly on non empty input")
	}
}

func TestCommit(t *testing.T) {
	sm := &testStateMachine{lastApplied: -1}
	lm := &logManager{
		logs:         make([]LogEntry, 0),
		lastIndex:    -1,
		lastTerm:     -1,
		lastApplied:  -1,
		statemachine: sm,
	}

	// append two logs to it
	entries := generateTestEntries(-1, 1)
	lm.appendLogs(-1, -1, entries)

	// try commit to a much larger index
	ret := lm.commit(3)
	if !ret {
		t.Error("commit to larger index should commit to last log entry correctly")
	}
	if lm.commitIndex != lm.lastIndex {
		t.Error("commit should update commitIndex correctly")
	}
	if lm.lastApplied != lm.lastIndex || lm.lastApplied != sm.lastApplied {
		t.Error("commit should apply entries to state machine as appropriate")
	}
	if !lm.logs[lm.lastIndex].Committed {
		t.Error("commit doesn't update entry Committed state correctly")
	}

	// commit again does nothing
	ret = lm.commit(5)
	if ret {
		t.Error("commit should be idempotent, and return false on second try")
	}

	if lm.commitIndex != 1 || lm.lastApplied != 1 || lm.lastApplied != sm.lastApplied {
		t.Error("noop commit not change anything")
	}
}

type testStateMachine struct {
	lastApplied int
}

func (sm *testStateMachine) Apply(cmd StateMachineCmd) {
	data := cmd.Data.(int)
	sm.lastApplied = data
}

func (sm *testStateMachine) Get(param ...interface{}) (result interface{}, err error) {
	return 0, nil
}

func generateTestEntries(prevIndex, newTerm int) []LogEntry {
	entries := make([]LogEntry, 2)
	entries[0].Index = prevIndex + 1
	entries[0].Term = newTerm
	entries[0].Cmd.Data = prevIndex + 1
	entries[1].Index = prevIndex + 2
	entries[1].Term = newTerm
	entries[1].Cmd.Data = prevIndex + 2

	return entries
}
