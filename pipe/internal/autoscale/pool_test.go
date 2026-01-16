package autoscale

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestPool_MinWorkers(t *testing.T) {
	cfg := Parse(2, 10, 30*time.Second, 5*time.Second, 10*time.Second, 100*time.Millisecond)

	fn := func(ctx context.Context, in int) ([]int, error) {
		return []int{in * 2}, nil
	}

	pool := NewPool(cfg, fn, func(any, error) {}, nil)

	in := make(chan int)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pool.Start(ctx, in, 0)

	// Give time for workers to spawn
	time.Sleep(50 * time.Millisecond)

	if pool.TotalWorkers() != 2 {
		t.Errorf("expected 2 workers, got %d", pool.TotalWorkers())
	}

	close(in)
}

func TestPool_MaxWorkers(t *testing.T) {
	// Use very short intervals for faster test
	cfg := Parse(1, 3, 30*time.Second, 10*time.Millisecond, 10*time.Millisecond, 10*time.Millisecond)

	var processing atomic.Int32

	fn := func(ctx context.Context, in int) ([]int, error) {
		processing.Add(1)
		time.Sleep(200 * time.Millisecond) // Simulate slow processing
		processing.Add(-1)
		return []int{in * 2}, nil
	}

	pool := NewPool(cfg, fn, func(any, error) {}, nil)

	in := make(chan int, 10)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pool.Start(ctx, in, 10)

	// Send many items to trigger scale-up
	for i := range 10 {
		in <- i
	}

	// Wait for scaling to happen
	time.Sleep(150 * time.Millisecond)

	// Should have scaled up to max (3)
	total := pool.TotalWorkers()
	if total > 3 {
		t.Errorf("expected at most 3 workers, got %d", total)
	}

	close(in)
}

func TestPool_ScaleUp(t *testing.T) {
	// Short cooldowns for faster test
	cfg := Parse(1, 5, 30*time.Second, 20*time.Millisecond, 10*time.Second, 10*time.Millisecond)

	var mu sync.Mutex
	blocked := make(chan struct{})

	fn := func(ctx context.Context, in int) ([]int, error) {
		mu.Lock()
		mu.Unlock()
		<-blocked
		return []int{in * 2}, nil
	}

	pool := NewPool(cfg, fn, func(any, error) {}, nil)

	in := make(chan int, 10)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pool.Start(ctx, in, 10)

	// Block the mutex so workers can't proceed past the lock
	mu.Lock()

	// Send items - workers will block
	for i := range 5 {
		in <- i
	}

	// Wait for initial workers to start processing (and block on the mutex)
	time.Sleep(30 * time.Millisecond)

	// All workers should be active (blocked)
	// Scaler should detect all workers busy and scale up

	// Wait for scaler cycles to run
	time.Sleep(100 * time.Millisecond)

	total := pool.TotalWorkers()
	if total < 2 {
		t.Errorf("expected workers to scale up from 1, got %d", total)
	}

	// Cleanup
	mu.Unlock()
	close(blocked)
	close(in)
}

func TestPool_ScaleDown(t *testing.T) {
	// Very short scale-down time for test
	cfg := Parse(1, 5, 50*time.Millisecond, 10*time.Millisecond, 10*time.Millisecond, 10*time.Millisecond)

	fn := func(ctx context.Context, in int) ([]int, error) {
		return []int{in * 2}, nil
	}

	pool := NewPool(cfg, fn, func(any, error) {}, nil)

	in := make(chan int, 100)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pool.Start(ctx, in, 100)

	// Send burst of items to trigger scale-up
	for i := range 50 {
		in <- i
	}

	// Wait for scaling up and processing
	time.Sleep(150 * time.Millisecond)

	totalAfterBurst := pool.TotalWorkers()

	// Now wait with no new items - should scale down
	time.Sleep(200 * time.Millisecond)

	totalAfterIdle := pool.TotalWorkers()

	if totalAfterIdle >= totalAfterBurst && totalAfterBurst > 1 {
		t.Errorf("expected workers to scale down after idle, got before=%d after=%d",
			totalAfterBurst, totalAfterIdle)
	}

	// Should not go below min
	if totalAfterIdle < 1 {
		t.Errorf("workers scaled below minimum: %d", totalAfterIdle)
	}

	close(in)
}

func TestPool_ProcessesAllItems(t *testing.T) {
	cfg := Parse(2, 8, 30*time.Second, 50*time.Millisecond, 100*time.Millisecond, 20*time.Millisecond)

	fn := func(ctx context.Context, in int) ([]int, error) {
		return []int{in * 2}, nil
	}

	pool := NewPool(cfg, fn, func(any, error) {}, nil)

	in := make(chan int)
	ctx := context.Background()

	out := pool.Start(ctx, in, 100)

	// Send items
	go func() {
		for i := range 100 {
			in <- i
		}
		close(in)
	}()

	// Collect results
	var results []int
	for v := range out {
		results = append(results, v)
	}

	if len(results) != 100 {
		t.Errorf("expected 100 results, got %d", len(results))
	}
}

func TestPool_ErrorHandler(t *testing.T) {
	cfg := Parse(1, 1, 30*time.Second, 5*time.Second, 10*time.Second, 100*time.Millisecond)

	var mu sync.Mutex
	var errors []int

	fn := func(ctx context.Context, in int) ([]int, error) {
		if in < 0 {
			return nil, context.DeadlineExceeded
		}
		return []int{in * 2}, nil
	}

	errorHandler := func(val any, err error) {
		mu.Lock()
		errors = append(errors, val.(int))
		mu.Unlock()
	}

	pool := NewPool(cfg, fn, errorHandler, nil)

	in := make(chan int, 5)
	ctx := context.Background()

	out := pool.Start(ctx, in, 10)

	in <- 1
	in <- -1 // Will error
	in <- 2
	in <- -2 // Will error
	in <- 3
	close(in)

	var results []int
	for v := range out {
		results = append(results, v)
	}

	if len(results) != 3 {
		t.Errorf("expected 3 results, got %d", len(results))
	}

	mu.Lock()
	if len(errors) != 2 {
		t.Errorf("expected 2 errors, got %d", len(errors))
	}
	mu.Unlock()
}

func TestConfig_Parse_Defaults(t *testing.T) {
	cfg := Parse(0, 0, 0, 0, 0, 0)

	if cfg.MinWorkers != DefaultMinWorkers {
		t.Errorf("expected MinWorkers=%d, got %d", DefaultMinWorkers, cfg.MinWorkers)
	}
	if cfg.MaxWorkers <= 0 {
		t.Errorf("expected MaxWorkers > 0, got %d", cfg.MaxWorkers)
	}
	if cfg.ScaleDownAfter != DefaultScaleDownAfter {
		t.Errorf("expected ScaleDownAfter=%v, got %v", DefaultScaleDownAfter, cfg.ScaleDownAfter)
	}
	if cfg.ScaleUpCooldown != DefaultScaleUpCooldown {
		t.Errorf("expected ScaleUpCooldown=%v, got %v", DefaultScaleUpCooldown, cfg.ScaleUpCooldown)
	}
	if cfg.ScaleDownCooldown != DefaultScaleDownCooldown {
		t.Errorf("expected ScaleDownCooldown=%v, got %v", DefaultScaleDownCooldown, cfg.ScaleDownCooldown)
	}
	if cfg.CheckInterval != DefaultCheckInterval {
		t.Errorf("expected CheckInterval=%v, got %v", DefaultCheckInterval, cfg.CheckInterval)
	}
}

func TestConfig_Parse_MinGreaterThanMax(t *testing.T) {
	cfg := Parse(10, 5, 0, 0, 0, 0)

	if cfg.MinWorkers > cfg.MaxWorkers {
		t.Errorf("MinWorkers (%d) should not exceed MaxWorkers (%d)", cfg.MinWorkers, cfg.MaxWorkers)
	}
}
