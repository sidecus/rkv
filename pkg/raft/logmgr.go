package raft

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
	}

	return lm
}

// append appends a set of cmds for the given term to the logs, and returns
// this should be called by leader
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
	lm.logs = append(lm.logs, entries...)
	lm.lastIndex = len(lm.logs) - 1
	lm.lastTerm = entries[lm.lastIndex].Term
}
