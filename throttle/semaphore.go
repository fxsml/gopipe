package throttle

import (
	"context"

	"golang.org/x/sync/semaphore"
)

// Semaphore is a simple wrapper around golang.org/x/sync/semaphore.Weighted
// for easier use in the throttle package.
type Semaphore struct {
	sem *semaphore.Weighted
}

// NewSemaphore creates a new Semaphore with the given weight.
func NewSemaphore(capacity int64) *Semaphore {
	return &Semaphore{
		sem: semaphore.NewWeighted(capacity),
	}
}

// Acquire tries to acquire a single resource from the semaphore, blocking until available or context is done.
func (s *Semaphore) Acquire(ctx context.Context) error {
	return s.AcquireN(ctx, 1)
}

// AcquireN tries to acquire n resources from the semaphore, blocking until available or context is done.
func (s *Semaphore) AcquireN(ctx context.Context, n int64) error {
	return s.sem.Acquire(ctx, n)
}

// Release releases a single resource back to the semaphore.
func (s *Semaphore) Release() {
	s.ReleaseN(1)
}

// Release releases n resources back to the semaphore.
func (s *Semaphore) ReleaseN(n int64) {
	s.sem.Release(n)
}
