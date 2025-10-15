package middleware

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/fxsml/gopipe"
)

// testMiddleware is a helper struct to track middleware execution order
type testMiddleware struct {
	name      string
	order     *[]string
	onProcess func(next func(context.Context, string) (string, error)) func(context.Context, string) (string, error)
	onCancel  func(next func(string, error)) func(string, error)
}

// Create a middleware function for testing
func (tm *testMiddleware) middleware() MiddlewareFunc[string, string] {
	return func(next gopipe.Processor[string, string]) gopipe.Processor[string, string] {
		return gopipe.NewProcessor(
			func(ctx context.Context, in string) (string, error) {
				// Record that this middleware's Process was called
				*tm.order = append(*tm.order, tm.name+":process")

				// If custom process logic exists, use it
				if tm.onProcess != nil {
					return tm.onProcess(next.Process)(ctx, in)
				}

				// Otherwise use default behavior
				return next.Process(ctx, in)
			},
			func(in string, err error) {
				// Record that this middleware's Cancel was called
				*tm.order = append(*tm.order, tm.name+":cancel")

				// If custom cancel logic exists, use it
				if tm.onCancel != nil {
					tm.onCancel(next.Cancel)(in, err)
				} else {
					// Otherwise use default behavior
					next.Cancel(in, err)
				}
			},
		)
	}
}

func TestNewProcessor_AppliesMiddlewaresInRightOrder(t *testing.T) {
	executionOrder := make([]string, 0)

	mw1 := &testMiddleware{name: "mw1", order: &executionOrder}
	mw2 := &testMiddleware{name: "mw2", order: &executionOrder}
	mw3 := &testMiddleware{name: "mw3", order: &executionOrder}

	processFunc := func(ctx context.Context, in string) (string, error) {
		executionOrder = append(executionOrder, "base:process")
		return in + "-processed", nil
	}

	processor := NewProcessor(
		processFunc,
		mw1.middleware(),
		mw2.middleware(),
		mw3.middleware(),
	)

	result, err := processor.Process(context.Background(), "input")
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	expected := "input-processed"
	if result != expected {
		t.Errorf("Expected result %q, got %q", expected, result)
	}

	expectedProcessOrder := []string{
		"mw1:process",
		"mw2:process",
		"mw3:process",
		"base:process",
	}

	if len(executionOrder) < len(expectedProcessOrder) {
		t.Fatalf("Expected at least %d items in execution order, got %d", len(expectedProcessOrder), len(executionOrder))
	}

	for i, expected := range expectedProcessOrder {
		if executionOrder[i] != expected {
			t.Errorf("Expected execution order[%d] to be %q, got %q", i, expected, executionOrder[i])
		}
	}

	executionOrder = executionOrder[:0]

	testError := errors.New("test error")
	processor.Cancel("input", testError)

	expectedCancelOrder := []string{
		"mw1:cancel",
		"mw2:cancel",
		"mw3:cancel",
	}

	if len(executionOrder) != len(expectedCancelOrder) {
		t.Fatalf("Expected %d items in cancel execution order, got %d", len(expectedCancelOrder), len(executionOrder))
	}

	for i, expected := range expectedCancelOrder {
		if executionOrder[i] != expected {
			t.Errorf("Expected cancel execution order[%d] to be %q, got %q", i, expected, executionOrder[i])
		}
	}
}

func TestNewProcessor_DataTransformationThroughMiddlewares(t *testing.T) {
	executionOrder := make([]string, 0)

	mw1 := &testMiddleware{
		name:  "mw1",
		order: &executionOrder,
		onProcess: func(next func(context.Context, string) (string, error)) func(context.Context, string) (string, error) {
			return func(ctx context.Context, in string) (string, error) {
				// Add prefix before passing to next
				modifiedIn := "mw1(" + in + ")"
				result, err := next(ctx, modifiedIn)
				if err != nil {
					return "", err
				}
				// Add suffix to result
				return result + "-mw1", err
			}
		},
	}

	mw2 := &testMiddleware{
		name:  "mw2",
		order: &executionOrder,
		onProcess: func(next func(context.Context, string) (string, error)) func(context.Context, string) (string, error) {
			return func(ctx context.Context, in string) (string, error) {
				// Add prefix before passing to next
				modifiedIn := "mw2(" + in + ")"
				result, err := next(ctx, modifiedIn)
				if err != nil {
					return "", err
				}
				// Add suffix to result
				return result + "-mw2", err
			}
		},
	}

	processFunc := func(ctx context.Context, in string) (string, error) {
		executionOrder = append(executionOrder, "base:process:"+in)
		return "result(" + in + ")", nil
	}

	processor := NewProcessor(processFunc, mw1.middleware(), mw2.middleware())

	result, err := processor.Process(context.Background(), "input")

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	expected := "result(mw2(mw1(input)))-mw2-mw1"
	if result != expected {
		t.Errorf("Expected transformed result %q, got %q", expected, result)
	}

	expectedBaseCall := "base:process:mw2(mw1(input))"
	found := slices.Contains(executionOrder, expectedBaseCall)

	if !found {
		t.Errorf("Base processor didn't receive properly transformed input. Expected to find %q in execution order", expectedBaseCall)
		t.Logf("Execution order: %v", executionOrder)
	}
}
