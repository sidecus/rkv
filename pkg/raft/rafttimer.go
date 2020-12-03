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
	node  INode
	wg    *sync.WaitGroup
	state chan NodeState
	timer *time.Timer
}

func newRaftTimer(node INode) IRaftTimer {
	return &raftTimer{
		node:  node,
		wg:    &sync.WaitGroup{},
		state: make(chan NodeState, 10),
		timer: nil, // only initialized upon start
	}
}

// getElectionTimeout gets a random election timeout
func (rt *raftTimer) getElectionTimeout() time.Duration {
	timeoutMS := rand.Intn(maxElectionTimeoutMS-minElectionTimeoutMS) + minElectionTimeoutMS
	return time.Duration(timeoutMS) * time.Millisecond
}

func (rt *raftTimer) Start() {
	if rt.timer != nil {
		util.Panicln("Timer is already started")
	}

	rt.timer = time.NewTimer(rt.getElectionTimeout())

	rt.wg.Add(1)
	go func() {
		stop := false
		for !stop {
			select {
			case state := <-rt.state:
				if state == Follower || state == Candidate {
					// reset timer on follower/candidate state with random election timeout
					util.ResetTimer(rt.timer, rt.getElectionTimeout())
				} else if state == Leader {
					// reset timer with hearbeat timeout for leader state
					util.ResetTimer(rt.timer, heartbeatTimeout)
				} else {
					stop = true
				}
			case <-rt.timer.C:
				// tell node that we need a new election or heatbeat
				rt.node.OnTimer()
			}
		}
		rt.wg.Done()
	}()
}

// refreshRaftTimer notifies the timer to refresh based on new state
func (rt *raftTimer) Refresh(newState NodeState) {
	rt.state <- newState
}

// stopRaftTimer stops the raft timer goroutine
func (rt *raftTimer) Stop() {
	rt.state <- -1
	rt.wg.Wait()
	rt.timer = nil
}
