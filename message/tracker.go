package message

import (
	"sync"
	"sync/atomic"
)

// messageTracker tracks in-flight messages for graceful shutdown.
// enter(n) increments count by n, exit(n) decrements by n. drained() closes
// when count reaches zero AND close() has been called.
//
// Thread-safe for concurrent use.
type messageTracker struct {
	count      atomic.Int64
	closed     atomic.Bool
	drainedCh  chan struct{}
	once       sync.Once
}

func newMessageTracker() *messageTracker {
	return &messageTracker{
		drainedCh: make(chan struct{}),
	}
}

func (t *messageTracker) enter(n int64) {
	t.count.Add(n)
}

func (t *messageTracker) exit(n int64) {
	if t.count.Add(-n) <= 0 && t.closed.Load() {
		t.once.Do(func() { close(t.drainedCh) })
	}
}

func (t *messageTracker) close() {
	if t.closed.CompareAndSwap(false, true) {
		if t.count.Load() == 0 {
			t.once.Do(func() { close(t.drainedCh) })
		}
	}
}

func (t *messageTracker) drained() <-chan struct{} {
	return t.drainedCh
}

func (t *messageTracker) inFlight() int64 {
	return t.count.Load()
}
