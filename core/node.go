package core

import (
	"log"
	"math/rand"
	"sync"
	"time"
)

// nodeState - node state type, can be one of 3: follower, candidate or leader
type nodeState int

// NodeState allowed values
const (
	follower  = 0
	candidate = 1
	leader    = 2
)

// INode defines the interface for a node
type INode interface {
	getState() nodeState
	setState(newState nodeState) nodeState

	stopElectionTimer()
	resetElectionTimer()
	stopHeartbeatTimer()
	resetHeartbeatTimer()

	startElection() bool
	vote(electMsg *RaftMessage) bool
	countVotes(ballotMsg *RaftMessage) bool
	ackHeartbeat(hbMsg *RaftMessage) bool
	sendHeartbeat() bool

	// Start starts the raft node event loop.
	// If a WaitGroup parameter is given, it'll be signaled when the event loop finishes
	// Usually this should be called in its own go routine
	Start(wg *sync.WaitGroup)
}

const minElectionTimeoutMs = 3500                    // millisecond
const maxElectionTimeoutMs = 5000                    // millisecond
const heartbeatTimeoutMs = minElectionTimeoutMs + 20 // millisecond, larger value so that we can mimic some failures

// raftNode represents a raft node
type raftNode struct {
	id            int
	term          int
	state         nodeState
	lastVotedTerm int
	votes         []bool
	size          int

	electionTimer  *time.Timer // timer for election timeout, used by follower and candidate
	heartbeatTimer *time.Timer // timer for heartbeat, used by leader

	stateMachine raftStateMachine
	network      IRaftNetwork // underlying network implementation for sending/receiving messages
	logger       *log.Logger
}

// CreateNode creates a new raft node
func CreateNode(id int, size int, network IRaftNetwork, logger *log.Logger) INode {
	// Create timer objects (stopped)
	electionTimer := time.NewTimer(time.Hour)
	stopTimer(electionTimer)
	heartbeatTimer := time.NewTimer(time.Hour)
	stopTimer(heartbeatTimer)

	return &raftNode{
		id:            id,
		term:          0,
		state:         follower,
		lastVotedTerm: 0,
		votes:         make([]bool, size),
		size:          size,

		electionTimer:  electionTimer,
		heartbeatTimer: heartbeatTimer,

		stateMachine: raftSM,
		network:      network,
		logger:       logger,
	}
}

// getState returns the node's current state
func (node *raftNode) getState() nodeState {
	return node.state
}

// setState changes the node's state
func (node *raftNode) setState(newState nodeState) nodeState {
	oldState := node.state
	node.state = newState
	return oldState
}

// stopElectionTimer stops the election timer for the node
func (node *raftNode) stopElectionTimer() {
	stopTimer(node.electionTimer)
}

// resetElectionTimer resets the election timer for the node
func (node *raftNode) resetElectionTimer() {
	timeout := rand.Intn(maxElectionTimeoutMs-minElectionTimeoutMs) + minElectionTimeoutMs
	resetTimer(node.electionTimer, time.Duration(timeout)*time.Millisecond)
}

// stopHeartbeatTimer stops the heart beat timer (none leader scenario)
func (node *raftNode) stopHeartbeatTimer() {
	stopTimer(node.heartbeatTimer)
}

// resetHeartbeatTimer resets the heart beat timer for the leader
func (node *raftNode) resetHeartbeatTimer() {
	resetTimer(node.heartbeatTimer, time.Duration(heartbeatTimeoutMs)*time.Millisecond)
}

// startElection starts an election and elect self
func (node *raftNode) startElection() bool {
	// set new candidate term
	node.term++

	// only start real election when we haven't voted for others for a higher term
	if node.lastVotedTerm < node.term {
		for i := range node.votes {
			node.votes[i] = false
		}
		// vote for self first
		node.votes[node.id] = true
		node.lastVotedTerm = node.term

		node.logger.Printf("\u270b T%d: Node%d starts election...\n", node.term, node.id)
		node.network.Broadcast(node.id, node.createRequestVoteMessage())
	} else {
		// We are a doomed candidate - alreayd voted for others for current term. Don't start election, wait for next term instead
		node.logger.Printf("\U0001F613 T%d: Node%d is a doomed candidate, waiting for next term...\n", node.term, node.id)
	}

	return true
}

// vote for newer term and when we haven't voted for it yet
func (node *raftNode) vote(electMsg *RaftMessage) bool {
	if electMsg.term > node.term && electMsg.term > node.lastVotedTerm {
		node.lastVotedTerm = electMsg.term
		node.logger.Printf("\U0001f4e7 T%d: Node%d votes for Node%d \n", electMsg.term, node.id, electMsg.nodeID)
		node.network.Send(node.id, electMsg.nodeID, node.createVoteMessage(electMsg))
		return true
	}

	return false
}

// countVotes counts votes received and decide whether we win
func (node *raftNode) countVotes(ballotMsg *RaftMessage) bool {
	if ballotMsg.data == node.id && ballotMsg.term == node.term {
		node.votes[ballotMsg.nodeID] = true

		totalVotes := 0
		for _, v := range node.votes {
			if v {
				totalVotes++
			}
		}

		if totalVotes > node.size/2 {
			// Won election, start heartbeat
			node.logger.Printf("\U0001f451 T%d: Node%d won election\n", node.term, node.id)
			node.sendHeartbeat()
			return true
		}
	}

	return false
}

// ackHeartbeat acks a heartbeat message
func (node *raftNode) ackHeartbeat(hbMsg *RaftMessage) bool {
	// handle heartbeat message with the same or newer term
	if hbMsg.term >= node.term {
		node.logger.Printf("\U0001f493 T%d: Node%d <- Node%d\n", hbMsg.term, node.id, hbMsg.nodeID)
		node.term = hbMsg.term
		return true
	}

	return false
}

// sendHeartbeat sends a heartbeat message
func (node *raftNode) sendHeartbeat() bool {
	node.network.Broadcast(node.id, node.createHeartBeatMessage())
	return true
}

// processMessage passes the message through the node statemachine
// it returns a signal about whether the node should stop
func (node *raftNode) processMessage(msg *RaftMessage) bool {
	node.stateMachine.processMessage(node, msg)
	return false
}

// Start starts the raft node event loop.
// If a WaitGroup parameter is given, it'll be signaled when the event loop finishes
// Usually this should be called in its own go routine
func (node *raftNode) Start(wg *sync.WaitGroup) {
	node.logger.Printf("Node%d starting...\n", node.id)

	node.resetElectionTimer()

	var msg *RaftMessage
	quit := false
	msgCh, _ := node.network.GetRecvChannel(node.id)
	electCh := node.electionTimer.C
	hbCh := node.heartbeatTimer.C

	for !quit {
		select {
		case msg = <-msgCh:
		case <-electCh:
			msg = node.createStartElectionMessage()
		case <-hbCh:
			msg = node.createSendHeartBeatMessage()
		}

		// Do the real processing
		quit = node.processMessage(msg)
	}

	if wg != nil {
		wg.Done()
	}
}
