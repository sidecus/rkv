package raft

import (
	"math/rand"
	"time"

	"github.com/sidecus/raft/pkg/util"
)

const minElectionTimeoutMS = 300
const maxElectionTimeoutMS = 400
const heartbeatTimeoutMS = 200
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
	// Create a timer
	timer := time.NewTimer(getElectionTimeout())

	for {
		select {
		case currentState := <-state:
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

// refreshRaftTimer notifies the timer to refresh based on new state
func refreshRaftTimer(newState NodeState) {
	state <- newState
}

// startRaftTimer starts the raft timer goroutine
func startRaftTimer(node INode) {
	go raftTimer(node)
}

// stopRaftTimer stops the raft timer goroutine
func stopRaftTimer() {
	state <- -1
}
