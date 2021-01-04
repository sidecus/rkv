package raft

import (
	"sync"
	"sync/atomic"
	"testing"
)

func TestBatchReplicate(t *testing.T) {
	lastMatch := int32(-1)
	replicator := newBatchReplicator(func() int {
		return int(atomic.AddInt32(&lastMatch, 5))
	})

	replicator.start()
	var wg sync.WaitGroup

	wg.Add(1)
	replicator.requestReplicateTo(0, &wg)
	wg.Wait()
	val := atomic.LoadInt32(&lastMatch)
	if val != 4 {
		t.Error("batch operation was not invoked")
	}

	wg.Add(1)
	replicator.requestReplicateTo(4, &wg)
	wg.Wait()
	val = atomic.LoadInt32(&lastMatch)
	if val != 4 {
		t.Error("batch operation was invoked when it should not be")
	}

	wg.Add(1)
	replicator.requestReplicateTo(5, &wg)
	wg.Wait()
	val = atomic.LoadInt32(&lastMatch)
	if val != 9 {
		t.Error("new batch operation was not invoked")
	}

	wg.Add(1)
	replicator.tryRequestReplicate(&wg)
	wg.Wait()
	val = atomic.LoadInt32(&lastMatch)
	if val != 14 {
		t.Error("tryRequestReplicate didn't trigger replicate")
	}

	replicator.stop()
}

func TestTryRequestReplicate(t *testing.T) {
	lastMatch := -1
	replicator := newBatchReplicator(func() int {
		lastMatch += 5
		return lastMatch
	})
	queueSize := len(replicator.requests)

	// below should never block
	for i := 0; i < queueSize*2; i++ {
		replicator.tryRequestReplicate(nil)
	}
}
