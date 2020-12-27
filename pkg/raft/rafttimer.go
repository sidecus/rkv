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
	Refresh(newState NodeState, term int)
}

type resetInfo struct {
	state NodeState
	term  int
}

type raftTimer struct {
	wg       *sync.WaitGroup
	timer    *time.Timer
	cterm    chan resetInfo
	callback func(state NodeState, term int)
}

// NewRaftTimer creates a new raft timer
func NewRaftTimer(timerCallback func(state NodeState, term int)) IRaftTimer {
	rt := &raftTimer{
		wg:       &sync.WaitGroup{},
		callback: timerCallback,
		cterm:    make(chan resetInfo, 100), // use buffered channels so that we don't block sender
	}

	rt.timer = time.NewTimer(rt.getElectionTimeout())
	util.StopTimer(rt.timer)
	return rt
}

// Start starts the timer with follower state
func (rt *raftTimer) Start() {
	if rt.timer == nil {
		util.Panicln("Timer not initialized")
	}

	// Start timer event loop
	rt.wg.Add(1)
	go rt.eventLoop()
}

// refreshRaftTimer refreshes the timer based on node state and tries to drain pending timer events if any
func (rt *raftTimer) Refresh(newState NodeState, term int) {
	rt.cterm <- resetInfo{state: newState, term: term}
}

// stopRaftTimer stops the raft timer goroutine
func (rt *raftTimer) Stop() {
	close(rt.cterm)
	util.StopTimer(rt.timer)
	rt.wg.Wait()
	rt.timer = nil
}

// timer event loop
func (rt *raftTimer) eventLoop() {
	state := NodeState(NodeStateFollower)
	term := -1
	stop := false
	for !stop {
		select {
		case info := <-rt.cterm:
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
