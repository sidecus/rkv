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
	processFn func() int
	requests  chan batcherReq
	wg        sync.WaitGroup
}

// NewBatcher creates a new batcher
func NewBatcher(process func() int, queuesize int) *Batcher {
	return &Batcher{
		processFn: process,
		requests:  make(chan batcherReq, queuesize),
	}
}

// Start starts the batcher service
func (b *Batcher) Start() {
	b.wg.Add(1)
	go func() {
		lastProcessed := -1
		for r := range b.requests {
			if r.targetID > lastProcessed {
				// Try to process only if we haven't processed the current target yet
				lastProcessed = b.processFn()
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

// TriggerProcessTo triggers a process towards the target id.
// Batcher signals the caller via the processed channel with:
// true - if the targetID is processed within one batch after the request has been picked up by the batcher
// false - otherwise
func (b *Batcher) TriggerProcessTo(targetID int, wg *sync.WaitGroup) {
	if targetID < 0 || targetID == targetAny {
		panic("invalid target index")
	}

	b.requests <- batcherReq{
		targetID: targetID,
		reqwg:    wg,
	}
}

// TryTriggerProcess triggers a process, it won't block the caller
func (b *Batcher) TryTriggerProcess() {
	select {
	case b.requests <- batcherReq{targetID: targetAny}:
	default:
	}
}
