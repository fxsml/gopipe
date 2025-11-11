package message_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/fxsml/gopipe"
	"github.com/fxsml/gopipe/message"
)

func TestNewProcessPipe_AutomaticAck(t *testing.T) {
	t.Parallel()

	// Track ack/nack calls
	var (
		ackCalled  bool
		nackCalled bool
		mu         sync.Mutex
	)

	ackFunc := func() {
		mu.Lock()
		defer mu.Unlock()
		ackCalled = true
	}

	nackFunc := func(err error) {
		mu.Lock()
		defer mu.Unlock()
		nackCalled = true
	}

	// Create a message with ack/nack handlers
	msg := message.NewMessageWithAck("test-1", 42, ackFunc, nackFunc)

	// Create a pipe that processes successfully
	pipe := message.NewProcessPipe(
		func(ctx context.Context, value int) ([]int, error) {
			return []int{value * 2}, nil
		},
	)

	// Process the message
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	in := make(chan *message.Message[int], 1)
	in <- msg
	close(in)

	out := pipe.Start(ctx, in)

	// Collect results
	var results []*message.Message[int]
	for result := range out {
		results = append(results, result)
	}

	// Verify ack was called
	mu.Lock()
	defer mu.Unlock()

	if !ackCalled {
		t.Error("Expected ack to be called on successful processing")
	}

	if nackCalled {
		t.Error("Expected nack NOT to be called on successful processing")
	}

	if len(results) != 1 {
		t.Errorf("Expected 1 result, got %d", len(results))
	}

	if len(results) > 0 && results[0].Payload != 84 {
		t.Errorf("Expected payload 84, got %d", results[0].Payload)
	}
}

func TestNewProcessPipe_AutomaticNack(t *testing.T) {
	t.Parallel()

	// Track ack/nack calls
	var (
		ackCalled  bool
		nackCalled bool
		nackErr    error
		mu         sync.Mutex
	)

	ackFunc := func() {
		mu.Lock()
		defer mu.Unlock()
		ackCalled = true
	}

	nackFunc := func(err error) {
		mu.Lock()
		defer mu.Unlock()
		nackCalled = true
		nackErr = err
	}

	// Create a message with ack/nack handlers
	msg := message.NewMessageWithAck("test-2", 42, ackFunc, nackFunc)

	// Error to return
	testErr := errors.New("processing failed")

	// Create a pipe that fails
	pipe := message.NewProcessPipe(
		func(ctx context.Context, value int) ([]int, error) {
			return nil, testErr
		},
	)

	// Process the message
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	in := make(chan *message.Message[int], 1)
	in <- msg
	close(in)

	out := pipe.Start(ctx, in)

	// Drain results (should be empty)
	var results []*message.Message[int]
	for result := range out {
		results = append(results, result)
	}

	// Give some time for cancel goroutine to complete
	time.Sleep(50 * time.Millisecond)

	// Verify nack was called
	mu.Lock()
	defer mu.Unlock()

	if ackCalled {
		t.Error("Expected ack NOT to be called on failed processing")
	}

	if !nackCalled {
		t.Error("Expected nack to be called on failed processing")
	}

	if len(results) != 0 {
		t.Errorf("Expected 0 results on error, got %d", len(results))
	}

	if nackErr == nil {
		t.Error("Expected nack to receive an error")
	}

	// The error should be wrapped in a gopipe.ErrFailure
	if !errors.Is(nackErr, gopipe.ErrFailure) {
		t.Errorf("Expected error to wrap gopipe.ErrFailure, got: %v", nackErr)
	}

	// The underlying cause should be our test error
	if !errors.Is(nackErr, testErr) {
		t.Errorf("Expected error to wrap test error, got: %v", nackErr)
	}
}

func TestNewProcessPipe_NackOnContextCancellation(t *testing.T) {
	t.Parallel()

	// Track ack/nack calls
	var (
		ackCalled  bool
		nackCalled bool
		nackErr    error
		mu         sync.Mutex
	)

	ackFunc := func() {
		mu.Lock()
		defer mu.Unlock()
		ackCalled = true
	}

	nackFunc := func(err error) {
		mu.Lock()
		defer mu.Unlock()
		nackCalled = true
		nackErr = err
	}

	// Create a message with ack/nack handlers
	msg := message.NewMessageWithAck("test-3", 42, ackFunc, nackFunc)

	// Create a pipe with a slow handler
	pipe := message.NewProcessPipe(
		func(ctx context.Context, value int) ([]int, error) {
			// Simulate slow processing
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(1 * time.Second):
				return []int{value * 2}, nil
			}
		},
	)

	// Process the message with immediate cancellation
	ctx, cancel := context.WithCancel(context.Background())

	in := make(chan *message.Message[int], 1)
	in <- msg
	close(in)

	out := pipe.Start(ctx, in)

	// Cancel immediately
	cancel()

	// Drain results
	var results []*message.Message[int]
	for result := range out {
		results = append(results, result)
	}

	// Give some time for cancel goroutine to complete
	time.Sleep(50 * time.Millisecond)

	// Verify nack was called
	mu.Lock()
	defer mu.Unlock()

	if ackCalled {
		t.Error("Expected ack NOT to be called when context is canceled")
	}

	if !nackCalled {
		t.Error("Expected nack to be called when context is canceled")
	}

	if nackErr == nil {
		t.Error("Expected nack to receive an error")
	}

	// The error should be related to context cancellation
	if !errors.Is(nackErr, context.Canceled) && !errors.Is(nackErr, gopipe.ErrCancel) {
		t.Errorf("Expected error to be context.Canceled or gopipe.ErrCancel, got: %v", nackErr)
	}
}

func TestNewProcessPipe_NoAckNackWhenNotProvided(t *testing.T) {
	t.Parallel()

	// Create a message WITHOUT ack/nack handlers
	msg := message.NewMessage("test-4", 42)

	// Create a pipe that processes successfully
	pipe := message.NewProcessPipe(
		func(ctx context.Context, value int) ([]int, error) {
			return []int{value * 2}, nil
		},
	)

	// Process the message
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	in := make(chan *message.Message[int], 1)
	in <- msg
	close(in)

	out := pipe.Start(ctx, in)

	// Collect results - should work fine even without ack/nack
	var results []*message.Message[int]
	for result := range out {
		results = append(results, result)
	}

	if len(results) != 1 {
		t.Errorf("Expected 1 result, got %d", len(results))
	}

	if len(results) > 0 && results[0].Payload != 84 {
		t.Errorf("Expected payload 84, got %d", results[0].Payload)
	}
}

func TestNewProcessPipe_MessageDeadline(t *testing.T) {
	t.Parallel()

	// Track ack/nack calls
	var (
		nackCalled bool
		nackErr    error
		mu         sync.Mutex
	)

	nackFunc := func(err error) {
		mu.Lock()
		defer mu.Unlock()
		nackCalled = true
		nackErr = err
	}

	// Create a message with a very short deadline
	msg := message.NewMessageWithAck("test-5", 42, func() {}, nackFunc)
	msg.SetTimeout(10 * time.Millisecond)

	// Create a pipe with a slow handler
	pipe := message.NewProcessPipe(
		func(ctx context.Context, value int) ([]int, error) {
			// Wait for context to be done
			<-ctx.Done()
			return nil, ctx.Err()
		},
	)

	// Process the message
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	in := make(chan *message.Message[int], 1)
	in <- msg
	close(in)

	out := pipe.Start(ctx, in)

	// Drain results
	for range out {
	}

	// Give some time for cancel goroutine to complete
	time.Sleep(50 * time.Millisecond)

	// Verify nack was called due to deadline
	mu.Lock()
	defer mu.Unlock()

	if !nackCalled {
		t.Error("Expected nack to be called when message deadline expires")
	}

	if nackErr == nil {
		t.Error("Expected nack to receive an error")
	}

	// The error should be related to deadline exceeded
	if !errors.Is(nackErr, context.DeadlineExceeded) {
		t.Errorf("Expected error to be context.DeadlineExceeded, got: %v", nackErr)
	}
}
