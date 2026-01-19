package message

import (
	"sync"
	"sync/atomic"
)

// messageTracker tracks in-flight messages for graceful shutdown.
// enter() increments count, exit() decrements. drained() closes when
// count reaches zero AND close() has been called.
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

func (t *messageTracker) enter() {
	t.count.Add(1)
}

func (t *messageTracker) exit() {
	if t.count.Add(-1) == 0 && t.closed.Load() {
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
