package raft

import (
	"math/rand"
	"sync"
	"time"

	"github.com/sidecus/raft/pkg/util"
)

const minElectionTimeoutMS = 150
const maxElectionTimeoutMS = 300
const heartbeatTimeoutMS = 100
const heartbeatTimeout = time.Duration(heartbeatTimeoutMS) * time.Millisecond

// IRaftTimer defines the raft timer interface
type IRaftTimer interface {
	Start()
	Stop()
	Reset(newState NodeState, term int)
}

type resetEvt struct {
	state NodeState
	term  int
}

type raftTimer struct {
	wg       *sync.WaitGroup
	timer    *time.Timer
	evt      chan resetEvt
	callback func(state NodeState, term int)
}

// NewRaftTimer creates a new raft timer
func NewRaftTimer(timerCallback func(state NodeState, term int)) IRaftTimer {
	rt := &raftTimer{
		wg:       &sync.WaitGroup{},
		callback: timerCallback,
		evt:      make(chan resetEvt, 100), // use buffered channels so that we don't block sender
	}

	rt.timer = time.NewTimer(rt.getElectionTimeout())
	util.StopTimer(rt.timer)
	return rt
}

// Start starts the timer with follower state
func (rt *raftTimer) Start() {
	if rt.timer == nil {
		util.Fatalf("Timer not initialized\n")
	}

	// Start timer event loop
	rt.wg.Add(1)
	go rt.eventLoop()
}

// stopRaftTimer stops the raft timer goroutine
func (rt *raftTimer) Stop() {
	util.StopTimer(rt.timer)
	rt.wg.Wait()
	rt.timer = nil
}

// Reset refreshes the timer based on node state and tries to drain pending timer events if any
func (rt *raftTimer) Reset(newState NodeState, term int) {
	rt.evt <- resetEvt{state: newState, term: term}
}

// timer event loop
func (rt *raftTimer) eventLoop() {
	state := NodeState(NodeStateFollower)
	term := -1
	stop := false
	for !stop {
		select {
		case info := <-rt.evt:
			state = info.state
			term = info.term

			// reset timer and drain events
			var timeout time.Duration
			if state == NodeStateLeader {
				timeout = heartbeatTimeout
			} else {
				timeout = rt.getElectionTimeout()
			}
			util.ResetTimer(rt.timer, timeout)
		case _, ok := <-rt.timer.C:
			if !ok {
				stop = true
			} else {
				rt.callback(state, term)
			}
		}
	}
	rt.wg.Done()
}

// getElectionTimeout gets a random election timeout
func (rt *raftTimer) getElectionTimeout() time.Duration {
	timeoutMS := rand.Intn(maxElectionTimeoutMS-minElectionTimeoutMS) + minElectionTimeoutMS
	return time.Duration(timeoutMS) * time.Millisecond
}
