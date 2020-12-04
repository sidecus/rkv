package raft

import (
	"math/rand"
	"sync"
	"time"

	"github.com/sidecus/raft/pkg/util"
)

const minElectionTimeoutMS = 300
const maxElectionTimeoutMS = 400
const heartbeatTimeoutMS = 200
const heartbeatTimeout = time.Duration(heartbeatTimeoutMS) * time.Millisecond

// IRaftTimer defines the raft timer interface
type IRaftTimer interface {
	Start()
	Stop()
	Refresh(newState NodeState)
}

type raftTimer struct {
	wg       *sync.WaitGroup
	timer    *time.Timer
	callback func()
}

// NewRaftTimer creates a new raft timer
func NewRaftTimer(timerCallback func()) IRaftTimer {
	return &raftTimer{
		wg:       &sync.WaitGroup{},
		callback: timerCallback,
	}
}

// Start starts the timer with follower state
func (rt *raftTimer) Start() {
	if rt.timer != nil {
		util.Panicln("Timer is already started")
	}

	// Use a longer initial timeout since establishing RPC connection takes time during startup
	// Think about this scenario when one node failed:
	// t0: leader sends heartbeat, it fails to send to the failed node (we don't retry here, unlike the paper)
	// t1: the failed node recovers right after t0, and sets election timer
	// t2: leader sends new heartbeat, it'll arrive at the recovered node, but takes time (timeout + RPC connection and processing time)
	// t3: recovered node processes the new heartbeat and follows leader term, and its election timer fires at the same time
	// t4: recovered node starts new election with higher term, but it never wins due to stale log
	// t5: original leader gets reset to follower due to the higher term and starts another election since recovered node is not getting elected
	// t6: recovered node starts another election due to failed vote
	// t7: repeating t4-t5 and we might have a temporary election storm
	// This is due to we never block on RPC calls. So we are adding the extra delay during startup to prevent this.
	rt.timer = time.NewTimer(rt.getElectionTimeout() + 2*time.Second)

	// Start timer event loop
	rt.wg.Add(1)
	go rt.eventLoop()
}

// refreshRaftTimer refreshes the timer based on node state and tries to drain pending timer events if any
func (rt *raftTimer) Refresh(newState NodeState) {
	if newState == Leader {
		util.ResetTimer(rt.timer, heartbeatTimeout)
	} else {
		util.ResetTimer(rt.timer, rt.getElectionTimeout())
	}
}

// stopRaftTimer stops the raft timer goroutine
func (rt *raftTimer) Stop() {
	util.StopTimer(rt.timer)
	rt.wg.Wait()
	rt.timer = nil
}

// timer event loop
func (rt *raftTimer) eventLoop() {
	stop := false
	for !stop {
		select {
		case _, ok := <-rt.timer.C:
			if !ok {
				stop = true
			} else {
				rt.callback()
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
