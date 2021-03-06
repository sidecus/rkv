package raft

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/sidecus/raft/pkg/util"
)

const rpcTimeOut = time.Duration(200) * time.Millisecond
const rpcSnapshotTimeout = rpcTimeOut * 3

var errNoLongerLeader = errors.New("Node is no longer leader")

// enterLeaderState resets leader indicies. Caller should acquire writer lock
func (n *node) enterLeaderState() {
	n.nodeState = NodeStateLeader
	n.currentLeader = n.nodeID

	// reset all follower's indicies
	n.peerMgr.resetFollowerIndicies(n.logMgr.LastIndex())

	// send heartbeat (which also resets timer)
	n.sendHeartbeat()

	util.WriteInfo("T%d: \U0001f451 Node%d won election\n", n.currentTerm, n.nodeID)
}

// send heartbeat. This is non blocking
func (n *node) sendHeartbeat() {
	n.peerMgr.tryReplicateAll()
	n.refreshTimer()
}

// replicateData replicates data to follower. It replicates snapshot or next batch of logs to the follower.
// If nothing more to replicate, it'll send message with empty payload.
// This is called in the replication goroutine for each follower, and can be triggered via three cases:
// 1. heartbeat
// 2. leader execute propogating entries
// 3. backfilling follower
func (n *node) replicateData(follower *Peer) int {
	doReplicate := n.prepareReplication(follower)
	reply, err := doReplicate()

	if err != nil {
		util.WriteTrace("T%d: Failed to replicate data to Node%d. %s", n.currentTerm, follower.NodeID, err)
		reply = nil
	}

	return n.processReplicationResult(follower, reply)
}

// prepareReplication prepares replication for the given node.
func (n *node) prepareReplication(follower *Peer) func() (*AppendEntriesReply, error) {
	n.mu.RLock()
	defer n.mu.RUnlock()

	if n.nodeState != NodeStateLeader {
		return func() (*AppendEntriesReply, error) { return nil, errNoLongerLeader }
	}

	currentTerm := n.currentTerm

	// Snapshot scenario
	if follower.shouldSendSnapshot(n.logMgr.SnapshotIndex()) {
		req := n.createSnapshotRequest()
		return func() (*AppendEntriesReply, error) {
			ctx, cancel := context.WithTimeout(context.Background(), rpcSnapshotTimeout)
			defer cancel()

			util.WriteTrace("T%d: Sending snapshot to Node%d (T%dL%d)\n", currentTerm, follower.NodeID, req.SnapshotTerm, req.SnapshotIndex)
			return follower.InstallSnapshot(ctx, req)
		}
	}

	// Pure logs
	nextIndex, entryCount := follower.getReplicationParams()
	req := n.createAERequest(nextIndex, entryCount)
	return func() (*AppendEntriesReply, error) {
		ctx, cancel := context.WithTimeout(context.Background(), rpcTimeOut)
		defer cancel()

		util.WriteVerbose("T%d: Sending AE request to Node%d. prevIndex: %d, prevTerm: %d, entryCnt: %d\n", currentTerm, follower.NodeID, req.PrevLogIndex, req.PrevLogTerm, len(req.Entries))
		return follower.AppendEntries(ctx, req)
	}
}

// processReplicationResult handles append entries reply for replications.
// returns lastMatchIndex, or -1 if there is any "error"
func (n *node) processReplicationResult(follower *Peer, reply *AppendEntriesReply) int {
	n.mu.Lock()
	defer n.mu.Unlock()

	if reply == nil {
		return follower.matchIndex
	}

	if follower.NodeID != reply.NodeID {
		util.Panicf("AE reply has different node id %d, expected %d", reply.NodeID, follower.NodeID)
	}

	if n.tryFollowNewTerm(reply.LeaderID, reply.Term, false) {
		return -1
	}

	if n.nodeState != NodeStateLeader {
		return -1
	}

	// 5.3 update follower indicies based on reply and last match index info from the reply
	follower.updateMatchIndex(reply.Success, reply.LastMatch)

	// Then check whether there are logs to commit
	newCommit := reply.Success && n.leaderCommit()

	// request more replication if there is new commit or data remaining
	if newCommit || !follower.upToDate(n.logMgr.LastIndex()) {
		// Use non blocking TryRequestReplicate to avoid potential deadlock when queue is full
		follower.tryRequestReplicate(nil)
	}

	return follower.matchIndex
}

// leaderCommit commits to the last entry with quorum
// This should only be called by leader upon AE reply handling
// Returns true if anything is committed
func (n *node) leaderCommit() bool {
	commitIndex := n.logMgr.CommitIndex()
	for i := n.logMgr.LastIndex(); i > n.logMgr.CommitIndex(); i-- {
		entry := n.logMgr.GetLogEntry(i)

		if entry.Term < n.currentTerm {
			// 5.4.2 Raft doesn't allow committing of previous terms
			// A leader shall only commit entries added by itself, and term is the indication of ownership
			break
		} else if entry.Term > n.currentTerm {
			util.Panicln("entry term is larger than leader's current term")
			continue
		}

		// If we reach here, we can safely declare sole ownership of the ith entry
		if n.peerMgr.quorumReached(i) {
			commitIndex = i
			break
		}
	}

	if commitIndex > n.logMgr.CommitIndex() {
		util.WriteTrace("T%d: Leader%d committing to L%d upon quorum", n.currentTerm, n.nodeID, commitIndex)
		n.commitTo(commitIndex)
		return true
	}

	return false
}

// Execute a cmd and propogate it to followers.
// This will trigger replicateData for all followers and wait for them to finish
func (n *node) leaderExecute(ctx context.Context, cmd *StateMachineCmd) (*ExecuteReply, error) {
	n.mu.Lock()
	targetIndex := n.logMgr.ProcessCmd(*cmd, n.currentTerm)
	n.mu.Unlock()

	// Try to replicate new entry to all followers
	n.peerMgr.waitAll(func(p *Peer, wg *sync.WaitGroup) {
		p.requestReplicateTo(targetIndex, wg)
	})

	success := n.logMgr.CommitIndex() >= targetIndex
	return &ExecuteReply{NodeID: n.nodeID, Success: success}, nil
}

// createAERequest creates an AppendEntriesRequest with proper log payload
func (n *node) createAERequest(startIdx int, maxCnt int) *AppendEntriesRequest {
	// make sure startIdx is larger than snapshotIndex, and endIdx is smaller or equal to lastIndex
	startIdx = util.Max(startIdx, n.logMgr.SnapshotIndex()+1)
	endIdx := util.Min(n.logMgr.LastIndex()+1, startIdx+maxCnt)

	// Get log entries and make a copy of them
	logs, prevIdx, prevTerm := n.logMgr.GetLogEntries(startIdx, endIdx)
	entries := make([]LogEntry, len(logs))
	copy(entries, logs)

	req := &AppendEntriesRequest{
		Term:         n.currentTerm,
		LeaderID:     n.nodeID,
		PrevLogIndex: prevIdx,
		PrevLogTerm:  prevTerm,
		Entries:      entries,
		LeaderCommit: n.logMgr.CommitIndex(),
	}

	return req
}

// createSnapshotRequest creates a snapshot request to send to follower
func (n *node) createSnapshotRequest() *SnapshotRequest {
	return &SnapshotRequest{
		SnapshotRequestHeader: SnapshotRequestHeader{
			Term:          n.currentTerm,
			LeaderID:      n.nodeID,
			SnapshotIndex: n.logMgr.SnapshotIndex(),
			SnapshotTerm:  n.logMgr.SnapshotTerm(),
		},
		File: n.logMgr.SnapshotFile(),
	}
}
