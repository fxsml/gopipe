package gopipeline_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	. "github.com/fxsml/gopipeline"
)

func TestProcess_Basic(t *testing.T) {
	ctx := context.Background()
	in := make(chan int, 3)
	in <- 1
	in <- 2
	in <- 3
	close(in)

	handler := func(ctx context.Context, v int) (int, error) {
		return v * 2, nil
	}

	out := Process(ctx, in, handler)
	results := []int{}
	for v := range out {
		results = append(results, v)
	}

	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
	for i, v := range results {
		exp := (i + 1) * 2
		if v != exp {
			t.Errorf("expected %d, got %d", exp, v)
		}
	}
}

func TestProcess_ErrorHandler(t *testing.T) {
	ctx := context.Background()
	in := make(chan int, 2)
	in <- 1
	in <- 2
	close(in)

	errMu := sync.Mutex{}
	errs := []error{}
	errHandler := func(val any, err error) {
		errMu.Lock()
		defer errMu.Unlock()
		errs = append(errs, err)
	}

	handler := func(ctx context.Context, v int) (int, error) {
		if v == 2 {
			return 0, errors.New("fail")
		}
		return v, nil
	}

	out := Process(ctx, in, handler, WithErrorHandler(errHandler))
	results := []int{}
	for v := range out {
		results = append(results, v)
	}

	if len(results) != 1 || results[0] != 1 {
		t.Errorf("expected only 1 result (1), got %v", results)
	}
	if len(errs) != 1 {
		t.Errorf("expected 1 error, got %d", len(errs))
	}
}

func TestProcess_Concurrency(t *testing.T) {
	ctx := context.Background()
	count := 10
	in := make(chan int, count)
	for i := 0; i < count; i++ {
		in <- i
	}
	close(in)

	concurrency := 5
	sleep := 50 * time.Millisecond
	handler := func(ctx context.Context, v int) (int, error) {
		time.Sleep(sleep)
		return v, nil
	}

	start := time.Now()
	out := Process(ctx, in, handler, WithConcurrency(concurrency))
	results := []int{}
	for v := range out {
		results = append(results, v)
	}
	dur := time.Since(start)

	if len(results) != count {
		t.Errorf("expected 10 results, got %d", len(results))
	}

	sequential := sleep * time.Duration(count)
	concurrent := sleep * time.Duration((10+concurrency-1)/concurrency)
	if dur >= sequential {
		t.Errorf("expected concurrent processing, but took %v (sequential would be %v)", dur, sequential)
	}
	if dur > concurrent+sleep {
		t.Errorf("expected duration to be close to %v, got %v", concurrent, dur)
	}
}

func TestProcess_Timeout(t *testing.T) {
	ctx := context.Background()
	in := make(chan int, 1)
	in <- 1
	close(in)

	var gotTimeout error
	handler := func(ctx context.Context, v int) (int, error) {
		select {
		case <-ctx.Done():
			gotTimeout = ctx.Err()
			return 0, ctx.Err()
		case <-time.After(50 * time.Millisecond):
			return v, nil
		}
	}

	out := Process(ctx, in, handler, WithProcessTimeout(10*time.Millisecond))
	select {
	case _, ok := <-out:
		if ok {
			t.Error("expected no output due to timeout")
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("timeout waiting for output")
	}

	if gotTimeout == nil {
		t.Error("expected process func to receive context cancellation error")
	} else if gotTimeout != context.DeadlineExceeded {
		t.Errorf("expected context.DeadlineExceeded, got %v", gotTimeout)
	}
}
