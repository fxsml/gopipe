package middleware

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
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

func TestLogger_LogsSuccessfulProcessing(t *testing.T) {
	logger := &mockLogger{}
	SetDefaultLogger(logger)

	// Create a process func that always succeeds
	processFunc := func(ctx context.Context, in string) ([]string, error) {
		return []string{in + "-processed"}, nil
	}

	// Apply logger middleware with default config
	logMw := Log[string, string](LogConfig{})
	wrappedFunc := logMw(processFunc)

	// Process an item - should succeed and log
	_, err := wrappedFunc(context.Background(), "test-input")
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

	// Create a process func that always fails
	processFunc := func(ctx context.Context, in string) ([]string, error) {
		return nil, testError
	}

	// Apply logger middleware with default config
	logMw := Log[string, string](LogConfig{})
	wrappedFunc := logMw(processFunc)

	// Process an item - should fail and log error
	_, err := wrappedFunc(context.Background(), "test-input")
	if err == nil {
		t.Fatal("Expected an error, got nil")
	}

	// Check error logs
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
		if i+1 < len(errorCallArgs) && errorCallArgs[i] == "error" {
			hasErrorArg = true
			break
		}
	}

	if !hasErrorArg {
		t.Errorf("Error log should include the error: %v", errorCallArgs)
	}
}

func TestLogger_CustomLogLevels(t *testing.T) {
	logger := &mockLogger{}
	SetDefaultLogger(logger)

	// Create a config with custom log levels
	config := LogConfig{
		LevelSuccess: LogLevelInfo, // Change from default debug
		LevelFailure: LogLevelError,
	}

	// Create a process func for testing
	processFunc := func(ctx context.Context, in string) ([]string, error) {
		return []string{in + "-processed"}, nil
	}

	// Apply logger middleware with custom config
	logMw := Log[string, string](config)
	wrappedFunc := logMw(processFunc)

	// Process an item successfully
	_, err := wrappedFunc(context.Background(), "test-input")
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
	config := LogConfig{
		MessageSuccess: "Custom success message",
		MessageFailure: "Custom failure message",
	}

	// Create a process func for testing
	processFunc := func(ctx context.Context, in string) ([]string, error) {
		if in == "fail" {
			return nil, testError
		}
		return []string{in + "-processed"}, nil
	}

	// Apply logger middleware with custom config
	logMw := Log[string, string](config)
	wrappedFunc := logMw(processFunc)

	// Test success
	_, err := wrappedFunc(context.Background(), "success")
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
	_, err = wrappedFunc(context.Background(), "fail")
	if err == nil {
		t.Fatal("Expected error, got nil")
	}
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

func TestLogger_WithRetry(t *testing.T) {
	logger := &mockLogger{}
	SetDefaultLogger(logger)

	attempts := 0
	processFunc := func(ctx context.Context, in string) ([]string, error) {
		attempts++
		if attempts < 3 {
			return nil, errors.New("temporary error")
		}
		return []string{in + "_processed"}, nil
	}

	retryConfig := RetryConfig{
		ShouldRetry: ShouldRetry(),
		Backoff:     ConstantBackoff(1*time.Millisecond, 0.0),
		MaxAttempts: 5,
	}

	// Apply middleware: Log inside Retry so it sees retry context
	fn := Retry[string, string](retryConfig)(
		Log[string, string](LogConfig{})(processFunc),
	)

	// Process an item
	result, err := fn(context.Background(), "test")
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	// Should succeed after retries
	if len(result) != 1 || result[0] != "test_processed" {
		t.Errorf("expected result [test_processed], got %v", result)
	}

	// Check retry warning logs
	logger.mu.Lock()
	warnCalls := logger.warnCalls
	debugCalls := logger.debugCalls
	logger.mu.Unlock()

	// Should have 2 retry warnings (for attempts 1 and 2)
	if len(warnCalls) != 2 {
		t.Fatalf("expected 2 retry warning logs, got %d", len(warnCalls))
	}

	// Check first retry warning
	if warnCalls[0].msg != "GOPIPE: Retry" {
		t.Errorf("expected first retry warning message 'GOPIPE: Retry', got %q", warnCalls[0].msg)
	}

	// Should have 1 success debug log
	if len(debugCalls) != 1 {
		t.Fatalf("expected 1 success debug log, got %d", len(debugCalls))
	}
	if debugCalls[0].msg != "GOPIPE: Success" {
		t.Errorf("expected success message 'GOPIPE: Success', got %q", debugCalls[0].msg)
	}
}

func TestLogger_Disabled(t *testing.T) {
	logger := &mockLogger{}
	SetDefaultLogger(logger)

	// Apply disabled log middleware
	fn := Log[string, string](LogConfig{Disabled: true})(
		func(ctx context.Context, in string) ([]string, error) {
			return []string{in + "-processed"}, nil
		},
	)

	// Process an item
	result, err := fn(context.Background(), "test-input")
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	// Should succeed
	if len(result) != 1 || result[0] != "test-input-processed" {
		t.Errorf("Expected result [test-input-processed], got %v", result)
	}

	// Check that no logs were generated
	logger.mu.Lock()
	debugCallsLen := len(logger.debugCalls)
	infoCallsLen := len(logger.infoCalls)
	warnCallsLen := len(logger.warnCalls)
	errorCallsLen := len(logger.errorCalls)
	logger.mu.Unlock()

	if debugCallsLen != 0 || infoCallsLen != 0 || warnCallsLen != 0 || errorCallsLen != 0 {
		t.Errorf("Expected 0 logs when disabled, got debug=%d info=%d warn=%d error=%d",
			debugCallsLen, infoCallsLen, warnCallsLen, errorCallsLen)
	}
}
