package raft

import (
	"github.com/sidecus/raft/pkg/util"
)

const snapshotEntriesCount = 3000

// LogEntry - one raft log entry, with term and index
type LogEntry struct {
	Index int
	Term  int
	Cmd   StateMachineCmd
}

// ILogManager defines the interface for log manager
type ILogManager interface {
	LastIndex() int
	LastTerm() int
	CommitIndex() int
	SnapshotIndex() int
	SnapshotTerm() int
	SnapshotFile() string
	GetLogEntry(index int) LogEntry
	GetLogEntries(start int, end int) (entries []LogEntry, prevIndex int, prevTerm int)

	ProcessCmd(cmd StateMachineCmd, term int) int
	ProcessLogs(prevLogIndex, prevLogTerm int, entries []LogEntry) (prevMatch bool)
	CommitAndApply(targetIndex int) (newCommit bool, newSnapshot bool)
	InstallSnapshot(snapshotFile string, snapshotIndex int, snapshotTerm int) error

	// proxy to state machine
	IValueGetter
}

// logManager manages logs and the statemachine, implements ILogManager
type logManager struct {
	nodeID      int
	lastIndex   int
	lastTerm    int
	commitIndex int

	// snap shot related
	snapshotIndex    int
	snapshotTerm     int
	lastSnapshotFile string

	// logs should be read from persistent storage upon init
	// it contains entries from snapshotIndex+1 to lastIndex
	logs []LogEntry

	// reference to statemachien for commit operations
	lastApplied  int
	statemachine IStateMachine
}

// newLogMgr creates a new logmgr
func newLogMgr(nodeID int, sm IStateMachine) ILogManager {
	if sm == nil {
		util.Panicf("state machien cannot be nil")
	}

	lm := &logManager{
		nodeID:        nodeID,
		lastIndex:     -1,
		lastTerm:      -1,
		commitIndex:   -1,
		snapshotIndex: -1,
		snapshotTerm:  -1,
		lastApplied:   -1,
		logs:          make([]LogEntry, 0, 100),
		statemachine:  sm,
	}

	return lm
}

// LastIndex returns the last index for the log
func (lm *logManager) LastIndex() int {
	return lm.lastIndex
}

// LastTerm returns the last term for the log
func (lm *logManager) LastTerm() int {
	return lm.lastTerm
}

// CommitIndex returns the commit index for the log
func (lm *logManager) CommitIndex() int {
	return lm.commitIndex
}

// SnapshotIndex returns the recent snapshot's last included index (-1 otherwise)
func (lm *logManager) SnapshotIndex() int {
	return lm.snapshotIndex
}

// SnapshotTerm returns the recent snapshot's last included term (-1 otherwise)
func (lm *logManager) SnapshotTerm() int {
	return lm.snapshotTerm
}

// SnapshotFile returns the recent snapshot file (string zero value otherwise)
func (lm *logManager) SnapshotFile() string {
	return lm.lastSnapshotFile
}

// GetLogEntry returns log entry for the given index
func (lm *logManager) GetLogEntry(index int) LogEntry {
	if index <= lm.snapshotIndex || index > lm.LastIndex() {
		util.Panicf("GetLogEntry index %d shall be between (snapshotIndex(%d), lastIndex(%d)]\n", index, lm.snapshotIndex, lm.LastIndex())
	}
	return lm.logs[index-(lm.snapshotIndex+1)]
}

// GetLogEntries returns entries between [start, end).
// Same behavior as normal slicing but with index shiftted as appropriate with snapshot/empty scenarios in mind
// It also returns the prevIndex/prevTerm
func (lm *logManager) GetLogEntries(start int, end int) (entries []LogEntry, prevIndex int, prevTerm int) {
	if start <= lm.snapshotIndex {
		util.Panicf("GetLogEntries start %d shall be between (snapshotIndex(%d), lastIndex(%d)]\n", start, lm.snapshotIndex, lm.LastIndex())
	}

	if end < start {
		util.Panicln("end should be greater than or equal to start")
	}

	if end > lm.lastIndex+1 {
		util.Panicln("end should not be greater than lastIndex+1")
	}

	prevIndex = start - 1
	prevTerm = lm.getLogEntryTerm(prevIndex)
	actualStart := start - (lm.snapshotIndex + 1)
	actualEnd := util.Min(end-(lm.snapshotIndex+1), len(lm.logs))
	entries = lm.logs[actualStart:actualEnd]

	return
}

// Get gets values from the underneath statemachine
func (lm *logManager) Get(param ...interface{}) (interface{}, error) {
	return lm.statemachine.Get(param...)
}

// ProcessCmd adds a cmd for the given term to the logs
// this should be called by leader when accepting client requests
func (lm *logManager) ProcessCmd(cmd StateMachineCmd, term int) int {
	entry := LogEntry{
		Index: lm.lastIndex + 1,
		Cmd:   cmd,
		Term:  term,
	}
	lm.appendLogs(entry)
	return lm.lastIndex
}

// ProcessLogs handles replicated logs from leader
// Returns true if we entries matching prevLogIndex/prevLogTerm, and if that's the case, log
// entries are processed and appended as appropriate. Note this happens even for heartbeats.
// Otherwise return false
func (lm *logManager) ProcessLogs(prevLogIndex, prevLogTerm int, entries []LogEntry) bool {
	lm.validateLogEntries(prevLogIndex, prevLogTerm, entries)

	prevMatch := lm.hasMatchingPrevEntry(prevLogIndex, prevLogTerm)
	util.WriteVerbose("Match on prevIndex(%d) prevTerm(%d): %v", prevLogIndex, prevLogTerm, prevMatch)
	if !prevMatch {
		return false
	}

	// Find first non matching entry's index, and drop local logs starting from that position,
	// then append from incoming entries starting from that position
	conflictIndex := lm.findFirstConflictIndex(prevLogIndex, entries)
	toAppend := entries[conflictIndex-(prevLogIndex+1):]
	lm.logs, _, _ = lm.GetLogEntries(lm.snapshotIndex+1, conflictIndex)

	// appendLogs adjusts lastIndex accordingly
	lm.appendLogs(toAppend...)

	return true
}

// CommitAndApply commits logs up to the target index and applies it to state machine
// returns true if anything is committed
func (lm *logManager) CommitAndApply(targetIndex int) (newCommit bool, newSnapshot bool) {
	if targetIndex > lm.lastIndex {
		util.Panicln("Cannot commit to a value larger than last index")
	}
	if targetIndex <= lm.commitIndex {
		return // nothing to commit
	}

	newCommit = true

	// Set new commit index and apply commands to state machine if needed
	lm.commitIndex = targetIndex
	if lm.commitIndex > lm.lastApplied {
		for i := lm.lastApplied + 1; i <= lm.commitIndex; i++ {
			lm.statemachine.Apply(lm.GetLogEntry(i).Cmd)
		}
		lm.lastApplied = lm.commitIndex
	}

	// take snapshot if needed
	if lm.lastApplied-lm.snapshotIndex >= snapshotEntriesCount {
		if err := lm.TakeSnapshot(); err != nil {
			util.WriteError("Failed to take snapshot: %s", err)
		} else {
			newSnapshot = true
		}
	}

	return
}

// TakeSnapshot takes a snap shot and saves it to a file
func (lm *logManager) TakeSnapshot() error {
	if lm.lastApplied == lm.snapshotIndex {
		return nil // nothing to do
	}

	index := lm.lastApplied
	term := lm.getLogEntryTerm(index)

	// serialize and create snapshot
	file, w, err := CreateSnapshot(lm.nodeID, term, index, "local")
	if err != nil {
		return err
	}

	defer w.Close()
	lm.statemachine.Serialize(w)

	// Truncate logs and update snapshotIndex and term
	lm.logs, _, _ = lm.GetLogEntries(index+1, lm.lastIndex+1)
	lm.snapshotIndex = index
	lm.snapshotTerm = term
	lm.lastSnapshotFile = file

	return nil
}

// InstallSnapshot installs a snapshot
// For simplicity, we drop all local logs after installing the snapshot
func (lm *logManager) InstallSnapshot(snapshotFile string, snapshotIndex int, snapshotTerm int) error {
	// Read snapshot and deserialize
	r, err := ReadSnapshot(snapshotFile)
	if err != nil {
		return err
	}

	defer r.Close()
	lm.statemachine.Deserialize(r)

	lm.snapshotIndex = snapshotIndex
	lm.snapshotTerm = snapshotTerm
	lm.lastSnapshotFile = snapshotFile
	lm.lastApplied = snapshotIndex
	lm.commitIndex = snapshotIndex
	lm.lastIndex = snapshotIndex
	lm.lastTerm = snapshotTerm
	lm.logs = lm.logs[0:0]

	return nil
}

// findFirstConflictIndex finds the first conflicting entry by comparing incoming entries with local log entries
// caller needs to ensure there is an entry matching prevLogIndex and prevLogTerm before calling this
// if there is such a conflicting entry, its index is returned
// if incoming entries is empty, prevLogIndex+1 will be returned
// if there is no overlap, current lastIndex+1 will be returned
// if everything matches, last matching entry's index + 1 is returned min(lastLogIndex+1, last entries index+1)
func (lm *logManager) findFirstConflictIndex(prevLogIndex int, entries []LogEntry) int {
	if prevLogIndex < lm.snapshotIndex {
		util.Panicln("prevLogIndex cannot be less than snapshotIndex")
	}
	start := prevLogIndex + 1
	end := start + len(entries)

	// Find first non-matching index
	index := start
	for index = start; index < end; index++ {
		if index > lm.lastIndex || entries[index-start].Term != lm.GetLogEntry(index).Term {
			break
		}
	}

	return index
}

// check to see whether we have a matching entry @prevLogIndex with prevLogTerm
func (lm *logManager) hasMatchingPrevEntry(prevLogIndex, prevLogTerm int) bool {
	if prevLogIndex < lm.snapshotIndex || prevLogIndex > lm.lastIndex {
		return false
	}
	term := lm.getLogEntryTerm(prevLogIndex)
	return term == prevLogTerm
}

// validates incoming logs, panicing on bad data
func (lm *logManager) validateLogEntries(prevLogIndex, prevLogTerm int, entries []LogEntry) {
	if prevLogIndex < 0 && prevLogIndex != -1 {
		util.Panicf("invalid prevLogIndex %d, less than 0 but not -1\n", prevLogIndex)
	}

	if prevLogTerm < 0 && prevLogTerm != -1 {
		util.Panicf("invalid prevLogTerm %d, less than 0 but not -1\n", prevLogTerm)
	}

	if prevLogIndex+prevLogTerm < 0 && prevLogIndex*prevLogTerm < 0 {
		util.Panicf("prevLogIndex %d or prevLogTerm %d is -1 but the other is not\n", prevLogIndex, prevLogTerm)
	}

	term := prevLogTerm
	for i, v := range entries {
		if v.Index != prevLogIndex+1+i {
			util.Panicf("new entry's index (%dth) is not continuous after match prevLogIndex %d\n", v.Index, prevLogIndex)
		}
		if v.Term < term {
			util.Panicf("new entry (index %d) has invalid term\n", v.Index)
		}
		term = v.Term
	}
}

// appendLogs appends new entries to logs, should only be called internally.
// Externall caller should use ProcessCmd or ProcessLogs instead
func (lm *logManager) appendLogs(entries ...LogEntry) {
	lm.logs = append(lm.logs, entries...)
	if len(lm.logs) == 0 {
		lm.lastIndex = lm.snapshotIndex
		lm.lastTerm = lm.snapshotTerm
	} else {
		lastEntry := lm.logs[len(lm.logs)-1]
		lm.lastIndex = lastEntry.Index
		lm.lastTerm = lastEntry.Term
	}
}

// getLogEntryTerm gets the term for a given log index
// Unlike GetLogEntry, this can return proper value when index == snapshotIndex (snapshot scenario) or index == -1 (prevIndex scenario)
func (lm *logManager) getLogEntryTerm(index int) int {
	if index == -1 {
		return -1
	} else if index < lm.snapshotIndex || index > lm.lastIndex {
		util.Panicf("getTerm index cannot be less than snapshotIndex or larger than lastIndex")
	} else if index == lm.snapshotIndex {
		return lm.snapshotTerm
	}

	return lm.GetLogEntry(index).Term
}
