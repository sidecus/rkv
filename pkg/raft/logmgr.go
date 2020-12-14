package raft

import (
	"github.com/sidecus/raft/pkg/util"
)

const snapshotEntriesCount = 1000

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
	GetLogEntries(start int, count int) (entries []LogEntry, prevIndex int, prevTerm int)

	ProcessCmd(cmd StateMachineCmd, term int)
	ProcessLogs(prevLogIndex, prevLogTerm int, entries []LogEntry) (prevMatch bool)
	Commit(targetIndex int) (newCommit bool, newSnapshot bool)
	InstallSnapshot(snapshotFile string, snapshotIndex int, snapshotTerm int) error

	// proxy to state machine
	IValueGetter
}

// LogManager manages logs and the statemachine, implements ILogManager
type LogManager struct {
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

// NewLogMgr creates a new logmgr
func NewLogMgr(nodeID int, sm IStateMachine) ILogManager {
	if sm == nil {
		util.Panicf("state machien cannot be nil")
	}

	lm := &LogManager{
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

// SnapshotIndex returns the recent snapshot's last included index (-1 otherwise)
func (lm *LogManager) SnapshotIndex() int {
	return lm.snapshotIndex
}

// SnapshotTerm returns the recent snapshot's last included term (-1 otherwise)
func (lm *LogManager) SnapshotTerm() int {
	return lm.snapshotTerm
}

// SnapshotFile returns the recent snapshot file (string zero value otherwise)
func (lm *LogManager) SnapshotFile() string {
	return lm.lastSnapshotFile
}

// GetLogEntry returns log entry for the given index
func (lm *LogManager) GetLogEntry(index int) LogEntry {
	if index <= lm.snapshotIndex || index > lm.LastIndex() {
		util.Panicf("GetLogEntry index %d shall be between (snapshotIndex(%d), lastIndex(%d)]\n", index, lm.snapshotIndex, lm.LastIndex())
	}
	return lm.logs[index-(lm.snapshotIndex+1)]
}

// GetLogEntries returns entries between [start, end).
// Same behavior as normal slicing but with index shiftted according to snapshotIndex.
// It also returns the prevIndex/prevTerm
func (lm *LogManager) GetLogEntries(start int, end int) (entries []LogEntry, prevIndex int, prevTerm int) {
	if start <= lm.snapshotIndex {
		util.Panicf("GetLogEntries start %d shall be between (snapshotIndex(%d), lastIndex(%d)]\n", start, lm.snapshotIndex, lm.LastIndex())
	}

	if end < start {
		util.Panicf("end should be greater than or equal to start")
	}

	prevIndex = start - 1
	prevTerm = lm.getLogEntryTerm(prevIndex)
	actualStart := start - (lm.snapshotIndex + 1)
	actualEnd := util.Min(end-(lm.snapshotIndex+1), len(lm.logs))
	entries = lm.logs[actualStart:actualEnd]

	return
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
// Returns true if we entries matching prevLogIndex/prevLogTerm, and if that's the case, log
// entries are processed and appended as appropriate. Note this happens even for heartbeats.
// Otherwise return false
func (lm *LogManager) ProcessLogs(prevLogIndex, prevLogTerm int, entries []LogEntry) bool {
	lm.validateLogEntries(prevLogIndex, prevLogTerm, entries)

	prevMatch := lm.hasMatchingPrevEntry(prevLogIndex, prevLogTerm)
	util.WriteTrace("Match on prevIndex(%d) prevTerm(%d): %v", prevLogIndex, prevLogTerm, prevMatch)
	if !prevMatch {
		return false
	}

	// Find first non matching entry's index, and drop local logs starting from that position,
	// then append from incoming entries starting from that position
	conflictIndex := lm.findFirstConflictIndex(prevLogIndex, entries)
	toAppend := entries[conflictIndex-(prevLogIndex+1):]
	lm.logs, _, _ = lm.GetLogEntries(lm.snapshotIndex+1, conflictIndex)

	// appendLogs adjusts lastIndex accordingly
	lm.appendLogs(toAppend)

	return true
}

// Commit tries to logs up to the target index
// returns true if anything is committed
func (lm *LogManager) Commit(targetIndex int) (newCommit bool, newSnapshot bool) {
	// cap to lastIndex
	targetIndex = util.Min(targetIndex, lm.lastIndex)
	if targetIndex <= lm.commitIndex {
		// nothing to commit
		return
	}

	newCommit = true

	// Set new commit index and apply commands to state machine if needed
	lm.commitIndex = targetIndex
	if targetIndex > lm.lastApplied {
		for i := lm.lastApplied + 1; i <= targetIndex; i++ {
			lm.statemachine.Apply(lm.GetLogEntry(i).Cmd)
		}
		lm.lastApplied = targetIndex
	}

	// take snapshot if needed
	if lm.lastApplied-lm.snapshotIndex >= snapshotEntriesCount {
		if err := lm.TakeSnapshot(); err != nil {
			util.WriteError("Snapshot failure: %s", err)
		} else {
			newSnapshot = true
		}
	}

	return
}

// TakeSnapshot takes a snap shot and saves it to a file
func (lm *LogManager) TakeSnapshot() error {
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
func (lm *LogManager) InstallSnapshot(snapshotFile string, snapshotIndex int, snapshotTerm int) error {
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
func (lm *LogManager) findFirstConflictIndex(prevLogIndex int, entries []LogEntry) int {
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
func (lm *LogManager) hasMatchingPrevEntry(prevLogIndex, prevLogTerm int) bool {
	if prevLogIndex < lm.snapshotIndex || prevLogIndex > lm.lastIndex {
		return false
	}
	term := lm.getLogEntryTerm(prevLogIndex)
	return term == prevLogTerm
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

	term := lm.lastTerm
	for i, v := range entries {
		if v.Index != prevLogIndex+1+i {
			util.Panicf("new entry's index (%dth) is not continuous after match prevLogIndex %d\n", v.Index, prevLogIndex)
		}
		if v.Term < term {
			util.Panicf("new entry (index %d) has invalid term d\n", v.Index)
		}
		term = v.Term
	}
}

// appendLogs appends new entries to logs, should only be called internally.
// Externall caller should use ProcessCmd or ProcessLogs instead
func (lm *LogManager) appendLogs(entries []LogEntry) {
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
func (lm *LogManager) getLogEntryTerm(index int) int {
	if index == -1 {
		return -1
	} else if index < lm.snapshotIndex || index > lm.lastIndex {
		util.Panicf("getTerm index cannot be less than snapshotIndex or larger than lastIndex")
	} else if index == lm.snapshotIndex {
		return lm.snapshotTerm
	}

	return lm.GetLogEntry(index).Term
}
