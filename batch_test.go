package gopipe

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func TestBatch_Success(t *testing.T) {
	ctx := context.Background()
	in := make(chan int, 5)

	// Send test data
	for i := 1; i <= 5; i++ {
		in <- i
	}
	close(in)

	// Batch processor that doubles each number in the batch
	batchProcessor := func(ctx context.Context, batch []int) ([]int, error) {
		result := make([]int, len(batch))
		for i, v := range batch {
			result[i] = v * 2
		}
		return result, nil
	}

	var cancelCalled bool
	cancel := func(batch []int, err error) {
		cancelCalled = true
	}

	// Process in batches of 3
	out := Batch(ctx, in, batchProcessor, cancel, 3, time.Millisecond*100)

	// Collect results
	var results []int
	for result := range out {
		results = append(results, result)
	}

	// Should have all 5 results, doubled
	if len(results) != 5 {
		t.Errorf("Expected 5 results, got %d", len(results))
	}
	if cancelCalled {
		t.Error("Cancel should not have been called")
	}
}

func TestBatch_Failure(t *testing.T) {
	ctx := context.Background()
	in := make(chan int, 5)

	// Send test data
	for i := 1; i <= 5; i++ {
		in <- i
	}
	close(in)

	// Batch processor that fails on batches containing number 3
	batchProcessor := func(ctx context.Context, batch []int) ([]int, error) {
		for _, v := range batch {
			if v == 3 {
				return nil, fmt.Errorf("batch contains forbidden number 3")
			}
		}
		result := make([]int, len(batch))
		for i, v := range batch {
			result[i] = v * 2
		}
		return result, nil
	}

	var cancelCalled bool
	cancel := func(batch []int, err error) {
		cancelCalled = true
	}

	// Process in batches of 2
	out := Batch(ctx, in, batchProcessor, cancel, 2, time.Millisecond*100)

	// Collect results
	var results []int
	for result := range out {
		results = append(results, result)
	}

	// Should have some results from successful batches
	if len(results) == 0 {
		t.Error("Expected some successful results")
	}
	if !cancelCalled {
		t.Error("Cancel should have been called for failed batch")
	}
}
