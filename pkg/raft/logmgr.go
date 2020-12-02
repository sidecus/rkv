package raft

import "github.com/sidecus/raft/pkg/util"

const maxAppendEntriesCount = 5

// StateMachineCmd holds one command to the statemachine
type StateMachineCmd struct {
	CmdType int
	Data    interface{}
}

// IStateMachine holds the interface to a statemachine
type IStateMachine interface {
	Apply(cmd StateMachineCmd)
	Get(param ...interface{}) (result interface{}, err error)
}

// LogEntry - one raft log entry, with term and index
type LogEntry struct {
	Index int
	Term  int
	Cmd   StateMachineCmd
}

// logManager contains the array of logs
type logManager struct {
	commitIndex int
	lastApplied int
	lastIndex   int
	lastTerm    int

	// logs should be read from persistent storage upon init
	logs []LogEntry

	// reference to statemachien for commit operations
	statemachine IStateMachine
}

// newLogMgr creates a new logmgr
func newLogMgr(sm IStateMachine) *logManager {
	lm := &logManager{
		commitIndex:  -1,
		lastIndex:    -1,
		lastTerm:     -1,
		lastApplied:  -1,
		logs:         make([]LogEntry, 0),
		statemachine: sm,
	}

	return lm
}

// append appends a set of cmds for the given term to the logs
// this should be called by leader when accepting client requests
func (lm *logManager) appendCmd(cmd StateMachineCmd, term int) {
	entry := LogEntry{
		Index: lm.lastIndex + 1,
		Cmd:   cmd,
		Term:  term,
	}
	entries := []LogEntry{entry}
	lm.append(entries)
}

// appendLogs handles replicated logs from leader
// returns true if we entries matching prevLogIndex/prevLogTerm, and if that's the case, log
// entries are processed and appended as appropriate
func (lm *logManager) appendLogs(prevLogIndex, prevLogTerm int, entries []LogEntry) (prevMatch bool) {
	lm.validateLogEntries(prevLogIndex, prevLogTerm, entries)

	prevMatch = lm.hasMatchingPrevEntry(prevLogIndex, prevLogTerm)
	if !prevMatch {
		return
	}

	num := len(entries)

	if num <= 0 {
		// nothing to append
		return
	}

	start, end := entries[0].Index, entries[0].Index+num

	// Find first non-matching index
	var index int
	for index = start; index < end; index++ {
		if index > lm.lastIndex || entries[index-start].Term != lm.logs[index].Term {
			break
		}
	}

	// Drop all entries after the first non-matching and append new ones
	lm.logs = lm.logs[:index]
	lm.append(entries[(index - start):])

	return
}

func (lm *logManager) commit(targetIndex int) bool {
	// cap to lastIndex
	targetIndex = util.Min(targetIndex, lm.lastIndex)

	if targetIndex <= lm.commitIndex {
		return false // nothing more to commit
	}

	// Set new commit index
	lm.commitIndex = targetIndex

	// Apply commands to state machine if needed
	if lm.commitIndex > lm.lastApplied {
		for i := lm.lastApplied + 1; i <= lm.commitIndex; i++ {
			lm.statemachine.Apply(lm.logs[i].Cmd)
		}
		lm.lastApplied = lm.commitIndex
	}

	return true
}

// createAERequest creates an AppendEntriesRequest with proper log payload
func (lm *logManager) createAERequest(term, leaderID, nextIdx int) *AppendEntriesRequest {
	if nextIdx < 0 || nextIdx > lm.lastIndex+1 {
		panic("nextIdx shall never be less than zero or larger than lastLogIndex+1")
	}

	prevIdx := nextIdx - 1
	prevTerm := -1
	if prevIdx >= 0 {
		prevTerm = lm.logs[prevIdx].Term
	}

	nextNext := util.Min(nextIdx+maxAppendEntriesCount, lm.lastIndex+1)

	req := &AppendEntriesRequest{
		Term:         term,
		LeaderID:     leaderID,
		PrevLogIndex: prevIdx,
		PrevLogTerm:  prevTerm,
		Entries:      lm.logs[nextIdx:nextNext],
		LeaderCommit: lm.commitIndex,
	}

	return req
}

// check to see whether we have a matching entry @prevLogIndex with prevLogTerm
func (lm *logManager) hasMatchingPrevEntry(prevLogIndex, prevLogTerm int) bool {
	if prevLogIndex == -1 && prevLogTerm == -1 {
		// empty logs after init, agree
		return true
	}

	if prevLogIndex > lm.lastIndex {
		return false
	}

	return lm.logs[prevLogIndex].Term == prevLogTerm
}

// validates incoming logs, panicing on bad data
func (lm *logManager) validateLogEntries(prevLogIndex, prevLogTerm int, entries []LogEntry) {
	if prevLogIndex < 0 && prevLogIndex != -1 {
		panic("invalid prevLogIndex, less than 0 but not -1")
	}

	if prevLogTerm < 0 && prevLogTerm != -1 {
		panic("invalid prevLogTerm, less than 0 but not -1")
	}

	if prevLogIndex+prevLogTerm < 0 && prevLogIndex*prevLogTerm < 0 {
		panic("prevLogIndex or prevLogTerm is -1 but the other is not")
	}

	for i, v := range entries {
		if v.Index != prevLogIndex+1+i {
			panic("new entries index is incorrect")
		}
	}
}

// append appends new entries to logs, should only be called internally.
// Caller should use appendCmd or appendLogs instead
func (lm *logManager) append(entries []LogEntry) {
	lm.logs = append(lm.logs, entries...)
	lm.lastIndex = len(lm.logs) - 1
	lm.lastTerm = lm.logs[lm.lastIndex].Term
}
