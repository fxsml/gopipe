package test

import (
	"strconv"
	"testing"
)

type TransformFunc[In, Out any] func(
	in <-chan In,
	handle func(In) Out,
) <-chan Out

func RunTransform_Success(t *testing.T, f TransformFunc[int, string]) {
	t.Run("transform success", func(t *testing.T) {
		// Create input channel
		in := make(chan int, 5)

		// Send test data
		for i := 1; i <= 5; i++ {
			in <- i
		}
		close(in)

		// Transform function that converts integers to strings
		handle := func(val int) string {
			return "Number: " + strconv.Itoa(val)
		}

		// Process values using the function from the map
		out := f(in, handle)

		// Collect results
		var results []string
		for result := range out {
			results = append(results, result)
		}

		// Should have all 5 results
		if len(results) != 5 {
			t.Errorf("Expected 5 results, got %d", len(results))
		}

		// Verify that each result is correctly transformed
		expected := []string{
			"Number: 1",
			"Number: 2",
			"Number: 3",
			"Number: 4",
			"Number: 5",
		}

		// Check if all expected values are in results
		for i, val := range expected {
			if results[i] != val {
				t.Errorf("Expected %q at position %d, got %q", val, i, results[i])
			}
		}
	})
}
