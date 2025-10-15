package middleware

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/fxsml/gopipe"
)

func TestRecoverSuccessfulProcessing(t *testing.T) {
	processor := NewProcessor(
		func(ctx context.Context, in string) (int, error) {
			return len(in), nil
		},
		UseRecover[string, int](),
	)

	result, err := processor.Process(context.Background(), "hello")

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	if result != 5 {
		t.Errorf("Expected result to be 5, got %d", result)
	}
}

func TestRecoverProcessingWithPanic(t *testing.T) {
	processor := NewProcessor(
		func(ctx context.Context, in string) (int, error) {
			panic("test panic")
		},
		UseRecover[string, int](),
	)

	_, err := processor.Process(context.Background(), "hello")

	if err == nil {
		t.Error("Expected an error, got nil")
		return
	}

	recoveryErr, ok := err.(*RecoveryError)
	if !ok {
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
		func(ctx context.Context, in string) (int, error) {
			return 0, nil
		},
		UseRecover[string, int](),
		UseCancel[string, int](func(in string, err error) {
			cancelCalled = true
			cancelErr = err
		}),
	)

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
		func(ctx context.Context, in string) (int, error) {
			if in == "hello" {
				panic("pipeline panic")
			}
			return len(in), nil
		},
		UseRecover[string, int](),
		UseCancel[string, int](func(in string, err error) {
			cancelCalls++
			var recError *RecoveryError
			if errors.As(err, &recError) {
				if recError.PanicValue == nil || recError.StackTrace == "" {
					t.Errorf("Expected to extract panic info from error: %T", err)
				}
			} else {
				t.Errorf("Expected error to be of type *RecoveryError, got %T", err)
			}
		}),
	)

	out := gopipe.Process(ctx, in, processor)

	// Drain the output channel
	for range out {
		// This should be empty since the only item caused a panic
	}

	if cancelCalls != 1 {
		t.Errorf("Expected 1 cancel call, got %d", cancelCalls)
	}
}
