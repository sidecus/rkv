package raft

import "sync"

const targetAny = int(^uint(0) >> 1)

type batcherReq struct {
	targetID int
	reqwg    *sync.WaitGroup
}

// Batcher processes incoming requests while at the same time tries to batch them for better efficency.
// Each individual request will be processed at most 1 times (0 if the requested target is already batched)
type Batcher struct {
	batchFn  func() int
	requests chan batcherReq
	wg       sync.WaitGroup
}

// NewBatcher creates a new batcher
func NewBatcher(process func() int, queuesize int) *Batcher {
	return &Batcher{
		batchFn:  process,
		requests: make(chan batcherReq, queuesize),
	}
}

// Start starts the batcher service
func (b *Batcher) Start() {
	b.wg.Add(1)
	go func() {
		lastProcessed := -1
		for r := range b.requests {
			if r.targetID > lastProcessed {
				// invoke new batch operation to see whether we can process up to targetID
				lastProcessed = b.batchFn()
			}

			if r.reqwg != nil {
				r.reqwg.Done()
			}
		}

		b.wg.Done()
	}()
}

// Stop stops the batcher and wait for finish
func (b *Batcher) Stop() {
	close(b.requests)
	b.wg.Wait()
}

// RequestProcessTo requests a process towards the target id.
// It'll block if current request queue is full.
// true - if the targetID is processed within one batch after the request has been picked up by the batcher
// false - otherwise
func (b *Batcher) RequestProcessTo(targetID int, wg *sync.WaitGroup) {
	if targetID < 0 || targetID == targetAny {
		panic("invalid target index")
	}

	b.requests <- batcherReq{
		targetID: targetID,
		reqwg:    wg,
	}
}

// RequestProcess request a batch process with no target.
// It won't block if request queue is full.
func (b *Batcher) RequestProcess() {
	select {
	case b.requests <- batcherReq{targetID: targetAny}:
	default:
	}
}
