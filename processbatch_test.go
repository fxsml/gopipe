package gopipeline_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	. "github.com/fxsml/gopipeline"
)

func TestProcessBatch_Basic(t *testing.T) {
	ctx := context.Background()
	in := make(chan int, 6)
	for i := 0; i < 6; i++ {
		in <- i
	}
	close(in)

	batchSize := 3
	handler := func(ctx context.Context, batch []int) ([]BatchRes[int], error) {
		results := make([]BatchRes[int], len(batch))
		for i, v := range batch {
			results[i] = NewBatchRes[int](v*10, nil)
		}
		return results, nil
	}

	out := ProcessBatch(ctx, in, handler, batchSize, 100*time.Millisecond)
	results := []int{}
	for v := range out {
		results = append(results, v)
	}

	expected := []int{0, 10, 20, 30, 40, 50}
	if len(results) != len(expected) {
		t.Fatalf("expected %d results, got %d", len(expected), len(results))
	}
	for i, v := range results {
		if v != expected[i] {
			t.Errorf("at %d: expected %d, got %d", i, expected[i], v)
		}
	}
}

func TestProcessBatch_Timeout(t *testing.T) {
	ctx := context.Background()
	in := make(chan int, 2)
	in <- 1
	in <- 2
	close(in)

	handler := func(ctx context.Context, batch []int) ([]BatchRes[int], error) {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(50 * time.Millisecond):
			results := make([]BatchRes[int], len(batch))
			for i, v := range batch {
				results[i] = NewBatchRes[int](v*10, nil)
			}
			return results, nil
		}
	}

	out := ProcessBatch(ctx, in, handler, 2, 10*time.Millisecond, WithProcessTimeout(30*time.Millisecond), WithErrorHandler(func(val any, err error) {
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("expected context.DeadlineExceeded, got %v", err)
		}
	}))
	results := []int{}
	for v := range out {
		results = append(results, v)
	}
	if len(results) != 0 {
		t.Errorf("expected no results due to timeout, got %v", results)
	}
}

func TestProcessBatch_Concurrency(t *testing.T) {
	ctx := context.Background()
	count := 10
	in := make(chan int, count)
	for i := 0; i < count; i++ {
		in <- i
	}
	close(in)

	concurrency := 5
	batchSize := 2
	sleep := 50 * time.Millisecond
	handler := func(ctx context.Context, batch []int) ([]BatchRes[int], error) {
		time.Sleep(sleep)
		results := make([]BatchRes[int], len(batch))
		for i, v := range batch {
			results[i] = NewBatchRes[int](v, nil)
		}
		return results, nil
	}

	start := time.Now()
	out := ProcessBatch(ctx, in, handler, batchSize, 100*time.Millisecond, WithConcurrency(concurrency))
	results := []int{}
	for v := range out {
		results = append(results, v)
	}
	dur := time.Since(start)

	if len(results) != count {
		t.Errorf("expected %d results, got %d", count, len(results))
	}

	sequential := sleep * time.Duration(count/batchSize)
	concurrentDur := sleep * time.Duration((count/batchSize+concurrency-1)/concurrency)
	if dur >= sequential {
		t.Errorf("expected concurrent batch processing, but took %v (sequential would be %v)", dur, sequential)
	}
	if dur > concurrentDur+sleep {
		t.Errorf("expected duration to be close to %v, got %v", concurrentDur, dur)
	}
}

func TestProcessBatch_ErrorHandler(t *testing.T) {
	ctx := context.Background()
	in := make(chan int, 3)
	in <- 1
	in <- 2
	in <- 3
	close(in)

	fail := errors.New("fail")
	errMu := sync.Mutex{}
	errs := []error{}
	errHandler := func(val any, err error) {
		errMu.Lock()
		defer errMu.Unlock()
		errs = append(errs, err)
		if val != 2 {
			t.Errorf("expected error handler to be called for value 2, got %v", val)
		}
	}

	handler := func(ctx context.Context, batch []int) ([]BatchRes[int], error) {
		results := make([]BatchRes[int], len(batch))
		for i, v := range batch {
			if v == 2 {
				results[i] = NewBatchRes[int](0, fail)
			} else {
				results[i] = NewBatchRes[int](v, nil)
			}
		}
		return results, nil
	}

	out := ProcessBatch(ctx, in, handler, 2, 100*time.Millisecond, WithErrorHandler(errHandler))
	results := []int{}
	for v := range out {
		results = append(results, v)
	}

	if len(results) != 2 {
		t.Errorf("expected 2 results, got %v", results)
	}
	if len(errs) != 1 {
		t.Errorf("expected 1 error, got %d", len(errs))
	}
	if !errors.Is(errs[0], fail) {
		t.Errorf("expected error to be %v, got %v", fail, errs[0])
	}
}

func TestProcessBatch_MaxDurationAndMaxSize(t *testing.T) {
	ctx := context.Background()
	in := make(chan int, 5)

	batchSize := 3
	maxDuration := 50 * time.Millisecond
	var batchLens []int
	var mu sync.Mutex
	handler := func(ctx context.Context, batch []int) ([]BatchRes[int], error) {
		mu.Lock()
		batchLens = append(batchLens, len(batch))
		mu.Unlock()
		results := make([]BatchRes[int], len(batch))
		for i, v := range batch {
			results[i] = NewBatchRes(v, nil)
		}
		return results, nil
	}

	out := ProcessBatch(ctx, in, handler, batchSize, maxDuration)

	for i := 0; i < 5; i++ {
		in <- i + 1 // Fill the channel with values 1 to 5
	}
	time.Sleep(100 * time.Millisecond) // Ensure we hit maxDuration
	close(in)

	results := []int{}
	for v := range out {
		results = append(results, v)
	}

	if len(results) != 5 {
		t.Errorf("expected 5 results, got %d", len(results))
	}
	// Should see one batch of 3 (maxSize), one batch of 2 (maxDuration)
	if len(batchLens) != 2 || batchLens[0] != 3 || batchLens[1] != 2 {
		t.Errorf("expected batch lens [3 2], got %v", batchLens)
	}
}

func TestProcessBatch_BatchErrorHandler(t *testing.T) {
	ctx := context.Background()
	in := make(chan int, 4)
	for i := 0; i < 4; i++ {
		in <- i
	}
	close(in)

	batchSize := 2
	fail := errors.New("batch fail")
	var handledBatches [][]int
	var handledErrs []error
	var mu sync.Mutex
	errHandler := func(val any, err error) {
		mu.Lock()
		defer mu.Unlock()
		batch, ok := val.([]int)
		if !ok {
			t.Errorf("expected batch to be passed to error handler, got %T", val)
		}
		handledBatches = append(handledBatches, batch)
		handledErrs = append(handledErrs, err)
	}

	handler := func(ctx context.Context, batch []int) ([]BatchRes[int], error) {
		return nil, fail
	}

	out := ProcessBatch(ctx, in, handler, batchSize, 100*time.Millisecond, WithErrorHandler(errHandler))
	for range out {
		// no output expected
	}

	if len(handledBatches) != 2 {
		t.Errorf("expected 2 error handler calls, got %d", len(handledBatches))
	}
	for i, batch := range handledBatches {
		if len(batch) != batchSize {
			t.Errorf("expected batch of size %d, got %d", batchSize, len(batch))
			if batch[i*2] != i*2 {
				t.Errorf("expected batch[%d] to be %d, got %d", i*2, i*2, batch[i*2])
			}
			if batch[i*2+1] != i*2+1 {
				t.Errorf("expected batch[%d] to be %d, got %d", i*2+1, i*2+1, batch[i*2+1])
			}
		}
		if !errors.Is(handledErrs[i], fail) {
			t.Errorf("expected error to be %v, got %v", fail, handledErrs[i])
		}
	}
}
