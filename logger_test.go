package gopipe

import (
	"context"
	"errors"
	"testing"
)

// mockLogger implements the Logger interface for testing
type mockLogger struct {
	debugCalls []logCall
	infoCalls  []logCall
	warnCalls  []logCall
	errorCalls []logCall
}

type logCall struct {
	msg  string
	args []any
}

func (l *mockLogger) Debug(msg string, args ...any) {
	l.debugCalls = append(l.debugCalls, logCall{msg: msg, args: args})
}

func (l *mockLogger) Info(msg string, args ...any) {
	l.infoCalls = append(l.infoCalls, logCall{msg: msg, args: args})
}

func (l *mockLogger) Warn(msg string, args ...any) {
	l.warnCalls = append(l.warnCalls, logCall{msg: msg, args: args})
}

func (l *mockLogger) Error(msg string, args ...any) {
	l.errorCalls = append(l.errorCalls, logCall{msg: msg, args: args})
}

func (l *mockLogger) reset() {
	l.debugCalls = nil
	l.infoCalls = nil
	l.warnCalls = nil
	l.errorCalls = nil
}

func TestLogger_LogsSuccessfulProcessing(t *testing.T) {
	logger := &mockLogger{}

	// Create a processor that always succeeds
	baseProcessor := NewProcessor(
		func(ctx context.Context, in string) ([]string, error) {
			return []string{in + "-processed"}, nil
		},
		func(in string, err error) {},
	)

	// Apply logger middleware with default config
	processor := UseLogger[string, string](logger, LoggerConfig{})(baseProcessor)

	// Process an item - should succeed and log
	_, err := processor.Process(context.Background(), "test-input")
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	// Check debug logs (default success level is debug)
	if len(logger.debugCalls) != 1 {
		t.Fatalf("Expected 1 debug log, got %d", len(logger.debugCalls))
	}

	if logger.debugCalls[0].msg != "GOPIPE: Success" {
		t.Errorf("Expected success message, got %q", logger.debugCalls[0].msg)
	}
}

func TestLogger_LogsFailure(t *testing.T) {
	logger := &mockLogger{}
	testError := errors.New("processing failed")

	// Create a processor that always fails
	baseProcessor := NewProcessor(
		func(ctx context.Context, in string) ([]string, error) {
			return nil, testError
		},
		func(in string, err error) {},
	)

	// Apply logger middleware with default config
	processor := UseLogger[string, string](logger, LoggerConfig{})(baseProcessor)

	// Process an item - should fail and log
	_, err := processor.Process(context.Background(), "test-input")
	if err == nil {
		t.Fatal("Expected an error, got nil")
	}

	// Manually invoke the Cancel function to trigger logging
	// This is needed because the logger middleware logs errors in Cancel, not Process
	processor.Cancel("test-input", testError)

	// Check error logs (default failure level is error)
	if len(logger.errorCalls) != 1 {
		t.Fatalf("Expected 1 error log, got %d", len(logger.errorCalls))
	}

	if logger.errorCalls[0].msg != "GOPIPE: Failure" {
		t.Errorf("Expected failure message, got %q", logger.errorCalls[0].msg)
	}

	// Check that error is included in log args
	hasErrorArg := false
	for i := 0; i < len(logger.errorCalls[0].args); i += 2 {
		if i+1 < len(logger.errorCalls[0].args) &&
			logger.errorCalls[0].args[i] == "error" {
			hasErrorArg = true
			break
		}
	}

	if !hasErrorArg {
		t.Errorf("Error log should include the error: %v", logger.errorCalls[0].args)
	}
}

func TestLogger_CustomLogLevels(t *testing.T) {
	logger := &mockLogger{}

	// Create a config with custom log levels
	config := LoggerConfig{
		LevelSuccess: LogLevelInfo,  // Change from default debug
		LevelFailure: LogLevelError, // Same as default
	}

	// Create a processor for testing
	baseProcessor := NewProcessor(
		func(ctx context.Context, in string) ([]string, error) {
			return []string{in + "-processed"}, nil
		},
		func(in string, err error) {},
	)

	// Apply logger middleware with custom config
	processor := UseLogger[string, string](logger, config)(baseProcessor)

	// Process an item successfully
	_, err := processor.Process(context.Background(), "test-input")
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	// Check that it logged at info level (not debug)
	if len(logger.debugCalls) != 0 {
		t.Errorf("Should not have debug logs, got %d", len(logger.debugCalls))
	}
	if len(logger.infoCalls) != 1 {
		t.Errorf("Expected 1 info log, got %d", len(logger.infoCalls))
	}
}

func TestLogger_CustomMessages(t *testing.T) {
	logger := &mockLogger{}
	testError := errors.New("failure")

	// Create a config with custom messages
	config := LoggerConfig{
		MessageSuccess: "Custom success message",
		MessageFailure: "Custom failure message",
	}

	// Create a processor for testing
	baseProcessor := NewProcessor(
		func(ctx context.Context, in string) ([]string, error) {
			if in == "fail" {
				return nil, testError
			}
			return []string{in + "-processed"}, nil
		},
		func(in string, err error) {},
	)

	// Apply logger middleware with custom config
	processor := UseLogger[string, string](logger, config)(baseProcessor)

	// Test success
	_, err := processor.Process(context.Background(), "success")
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if len(logger.debugCalls) != 1 || logger.debugCalls[0].msg != "Custom success message" {
		t.Errorf("Expected custom success message, got %v", logger.debugCalls)
	}

	logger.reset()

	// Test failure
	_, err = processor.Process(context.Background(), "fail")
	if err == nil {
		t.Fatal("Expected error, got nil")
	}

	// Manually invoke Cancel to trigger failure logging
	processor.Cancel("fail", testError)

	if len(logger.errorCalls) != 1 || logger.errorCalls[0].msg != "Custom failure message" {
		t.Errorf("Expected custom failure message, got %v", logger.errorCalls)
	}
}

func TestUseSlog(t *testing.T) {
	// This is just a smoke test to ensure the function doesn't panic
	// Real slog testing would require a more complex setup with output capture

	baseProcessor := NewProcessor(
		func(ctx context.Context, in string) ([]string, error) {
			return []string{in + "-processed"}, nil
		},
		func(in string, err error) {},
	)

	// Should not panic
	processor := UseSlog[string, string]("service", "logger-test")(baseProcessor)

	_, err := processor.Process(context.Background(), "test-input")
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	// No assertions, just ensuring it doesn't panic
}
