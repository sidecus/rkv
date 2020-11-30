package raft

// This file implements node methods related to raft RPC calls

// AppendEntries handles raft RPC AE calls
func (n *node) AppendEntries(req *AppendEntriesRequest) (*AppendEntriesReply, error) {
	n.mu.Lock()
	defer n.mu.Unlock()

	n.tryFollowHigherTerm(req.LeaderID, req.Term, true)

	success := false
	if req.Term >= n.currentTerm {
		// only process logs when term is the same or larger
		success = n.logMgr.appendLogs(req.PrevLogIndex, req.PrevLogTerm, req.Entries)
		n.commitTo(req.LeaderCommit)
	}

	return &AppendEntriesReply{
		Term:     n.currentTerm,
		LeaderID: req.LeaderID,
		Success:  success,
	}, nil
}

// handleAppendEntriesReply handles append entries reply. Need locking since this will be
// running on different goroutine for reply from each node
func (n *node) handleAppendEntriesReply(reply *AppendEntriesReply) {
	n.mu.Lock()
	defer n.mu.Unlock()

	// If there is a higher term, follow and stop processing
	if n.tryFollowHigherTerm(reply.LeaderID, reply.Term, false) {
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
	n.followers.update(nodeID, reply.Success, n.logMgr.lastIndex)

	// Check whether there are logs to commit
	n.commitIfAny()

	// replicate more logs if needed
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
	n.tryFollowHigherTerm(req.CandidateID, req.Term, false)
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
		NodeID:      n.nodeID,
		Term:        n.currentTerm,
		VotedTerm:   req.Term,
		VoteGranted: voteGranted,
	}, nil
}

// callback to be invoked when reply is received (on different goroutine so we need to acquire lock)
func (n *node) handleRequestVoteReply(reply *RequestVoteReply) {
	n.mu.Lock()
	defer n.mu.Unlock()

	if n.tryFollowHigherTerm(reply.NodeID, reply.Term, false) {
		// there is a higher term, no need to continue
		return
	}

	if reply.VotedTerm != n.currentTerm {
		// stale vote reply, ignore
		return
	}

	if !reply.VoteGranted {
		// vote denied, ignore
		return
	}

	// record and count votes
	n.votes[reply.NodeID] = true
	total := 0
	for _, v := range n.votes {
		if v {
			total++
		}
	}

	if total > n.clusterSize/2 {
		// we won, set leader status and send heartbeat
		n.enterLeaderState()
		n.sendHeartbeat()
	}
}

// Get gets values from state machine
func (n *node) Get(req *GetRequest) (*GetReply, error) {
	n.mu.RLock()

	if n.nodeState == Leader {
		ret, err := n.stateMachine.Get(req.Params...)
		n.mu.RUnlock()
		if err != nil {
			return nil, err
		}

		return &GetReply{Data: ret}, nil
	}

	n.mu.RUnlock()

	if n.currentLeader == -1 {
		return nil, ErrorNoLeaderAvailable
	}

	// proxy to leader instead, no lock needed
	// otherwise we might have a deadlock between this and the leader
	return n.proxy.Get(n.currentLeader, req)
}

// Execute runs a command via the raft node
func (n *node) Execute(cmd *StateMachineCmd) (bool, error) {
	n.mu.Lock()

	if n.nodeState == Leader {
		cmds := []StateMachineCmd{*cmd}
		n.logMgr.appendCmds(cmds, n.currentTerm)

		for _, follower := range n.followers {
			n.replicateLogsIfAny(follower.nodeID)
		}

		// TODO[sidecus]: 5.3, 5.4 - wait for response and then commit
		// For now we don't wait for commit in Execute
		// Instead commit is done asynchronously after replication

		n.mu.Unlock()
		return true, nil
	}

	n.mu.Unlock()

	if n.currentLeader == -1 {
		return false, ErrorNoLeaderAvailable
	}

	// proxy to leader instead, no lock needed
	// otherwise we might have a deadlock between this and the leader
	return n.proxy.Execute(n.currentLeader, cmd)
}
