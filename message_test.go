package gopipe_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/fxsml/gopipe"
)

func TestMessage_NewMessage(t *testing.T) {
	t.Parallel()

	metadata := gopipe.Metadata{"key": "value"}
	deadline := time.Now().Add(1 * time.Hour)
	var ackCalled, nackCalled bool

	msg := gopipe.NewMessage(
		metadata,
		"payload",
		deadline,
		func() { ackCalled = true },
		func(error) { nackCalled = true },
	)

	if msg.Metadata["key"] != "value" {
		t.Errorf("Expected Metadata key='value', got %v", msg.Metadata["key"])
	}

	if msg.Payload != "payload" {
		t.Errorf("Expected Payload 'payload', got %q", msg.Payload)
	}

	if msg.Deadline() != deadline {
		t.Errorf("Expected deadline %v, got %v", deadline, msg.Deadline())
	}

	// Verify ack works
	if !msg.Ack() {
		t.Error("Expected Ack to return true")
	}

	if !ackCalled {
		t.Error("Expected ack function to be called")
	}

	if nackCalled {
		t.Error("Expected nack function NOT to be called")
	}
}

func TestMessage_AckIdempotency(t *testing.T) {
	t.Parallel()

	var ackCount int
	msg := gopipe.NewMessage(
		nil,
		42,
		time.Time{},
		func() { ackCount++ },
		func(error) {},
	)

	// First ack should succeed
	if !msg.Ack() {
		t.Error("Expected first Ack to return true")
	}

	// Second ack should be idempotent (returns true but doesn't call function again)
	if !msg.Ack() {
		t.Error("Expected second Ack to return true (idempotent)")
	}

	if ackCount != 1 {
		t.Errorf("Expected ack function to be called exactly once, got %d", ackCount)
	}
}

func TestMessage_NackIdempotency(t *testing.T) {
	t.Parallel()

	var nackCount int
	var capturedErr error
	msg := gopipe.NewMessage(
		nil,
		42,
		time.Time{},
		func() {},
		func(err error) {
			nackCount++
			capturedErr = err
		},
	)

	testErr := errors.New("test error")

	// First nack should succeed
	if !msg.Nack(testErr) {
		t.Error("Expected first Nack to return true")
	}

	// Second nack should be idempotent (returns true but doesn't call function again)
	if !msg.Nack(errors.New("different error")) {
		t.Error("Expected second Nack to return true (idempotent)")
	}

	if nackCount != 1 {
		t.Errorf("Expected nack function to be called exactly once, got %d", nackCount)
	}

	if !errors.Is(capturedErr, testErr) {
		t.Errorf("Expected first error to be captured, got %v", capturedErr)
	}
}

func TestMessage_AckAfterNack(t *testing.T) {
	t.Parallel()

	var ackCalled, nackCalled bool
	msg := gopipe.NewMessage(
		nil,
		42,
		time.Time{},
		func() { ackCalled = true },
		func(error) { nackCalled = true },
	)

	// Nack first
	if !msg.Nack(errors.New("error")) {
		t.Error("Expected Nack to return true")
	}

	// Ack after nack should fail
	if msg.Ack() {
		t.Error("Expected Ack after Nack to return false")
	}

	if !nackCalled {
		t.Error("Expected nack to be called")
	}

	if ackCalled {
		t.Error("Expected ack NOT to be called after nack")
	}
}

func TestMessage_NackAfterAck(t *testing.T) {
	t.Parallel()

	var ackCalled, nackCalled bool
	msg := gopipe.NewMessage(
		nil,
		42,
		time.Time{},
		func() { ackCalled = true },
		func(error) { nackCalled = true },
	)

	// Ack first
	if !msg.Ack() {
		t.Error("Expected Ack to return true")
	}

	// Nack after ack should fail
	if msg.Nack(errors.New("error")) {
		t.Error("Expected Nack after Ack to return false")
	}

	if !ackCalled {
		t.Error("Expected ack to be called")
	}

	if nackCalled {
		t.Error("Expected nack NOT to be called after ack")
	}
}

func TestMessage_AckWithoutHandler(t *testing.T) {
	t.Parallel()

	// Message without ack handler
	msg := gopipe.NewMessage(nil, 42, time.Time{}, nil, func(error) {})

	// Ack should return false when no handler is set
	if msg.Ack() {
		t.Error("Expected Ack to return false when no handler is set")
	}
}

func TestMessage_NackWithoutHandler(t *testing.T) {
	t.Parallel()

	// Message without nack handler
	msg := gopipe.NewMessage(nil, 42, time.Time{}, func() {}, nil)

	// Nack should return false when no handler is set
	if msg.Nack(errors.New("error")) {
		t.Error("Expected Nack to return false when no handler is set")
	}
}

func TestMessage_ConcurrentAck(t *testing.T) {
	t.Parallel()

	var ackCount int
	var mu sync.Mutex
	msg := gopipe.NewMessage(
		nil,
		42,
		time.Time{},
		func() {
			mu.Lock()
			ackCount++
			mu.Unlock()
		},
		func(error) {},
	)

	// Call Ack concurrently
	const goroutines = 100
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			msg.Ack()
		}()
	}

	wg.Wait()

	// Verify ack was called exactly once despite concurrent calls
	mu.Lock()
	defer mu.Unlock()

	if ackCount != 1 {
		t.Errorf("Expected ack to be called exactly once, got %d", ackCount)
	}
}

func TestMessage_ConcurrentNack(t *testing.T) {
	t.Parallel()

	var nackCount int
	var mu sync.Mutex
	msg := gopipe.NewMessage(
		nil,
		42,
		time.Time{},
		func() {},
		func(err error) {
			mu.Lock()
			nackCount++
			mu.Unlock()
		},
	)

	// Call Nack concurrently
	const goroutines = 100
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func(i int) {
			defer wg.Done()
			msg.Nack(errors.New("error"))
		}(i)
	}

	wg.Wait()

	// Verify nack was called exactly once despite concurrent calls
	mu.Lock()
	defer mu.Unlock()

	if nackCount != 1 {
		t.Errorf("Expected nack to be called exactly once, got %d", nackCount)
	}
}

func TestMessage_ConcurrentAckNack(t *testing.T) {
	t.Parallel()

	var ackCount, nackCount int
	var mu sync.Mutex

	msg := gopipe.NewMessage(
		nil,
		42,
		time.Time{},
		func() {
			mu.Lock()
			ackCount++
			mu.Unlock()
		},
		func(err error) {
			mu.Lock()
			nackCount++
			mu.Unlock()
		},
	)

	// Call Ack and Nack concurrently
	const goroutines = 100
	var wg sync.WaitGroup
	wg.Add(goroutines * 2)

	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			msg.Ack()
		}()
		go func() {
			defer wg.Done()
			msg.Nack(errors.New("error"))
		}()
	}

	wg.Wait()

	// Verify exactly one of ack or nack was called
	mu.Lock()
	defer mu.Unlock()

	total := ackCount + nackCount
	if total != 1 {
		t.Errorf("Expected exactly one of ack or nack to be called, got %d acks and %d nacks", ackCount, nackCount)
	}
}

func TestMessage_CopyMessage(t *testing.T) {
	t.Parallel()

	metadata := gopipe.Metadata{"key": "value"}
	deadline := time.Now().Add(1 * time.Hour)
	var ackCalled bool

	original := gopipe.NewMessage(
		metadata,
		"original",
		deadline,
		func() { ackCalled = true },
		func(error) {},
	)

	// Copy with new payload
	copy := gopipe.CopyMessage(original, "copied")

	if copy.Payload != "copied" {
		t.Errorf("Expected Payload 'copied', got %q", copy.Payload)
	}

	if copy.Metadata["key"] != "value" {
		t.Errorf("Expected Metadata key='value', got %v", copy.Metadata["key"])
	}

	if copy.Deadline() != deadline {
		t.Errorf("Expected deadline %v, got %v", deadline, copy.Deadline())
	}

	// Verify ack/nack are shared
	if !copy.Ack() {
		t.Error("Expected Ack to succeed on copied message")
	}

	if !ackCalled {
		t.Error("Expected ack to be called via copied message")
	}

	// Original should also be acked
	if !original.Ack() {
		t.Error("Expected original to report as acked")
	}
}

func TestMessage_Metadata(t *testing.T) {
	t.Parallel()

	metadata := gopipe.Metadata{
		"key1": "value1",
		"key2": 123,
	}

	msg := gopipe.NewMessage(metadata, 42, time.Time{}, nil, nil)

	if msg.Metadata["key1"] != "value1" {
		t.Errorf("Expected metadata key1='value1', got %v", msg.Metadata["key1"])
	}

	if msg.Metadata["key2"] != 123 {
		t.Errorf("Expected metadata key2=123, got %v", msg.Metadata["key2"])
	}

	// Modify metadata
	msg.Metadata["key3"] = "value3"

	if msg.Metadata["key3"] != "value3" {
		t.Errorf("Expected metadata key3='value3', got %v", msg.Metadata["key3"])
	}
}

func TestMessage_Deadline(t *testing.T) {
	t.Parallel()

	deadline := time.Now().Add(1 * time.Hour)
	msg := gopipe.NewMessage(nil, 42, deadline, nil, nil)

	if msg.Deadline() != deadline {
		t.Errorf("Expected deadline %v, got %v", deadline, msg.Deadline())
	}

	// Zero deadline
	msg2 := gopipe.NewMessage(nil, 42, time.Time{}, nil, nil)

	if !msg2.Deadline().IsZero() {
		t.Errorf("Expected zero deadline, got %v", msg2.Deadline())
	}
}

func TestMessage_PayloadTypes(t *testing.T) {
	t.Parallel()

	t.Run("String", func(t *testing.T) {
		msg := gopipe.NewMessage(nil, "string payload", time.Time{}, nil, nil)
		if msg.Payload != "string payload" {
			t.Errorf("Expected 'string payload', got %q", msg.Payload)
		}
	})

	t.Run("Int", func(t *testing.T) {
		msg := gopipe.NewMessage(nil, 42, time.Time{}, nil, nil)
		if msg.Payload != 42 {
			t.Errorf("Expected 42, got %d", msg.Payload)
		}
	})

	t.Run("Struct", func(t *testing.T) {
		type Data struct {
			Field string
		}
		msg := gopipe.NewMessage(nil, Data{Field: "test"}, time.Time{}, nil, nil)
		if msg.Payload.Field != "test" {
			t.Errorf("Expected 'test', got %q", msg.Payload.Field)
		}
	})

	t.Run("Pointer", func(t *testing.T) {
		val := "pointer"
		msg := gopipe.NewMessage(nil, &val, time.Time{}, nil, nil)
		if *msg.Payload != "pointer" {
			t.Errorf("Expected 'pointer', got %q", *msg.Payload)
		}
	})

	t.Run("Slice", func(t *testing.T) {
		msg := gopipe.NewMessage(nil, []int{1, 2, 3}, time.Time{}, nil, nil)
		if len(msg.Payload) != 3 {
			t.Errorf("Expected length 3, got %d", len(msg.Payload))
		}
	})

	t.Run("Map", func(t *testing.T) {
		msg := gopipe.NewMessage(nil, map[string]int{"key": 42}, time.Time{}, nil, nil)
		if msg.Payload["key"] != 42 {
			t.Errorf("Expected 42, got %d", msg.Payload["key"])
		}
	})
}

func TestNewMessagePipe_AutomaticAck(t *testing.T) {
	t.Parallel()

	// Track ack/nack calls
	var (
		ackCalled  bool
		nackCalled bool
		mu         sync.Mutex
	)

	msg := gopipe.NewMessage(
		nil,
		42,
		time.Time{},
		func() {
			mu.Lock()
			ackCalled = true
			mu.Unlock()
		},
		func(error) {
			mu.Lock()
			nackCalled = true
			mu.Unlock()
		},
	)

	// Create a pipe that processes successfully
	pipe := gopipe.NewMessagePipe(
		func(ctx context.Context, value int) ([]int, error) {
			return []int{value * 2}, nil
		},
	)

	// Process the message
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	in := make(chan *gopipe.Message[int], 1)
	in <- msg
	close(in)

	out := pipe.Start(ctx, in)

	// Collect results
	var results []*gopipe.Message[int]
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

func TestNewMessagePipe_AutomaticNack(t *testing.T) {
	t.Parallel()

	// Track ack/nack calls
	var (
		ackCalled  bool
		nackCalled bool
		nackErr    error
		mu         sync.Mutex
	)

	msg := gopipe.NewMessage(
		nil,
		42,
		time.Time{},
		func() {
			mu.Lock()
			ackCalled = true
			mu.Unlock()
		},
		func(err error) {
			mu.Lock()
			nackCalled = true
			nackErr = err
			mu.Unlock()
		},
	)

	// Error to return
	testErr := errors.New("processing failed")

	// Create a pipe that fails
	pipe := gopipe.NewMessagePipe(
		func(ctx context.Context, value int) ([]int, error) {
			return nil, testErr
		},
	)

	// Process the message
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	in := make(chan *gopipe.Message[int], 1)
	in <- msg
	close(in)

	out := pipe.Start(ctx, in)

	// Drain results (should be empty)
	for range out {
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

func TestNewMessagePipe_MessageDeadline(t *testing.T) {
	t.Parallel()

	// Track ack/nack calls
	var (
		nackCalled bool
		nackErr    error
		mu         sync.Mutex
	)

	// Create a message with a very short deadline
	msg := gopipe.NewMessage(
		nil,
		42,
		time.Now().Add(10*time.Millisecond),
		func() {},
		func(err error) {
			mu.Lock()
			nackCalled = true
			nackErr = err
			mu.Unlock()
		},
	)

	// Create a pipe with a slow handler
	pipe := gopipe.NewMessagePipe(
		func(ctx context.Context, value int) ([]int, error) {
			// Wait for context to be done
			<-ctx.Done()
			return nil, ctx.Err()
		},
	)

	// Process the message
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	in := make(chan *gopipe.Message[int], 1)
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

func TestNewMessagePipe_WithAdditionalCancel(t *testing.T) {
	t.Parallel()

	// Track both cancel functions
	var (
		msgNackCalled      bool
		additionalCanceled bool
		msgNackErr         error
		additionalErr      error
		mu                 sync.Mutex
	)

	msg := gopipe.NewMessage(
		nil,
		42,
		time.Time{},
		func() {},
		func(err error) {
			mu.Lock()
			msgNackCalled = true
			msgNackErr = err
			mu.Unlock()
		},
	)

	testErr := errors.New("processing failed")

	// Create a pipe that fails with an additional cancel function
	pipe := gopipe.NewMessagePipe(
		func(ctx context.Context, value int) ([]int, error) {
			return nil, testErr
		},
		gopipe.WithCancel[*gopipe.Message[int], *gopipe.Message[int]](
			func(msg *gopipe.Message[int], err error) {
				mu.Lock()
				additionalCanceled = true
				additionalErr = err
				mu.Unlock()
			},
		),
	)

	// Process the message
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	in := make(chan *gopipe.Message[int], 1)
	in <- msg
	close(in)

	out := pipe.Start(ctx, in)

	// Drain results
	for range out {
	}

	// Give time for cancel goroutines to complete
	time.Sleep(50 * time.Millisecond)

	// Verify both cancel functions were called
	mu.Lock()
	defer mu.Unlock()

	if !msgNackCalled {
		t.Error("Expected message nack to be called")
	}

	if !additionalCanceled {
		t.Error("Expected additional cancel to be called")
	}

	if msgNackErr == nil {
		t.Error("Expected message nack to receive an error")
	}

	if additionalErr == nil {
		t.Error("Expected additional cancel to receive an error")
	}

	// Both should receive the same error
	if !errors.Is(msgNackErr, testErr) {
		t.Errorf("Expected message nack error to wrap test error, got: %v", msgNackErr)
	}

	if !errors.Is(additionalErr, testErr) {
		t.Errorf("Expected additional cancel error to wrap test error, got: %v", additionalErr)
	}
}

func TestNewMessagePipe_MultipleCancelsExecutionOrder(t *testing.T) {
	t.Parallel()

	// Track execution order
	var (
		callOrder []string
		mu        sync.Mutex
	)

	msg := gopipe.NewMessage(
		nil,
		42,
		time.Time{},
		func() {},
		func(err error) {
			mu.Lock()
			callOrder = append(callOrder, "message-nack")
			mu.Unlock()
		},
	)

	testErr := errors.New("processing failed")

	// Create a pipe with multiple cancel functions
	// The message nack is prepended first by NewMessagePipe
	pipe := gopipe.NewMessagePipe(
		func(ctx context.Context, value int) ([]int, error) {
			return nil, testErr
		},
		gopipe.WithCancel[*gopipe.Message[int], *gopipe.Message[int]](
			func(msg *gopipe.Message[int], err error) {
				mu.Lock()
				callOrder = append(callOrder, "cancel1")
				mu.Unlock()
			},
		),
		gopipe.WithCancel[*gopipe.Message[int], *gopipe.Message[int]](
			func(msg *gopipe.Message[int], err error) {
				mu.Lock()
				callOrder = append(callOrder, "cancel2")
				mu.Unlock()
			},
		),
	)

	// Process the message
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	in := make(chan *gopipe.Message[int], 1)
	in <- msg
	close(in)

	out := pipe.Start(ctx, in)

	// Drain results
	for range out {
	}

	// Give time for cancel goroutines to complete
	time.Sleep(50 * time.Millisecond)

	// Verify execution order
	mu.Lock()
	defer mu.Unlock()

	// Expected order: cancel2, cancel1, message-nack (LIFO)
	// NewMessagePipe prepends message-nack first, then user adds cancel1, cancel2
	// Execution is reverse: cancel2, cancel1, message-nack
	expectedOrder := []string{"cancel2", "cancel1", "message-nack"}

	if len(callOrder) != len(expectedOrder) {
		t.Errorf("Expected %d cancel calls, got %d: %v", len(expectedOrder), len(callOrder), callOrder)
	}

	for i, expected := range expectedOrder {
		if i >= len(callOrder) {
			t.Errorf("Missing call at position %d, expected %s", i, expected)
			continue
		}
		if callOrder[i] != expected {
			t.Errorf("Call order at position %d: expected %s, got %s", i, expected, callOrder[i])
		}
	}
}

func TestNewMessagePipe_MessageNackNotOverridden(t *testing.T) {
	t.Parallel()

	// Verify that adding WithCancel doesn't prevent message nack from being called
	var (
		msgNackCalled bool
		msgNackErr    error
		mu            sync.Mutex
	)

	msg := gopipe.NewMessage(
		nil,
		42,
		time.Time{},
		func() {},
		func(err error) {
			mu.Lock()
			msgNackCalled = true
			msgNackErr = err
			mu.Unlock()
		},
	)

	testErr := errors.New("test error")

	// Create pipe with multiple cancel handlers
	pipe := gopipe.NewMessagePipe(
		func(ctx context.Context, value int) ([]int, error) {
			return nil, testErr
		},
		gopipe.WithCancel[*gopipe.Message[int], *gopipe.Message[int]](
			func(msg *gopipe.Message[int], err error) {
				// Additional cancel handler
			},
		),
		gopipe.WithCancel[*gopipe.Message[int], *gopipe.Message[int]](
			func(msg *gopipe.Message[int], err error) {
				// Another cancel handler
			},
		),
		gopipe.WithCancel[*gopipe.Message[int], *gopipe.Message[int]](
			func(msg *gopipe.Message[int], err error) {
				// Yet another cancel handler
			},
		),
	)

	// Process the message
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	in := make(chan *gopipe.Message[int], 1)
	in <- msg
	close(in)

	out := pipe.Start(ctx, in)

	// Drain results
	for range out {
	}

	// Give time for cancel goroutines to complete
	time.Sleep(50 * time.Millisecond)

	// Verify message nack was called despite multiple WithCancel options
	mu.Lock()
	defer mu.Unlock()

	if !msgNackCalled {
		t.Error("Expected message nack to be called even with multiple WithCancel options")
	}

	if msgNackErr == nil {
		t.Error("Expected message nack to receive an error")
	}

	if !errors.Is(msgNackErr, testErr) {
		t.Errorf("Expected message nack error to wrap test error, got: %v", msgNackErr)
	}
}
