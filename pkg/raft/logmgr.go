package raft

import "github.com/sidecus/raft/pkg/util"

const maxAppendEntriesCount = 5

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
	IValueGetter
}

// LogEntry - one raft log entry, with term and index
type LogEntry struct {
	Index int
	Term  int
	Cmd   StateMachineCmd
}

// IAERequestCreator defines an interface to create AppendEntries request
type IAERequestCreator interface {
	CreateAERequest(term, leaderID, nextIdx int) *AppendEntriesRequest
}

// ILogManager defines the interface for log manager
type ILogManager interface {
	LastIndex() int
	LastTerm() int
	CommitIndex() int
	GetLogEntry(index int) LogEntry

	ProcessCmd(cmd StateMachineCmd, term int)
	ProcessLogs(prevLogIndex, prevLogTerm int, entries []LogEntry) (prevMatch bool)
	Commit(targetIndex int) bool

	// AppendEntries request creator
	IAERequestCreator

	// proxy to state machine
	IValueGetter
}

// LogManager manages logs and the statemachine, implements ILogManager
type LogManager struct {
	commitIndex int
	lastApplied int
	lastIndex   int
	lastTerm    int

	// logs should be read from persistent storage upon init
	logs []LogEntry

	// reference to statemachien for commit operations
	statemachine IStateMachine
}

// NewLogMgr creates a new logmgr
func NewLogMgr(sm IStateMachine) ILogManager {
	lm := &LogManager{
		commitIndex:  -1,
		lastIndex:    -1,
		lastTerm:     -1,
		lastApplied:  -1,
		logs:         make([]LogEntry, 0),
		statemachine: sm,
	}

	return lm
}

// LastIndex returns the last index for the log
func (lm *LogManager) LastIndex() int {
	return lm.lastIndex
}

// LastTerm returns the last term for the log
func (lm *LogManager) LastTerm() int {
	return lm.lastTerm
}

// CommitIndex returns the commit index for the log
func (lm *LogManager) CommitIndex() int {
	return lm.commitIndex
}

// GetLogEntry returns log entry for the given index
func (lm *LogManager) GetLogEntry(index int) LogEntry {
	return lm.logs[index]
}

// Get gets values from the underneath statemachine
func (lm *LogManager) Get(param ...interface{}) (interface{}, error) {
	return lm.statemachine.Get(param...)
}

// ProcessCmd adds a cmd for the given term to the logs
// this should be called by leader when accepting client requests
func (lm *LogManager) ProcessCmd(cmd StateMachineCmd, term int) {
	entry := LogEntry{
		Index: lm.lastIndex + 1,
		Cmd:   cmd,
		Term:  term,
	}
	entries := []LogEntry{entry}
	lm.appendLogs(entries)
}

// ProcessLogs handles replicated logs from leader
// returns true if we entries matching prevLogIndex/prevLogTerm, and if that's the case, log
// entries are processed and appended as appropriate
func (lm *LogManager) ProcessLogs(prevLogIndex, prevLogTerm int, entries []LogEntry) (prevMatch bool) {
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
	lm.appendLogs(entries[(index - start):])

	return
}

// Commit tries to logs up to the target index
func (lm *LogManager) Commit(targetIndex int) bool {
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

// CreateAERequest creates an AppendEntriesRequest with proper log payload
func (lm *LogManager) CreateAERequest(term, leaderID, nextIdx int) *AppendEntriesRequest {
	if nextIdx < 0 || nextIdx > lm.lastIndex+1 {
		util.Panicf("nextIdx %d shall never be less than zero or larger than lastLogIndex(%d) + 1\n", nextIdx, lm.lastIndex+1)
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
func (lm *LogManager) hasMatchingPrevEntry(prevLogIndex, prevLogTerm int) bool {
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
func (lm *LogManager) validateLogEntries(prevLogIndex, prevLogTerm int, entries []LogEntry) {
	if prevLogIndex < 0 && prevLogIndex != -1 {
		util.Panicf("invalid prevLogIndex %d, less than 0 but not -1\n", prevLogIndex)
	}

	if prevLogTerm < 0 && prevLogTerm != -1 {
		util.Panicf("invalid prevLogTerm %d, less than 0 but not -1\n", prevLogTerm)
	}

	if prevLogIndex+prevLogTerm < 0 && prevLogIndex*prevLogTerm < 0 {
		util.Panicf("prevLogIndex %d or prevLogTerm %d is -1 but the other is not\n", prevLogIndex, prevLogTerm)
	}

	for i, v := range entries {
		if v.Index != prevLogIndex+1+i {
			util.Panicf("new entries index %d (%dth entry) doesn't match prevLogIndex %d\n", v.Index, i, prevLogIndex)
		}
	}
}

// appendLogs appends new entries to logs, should only be called internally.
// Caller should use appendCmd or appendLogs instead
func (lm *LogManager) appendLogs(entries []LogEntry) {
	lm.logs = append(lm.logs, entries...)
	lm.lastIndex = len(lm.logs) - 1
	lm.lastTerm = lm.logs[lm.lastIndex].Term
}
