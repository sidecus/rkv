package raft

import (
	"github.com/sidecus/raft/pkg/util"
)

const maxAppendEntriesCount = 20

// enterLeaderState resets leader indicies. Caller should acquire writer lock
func (n *node) enterLeaderState() {
	n.nodeState = Leader
	n.currentLeader = n.nodeID

	// reset all follower's indicies
	n.peerMgr.ResetFollowerIndicies(n.logMgr.LastIndex())

	util.WriteInfo("T%d: \U0001f451 Node%d won election\n", n.currentTerm, n.nodeID)
}

// send heartbeat, caller should acquire at least reader lock
func (n *node) sendHeartbeat() {
	// create empty AE request
	// TODO[sidecus]: shall we use lastindex+1 all the time for heartbeat?
	// This can cause unnecessary nextIndex decrement.
	// We might want to send different heratbeat based on nextIndex of different node
	req := n.createAERequest(n.logMgr.LastIndex()+1, 0)
	util.WriteTrace("T%d: \U0001f493 Node%d sending heartbeat\n", n.currentTerm, n.nodeID)

	// send heart beat (on different goroutines), response will be processed there
	n.peerMgr.BroadcastAppendEntries(req, n.handleAppendEntriesReply)

	// 5.2 - refresh timer
	n.refreshTimer()
}

// replicateLogsTo replicate logs to follower as needed
// This should be only be called by leader
func (n *node) replicateLogsTo(targetNodeID int) bool {
	follower := n.peerMgr.GetPeer(targetNodeID)

	if follower.nextIndex > n.logMgr.LastIndex() {
		// nothing to replicate
		return false
	}

	// TODO[sidecus]: how to reduce leader's burden on duplicate replications?
	// // precaution to avoid parallel replication to the same node
	// counter := atomic.AddInt32(&follower.replicationCounter, 1)
	// atomic.AddInt32(&follower.replicationCounter, -1)
	// if counter > 1 {
	// 	return false
	// }

	if follower.nextIndex <= n.logMgr.SnapshotIndex() {
		// Send snapshot
		req := n.createSnapshotRequest()
		util.WriteInfo("T%d: Node%d sending snapshot to Node%d (L%d)\n", n.currentTerm, n.nodeID, targetNodeID, n.logMgr.SnapshotIndex())

		n.peerMgr.InstallSnapshot(follower.NodeID, req, n.handleAppendEntriesReply)
	} else {
		// there are logs to replicate, create AE request and send
		req := n.createAERequest(follower.nextIndex, maxAppendEntriesCount)
		minIdx := req.Entries[0].Index
		maxIdx := req.Entries[len(req.Entries)-1].Index
		util.WriteInfo("T%d: Node%d replicating logs to Node%d (L%d-L%d)\n", n.currentTerm, n.nodeID, targetNodeID, minIdx, maxIdx)

		n.peerMgr.AppendEntries(follower.NodeID, req, n.handleAppendEntriesReply)
	}

	return true
}

// handleAppendEntriesReply handles append entries reply. Need locking since this will be
// running on different goroutine for reply from each node
func (n *node) handleAppendEntriesReply(reply *AppendEntriesReply) {
	n.mu.Lock()
	defer n.mu.Unlock()

	// If there is a higher term, follow and stop processing
	if n.tryFollowNewTerm(reply.LeaderID, reply.Term, false) {
		return
	}

	// Different from the paper, we don't wait for AE call to finish
	// so there is a chance that we are no longer the leader
	// In that case no need to continue
	if n.nodeState != Leader || n.currentTerm != reply.Term {
		return
	}

	followerID := reply.NodeID

	// 5.3 update leader indicies.
	// Kindly note: since we proces this asynchronously, we cannot use n.logMgr.lastIndex
	// to update follower indicies (it might be different from when the AE request is sent and when the reply is received).
	// Here we added a LastMatch field on AppendEntries reply. And it's used instead.
	n.peerMgr.UpdateFollowerMatchIndex(followerID, reply.Success, reply.LastMatch)

	// Check whether there are logs to commit and then replicate
	n.leaderCommit()
	n.replicateLogsTo(followerID)
}

// leaderCommit commits logs as needed by checking each follower's match index
// This should only be called by leader
func (n *node) leaderCommit() {
	commitIndex := n.logMgr.CommitIndex()
	for i := n.logMgr.LastIndex(); i > n.logMgr.CommitIndex(); i-- {
		entry := n.logMgr.GetLogEntry(i)

		if entry.Term < n.currentTerm {
			// 5.4.2 Raft doesn't allow committing of previous terms
			// A leader shall only commit entries added by itself, and term is the indication of ownership
			break
		} else if entry.Term > n.currentTerm {
			// This will never happen, adding for safety purpose
			continue
		}

		// If we reach here, we can safely declare sole ownership of the ith entry
		if n.peerMgr.MajorityMatch(i) {
			commitIndex = i
			break
		}
	}

	n.commitTo(commitIndex)
}

// createAERequest creates an AppendEntriesRequest with proper log payload
func (n *node) createAERequest(nextIdx int, count int) *AppendEntriesRequest {
	entris, prevIdx, prevTerm := n.logMgr.GetLogEntries(nextIdx, nextIdx+count)

	req := &AppendEntriesRequest{
		Term:         n.currentTerm,
		LeaderID:     n.nodeID,
		PrevLogIndex: prevIdx,
		PrevLogTerm:  prevTerm,
		Entries:      entris,
		LeaderCommit: n.logMgr.CommitIndex(),
	}

	return req
}

func (n *node) createSnapshotRequest() *SnapshotRequest {
	return &SnapshotRequest{
		Term:          n.currentTerm,
		LeaderID:      n.nodeID,
		SnapshotIndex: n.logMgr.SnapshotIndex(),
		SnapshotTerm:  n.logMgr.SnapshotTerm(),
		File:          n.logMgr.SnapshotFile(),
	}
}
