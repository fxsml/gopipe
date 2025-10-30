package throttle

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// Allower is an interface for a rate limiter that allows a certain number of tokens to be consumed.
type Allower interface {
	// Allow blocks until n tokens are available or ctx is done.
	Allow(ctx context.Context, n int64) error
}

type leakyBucketAllower struct {
	rate     float64 // tokens per second
	capacity int64   // bucket size
	tokens   float64
	last     time.Time
	mu       sync.Mutex
}

// NewLeakyBucketAllower creates a new leaky bucket rate limiter with the given refill rate and capacity.
// The rate is the number of tokens added to the bucket per second, and capacity is the maximum number of tokens the bucket can hold.
// The bucket starts full with capacity tokens.
func NewLeakyBucketAllower(rate float64, capacity int64) Allower {
	return &leakyBucketAllower{
		rate:     rate,
		capacity: capacity,
		tokens:   float64(capacity),
		last:     time.Now(),
	}
}

func (a *leakyBucketAllower) Allow(ctx context.Context, n int64) error {
	if n <= 0 {
		n = 1
	}
	if n > a.capacity {
		return fmt.Errorf("throttle: requested %d tokens, but capacity is %d", n, a.capacity)
	}
	for {
		// refill bucket
		a.mu.Lock()
		now := time.Now()
		elapsed := now.Sub(a.last).Seconds()
		a.tokens += elapsed * a.rate
		if a.tokens > float64(a.capacity) {
			a.tokens = float64(a.capacity)
		}
		a.last = now

		// check if we have enough tokens
		if a.tokens >= float64(n) {
			a.tokens -= float64(n)
			a.mu.Unlock()
			return nil
		}
		a.mu.Unlock()

		// wait and retry with a small sleep to avoid busy waiting
		select {
		case <-ctx.Done():
			return fmt.Errorf("throttle: %w", ctx.Err())
		case <-time.After(10 * time.Millisecond):
		}
	}
}

// noopAllower is a no-op implementation of the Allower interface.
type noopAllower struct{}

// NewNoopAllower returns a no-op Allower that does not limit any operations.
func NewNoopAllower() Allower {
	return noopAllower{}
}

func (a noopAllower) Allow(ctx context.Context, n int64) error {
	select {
	case <-ctx.Done():
		return fmt.Errorf("throttle: %w", ctx.Err())
	default:
		return nil
	}
}

// TODO: implement FifoAllower
