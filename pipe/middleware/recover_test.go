package middleware

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestRecoverSuccessfulProcessing(t *testing.T) {
	processFunc := func(ctx context.Context, in string) ([]int, error) {
		return []int{len(in)}, nil
	}
	fn := Recover[string, int]()(processFunc)

	result, err := fn(context.Background(), "hello")

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
	processFunc := func(ctx context.Context, in string) ([]int, error) {
		panic("test panic")
	}
	fn := Recover[string, int]()(processFunc)

	_, err := fn(context.Background(), "hello")

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
