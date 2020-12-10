package raft

import "github.com/sidecus/raft/pkg/util"

// enter follower state and follows new leader (or potential leader)
func (n *node) enterFollowerState(sourceNodeID, newTerm int) {
	oldLeader := n.currentLeader
	n.nodeState = Follower
	n.currentLeader = sourceNodeID
	n.setTerm(newTerm)

	if n.nodeID != sourceNodeID && oldLeader != n.currentLeader {
		util.WriteInfo("T%d: Node%d follows Node%d on new Term\n", n.currentTerm, n.nodeID, sourceNodeID)
	}
}

// enter candidate state
func (n *node) enterCandidateState() {
	n.nodeState = Candidate
	n.currentLeader = -1
	n.setTerm(n.currentTerm + 1)

	// vote for self first
	n.votedFor = n.nodeID
	n.votes = make(map[int]bool, n.clusterSize)
	n.votes[n.nodeID] = true

	util.WriteInfo("T%d: \u270b Node%d starts election\n", n.currentTerm, n.nodeID)
}

// start an election, caller should acquire write lock
func (n *node) startElection() {
	n.enterCandidateState()

	// create RV request
	req := &RequestVoteRequest{
		Term:         n.currentTerm,
		CandidateID:  n.nodeID,
		LastLogIndex: n.logMgr.LastIndex(),
		LastLogTerm:  n.logMgr.LastTerm(),
	}

	// request vote (on different goroutines), response will be processed there
	n.peerMgr.BroadcastRequestVote(req, func(reply *RequestVoteReply) { n.handleRequestVoteReply(reply) })

	n.refreshTimer()
}

// AppendEntries handles raft RPC AE calls
func (n *node) AppendEntries(req *AppendEntriesRequest) (*AppendEntriesReply, error) {
	n.mu.Lock()
	defer n.mu.Unlock()

	n.tryFollowNewTerm(req.LeaderID, req.Term, true)

	// After above call, n.currentLeader has been updated accordingly if req.Term is the same or higher

	lastMatchIndex, prevMatch := -1, false
	if req.Term >= n.currentTerm {
		// only process logs when term is valid
		prevMatch = n.logMgr.ProcessLogs(req.PrevLogIndex, req.PrevLogTerm, req.Entries)
		if prevMatch {
			// logs are catching up - at least matching up to n.logMgr.lastIndex. record it and try to commit
			lastMatchIndex = n.logMgr.LastIndex()
			n.commitTo(req.LeaderCommit)
		}
	}

	return &AppendEntriesReply{
		Term:      n.currentTerm,
		NodeID:    n.nodeID,
		LeaderID:  n.currentLeader,
		Success:   prevMatch,
		LastMatch: lastMatchIndex, // this is only meaningful when Success is true
	}, nil
}

// InstallSnapshot installs a snapshot
func (n *node) InstallSnapshot(req *SnapshotRequest) (*AppendEntriesReply, error) {
	n.mu.Lock()
	defer n.mu.Unlock()

	n.tryFollowNewTerm(req.LeaderID, req.Term, true)

	// After above call, n.currentLeader has been updated accordingly if req.Term is the same or higher

	success := false
	lastMatchIndex := -1
	if req.Term >= n.currentTerm {
		// TODO[sidecus]: test case for the if branch
		if n.logMgr.SnapshotIndex() == req.SnapshotIndex && n.logMgr.SnapshotTerm() == req.SnapshotTerm {
			util.WriteInfo("T%d: Node%d ignoring already installed snapshot from Node%d upto L%d\n", n.currentTerm, n.nodeID, req.LeaderID, req.SnapshotIndex)
			success = true
			lastMatchIndex = req.SnapshotIndex
		} else {
			// only process logs when term is valid
			util.WriteInfo("T%d: Node%d installing snapshot from Node%d upto L%d\n", n.currentTerm, n.nodeID, req.LeaderID, req.SnapshotIndex)
			err := n.logMgr.InstallSnapshot(req.File, req.SnapshotIndex, req.SnapshotTerm)
			if err == nil {
				success = true
				lastMatchIndex = req.SnapshotIndex
				util.WriteInfo("T%d: Snapshot installed.\n", n.currentTerm)
			} else {
				util.WriteError("T%d: Install snapshot failed. %s\n", n.currentTerm, err)
			}
		}
	}

	return &AppendEntriesReply{
		Term:      n.currentTerm,
		NodeID:    n.nodeID,
		LeaderID:  n.currentLeader,
		Success:   success,
		LastMatch: lastMatchIndex, // this is only meaningful when Success is true
	}, nil

}

// RequestVote handles raft RPC RV calls
func (n *node) RequestVote(req *RequestVoteRequest) (*RequestVoteReply, error) {
	n.mu.Lock()
	defer n.mu.Unlock()

	// Here is what the paper says (5.2 and 5.4):
	// 1. if req.Term > currentTerm, convert to follower state (reset votedFor)
	// 2. if req.Term < currentTerm deny vote
	// 3. if req.Term >= currentTerm, and if votedFor is null or candidateId, and logs are up to date, grant vote
	// The req.Term == currentTerm situation AFAIK can only happen when we receive a duplicate RV request
	n.tryFollowNewTerm(req.CandidateID, req.Term, false)
	voteGranted := false
	if req.Term >= n.currentTerm && (n.votedFor == -1 || n.votedFor == req.CandidateID) {
		if req.LastLogIndex >= n.logMgr.LastIndex() && req.LastLogTerm >= n.logMgr.LastTerm() {
			n.votedFor = req.CandidateID
			voteGranted = true
			util.WriteInfo("T%d: \U0001f4e7 Node%d voted for Node%d\n", req.Term, n.nodeID, req.CandidateID)
		}
	}

	return &RequestVoteReply{
		Term:        n.currentTerm,
		NodeID:      n.nodeID,
		VotedTerm:   req.Term,
		VoteGranted: voteGranted,
	}, nil
}

// callback to be invoked when reply is received (on different goroutine so we need to acquire lock)
func (n *node) handleRequestVoteReply(reply *RequestVoteReply) {
	n.mu.Lock()
	defer n.mu.Unlock()

	if n.tryFollowNewTerm(reply.NodeID, reply.Term, false) {
		// there is a higher term, no need to continue
		return
	}

	if reply.VotedTerm != n.currentTerm || n.nodeState != Candidate || !reply.VoteGranted {
		// stale vote or denied, ignore
		return
	}

	// record and count votes
	n.votes[reply.NodeID] = true
	if n.wonElection() {
		n.enterLeaderState()
		n.sendHeartbeat()
	}
}

// count votes for current node and term and return true if we won
func (n *node) wonElection() bool {
	total := 0
	for _, v := range n.votes {
		if v {
			total++
		}
	}
	return total > n.clusterSize/2
}

// setTerm sets a new term
func (n *node) setTerm(newTerm int) {
	if newTerm < n.currentTerm {
		util.Panicf("can't set new term %d, which is less than current term %d\n", newTerm, n.currentTerm)
	}

	if newTerm > n.currentTerm {
		// reset vote on higher term
		n.votedFor = -1
	}

	n.currentTerm = newTerm
}
