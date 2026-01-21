package middleware

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/fxsml/gopipe/message"
)

func TestRecoverSuccessfulProcessing(t *testing.T) {
	processFunc := func(ctx context.Context, msg *message.Message) ([]*message.Message, error) {
		return []*message.Message{msg}, nil
	}
	fn := Recover()(processFunc)

	msg := message.New("test", nil, nil)
	result, err := fn(context.Background(), msg)

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("Expected len to be 1, got %d", len(result))
	}
	if result[0] != msg {
		t.Errorf("Expected same message returned")
	}
}

func TestRecoverProcessingWithPanic(t *testing.T) {
	processFunc := func(ctx context.Context, msg *message.Message) ([]*message.Message, error) {
		panic("test panic")
	}
	fn := Recover()(processFunc)

	msg := message.New("test", nil, nil)
	_, err := fn(context.Background(), msg)

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

func TestRecoverProcessingWithError(t *testing.T) {
	expectedErr := errors.New("handler error")
	processFunc := func(ctx context.Context, msg *message.Message) ([]*message.Message, error) {
		return nil, expectedErr
	}
	fn := Recover()(processFunc)

	msg := message.New("test", nil, nil)
	_, err := fn(context.Background(), msg)

	if !errors.Is(err, expectedErr) {
		t.Errorf("Expected %v, got %v", expectedErr, err)
	}
}
