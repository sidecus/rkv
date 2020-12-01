package raft

import "errors"

// This file implements node methods related to raft RPC calls

// errorNoLeaderAvailable means there is no leader elected yet (or at least not known to current node)
var errorNoLeaderAvailable = errors.New("No leader currently available")

// AppendEntries handles raft RPC AE calls
func (n *node) AppendEntries(req *AppendEntriesRequest) (*AppendEntriesReply, error) {
	n.mu.Lock()
	defer n.mu.Unlock()

	n.tryFollowNewTerm(req.LeaderID, req.Term, true)

	// After above call, n.currentLeader has been updated accordingly if req.Term is the same or higher

	lastMatch, success := -1, false
	if req.Term >= n.currentTerm {
		// only process logs when term is valid
		success = n.logMgr.appendLogs(req.PrevLogIndex, req.PrevLogTerm, req.Entries)
		if success {
			// logs are catching up - at least matching up to n.logMgr.lastIndex
			// try to commit
			lastMatch = n.logMgr.lastIndex
			n.commitTo(req.LeaderCommit)
		}
	}

	return &AppendEntriesReply{
		Term:      n.currentTerm,
		NodeID:    n.nodeID,
		LeaderID:  n.currentLeader,
		Success:   success,
		LastIndex: lastMatch, // this is only meaningful when Success is true
	}, nil
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

	// 5.3 update leader indicies
	// TODO[sidecus]: low pri, we might want to include the match index in the AE reply
	// using n.logMgr.lastIndex is not safe here since we are asynchronous unlike the paper
	nodeID := reply.NodeID
	n.followerIndicies.update(nodeID, reply.Success, n.logMgr.lastIndex)

	// Check whether there are logs to commit and then replicate
	n.commitIfAny()
	n.replicateLogsIfAny(nodeID)
}

// RequestVote handles raft RPC RV calls
func (n *node) RequestVote(req *RequestVoteRequest) (*RequestVoteReply, error) {
	n.mu.Lock()
	defer n.mu.Unlock()

	// Here is what the paper says:
	// 1. if req.Term > currentTerm, convert to follower state (reset votedFor)
	// 2. if req.Term < currentTerm deny vote
	// 3. if req.Term >= currentTerm:
	//   a. if votedFor is null or candidateId, and logs are up to date, grant vote
	// The req.Term == currentTerm situation AFAIK can only happen when we receive a duplicate RV request
	n.tryFollowNewTerm(req.CandidateID, req.Term, false)
	voteGranted := false
	if req.Term >= n.currentTerm && (n.votedFor == -1 || n.votedFor == req.CandidateID) {
		// 5.2&5.4 - vote only when candidate's log is at least up to date with current node
		if req.LastLogIndex >= n.logMgr.lastIndex && req.LastLogTerm >= n.logMgr.lastTerm {
			n.votedFor = req.CandidateID
			voteGranted = true
			writeInfo("T%d: \U0001f4e7 Node%d voted for Node%d\n", req.Term, n.nodeID, req.CandidateID)
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

	if reply.VotedTerm != n.currentTerm ||
		n.nodeState != Candidate ||
		!reply.VoteGranted {
		// stale vote or denied, ignore
		return
	}

	n.votes[reply.NodeID] = true

	// count votes
	total := n.countVotes()
	if total > n.clusterSize/2 {
		// we won, set leader status and send heartbeat
		n.enterLeaderState()
		n.sendHeartbeat()
	}
}

// Get gets values from state machine, no need to proxy
func (n *node) Get(req *GetRequest) (*GetReply, error) {
	n.mu.RLock()
	defer n.mu.RUnlock()

	ret, err := n.stateMachine.Get(req.Params...)

	if err != nil {
		return nil, err
	}

	return &GetReply{
		NodeID: n.nodeID,
		Data:   ret,
	}, nil
}

// Execute runs a command via the raft node
// If current node is the leader, it'll append the cmd to logs
// If current node is not the leader, it'll proxy the request to leader node
func (n *node) Execute(cmd *StateMachineCmd) (*ExecuteReply, error) {
	n.mu.Lock()

	leader := n.currentLeader
	if n.nodeState == Leader {
		n.logMgr.appendCmd(*cmd, n.currentTerm)

		for _, follower := range n.followerIndicies {
			n.replicateLogsIfAny(follower.nodeID)
		}

		// TODO[sidecus]: 5.3, 5.4 - wait for response and then commit.
		// For now we return eagerly and don't wait for agreement from majority.
		// Instead commit is done asynchronously after replication.

		n.mu.Unlock()
		return &ExecuteReply{
			NodeID:  n.nodeID,
			Success: true,
		}, nil
	}

	n.mu.Unlock()

	if leader == -1 {
		return nil, errorNoLeaderAvailable
	}

	// proxy to leader instead, no lock needed
	// otherwise we might have a deadlock between this and the leader
	return n.peerMgr.Execute(leader, cmd)
}
