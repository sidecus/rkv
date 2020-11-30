package raft

import (
	"github.com/sidecus/raft/pkg/util"
)

// LogEntry - one raft log entry, with term and index
type LogEntry struct {
	Index     int
	Term      int
	Committed bool
	Cmd       StateMachineCmd
}

// logManager contains the array of logs
type logManager struct {
	commitIndex int
	lastApplied int
	lastIndex   int
	lastTerm    int

	// logs should be read from persistent storage upon init
	logs []LogEntry
}

// newLogMgr creates a new logmgr
func newLogMgr() *logManager {
	lm := &logManager{
		commitIndex: -1,
		lastIndex:   -1,
		lastTerm:    -1,
		lastApplied: -1,
		logs:        make([]LogEntry, 0),
	}

	return lm
}

// append appends a set of cmds for the given term to the logs
// this should be called by leader when accepting client requests
func (lm *logManager) appendCmds(cmds []StateMachineCmd, term int) {
	entries := make([]LogEntry, len(cmds))
	for i := range entries {
		entries[i] = LogEntry{
			Index:     lm.lastIndex + 1 + i,
			Cmd:       cmds[i],
			Term:      term,
			Committed: false,
		}
	}
	lm.appendEntries(entries)
}

// replicateLogs handles replicated logs from leader
func (lm *logManager) replicateLogs(prevLogIndex, prevLogTerm, leaderCommit int, entries []LogEntry) bool {
	lm.validateReplicatedLogs(prevLogIndex, prevLogTerm, entries)

	if !lm.hasMatchingPrevEntry(prevLogIndex, prevLogTerm) {
		return false
	}

	if len(entries) > 0 {
		index := 0
		for _, v := range entries {
			index = v.Index
			if index > lm.lastIndex || v.Term != lm.logs[index].Term {
				break
			}
		}

		// Drop all entries after the first non-matching and append new ones
		lm.logs = lm.logs[:index]
		lm.appendEntries(entries)
	}

	// Update commit index as appropriate
	if leaderCommit > lm.commitIndex {
		lm.commitIndex = util.Min(leaderCommit, lm.lastIndex)
	}

	return true
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
func (lm *logManager) validateReplicatedLogs(prevLogIndex, prevLogTerm int, entries []LogEntry) {
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

// appendEntries appends entries to logs
func (lm *logManager) appendEntries(entries []LogEntry) {
	lm.logs = append(lm.logs, entries...)
	lm.lastIndex = len(lm.logs) - 1
	lm.lastTerm = lm.logs[lm.lastIndex].Term
}
