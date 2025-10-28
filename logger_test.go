package gopipe

import (
	"context"
	"errors"
	"sync"
	"testing"
)

// mockLogger implements the Logger interface for testing
type mockLogger struct {
	mu         sync.Mutex
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
	l.mu.Lock()
	l.debugCalls = append(l.debugCalls, logCall{msg: msg, args: args})
	l.mu.Unlock()
}

func (l *mockLogger) Info(msg string, args ...any) {
	l.mu.Lock()
	l.infoCalls = append(l.infoCalls, logCall{msg: msg, args: args})
	l.mu.Unlock()
}

func (l *mockLogger) Warn(msg string, args ...any) {
	l.mu.Lock()
	l.warnCalls = append(l.warnCalls, logCall{msg: msg, args: args})
	l.mu.Unlock()
}

func (l *mockLogger) Error(msg string, args ...any) {
	l.mu.Lock()
	l.errorCalls = append(l.errorCalls, logCall{msg: msg, args: args})
	l.mu.Unlock()
}

func (l *mockLogger) reset() {
	l.mu.Lock()
	l.debugCalls = nil
	l.infoCalls = nil
	l.warnCalls = nil
	l.errorCalls = nil
	l.mu.Unlock()
}

func useLogger[In, Out any](config *LoggerConfig) MiddlewareFunc[In, Out] {
	return useMetrics[In, Out](newMetricsLogger(config))
}

func TestLogger_LogsSuccessfulProcessing(t *testing.T) {
	logger := &mockLogger{}
	SetDefaultLogger(logger)

	// Create a processor that always succeeds
	baseProcessor := NewProcessor(
		func(ctx context.Context, in string) ([]string, error) {
			return []string{in + "-processed"}, nil
		},
		func(in string, err error) {},
	)

	// Apply logger middleware with default config
	processor := useLogger[string, string](&LoggerConfig{})(baseProcessor)

	// Process an item - should succeed and log
	_, err := processor.Process(context.Background(), "test-input")
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	// Check debug logs (default success level is debug)
	logger.mu.Lock()
	debugCallsLen := len(logger.debugCalls)
	var debugCallMsg string
	if debugCallsLen > 0 {
		debugCallMsg = logger.debugCalls[0].msg
	}
	logger.mu.Unlock()

	if debugCallsLen != 1 {
		t.Fatalf("Expected 1 debug log, got %d", debugCallsLen)
	}

	if debugCallMsg != "GOPIPE: Success" {
		t.Errorf("Expected success message, got %q", debugCallMsg)
	}
}

func TestLogger_LogsFailure(t *testing.T) {
	logger := &mockLogger{}
	SetDefaultLogger(logger)
	testError := errors.New("processing failed")

	// Create a processor that always fails
	baseProcessor := NewProcessor(
		func(ctx context.Context, in string) ([]string, error) {
			return nil, testError
		},
		func(in string, err error) {},
	)

	// Apply logger middleware with default config
	processor := useLogger[string, string](&LoggerConfig{})(baseProcessor)

	// Process an item - should fail and log
	_, err := processor.Process(context.Background(), "test-input")
	if err == nil {
		t.Fatal("Expected an error, got nil")
	}

	// Manually invoke the Cancel function to trigger logging
	// This is needed because the logger middleware logs errors in Cancel, not Process
	processor.Cancel("test-input", testError)

	// Check error logs (default failure level is error)
	logger.mu.Lock()
	errorCallsLen := len(logger.errorCalls)
	var errorCallMsg string
	var errorCallArgs []any
	if errorCallsLen > 0 {
		errorCallMsg = logger.errorCalls[0].msg
		errorCallArgs = logger.errorCalls[0].args
	}
	logger.mu.Unlock()

	if errorCallsLen != 1 {
		t.Fatalf("Expected 1 error log, got %d", errorCallsLen)
	}

	if errorCallMsg != "GOPIPE: Failure" {
		t.Errorf("Expected failure message, got %q", errorCallMsg)
	}

	// Check that error is included in log args
	hasErrorArg := false
	for i := 0; i < len(errorCallArgs); i += 2 {
		if i+1 < len(errorCallArgs) &&
			errorCallArgs[i] == "error" {
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
	SetDefaultLogger(logger)

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
	processor := useLogger[string, string](&config)(baseProcessor)

	// Process an item successfully
	_, err := processor.Process(context.Background(), "test-input")
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	// Check that it logged at info level (not debug)
	logger.mu.Lock()
	debugCallsLen := len(logger.debugCalls)
	infoCallsLen := len(logger.infoCalls)
	logger.mu.Unlock()

	if debugCallsLen != 0 {
		t.Errorf("Should not have debug logs, got %d", debugCallsLen)
	}
	if infoCallsLen != 1 {
		t.Errorf("Expected 1 info log, got %d", infoCallsLen)
	}
}

func TestLogger_CustomMessages(t *testing.T) {
	logger := &mockLogger{}
	SetDefaultLogger(logger)
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
	processor := useLogger[string, string](&config)(baseProcessor)

	// Test success
	_, err := processor.Process(context.Background(), "success")
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	logger.mu.Lock()
	debugCallsLen := len(logger.debugCalls)
	var debugCallMsg string
	if debugCallsLen > 0 {
		debugCallMsg = logger.debugCalls[0].msg
	}
	logger.mu.Unlock()

	if debugCallsLen != 1 || debugCallMsg != "Custom success message" {
		t.Errorf("Expected custom success message, got len=%d msg=%q", debugCallsLen, debugCallMsg)
	}

	logger.reset()

	// Test failure
	_, err = processor.Process(context.Background(), "fail")
	if err == nil {
		t.Fatal("Expected error, got nil")
	}

	// Manually invoke Cancel to trigger failure logging
	processor.Cancel("fail", testError)

	logger.mu.Lock()
	errorCallsLen := len(logger.errorCalls)
	var errorCallMsg string
	if errorCallsLen > 0 {
		errorCallMsg = logger.errorCalls[0].msg
	}
	logger.mu.Unlock()

	if errorCallsLen != 1 || errorCallMsg != "Custom failure message" {
		t.Errorf("Expected custom failure message, got len=%d msg=%q", errorCallsLen, errorCallMsg)
	}
}
