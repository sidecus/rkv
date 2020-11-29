package raft

import (
	"testing"
)

func TestRaftTimer(t *testing.T) {
	// stop
	RefreshRaftTimer(NodeState(-1))
}
