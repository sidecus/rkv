package raft

import (
	"testing"
)

// Fake timer implementation for other unit tests
type fakeRaftTimer struct {
	state NodeState
	term  int
}

func (t *fakeRaftTimer) Start() {
}
func (t *fakeRaftTimer) Stop() {
}
func (t *fakeRaftTimer) Refresh(newState NodeState, term int) {
	t.state = newState
	t.term = term
}

func TestRaftTimer(t *testing.T) {
	// stop
}
