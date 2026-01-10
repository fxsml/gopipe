package test

import (
	"testing"
	"time"
)

type BatchFunc[In, Out any] func(
	in <-chan In,
	handle func([]In) []Out,
	maxSize int,
	maxDuration time.Duration,
) <-chan Out

func RunBatch_Success(t *testing.T, f BatchFunc[int, int]) {
	t.Run("batch success", func(t *testing.T) {
		in := make(chan int, 5)

		// Send test data
		for i := 1; i <= 5; i++ {
			in <- i
		}
		close(in)

		// Batch handler that doubles each number in the batch
		handle := func(batch []int) []int {
			result := make([]int, len(batch))
			for i, v := range batch {
				result[i] = v * 2
			}
			return result
		}

		// Process in batches of 3 using the function from the map
		out := f(in, handle, 3, time.Millisecond*100)

		// Collect results
		var results []int
		for result := range out {
			results = append(results, result)
		}

		// Should have all 5 results, doubled
		if len(results) != 5 {
			t.Errorf("Expected 5 results, got %d", len(results))
		}

		// Verify that each result is doubled
		expectedValues := map[int]bool{2: true, 4: true, 6: true, 8: true, 10: true}
		for _, val := range results {
			if !expectedValues[val] {
				t.Errorf("Unexpected result value: %d", val)
			}
			delete(expectedValues, val) // Remove to catch duplicates
		}

		// Make sure we got all expected values
		if len(expectedValues) != 0 {
			t.Errorf("Missing expected values: %v", expectedValues)
		}
	})
}
