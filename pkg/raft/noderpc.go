package raft

import "fmt"

// This file implements node methods related to raft RPC calls

// AppendEntries handles raft RPC AE calls
func (n *node) AppendEntries(req *AppendEntriesRequest) (*AppendEntriesReply, error) {
	n.mu.Lock()
	defer n.mu.Unlock()

	success := false
	n.tryFollowHigherTerm(req.LeaderID, req.Term, true)
	if req.Term >= n.currentTerm {
		// only process when term is the same or larger

		// TODO[sidecus] - implement 5.3

		success = true
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

	// TODO[sidecus] update leader indicies and resend new data as needed
	// implement 5.3
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
		n.votedFor = req.CandidateID
		voteGranted = true

		fmt.Printf("T%d: Node%d voted for Node%d\n", req.Term, n.nodeID, req.CandidateID)
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
		n.stateMachine.Apply(*cmd)
		n.mu.Unlock()
		// TODO[sidecus] - broad cast to followers and wait for response
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
