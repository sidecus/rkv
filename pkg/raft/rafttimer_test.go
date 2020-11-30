package raft

import (
	"testing"
)

func TestRaftTimer(t *testing.T) {
	// stop
	refreshRaftTimer(NodeState(-1))
}
