package raft

import (
	"math/rand"
	"time"

	"github.com/sidecus/raft/pkg/util"
)

const minElectionTimeoutMS = 20000
const maxElectionTimeoutMS = 30000
const heartbeatTimeoutMS = 15000
const heartbeatTimeout = time.Duration(heartbeatTimeoutMS) * time.Millisecond

// node state channel
var state = make(chan NodeState, 10)

// getElectionTimeout gets a random election timeout
func getElectionTimeout() time.Duration {
	timeoutMS := rand.Intn(maxElectionTimeoutMS-minElectionTimeoutMS) + minElectionTimeoutMS
	return time.Duration(timeoutMS) * time.Millisecond
}

// election timer starts an election timer
func raftTimer(node INode) {
	// Create a timer (stopped right away)
	timer := time.NewTimer(time.Hour)
	util.StopTimer(timer)
	currentState := NodeState(Candidate)

	for {
		select {
		case currentState = <-state:
			if currentState == Follower || currentState == Candidate {
				// reset timer on follower/candidate state with random election timeout
				util.ResetTimer(timer, getElectionTimeout())
			} else if currentState == Leader {
				// reset timer with hearbeat timeout for leader state
				util.ResetTimer(timer, heartbeatTimeout)
			} else {
				// return on any other values (stopping)
				return
			}
		case <-timer.C:
			// tell node that we need a new election
			node.OnTimer()
		}
	}
}

// RefreshRaftTimer notifies the timer to refresh based on new state
func RefreshRaftTimer(newState NodeState) {
	state <- newState
}

// StartRaftTimer starts the raft timer goroutine
func StartRaftTimer(node INode) {
	go raftTimer(node)
}

// StopRaftTimer stops the raft timer goroutine
func StopRaftTimer() {
	state <- -1
}
