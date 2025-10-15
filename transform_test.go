package gopipe

import (
	"context"
	"strconv"
	"testing"
)

func TestTransform_Success(t *testing.T) {
	// Define a function type for both Transform implementations
	type transformFunc[In, Out any] func(
		in <-chan In,
		handle func(In) Out,
	) <-chan Out

	// Map of transform functions to test
	transformFuncs := map[string]transformFunc[int, string]{
		"Transform": Transform[int, string],
		"TransformPipe": func(in <-chan int, handle func(int) string) <-chan string {
			// Adapter to use NewTransformPipe with the same signature
			handlePipe := func(_ context.Context, val int) (string, error) {
				return handle(val), nil
			}
			return NewTransformPipe(
				handlePipe,
			).Start(context.Background(), in)
		},
	}

	// Run the test for each implementation
	for name, transformFunc := range transformFuncs {
		t.Run(name, func(t *testing.T) {
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
			out := transformFunc(in, handle)

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
}
