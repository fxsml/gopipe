package gopipe

import (
	"context"
	"strconv"
	"testing"
)

func TestProcess_Success(t *testing.T) {
	// Define a function type for both Process implementations
	type processFunc[In, Out any] func(
		in <-chan In,
		handle func(In) []Out,
	) <-chan Out

	// Map of process functions to test
	processFuncs := map[string]processFunc[int, string]{
		"Process": Process[int, string],
		"ProcessPipe": func(in <-chan int, handle func(int) []string) <-chan string {
			// Adapter to use NewProcessPipe with the same signature
			handlePipe := func(_ context.Context, val int) ([]string, error) {
				return handle(val), nil
			}
			return NewProcessPipe(
				handlePipe,
			).Start(context.Background(), in)
		},
	}

	// Run the test for each implementation
	for name, procFunc := range processFuncs {
		t.Run(name, func(t *testing.T) {
			// Create input channel
			in := make(chan int, 5)

			// Send test data
			for i := 1; i <= 5; i++ {
				in <- i
			}
			close(in)

			// Process function that converts integers to strings
			// Each integer produces multiple outputs: the integer itself and its square
			handle := func(val int) []string {
				if val == 3 {
					// Test empty result for specific value
					return []string{}
				}
				if val == 4 {
					// Test nil result for specific value
					return nil
				}
				return []string{
					"Value: " + strconv.Itoa(val),
					"Square: " + strconv.Itoa(val*val),
				}
			}

			// Process values using the function from the map
			out := procFunc(in, handle)

			// Collect results
			var results []string
			for result := range out {
				results = append(results, result)
			}

			// Expected results: val 1, 2, 5 produce 2 outputs each, 3 and 4 produce none
			expectedCount := 6
			if len(results) != expectedCount {
				t.Errorf("Expected %d results, got %d", expectedCount, len(results))
			}

			// Verify that the results contain the expected values
			expectedValues := map[string]bool{
				"Value: 1": true, "Square: 1": true,
				"Value: 2": true, "Square: 4": true,
				"Value: 5": true, "Square: 25": true,
			}

			for _, val := range results {
				if !expectedValues[val] {
					t.Errorf("Unexpected result value: %q", val)
				}
				delete(expectedValues, val) // Remove to catch duplicates
			}

			// Make sure we got all expected values
			if len(expectedValues) != 0 {
				t.Errorf("Missing expected values: %v", expectedValues)
			}
		})
	}
}
