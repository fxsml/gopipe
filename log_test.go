package gopipe

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

func useLogger[In, Out any](config *LogConfig) MiddlewareFunc[In, Out] {
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
	processor := useLogger[string, string](&LogConfig{})(baseProcessor)

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
	processor := useLogger[string, string](&LogConfig{})(baseProcessor)

	// Process an item - should fail and log
	_, err := processor.Process(context.Background(), "test-input")
	if err == nil {
		t.Fatal("Expected an error, got nil")
	}

	logger.reset()

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
	config := LogConfig{
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
	config := LogConfig{
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

	logger.reset()

	// Manually invoke Cancel to trigger failure logging
	processor.Cancel("fail", testError)

	logger.mu.Lock()
	errorCallsLen = len(logger.errorCalls)
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

	// Create a pipe with retry and logging
	pipe := NewProcessPipe(
		processFunc,
		WithRetryConfig[string, string](&RetryConfig{
			ShouldRetry: ShouldRetry(),
			Backoff:     ConstantBackoff(1*time.Millisecond, 0.0),
			MaxAttempts: 5,
		}),
		WithLogConfig[string, string](&LogConfig{}),
	)

	// Create input channel
	in := make(chan string, 1)
	in <- "test"
	close(in)

	// Start the pipe
	out := pipe.Start(context.Background(), in)

	// Collect results
	var results []string
	for val := range out {
		results = append(results, val)
	}

	// Should succeed after retries
	if len(results) != 1 || results[0] != "test_processed" {
		t.Errorf("expected result [test_processed], got %v", results)
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
	// Verify retry arguments are present
	args := warnCalls[0].args
	var foundRetryAttempts, foundRetryDuration bool
	for i := 0; i < len(args); i += 2 {
		if i+1 >= len(args) {
			break
		}
		key, ok := args[i].(string)
		if !ok {
			continue
		}
		if key == "retry_attempts" {
			foundRetryAttempts = true
			if attempts, ok := args[i+1].(int); !ok || attempts != 1 {
				t.Errorf("expected retry_attempts=1 in first retry, got %v", args[i+1])
			}
		}
		if key == "retry_duration" {
			foundRetryDuration = true
		}
	}
	if !foundRetryAttempts {
		t.Error("expected retry_attempts in retry warning log")
	}
	if !foundRetryDuration {
		t.Error("expected retry_duration in retry warning log")
	}

	// Should have 1 success debug log
	if len(debugCalls) != 1 {
		t.Fatalf("expected 1 success debug log, got %d", len(debugCalls))
	}
	if debugCalls[0].msg != "GOPIPE: Success" {
		t.Errorf("expected success message 'GOPIPE: Success', got %q", debugCalls[0].msg)
	}

	// Test failure case
	logger.reset()
	attempts = 0

	failingProcessFunc := func(ctx context.Context, in string) ([]string, error) {
		attempts++
		return nil, errors.New("persistent error")
	}

	// Create a pipe that will fail after max attempts
	failPipe := NewProcessPipe(
		failingProcessFunc,
		WithRetryConfig[string, string](&RetryConfig{
			ShouldRetry: ShouldRetry(),
			Backoff:     ConstantBackoff(1*time.Millisecond, 0.0),
			MaxAttempts: 2,
		}),
		WithLogConfig[string, string](&LogConfig{}),
	)

	// Create input channel
	failIn := make(chan string, 1)
	failIn <- "test"
	close(failIn)

	// Start the pipe
	failOut := failPipe.Start(context.Background(), failIn)

	// Collect results (should be empty since all attempts fail)
	var failResults []string
	for val := range failOut {
		failResults = append(failResults, val)
	}

	// Should have no results due to failure
	if len(failResults) != 0 {
		t.Errorf("expected no results due to failure, got %v", failResults)
	}

	// Check logs for failure case
	logger.mu.Lock()
	warnCalls = logger.warnCalls
	errorCalls := logger.errorCalls
	logger.mu.Unlock()

	// Should have 2 retry warnings
	if len(warnCalls) != 2 {
		t.Fatalf("expected 2 retry warning logs for failure case, got %d", len(warnCalls))
	}

	// Should have 1 error log for final failure
	if len(errorCalls) != 1 {
		t.Fatalf("expected 1 error log for failure, got %d", len(errorCalls))
	}
	if errorCalls[0].msg != "GOPIPE: Failure" {
		t.Errorf("expected failure message 'GOPIPE: Failure', got %q", errorCalls[0].msg)
	}

	// Verify failure log contains retry information
	failArgs := errorCalls[0].args
	var foundFailRetryAttempts bool
	for i := 0; i < len(failArgs); i += 2 {
		if i+1 >= len(failArgs) {
			break
		}
		key, ok := failArgs[i].(string)
		if !ok {
			continue
		}
		if key == "retry_attempts" {
			foundFailRetryAttempts = true
			if attempts, ok := failArgs[i+1].(int); !ok || attempts != 2 {
				t.Errorf("expected retry_attempts=2 in failure log, got %v", failArgs[i+1])
			}
		}
	}
	if !foundFailRetryAttempts {
		t.Error("expected retry_attempts in failure log")
	}
}
