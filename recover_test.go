package gopipe

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestRecoverSuccessfulProcessing(t *testing.T) {
	processor := NewProcessor(
		func(ctx context.Context, in string) ([]int, error) {
			return []int{len(in)}, nil
		},
		nil,
	)
	processor = UseRecover[string, int]()(processor)

	result, err := processor.Process(context.Background(), "hello")

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("Expected len to be 1, got %d", len(result))
	}
	if result[0] != 5 {
		t.Errorf("Expected result to be 5, got %d", result)
	}
}

func TestRecoverProcessingWithPanic(t *testing.T) {
	processor := NewProcessor(
		func(ctx context.Context, in string) ([]int, error) {
			panic("test panic")
		},
		nil,
	)
	processor = UseRecover[string, int]()(processor)

	_, err := processor.Process(context.Background(), "hello")

	if err == nil {
		t.Error("Expected an error, got nil")
		return
	}

	var recoveryErr *RecoveryError
	if !errors.As(err, &recoveryErr) {
		t.Errorf("Expected *RecoveryError, got %T", err)
		return
	}

	if recoveryErr.PanicValue != "test panic" {
		t.Errorf("Expected panic value 'test panic', got %v", recoveryErr.PanicValue)
	}

	if !strings.Contains(recoveryErr.StackTrace, "runtime/debug.Stack") {
		t.Error("Expected stack trace to contain 'runtime/debug.Stack'")
	}
}

func TestRecoverCancellation(t *testing.T) {
	var cancelCalled bool
	var cancelErr error

	processor := NewProcessor(
		func(ctx context.Context, in string) ([]int, error) {
			return []int{0}, nil
		},
		func(in string, err error) {
			cancelCalled = true
			cancelErr = err
		})
	processor = UseRecover[string, int]()(processor)

	testErr := errors.New("test error")
	processor.Cancel("test", testErr)

	if !cancelCalled {
		t.Error("Cancel function was not called")
	}

	if cancelErr != testErr {
		t.Errorf("Expected error %v, got %v", testErr, cancelErr)
	}
}

func TestRecoverIntegrationWithPipeline(t *testing.T) {
	ctx := context.Background()
	in := make(chan string, 1)
	in <- "hello"
	close(in)

	var cancelCalls int

	processor := NewProcessor(
		func(ctx context.Context, in string) ([]int, error) {
			if in == "hello" {
				panic("pipeline panic")
			}
			return []int{len(in)}, nil
		},
		func(in string, err error) {
			cancelCalls++
			var recError *RecoveryError
			if errors.As(err, &recError) {
				if recError.PanicValue == nil || recError.StackTrace == "" {
					t.Errorf("Expected to extract panic info from error: %T", err)
				}
			} else {
				t.Errorf("Expected error to be of type *RecoveryError, got %T", err)
			}
		})

	out := StartProcessor(ctx, in, processor, WithMiddleware(UseRecover[string, int]()))

	// Drain the output channel
	for range out {
		// This should be empty since the only item caused a panic
	}

	if cancelCalls != 1 {
		t.Errorf("Expected 1 cancel call, got %d", cancelCalls)
	}
}
