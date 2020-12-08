package raft

import (
	"fmt"
	"io/ioutil"
	"os"

	"github.com/sidecus/raft/pkg/util"
)

const snapshotEntriesCount = 5

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
	GetLogEntry(index int) LogEntry
	GetLogEntries(start int, count int) (entries []LogEntry, prevIndex int, prevTerm int)

	ProcessCmd(cmd StateMachineCmd, term int)
	ProcessLogs(prevLogIndex, prevLogTerm int, entries []LogEntry) (prevMatch bool)
	Commit(targetIndex int) bool

	// proxy to state machine
	IValueGetter
}

// LogManager manages logs and the statemachine, implements ILogManager
type LogManager struct {
	nodeID      int
	lastIndex   int
	lastTerm    int
	commitIndex int

	// Last snapshot index and term
	snapshotIndex int
	snapshotTerm  int
	snapshotFile  string
	snapshotPath  string

	// logs should be read from persistent storage upon init
	logs []LogEntry

	// reference to statemachien for commit operations
	lastApplied  int
	statemachine IStateMachine
}

// NewLogMgr creates a new logmgr
func NewLogMgr(nodeID int, sm IStateMachine, snapshotPath string) ILogManager {
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
		snapshotPath:  snapshotPath,
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

// GetLogEntry returns log entry for the given index
func (lm *LogManager) GetLogEntry(index int) LogEntry {
	return lm.logs[index-(lm.snapshotIndex+1)]
}

// GetLogEntries returns entries starting from startIndex (log index, not slice index)
// Number of elements returned will be count if there is enough, or whatever is available after startIndex
// If logs is empty and startIndex is 0, empty is returned.
// If count is 0, empty is returned.
// It also returns the prevIndex/prevTerm
func (lm *LogManager) GetLogEntries(startIndex int, count int) (entries []LogEntry, prevIndex int, prevTerm int) {
	if startIndex < 0 || startIndex > lm.LastIndex()+1 {
		util.Panicf("start %d shall never be less than zero or larger than lastIndex(%d) + 1\n", startIndex, lm.LastIndex()+1)
	}

	if count < 0 {
		util.Panicf("count should be greater than or equal to 0")
	}

	prevIndex = startIndex - 1
	prevTerm = -1
	if prevIndex >= 0 {
		prevTerm = lm.GetLogEntry(prevIndex).Term
	}

	endIndex := util.Min(startIndex+count, lm.LastIndex()+1)
	entries = lm.logs[startIndex:endIndex]

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
// returns true if we entries matching prevLogIndex/prevLogTerm, and if that's the case, log
// entries are processed and appended as appropriate. Otherwise return false
func (lm *LogManager) ProcessLogs(prevLogIndex, prevLogTerm int, entries []LogEntry) bool {
	lm.validateLogEntries(prevLogIndex, prevLogTerm, entries)

	prevMatch := lm.hasMatchingPrevEntry(prevLogIndex, prevLogTerm)
	if !prevMatch {
		return false
	}

	// Find first non matching entry's index, and drop local logs starting from that position,
	// then append from incoming entries starting from that position
	conflictIndex := lm.findFirstConflictIndex(prevLogIndex, entries)
	toAppend := entries[conflictIndex-(prevLogIndex+1):]

	// Truncate logs and then call appendLogs, which will adjust lastIndex accordingly
	lm.logs, _, _ = lm.GetLogEntries(0, conflictIndex)
	lm.appendLogs(toAppend)

	return true
}

// Commit tries to logs up to the target index
// returns true if anything is committed
func (lm *LogManager) Commit(targetIndex int) bool {
	// cap to lastIndex
	targetIndex = util.Min(targetIndex, lm.lastIndex)
	if targetIndex <= lm.commitIndex {
		return false // nothing more to commit
	}

	// Set new commit index and apply commands to state machine if needed
	lm.commitIndex = targetIndex
	if targetIndex > lm.lastApplied {
		for i := lm.lastApplied + 1; i <= targetIndex; i++ {
			lm.statemachine.Apply(lm.GetLogEntry(i).Cmd)
		}
		lm.lastApplied = targetIndex
	}

	// take snapshot if needed
	if lm.lastApplied-lm.snapshotIndex > snapshotEntriesCount {
		// lm.takeSnapshot()
	}

	return true
}

// TakeSnapshot takes a snap shot and saves it to a file
func (lm *LogManager) TakeSnapshot() error {
	index := lm.lastApplied
	term := lm.GetLogEntry(index).Term

	f, err := ioutil.TempFile(os.TempDir(), fmt.Sprintf("snapshot_%d_%d", index, term))
	defer f.Close()

	err = lm.statemachine.Serialize(f)
	if err != nil {
		return err
	}

	// Truncate logs and update snapshotIndex and term
	lm.logs, _, _ = lm.GetLogEntries(index+1, lm.lastIndex-index)
	lm.snapshotIndex = index
	lm.snapshotTerm = term

	return nil
}

// InstallSnapshot installs a snapshot
func (lm *LogManager) InstallSnapshot(fileName string, snapshotIndex int, snapshotTerm int) error {
	return nil
}

// check to see whether we have a matching entry @prevLogIndex with prevLogTerm
func (lm *LogManager) hasMatchingPrevEntry(prevLogIndex, prevLogTerm int) bool {
	if prevLogIndex == -1 && prevLogTerm == -1 {
		return true // empty logs after init
	}

	if prevLogIndex > lm.lastIndex {
		return false
	}

	return lm.GetLogEntry(prevLogIndex).Term == prevLogTerm
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
// Caller should use appendCmd or appendLogs instead
func (lm *LogManager) appendLogs(entries []LogEntry) {
	lm.logs = append(lm.logs, entries...)
	if len(lm.logs) == 0 {
		lm.lastIndex = -1
		lm.lastTerm = -1
	} else {
		lastEntry := lm.logs[len(lm.logs)-1]
		lm.lastIndex = lastEntry.Index
		lm.lastTerm = lastEntry.Term
	}
}

// findFirstConflictIndex finds the first conflicting entry by comparing incoming entries with local log entries
// If incoming entries is empty, prevLogIndex+1 will be returned
// if there is such a conflicting entry, its index is returned
// if everything matches, min(lastLogIndex+1, last entries index+1) is returned
// The last scenario likely will never happen because in that case prevLogIndex should be bigger
func (lm *LogManager) findFirstConflictIndex(prevLogIndex int, entries []LogEntry) int {
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
