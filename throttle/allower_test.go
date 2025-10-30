package throttle

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestLeakyBucket_AllowN_Basic(t *testing.T) {
	bucket := NewLeakyBucketAllower(2, 4) // 2 tokens/sec, capacity 4
	ctx := context.Background()

	// Should allow up to capacity immediately
	for range 4 {
		if err := bucket.Allow(ctx, 1); err != nil {
			t.Fatalf("unexpected error on initial AllowN: %v", err)
		}
	}

	// Next call should block until a token is available (simulate with timeout context)
	ctxTimeout, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()
	if err := bucket.Allow(ctxTimeout, 1); err == nil {
		t.Errorf("expected context deadline exceeded for weighted call")
	} else if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("expected context deadline exceeded, got: %v", err)
	}
}

func TestLeakyBucket_AllowN_Weight(t *testing.T) {
	bucket := NewLeakyBucketAllower(10, 10) // 10 tokens/sec, capacity 10
	ctx := context.Background()

	// Use up all tokens with a single weighted call
	if err := bucket.Allow(ctx, 10); err != nil {
		t.Fatalf("unexpected error on weighted AllowN: %v", err)
	}
	// Should block for next weighted call
	ctxTimeout, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()
	if err := bucket.Allow(ctxTimeout, 5); err == nil {
		t.Errorf("expected context deadline exceeded for weighted call")
	} else if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("expected context deadline exceeded, got: %v", err)
	}
}

func TestLeakyBucket_AllowN_Refill(t *testing.T) {
	bucket := NewLeakyBucketAllower(5, 5) // 5 tokens/sec, capacity 5
	ctx := context.Background()
	_ = bucket.Allow(ctx, 5) // drain
	// Wait for refill
	time.Sleep(250 * time.Millisecond)
	start := time.Now()
	if err := bucket.Allow(ctx, 1); err != nil {
		t.Errorf("expected token to be refilled, got error: %v", err)
	}
	elapsed := time.Since(start)
	if elapsed > 10*time.Millisecond {
		t.Errorf("expected AllowN to return quickly, took: %v", elapsed)
	}
}
