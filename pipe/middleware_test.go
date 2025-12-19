package pipe

import (
	"context"
	"errors"
	"testing"
)

func TestApplyMiddleware(t *testing.T) {
	// Create a test processor
	baseProcessor := NewProcessor(
		func(ctx context.Context, in string) ([]int, error) {
			return []int{len(in)}, nil
		},
		func(in string, err error) {},
	)

	// Test applying no middleware
	t.Run("NoMiddleware", func(t *testing.T) {
		proc := applyMiddleware(baseProcessor)

		// Verify it's the same processor (functionality unchanged)
		result, err := proc.Process(context.Background(), "test")

		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if len(result) != 1 || result[0] != 4 {
			t.Errorf("Expected [4], got %v", result)
		}
	})

	// Test applying a single middleware
	t.Run("SingleMiddleware", func(t *testing.T) {
		var middlewareCalled bool

		middleware := func(next Processor[string, int]) Processor[string, int] {
			return NewProcessor(
				func(ctx context.Context, in string) ([]int, error) {
					middlewareCalled = true
					// Modify the input before passing to next processor
					in = in + "!"
					return next.Process(ctx, in)
				},
				func(in string, err error) {
					next.Cancel(in, err)
				},
			)
		}

		proc := applyMiddleware(baseProcessor, middleware)

		// Process should go through our middleware
		result, err := proc.Process(context.Background(), "test")

		if !middlewareCalled {
			t.Error("Middleware was not called")
		}
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if len(result) != 1 || result[0] != 5 { // len("test!")
			t.Errorf("Expected [5], got %v", result)
		}
	})

	// Test applying multiple middleware in order
	t.Run("MultipleMiddlewareOrder", func(t *testing.T) {
		executionOrder := []string{}

		middleware1 := func(next Processor[string, int]) Processor[string, int] {
			return NewProcessor(
				func(ctx context.Context, in string) ([]int, error) {
					executionOrder = append(executionOrder, "middleware1:before")
					result, err := next.Process(ctx, in)
					executionOrder = append(executionOrder, "middleware1:after")
					return result, err
				},
				next.Cancel,
			)
		}

		middleware2 := func(next Processor[string, int]) Processor[string, int] {
			return NewProcessor(
				func(ctx context.Context, in string) ([]int, error) {
					executionOrder = append(executionOrder, "middleware2:before")
					result, err := next.Process(ctx, in)
					executionOrder = append(executionOrder, "middleware2:after")
					return result, err
				},
				next.Cancel,
			)
		}

		middleware3 := func(next Processor[string, int]) Processor[string, int] {
			return NewProcessor(
				func(ctx context.Context, in string) ([]int, error) {
					executionOrder = append(executionOrder, "middleware3:before")
					result, err := next.Process(ctx, in)
					executionOrder = append(executionOrder, "middleware3:after")
					return result, err
				},
				next.Cancel,
			)
		}

		testProcessor := NewProcessor(
			func(ctx context.Context, in string) ([]int, error) {
				executionOrder = append(executionOrder, "base:process")
				return []int{len(in)}, nil
			},
			func(in string, err error) {
				executionOrder = append(executionOrder, "base:cancel")
			},
		)

		proc := applyMiddleware(testProcessor, middleware1, middleware2, middleware3)

		// Process should go through our middleware in correct order
		_, _ = proc.Process(context.Background(), "test")

		// Verify middleware execution order
		// For middlewares 1, 2, 3, the order should be:
		// middleware1:before -> middleware2:before -> middleware3:before -> base:process ->
		// middleware3:after -> middleware2:after -> middleware1:after
		expected := []string{
			"middleware1:before",
			"middleware2:before",
			"middleware3:before",
			"base:process",
			"middleware3:after",
			"middleware2:after",
			"middleware1:after",
		}

		if len(executionOrder) != len(expected) {
			t.Fatalf("Expected %d middleware calls, got %d: %v", len(expected), len(executionOrder), executionOrder)
		}

		for i, call := range expected {
			if executionOrder[i] != call {
				t.Errorf("Expected execution order[%d] to be %q, got %q", i, call, executionOrder[i])
			}
		}
	})

	// Test that middleware can intercept errors
	t.Run("MiddlewareErrorHandling", func(t *testing.T) {
		var cancelCalled bool
		testError := errors.New("test error")

		errorProcessor := NewProcessor(
			func(ctx context.Context, in string) ([]int, error) {
				return nil, testError
			},
			func(in string, err error) {},
		)

		errorHandlingMiddleware := func(next Processor[string, int]) Processor[string, int] {
			return NewProcessor(
				func(ctx context.Context, in string) ([]int, error) {
					result, err := next.Process(ctx, in)
					if err != nil {
						// Intercept error and return empty result instead
						return []int{0}, nil
					}
					return result, nil
				},
				func(in string, err error) {
					cancelCalled = true
					next.Cancel(in, err)
				},
			)
		}

		proc := applyMiddleware(errorProcessor, errorHandlingMiddleware)

		// Process should not return an error despite the base processor returning one
		result, err := proc.Process(context.Background(), "test")

		if err != nil {
			t.Errorf("Expected error to be intercepted, got %v", err)
		}

		if len(result) != 1 || result[0] != 0 {
			t.Errorf("Expected [0] (intercepted result), got %v", result)
		}

		// Now test the cancel path
		proc.Cancel("test", testError)

		if !cancelCalled {
			t.Error("Cancel middleware was not called")
		}
	})

	// Test middleware that modifies the output
	t.Run("MiddlewareModifiesOutput", func(t *testing.T) {
		outputModifyingMiddleware := func(next Processor[string, int]) Processor[string, int] {
			return NewProcessor(
				func(ctx context.Context, in string) ([]int, error) {
					result, err := next.Process(ctx, in)
					if err != nil {
						return nil, err
					}

					// Double each value in the result
					for i := range result {
						result[i] *= 2
					}
					return result, nil
				},
				next.Cancel,
			)
		}

		proc := applyMiddleware(baseProcessor, outputModifyingMiddleware)

		// Process should modify the output
		result, err := proc.Process(context.Background(), "test")

		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		if len(result) != 1 || result[0] != 8 { // 4 (len("test")) * 2
			t.Errorf("Expected [8], got %v", result)
		}
	})
}
