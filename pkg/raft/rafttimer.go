package raft

import (
	"math/rand"
	"sync"
	"time"

	"github.com/sidecus/raft/pkg/util"
)

const minElectionTimeoutMS = 600
const maxElectionTimeoutMS = 2000
const heartbeatTimeoutMS = 150
const heartbeatTimeout = time.Duration(heartbeatTimeoutMS) * time.Millisecond

// IRaftTimer defines the raft timer interface
type IRaftTimer interface {
	start()
	stop()
	reset(newState NodeState, term int)
}

type resetEvt struct {
	state NodeState
	term  int
}

type raftTimer struct {
	wg       sync.WaitGroup
	timer    *time.Timer
	evt      chan resetEvt
	callback func(state NodeState, term int)
}

// newRaftTimer creates a new raft timer
func newRaftTimer(timerCallback func(state NodeState, term int)) IRaftTimer {
	rt := &raftTimer{
		callback: timerCallback,
		evt:      make(chan resetEvt, 100), // use buffered channels so that we don't block sender
	}

	return rt
}

// start starts the timer with a large interval
func (rt *raftTimer) start() {
	rt.timer = time.NewTimer(time.Hour * 24)
	rt.wg.Add(1)
	go rt.run()
}

// stopRaftTimer stops the raft timer goroutine
func (rt *raftTimer) stop() {
	util.StopTimer(rt.timer)
	rt.wg.Wait()
	rt.timer = nil
}

// Reset refreshes the timer based on node state and tries to drain pending timer events if any
func (rt *raftTimer) reset(newState NodeState, term int) {
	rt.evt <- resetEvt{state: newState, term: term}
}

// run runs the timer event loop
func (rt *raftTimer) run() {
	state, term := NodeStateFollower, 0
	stop := false
	for !stop {
		select {
		case info := <-rt.evt:
			state, term = info.state, info.term
			timeout := getTimeout(state, term)
			util.WriteVerbose("Resetting timer. state:%d, term:%d, timeout:%dms", state, term, timeout/time.Millisecond)
			util.ResetTimer(rt.timer, timeout)
		case _, ok := <-rt.timer.C:
			if !ok {
				stop = true
				break
			}
			util.WriteVerbose("Timer event received. state:%d, term:%d", state, term)
			rt.callback(state, term)
		}
	}
	rt.wg.Done()
}

var firstFollow = true

func getTimeout(state NodeState, term int) (timeout time.Duration) {
	if state == NodeStateLeader {
		timeout = heartbeatTimeout
		return
	}

	ms := rand.Intn(maxElectionTimeoutMS-minElectionTimeoutMS) + minElectionTimeoutMS
	timeout = time.Duration(ms) * time.Millisecond

	isFollow := term > 0 && state == NodeStateFollower
	if firstFollow && isFollow {
		// Hack:
		// When this node starts, it will start voting and follow current cluster term based on negtive vote results we receive.
		// However it might take time for leader to establish RPC connection with us. This means heartbeat to us will get delayed.
		// If we start a new vote again too quickly, it'll increase the cluster's current term and cause other nodes to start election as well.
		// Worst case scenario it leads to a short "election storm" even when leader and all other nodes are working as usual.
		// To mitigate this, we use a larger election timeout after first time becoming follower.
		timeout = timeout * 10
		firstFollow = false
	}

	return
}
