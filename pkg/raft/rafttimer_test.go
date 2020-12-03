package raft

import (
	"testing"
)

// Fake timer implementation for other unit tests
type fakeRaftTimer struct {
	state NodeState
}

func (t *fakeRaftTimer) Start() {
}
func (t *fakeRaftTimer) Stop() {
}
func (t *fakeRaftTimer) Refresh(newState NodeState) {
	t.state = newState
}

func TestRaftTimer(t *testing.T) {
	// stop
}
