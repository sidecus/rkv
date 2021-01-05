package raft

import (
	"testing"
)

// Fake timer implementation for other unit tests
type fakeRaftTimer struct {
	state NodeState
	term  int
}

func (t *fakeRaftTimer) start() {
}
func (t *fakeRaftTimer) stop() {
}
func (t *fakeRaftTimer) reset(newState NodeState, term int) {
	t.state = newState
	t.term = term
}

func TestRaftTimer(t *testing.T) {
	// stop
}
