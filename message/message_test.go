package message_test

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/fxsml/gopipe/message"
)

func TestMessage_NewMessage(t *testing.T) {
	t.Parallel()

	msg := message.NewMessage("test-id", "payload")

	if msg.ID != "test-id" {
		t.Errorf("Expected ID 'test-id', got %q", msg.ID)
	}

	if msg.Payload != "payload" {
		t.Errorf("Expected Payload 'payload', got %q", msg.Payload)
	}

	if msg.Metadata == nil {
		t.Error("Expected Metadata to be initialized")
	}
}

func TestMessage_NewMessageWithAck(t *testing.T) {
	t.Parallel()

	var ackCalled, nackCalled bool
	ackFunc := func() { ackCalled = true }
	nackFunc := func(err error) { nackCalled = true }

	msg := message.NewMessageWithAck("test-id", 42, ackFunc, nackFunc)

	if msg.ID != "test-id" {
		t.Errorf("Expected ID 'test-id', got %q", msg.ID)
	}

	if msg.Payload != 42 {
		t.Errorf("Expected Payload 42, got %d", msg.Payload)
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
	ackFunc := func() { ackCount++ }

	msg := message.NewMessageWithAck("test-id", 42, ackFunc, func(error) {})

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
	nackFunc := func(err error) {
		nackCount++
		capturedErr = err
	}

	msg := message.NewMessageWithAck("test-id", 42, func() {}, nackFunc)

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
	ackFunc := func() { ackCalled = true }
	nackFunc := func(err error) { nackCalled = true }

	msg := message.NewMessageWithAck("test-id", 42, ackFunc, nackFunc)

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
	ackFunc := func() { ackCalled = true }
	nackFunc := func(err error) { nackCalled = true }

	msg := message.NewMessageWithAck("test-id", 42, ackFunc, nackFunc)

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
	msg := message.NewMessage("test-id", 42)

	// Ack should return false when no handler is set
	if msg.Ack() {
		t.Error("Expected Ack to return false when no handler is set")
	}
}

func TestMessage_NackWithoutHandler(t *testing.T) {
	t.Parallel()

	// Message without nack handler
	msg := message.NewMessage("test-id", 42)

	// Nack should return false when no handler is set
	if msg.Nack(errors.New("error")) {
		t.Error("Expected Nack to return false when no handler is set")
	}
}

func TestMessage_ConcurrentAck(t *testing.T) {
	t.Parallel()

	var ackCount int
	var mu sync.Mutex
	ackFunc := func() {
		mu.Lock()
		ackCount++
		mu.Unlock()
	}

	msg := message.NewMessageWithAck("test-id", 42, ackFunc, func(error) {})

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
	nackFunc := func(err error) {
		mu.Lock()
		nackCount++
		mu.Unlock()
	}

	msg := message.NewMessageWithAck("test-id", 42, func() {}, nackFunc)

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

	ackFunc := func() {
		mu.Lock()
		ackCount++
		mu.Unlock()
	}

	nackFunc := func(err error) {
		mu.Lock()
		nackCount++
		mu.Unlock()
	}

	msg := message.NewMessageWithAck("test-id", 42, ackFunc, nackFunc)

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

func TestMessage_SetTimeout(t *testing.T) {
	t.Parallel()

	msg := message.NewMessage("test-id", 42)

	// Set a timeout
	msg.SetTimeout(100 * time.Millisecond)

	// We can't directly access deadline, but we can verify behavior through processing
	// This is tested more thoroughly in pipe_test.go
}

func TestMessage_Metadata(t *testing.T) {
	t.Parallel()

	msg := message.NewMessage("test-id", 42)

	// Add metadata
	msg.Metadata["key1"] = "value1"
	msg.Metadata["key2"] = 123

	if msg.Metadata["key1"] != "value1" {
		t.Errorf("Expected metadata key1='value1', got %v", msg.Metadata["key1"])
	}

	if msg.Metadata["key2"] != 123 {
		t.Errorf("Expected metadata key2=123, got %v", msg.Metadata["key2"])
	}
}

func TestMessage_MetadataWithAck(t *testing.T) {
	t.Parallel()

	msg := message.NewMessageWithAck("test-id", 42, func() {}, func(error) {})

	// Verify metadata is initialized
	if msg.Metadata == nil {
		t.Fatal("Expected Metadata to be initialized")
	}

	// Add metadata
	msg.Metadata["key"] = "value"

	if msg.Metadata["key"] != "value" {
		t.Errorf("Expected metadata key='value', got %v", msg.Metadata["key"])
	}
}

func TestMessage_PayloadTypes(t *testing.T) {
	t.Parallel()

	t.Run("String", func(t *testing.T) {
		msg := message.NewMessage("id", "string payload")
		if msg.Payload != "string payload" {
			t.Errorf("Expected 'string payload', got %q", msg.Payload)
		}
	})

	t.Run("Int", func(t *testing.T) {
		msg := message.NewMessage("id", 42)
		if msg.Payload != 42 {
			t.Errorf("Expected 42, got %d", msg.Payload)
		}
	})

	t.Run("Struct", func(t *testing.T) {
		type Data struct {
			Field string
		}
		msg := message.NewMessage("id", Data{Field: "test"})
		if msg.Payload.Field != "test" {
			t.Errorf("Expected 'test', got %q", msg.Payload.Field)
		}
	})

	t.Run("Pointer", func(t *testing.T) {
		val := "pointer"
		msg := message.NewMessage("id", &val)
		if *msg.Payload != "pointer" {
			t.Errorf("Expected 'pointer', got %q", *msg.Payload)
		}
	})

	t.Run("Slice", func(t *testing.T) {
		msg := message.NewMessage("id", []int{1, 2, 3})
		if len(msg.Payload) != 3 {
			t.Errorf("Expected length 3, got %d", len(msg.Payload))
		}
	})

	t.Run("Map", func(t *testing.T) {
		msg := message.NewMessage("id", map[string]int{"key": 42})
		if msg.Payload["key"] != 42 {
			t.Errorf("Expected 42, got %d", msg.Payload["key"])
		}
	})
}

func TestMessage_Integration(t *testing.T) {
	t.Parallel()

	// Simulate a complete message lifecycle
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

	// Create message with metadata and timeout
	msg := message.NewMessageWithAck("order-123", map[string]interface{}{
		"user_id":  "user-456",
		"amount":   99.99,
		"currency": "USD",
	}, ackFunc, nackFunc)

	// Add processing metadata
	msg.Metadata["processing_started"] = time.Now()
	msg.SetTimeout(5 * time.Second)

	// Simulate successful processing
	time.Sleep(10 * time.Millisecond)
	msg.Metadata["processing_completed"] = time.Now()

	// Acknowledge success
	if !msg.Ack() {
		t.Fatal("Expected Ack to succeed")
	}

	// Verify final state
	mu.Lock()
	defer mu.Unlock()

	if !ackCalled {
		t.Error("Expected ack to be called")
	}

	if nackCalled {
		t.Error("Expected nack NOT to be called")
	}

	if msg.Metadata["processing_started"] == nil {
		t.Error("Expected processing_started metadata to be set")
	}

	if msg.Metadata["processing_completed"] == nil {
		t.Error("Expected processing_completed metadata to be set")
	}
}

func TestMessage_ErrorPropagation(t *testing.T) {
	t.Parallel()

	var capturedErr error
	var mu sync.Mutex

	nackFunc := func(err error) {
		mu.Lock()
		capturedErr = err
		mu.Unlock()
	}

	msg := message.NewMessageWithAck("test-id", 42, func() {}, nackFunc)

	// Create a test error
	testErr := errors.New("test error")

	// Nack with error
	if !msg.Nack(testErr) {
		t.Fatal("Expected Nack to succeed")
	}

	// Verify error is captured correctly
	mu.Lock()
	defer mu.Unlock()

	if capturedErr == nil {
		t.Fatal("Expected error to be captured")
	}

	if !errors.Is(capturedErr, testErr) {
		t.Errorf("Expected captured error to be test error, got %v", capturedErr)
	}
}
