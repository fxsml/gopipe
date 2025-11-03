package gopipe

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestUseMetrics_Basic(t *testing.T) {
	var got []*Metrics
	collector := func(m *Metrics) {
		got = append(got, m)
	}

	proc := NewProcessor(
		func(ctx context.Context, in int) ([]int, error) {
			time.Sleep(10 * time.Millisecond)
			return []int{in * 2}, nil
		},
		nil,
	)
	mw := useMetrics[int, int](collector)
	procWithMetrics := mw(proc)

	_, _ = procWithMetrics.Process(context.Background(), 5)

	if len(got) != 1 {
		t.Fatalf("expected 1 metrics, got %d", len(got))
	}
	if got[0].Output != 1 || got[0].Input != 1 {
		t.Errorf("unexpected metrics: %+v", got[0])
	}
	if got[0].Duration < 10*time.Millisecond {
		t.Errorf("expected duration >= 10ms, got %v", got[0].Duration)
	}
}

func TestUseMetrics_WithStartProcessor(t *testing.T) {
	var got []*Metrics
	collector := func(m *Metrics) {
		got = append(got, m)
	}

	proc := NewProcessor(
		func(ctx context.Context, in int) ([]int, error) {
			return []int{in + 1}, nil
		},
		nil,
	)
	mw := useMetrics[int, int](collector)
	procWithMetrics := mw(proc)

	in := make(chan int, 3)
	in <- 1
	in <- 2
	in <- 3
	close(in)

	out := StartProcessor(context.Background(), in, procWithMetrics)
	for range out {
		// drain output
	}

	if len(got) != 3 {
		t.Fatalf("expected 3 metrics, got %d", len(got))
	}
	for i, m := range got {
		if m.Output != 1 || m.Input != 1 {
			t.Errorf("metrics[%d] unexpected: %+v", i, m)
		}
	}
}

func TestNewMetricsDistributor(t *testing.T) {
	var got1, got2 []*Metrics

	collector1 := func(m *Metrics) { got1 = append(got1, m) }
	collector2 := func(m *Metrics) { got2 = append(got2, m) }

	distributor := newMetricsDistributor(collector1, collector2)

	// Send some metrics
	for i := range 3 {
		distributor(&Metrics{Input: 1, Output: i})
	}

	// Both collectors should have received all metrics
	if len(got1) != 3 || len(got2) != 3 {
		t.Errorf("expected 3 metrics in each collector, got %d and %d", len(got1), len(got2))
	}
	for i := range 3 {
		if got1[i].Output != i || got2[i].Output != i {
			t.Errorf("unexpected OutputCount at index %d: got1=%d, got2=%d", i, got1[i].Output, got2[i].Output)
		}
	}
}

func TestUseMetrics_WithRetry(t *testing.T) {
	var got []*Metrics
	collector := func(m *Metrics) {
		got = append(got, m)
	}

	attempts := 0
	processFunc := func(ctx context.Context, in int) ([]int, error) {
		attempts++
		if attempts < 3 {
			return nil, errors.New("temporary error")
		}
		return []int{in * 2}, nil
	}

	// Create a pipe with retry and metrics
	pipe := NewProcessPipe(
		processFunc,
		WithRetryConfig[int, int](&RetryConfig{
			ShouldRetry: ShouldRetry(),
			Backoff:     ConstantBackoff(1*time.Millisecond, 0.0),
			MaxAttempts: 5,
		}),
		WithMetricsCollector[int, int](collector),
	)

	// Create input channel
	in := make(chan int, 1)
	in <- 5
	close(in)

	// Start the pipe
	out := pipe.Start(context.Background(), in)

	// Collect results
	var results []int
	for val := range out {
		results = append(results, val)
	}

	// Should succeed after retries
	if len(results) != 1 || results[0] != 10 {
		t.Errorf("expected result [10], got %v", results)
	}

	// Should have collected metrics for each retry attempt (3 total: 2 failures + 1 success)
	if len(got) != 3 {
		t.Fatalf("expected 3 metrics collections (one per attempt), got %d", len(got))
	}

	// Check the final successful attempt
	finalMetrics := got[2]
	if finalMetrics.Error != nil {
		t.Errorf("expected no error in final metrics, got %v", finalMetrics.Error)
	}
	if finalMetrics.Input != 1 || finalMetrics.Output != 1 {
		t.Errorf("unexpected counts: input=%d, output=%d", finalMetrics.Input, finalMetrics.Output)
	}

	// Verify RetryState is captured in final attempt
	if finalMetrics.RetryState == nil {
		t.Fatal("expected RetryState in final metrics, got nil")
	}
	if finalMetrics.RetryState.Attempts != 3 {
		t.Errorf("expected 3 retry attempts (including initial), got %d", finalMetrics.RetryState.Attempts)
	}
	if finalMetrics.RetryState.MaxAttempts != 5 {
		t.Errorf("expected MaxAttempts=5, got %d", finalMetrics.RetryState.MaxAttempts)
	}
	if len(finalMetrics.RetryState.Causes) != 2 {
		t.Errorf("expected 2 retry causes, got %d", len(finalMetrics.RetryState.Causes))
	}

	// Verify that earlier attempts also have RetryState
	for i, metrics := range got[:2] {
		if metrics.RetryState == nil {
			t.Errorf("expected RetryState in attempt %d metrics, got nil", i+1)
		}
		if metrics.Error == nil {
			t.Errorf("expected error in attempt %d metrics, got nil", i+1)
		}
	}

	// Test failure case - max attempts reached
	got = nil // Reset metrics collection
	attempts = 0

	failingProcessFunc := func(ctx context.Context, in int) ([]int, error) {
		attempts++
		return nil, errors.New("persistent error")
	}

	// Create a pipe that will fail after max attempts
	failPipe := NewProcessPipe(
		failingProcessFunc,
		WithRetryConfig[int, int](&RetryConfig{
			ShouldRetry: ShouldRetry(),
			Backoff:     ConstantBackoff(1*time.Millisecond, 0.0),
			MaxAttempts: 2,
		}),
		WithMetricsCollector[int, int](collector),
	)

	// Create input channel
	failIn := make(chan int, 1)
	failIn <- 5
	close(failIn)

	// Start the pipe
	failOut := failPipe.Start(context.Background(), failIn)

	// Collect results (should be empty since all attempts fail)
	var failResults []int
	for val := range failOut {
		failResults = append(failResults, val)
	}

	// Should have no results due to failure
	if len(failResults) != 0 {
		t.Errorf("expected no results due to failure, got %v", failResults)
	}

	// Should have collected metrics for each failed attempt plus final failure (3 total: 2 retries + 1 final)
	if len(got) != 3 {
		t.Fatalf("expected 3 metrics collections for failure case, got %d", len(got))
	}

	// Check the final failed attempt
	finalFailMetrics := got[2]
	if finalFailMetrics.Error == nil {
		t.Error("expected error in final failure metrics, got nil")
	}

	// Verify RetryState is captured for failure
	if finalFailMetrics.RetryState == nil {
		t.Fatal("expected RetryState in failure metrics, got nil")
	}
	if finalFailMetrics.RetryState.Attempts != 2 {
		t.Errorf("expected 2 retry attempts in failure, got %d", finalFailMetrics.RetryState.Attempts)
	}
	if finalFailMetrics.RetryState.Err == nil {
		t.Error("expected RetryState.Err in failure metrics, got nil")
	}
}
