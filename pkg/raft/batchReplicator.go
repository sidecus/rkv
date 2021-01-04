package raft

import "sync"

const targetAny = int(^uint(0) >> 1)

type replicationReq struct {
	targetID int
	reqwg    *sync.WaitGroup
}

// batchReplicator processes incoming requests (best effort) while at the same time tries to batch them for better efficency.
// For each request in the request queue:
// 1. If request id is less than lastMatch, signal done direclty (already replicated)
// 2. If request id is larger than lastMatch, trigger a new replicate (a few items in batch). signal done afterwards regardless
//    whether the target id is satisfied or not.
// In short, each request in the queue will trigger at most 1 replicate
type batchReplicator struct {
	replicateFn func() int
	requests    chan replicationReq
	wg          sync.WaitGroup
}

// newBatchReplicator creates a new batcher
func newBatchReplicator(replicate func() int) *batchReplicator {
	return &batchReplicator{
		replicateFn: replicate,
		requests:    make(chan replicationReq, maxAppendEntriesCount),
	}
}

// Start starts the batcher service
func (b *batchReplicator) Start() {
	b.wg.Add(1)
	go func() {
		lastMatch := -1
		for r := range b.requests {
			if r.targetID > lastMatch {
				// invoke new batch operation to see whether we can process up to targetID
				lastMatch = b.replicateFn()
			}

			if r.reqwg != nil {
				r.reqwg.Done()
			}
		}

		b.wg.Done()
	}()
}

// Stop stops the batcher and wait for finish
func (b *batchReplicator) Stop() {
	close(b.requests)
	b.wg.Wait()
}

// RequestReplicateTo requests a process towards the target id.
// It'll block if current request queue is full.
// true - if the targetID is processed within one batch after the request has been picked up by the batcher
// false - otherwise
func (b *batchReplicator) RequestReplicateTo(targetID int, wg *sync.WaitGroup) {
	if targetID < 0 || targetID == targetAny {
		panic("invalid target index")
	}

	b.requests <- replicationReq{
		targetID: targetID,
		reqwg:    wg,
	}
}

// TryRequestReplicate request a batch process with no target.
// It won't block if request queue is full.
func (b *batchReplicator) TryRequestReplicate() {
	select {
	case b.requests <- replicationReq{targetID: targetAny}:
	default:
	}
}
