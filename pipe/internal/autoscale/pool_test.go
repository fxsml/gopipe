package autoscale

import (
	"context"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestPool_MinWorkers(t *testing.T) {
	cfg := PoolConfig{
		MinWorkers:        2,
		MaxWorkers:        10,
		ScaleDownAfter:    30 * time.Second,
		ScaleUpCooldown:   5 * time.Second,
		ScaleDownCooldown: 10 * time.Second,
		CheckInterval:     100 * time.Millisecond,
	}

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
	cfg := PoolConfig{
		MinWorkers:        1,
		MaxWorkers:        3,
		ScaleDownAfter:    30 * time.Second,
		ScaleUpCooldown:   10 * time.Millisecond,
		ScaleDownCooldown: 10 * time.Millisecond,
		CheckInterval:     10 * time.Millisecond,
	}

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
	cfg := PoolConfig{
		MinWorkers:        1,
		MaxWorkers:        5,
		ScaleDownAfter:    30 * time.Second,
		ScaleUpCooldown:   20 * time.Millisecond,
		ScaleDownCooldown: 10 * time.Second,
		CheckInterval:     10 * time.Millisecond,
	}

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
	cfg := PoolConfig{
		MinWorkers:        1,
		MaxWorkers:        5,
		ScaleDownAfter:    50 * time.Millisecond,
		ScaleUpCooldown:   10 * time.Millisecond,
		ScaleDownCooldown: 10 * time.Millisecond,
		CheckInterval:     10 * time.Millisecond,
	}

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
	cfg := PoolConfig{
		MinWorkers:        2,
		MaxWorkers:        8,
		ScaleDownAfter:    30 * time.Second,
		ScaleUpCooldown:   50 * time.Millisecond,
		ScaleDownCooldown: 100 * time.Millisecond,
		CheckInterval:     20 * time.Millisecond,
	}

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
	cfg := PoolConfig{
		MinWorkers:        1,
		MaxWorkers:        1,
		ScaleDownAfter:    30 * time.Second,
		ScaleUpCooldown:   5 * time.Second,
		ScaleDownCooldown: 10 * time.Second,
		CheckInterval:     100 * time.Millisecond,
	}

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

func TestPoolConfig_Defaults(t *testing.T) {
	// Test that defaults are applied when using zero values
	// This tests the constants, not a Parse function
	if DefaultMinWorkers != 1 {
		t.Errorf("expected DefaultMinWorkers=1, got %d", DefaultMinWorkers)
	}
	if DefaultScaleDownAfter != 30*time.Second {
		t.Errorf("expected DefaultScaleDownAfter=30s, got %v", DefaultScaleDownAfter)
	}
	if DefaultScaleUpCooldown != 5*time.Second {
		t.Errorf("expected DefaultScaleUpCooldown=5s, got %v", DefaultScaleUpCooldown)
	}
	if DefaultScaleDownCooldown != 10*time.Second {
		t.Errorf("expected DefaultScaleDownCooldown=10s, got %v", DefaultScaleDownCooldown)
	}
	if DefaultCheckInterval != 1*time.Second {
		t.Errorf("expected DefaultCheckInterval=1s, got %v", DefaultCheckInterval)
	}
}

func TestPool_NoGoroutineLeak_InputClose(t *testing.T) {
	// Count goroutines before starting
	runtime.GC()
	time.Sleep(10 * time.Millisecond)
	initialGoroutines := runtime.NumGoroutine()

	cfg := PoolConfig{
		MinWorkers:        2,
		MaxWorkers:        8,
		ScaleDownAfter:    30 * time.Second,
		ScaleUpCooldown:   50 * time.Millisecond,
		ScaleDownCooldown: 100 * time.Millisecond,
		CheckInterval:     20 * time.Millisecond,
	}

	fn := func(ctx context.Context, in int) ([]int, error) {
		return []int{in * 2}, nil
	}

	pool := NewPool(cfg, fn, func(any, error) {}, nil)

	in := make(chan int)
	ctx := context.Background()

	out := pool.Start(ctx, in, 10)

	// Send some items then close
	go func() {
		for i := range 10 {
			in <- i
		}
		close(in)
	}()

	// Drain output
	for range out {
	}

	// Allow goroutines to clean up
	runtime.GC()
	time.Sleep(50 * time.Millisecond)

	finalGoroutines := runtime.NumGoroutine()
	leaked := finalGoroutines - initialGoroutines

	if leaked > 0 {
		t.Errorf("goroutine leak detected: %d goroutine(s) leaked after input close", leaked)
	}
}

func TestPool_NoGoroutineLeak_ContextCancel(t *testing.T) {
	// Count goroutines before starting
	runtime.GC()
	time.Sleep(10 * time.Millisecond)
	initialGoroutines := runtime.NumGoroutine()

	cfg := PoolConfig{
		MinWorkers:        2,
		MaxWorkers:        4,
		ScaleDownAfter:    30 * time.Second,
		ScaleUpCooldown:   50 * time.Millisecond,
		ScaleDownCooldown: 100 * time.Millisecond,
		CheckInterval:     20 * time.Millisecond,
	}

	fn := func(ctx context.Context, in int) ([]int, error) {
		return []int{in * 2}, nil
	}

	pool := NewPool(cfg, fn, func(any, error) {}, nil)

	in := make(chan int)
	ctx, cancel := context.WithCancel(context.Background())

	out := pool.Start(ctx, in, 10)

	// Send some items
	go func() {
		for i := range 5 {
			select {
			case in <- i:
			case <-ctx.Done():
				return
			}
		}
	}()

	// Read a few results then cancel
	count := 0
	for range out {
		count++
		if count >= 2 {
			cancel()
			break
		}
	}

	// Close input to allow cleanup
	close(in)

	// Drain remaining output
	for range out {
	}

	// Allow goroutines to clean up
	runtime.GC()
	time.Sleep(50 * time.Millisecond)

	finalGoroutines := runtime.NumGoroutine()
	leaked := finalGoroutines - initialGoroutines

	if leaked > 0 {
		t.Errorf("goroutine leak detected: %d goroutine(s) leaked after context cancel", leaked)
	}
}

func TestPool_NoGoroutineLeak_Stop(t *testing.T) {
	// Count goroutines before starting
	runtime.GC()
	time.Sleep(10 * time.Millisecond)
	initialGoroutines := runtime.NumGoroutine()

	cfg := PoolConfig{
		MinWorkers:        2,
		MaxWorkers:        4,
		ScaleDownAfter:    30 * time.Second,
		ScaleUpCooldown:   50 * time.Millisecond,
		ScaleDownCooldown: 100 * time.Millisecond,
		CheckInterval:     20 * time.Millisecond,
	}

	fn := func(ctx context.Context, in int) ([]int, error) {
		time.Sleep(10 * time.Millisecond)
		return []int{in * 2}, nil
	}

	pool := NewPool(cfg, fn, func(any, error) {}, nil)

	in := make(chan int, 10)
	ctx := context.Background()

	out := pool.Start(ctx, in, 10)

	// Send items
	for i := range 5 {
		in <- i
	}

	// Stop the pool explicitly
	pool.Stop()

	// Drain output
	for range out {
	}

	// Close input
	close(in)

	// Allow goroutines to clean up
	runtime.GC()
	time.Sleep(50 * time.Millisecond)

	finalGoroutines := runtime.NumGoroutine()
	leaked := finalGoroutines - initialGoroutines

	if leaked > 0 {
		t.Errorf("goroutine leak detected: %d goroutine(s) leaked after Stop()", leaked)
	}
}

func TestPool_EmptyInput(t *testing.T) {
	cfg := PoolConfig{
		MinWorkers:        2,
		MaxWorkers:        4,
		ScaleDownAfter:    30 * time.Second,
		ScaleUpCooldown:   50 * time.Millisecond,
		ScaleDownCooldown: 100 * time.Millisecond,
		CheckInterval:     20 * time.Millisecond,
	}

	fn := func(ctx context.Context, in int) ([]int, error) {
		return []int{in * 2}, nil
	}

	pool := NewPool(cfg, fn, func(any, error) {}, nil)

	in := make(chan int)
	ctx := context.Background()

	out := pool.Start(ctx, in, 10)

	// Immediately close input without sending anything
	close(in)

	// Output should close without blocking
	var results []int
	for v := range out {
		results = append(results, v)
	}

	if len(results) != 0 {
		t.Errorf("expected 0 results from empty input, got %d", len(results))
	}
}

func TestPool_ContextCancellation(t *testing.T) {
	cfg := PoolConfig{
		MinWorkers:        1,
		MaxWorkers:        2,
		ScaleDownAfter:    30 * time.Second,
		ScaleUpCooldown:   50 * time.Millisecond,
		ScaleDownCooldown: 100 * time.Millisecond,
		CheckInterval:     20 * time.Millisecond,
	}

	var processed atomic.Int32
	fn := func(ctx context.Context, in int) ([]int, error) {
		select {
		case <-time.After(100 * time.Millisecond):
			processed.Add(1)
			return []int{in * 2}, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	pool := NewPool(cfg, fn, func(any, error) {}, nil)

	in := make(chan int, 10)
	ctx, cancel := context.WithCancel(context.Background())

	out := pool.Start(ctx, in, 10)

	// Send items
	for i := range 5 {
		in <- i
	}

	// Cancel after a short delay
	time.AfterFunc(50*time.Millisecond, cancel)

	// Close input to allow cleanup
	time.AfterFunc(60*time.Millisecond, func() { close(in) })

	// Drain output
	for range out {
	}

	// Should have processed fewer items due to cancellation
	if processed.Load() >= 5 {
		t.Errorf("expected fewer than 5 items processed due to cancellation, got %d", processed.Load())
	}
}

func TestPool_ScaleUpCooldown(t *testing.T) {
	// Set a long scale-up cooldown
	cfg := PoolConfig{
		MinWorkers:        1,
		MaxWorkers:        10,
		ScaleDownAfter:    30 * time.Second,
		ScaleUpCooldown:   200 * time.Millisecond,
		ScaleDownCooldown: 10 * time.Second,
		CheckInterval:     10 * time.Millisecond,
	}

	fn := func(ctx context.Context, in int) ([]int, error) {
		time.Sleep(50 * time.Millisecond) // Simulate work
		return []int{in * 2}, nil
	}

	pool := NewPool(cfg, fn, func(any, error) {}, nil)

	in := make(chan int, 20)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pool.Start(ctx, in, 20)

	// Send burst to trigger scale-up attempts
	for i := range 10 {
		in <- i
	}

	// Wait less than cooldown period
	time.Sleep(100 * time.Millisecond)

	// Should only have scaled up once due to cooldown
	workers := pool.TotalWorkers()
	if workers > 2 {
		t.Errorf("expected at most 2 workers due to cooldown, got %d", workers)
	}

	close(in)
}

func TestPool_MultipleOutputsPerInput(t *testing.T) {
	cfg := PoolConfig{
		MinWorkers:        2,
		MaxWorkers:        4,
		ScaleDownAfter:    30 * time.Second,
		ScaleUpCooldown:   50 * time.Millisecond,
		ScaleDownCooldown: 100 * time.Millisecond,
		CheckInterval:     20 * time.Millisecond,
	}

	// Function that returns multiple outputs per input
	fn := func(ctx context.Context, in int) ([]int, error) {
		return []int{in, in * 2, in * 3}, nil
	}

	pool := NewPool(cfg, fn, func(any, error) {}, nil)

	in := make(chan int)
	ctx := context.Background()

	out := pool.Start(ctx, in, 100)

	go func() {
		for i := 1; i <= 5; i++ {
			in <- i
		}
		close(in)
	}()

	var results []int
	for v := range out {
		results = append(results, v)
	}

	// Should have 15 results (5 inputs * 3 outputs each)
	if len(results) != 15 {
		t.Errorf("expected 15 results, got %d", len(results))
	}
}
