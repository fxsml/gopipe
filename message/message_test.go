package message_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/fxsml/gopipe/message"
)

func TestMessage_NewMessage(t *testing.T) {
	t.Parallel()

	deadline := time.Now().Add(1 * time.Hour)
	ctx, cancel := context.WithDeadline(context.Background(), deadline)
	defer cancel()
	var ackCalled, nackCalled bool

	msg := message.New("payload",
		message.WithContext[string](ctx),
		message.WithAcking[string](
			func() { ackCalled = true },
			func(error) { nackCalled = true },
		),
		message.WithProperty[string]("key", "value"),
	)

	if val, ok := msg.Properties().Get("key"); !ok || val != "value" {
		t.Errorf("Expected properties key='value', got %v", val)
	}

	if msg.Payload() != "payload" {
		t.Errorf("Expected Payload 'payload', got %v", msg.Payload())
	}

	if msgDeadline, ok := msg.Deadline(); !ok || msgDeadline != deadline {
		t.Errorf("Expected deadline %v, got %v (ok=%v)", deadline, msgDeadline, ok)
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
	msg := message.New(42,
		message.WithContext[int](context.Background()),
		message.WithAcking[int](
			func() { ackCount++ },
			func(error) {},
		),
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
	msg := message.New(42,
		message.WithContext[int](context.Background()),
		message.WithAcking[int](
			func() {},
			func(err error) {
				nackCount++
				capturedErr = err
			},
		),
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

func TestMessage_Copy(t *testing.T) {
	t.Parallel()

	deadline := time.Now().Add(1 * time.Hour)
	ctx, cancel := context.WithDeadline(context.Background(), deadline)
	defer cancel()
	var ackCalled bool

	original := message.New("original",
		message.WithContext[string](ctx),
		message.WithAcking[string](
			func() { ackCalled = true },
			func(error) {},
		),
		message.WithProperty[string]("key", "value"),
	)

	// Copy with new payload
	copy := message.Copy(original, "copied")

	// Type assert the payload
	if copy.Payload() != "copied" {
		t.Errorf("Expected Payload 'copied', got %v", copy.Payload())
	}

	if val, ok := copy.Properties().Get("key"); !ok || val != "value" {
		t.Errorf("Expected properties key='value', got %v", val)
	}

	if copyDeadline, ok := copy.Deadline(); !ok || copyDeadline != deadline {
		t.Errorf("Expected deadline %v, got %v (ok=%v)", deadline, copyDeadline, ok)
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

func TestMessage_PayloadTypes(t *testing.T) {
	t.Parallel()

	t.Run("String", func(t *testing.T) {
		msg := message.New("string payload")
		if msg.Payload() != "string payload" {
			t.Errorf("Expected 'string payload', got %v", msg.Payload())
		}
	})

	t.Run("Int", func(t *testing.T) {
		msg := message.New(42)
		if msg.Payload() != 42 {
			t.Errorf("Expected 42, got %v", msg.Payload())
		}
	})

	t.Run("Struct", func(t *testing.T) {
		type Data struct {
			Field string
		}
		msg := message.New(Data{Field: "test"})
		if msg.Payload() != (Data{Field: "test"}) {
			t.Errorf("Expected Data{Field: 'test'}, got %v", msg.Payload())
		}
	})
}

func TestMessage_MultiStageAcking(t *testing.T) {
	t.Parallel()

	var ackCalled bool
	msg := message.New(42,
		message.WithContext[int](context.Background()),
		message.WithAcking[int](
			func() { ackCalled = true },
			func(error) {},
		),
	)

	// Set expected ack count to 2
	if !msg.SetExpectedAckCount(2) {
		t.Fatal("Expected SetExpectedAckCount to succeed")
	}

	// First ack should not trigger callback
	if !msg.Ack() {
		t.Error("Expected first Ack to return true")
	}

	if ackCalled {
		t.Error("Expected ack callback NOT to be called after first ack")
	}

	// Second ack should trigger callback
	if !msg.Ack() {
		t.Error("Expected second Ack to return true")
	}

	if !ackCalled {
		t.Error("Expected ack callback to be called after second ack")
	}
}

func TestMessageProperties_ConcurrentAccess(t *testing.T) {
	t.Parallel()

	msg := message.New("payload",
		message.WithProperty[string]("initial", "value"),
	)

	const numGoroutines = 50
	const numOperations = 100

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	// Launch multiple goroutines performing concurrent reads and writes
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < numOperations; j++ {
				// Concurrent writes
				key := "key" + string(rune(id))
				msg.Properties().Set(key, id*numOperations+j)

				// Concurrent reads
				msg.Properties().Get(key)

				// Concurrent iteration
				count := 0
				msg.Properties().Range(func(k string, v any) bool {
					count++
					return true
				})

				// Concurrent deletes (occasionally)
				if j%10 == 0 {
					msg.Properties().Delete(key)
				}
			}
		}(i)
	}

	wg.Wait()

	// Verify the message is still functional
	if val, ok := msg.Properties().Get("initial"); !ok || val != "value" {
		t.Errorf("Expected initial value to be preserved, got %v", val)
	}
}

// TestMessage_PropertiesSetAfterCreation verifies that properties can be set
// using msg.Properties().Set() after the message is created with NewMessage.
func TestMessage_PropertiesSetAfterCreation(t *testing.T) {
	t.Parallel()

	t.Run("SetPropertiesOnNilProperties", func(t *testing.T) {
		// Create minimal message - properties are always created
		msg := message.New("payload")

		// Properties should exist
		props := msg.Properties()
		if props == nil {
			t.Error("Expected Properties() to return non-nil")
		}
	})

	t.Run("SetPropertiesOnEmptyProperties", func(t *testing.T) {
		msg := message.New("payload")

		// Set properties after message creation
		msg.Properties().Set("key1", "value1")
		msg.Properties().Set("key2", 42)
		msg.Properties().Set("key3", true)

		// Verify all properties were set correctly
		if val, ok := msg.Properties().Get("key1"); !ok || val != "value1" {
			t.Errorf("Expected key1='value1', got %v (ok=%v)", val, ok)
		}

		if val, ok := msg.Properties().Get("key2"); !ok || val != 42 {
			t.Errorf("Expected key2=42, got %v (ok=%v)", val, ok)
		}

		if val, ok := msg.Properties().Get("key3"); !ok || val != true {
			t.Errorf("Expected key3=true, got %v (ok=%v)", val, ok)
		}
	})

	t.Run("MixPreAndPostCreationProperties", func(t *testing.T) {
		msg := message.New("payload",
			message.WithProperty[string]("initial1", "value1"),
			message.WithProperty[string]("initial2", "value2"),
		)

		// Verify initial properties exist
		if val, ok := msg.Properties().Get("initial1"); !ok || val != "value1" {
			t.Errorf("Expected initial1='value1', got %v (ok=%v)", val, ok)
		}

		// Add more properties after message creation
		msg.Properties().Set("added1", "new_value1")
		msg.Properties().Set("added2", 100)

		// Verify all properties are accessible
		if val, ok := msg.Properties().Get("initial1"); !ok || val != "value1" {
			t.Errorf("Expected initial1='value1', got %v", val)
		}

		if val, ok := msg.Properties().Get("initial2"); !ok || val != "value2" {
			t.Errorf("Expected initial2='value2', got %v", val)
		}

		if val, ok := msg.Properties().Get("added1"); !ok || val != "new_value1" {
			t.Errorf("Expected added1='new_value1', got %v", val)
		}

		if val, ok := msg.Properties().Get("added2"); !ok || val != 100 {
			t.Errorf("Expected added2=100, got %v", val)
		}

		// Verify properties count using Range
		count := 0
		msg.Properties().Range(func(k string, v any) bool {
			count++
			return true
		})

		if count != 4 {
			t.Errorf("Expected 4 properties total, got %d", count)
		}
	})

	t.Run("UpdateExistingProperty", func(t *testing.T) {
		msg := message.New("payload",
			message.WithProperty[string]("key", "original"),
		)

		// Verify original value
		if val, ok := msg.Properties().Get("key"); !ok || val != "original" {
			t.Errorf("Expected key='original', got %v", val)
		}

		// Update the property after message creation
		msg.Properties().Set("key", "updated")

		// Verify updated value
		if val, ok := msg.Properties().Get("key"); !ok || val != "updated" {
			t.Errorf("Expected key='updated', got %v", val)
		}
	})

	t.Run("DeletePropertyAfterCreation", func(t *testing.T) {
		msg := message.New("payload",
			message.WithProperty[string]("key1", "value1"),
			message.WithProperty[string]("key2", "value2"),
		)

		// Delete a property after message creation
		msg.Properties().Delete("key1")

		// Verify key1 is deleted
		if val, ok := msg.Properties().Get("key1"); ok {
			t.Errorf("Expected key1 to be deleted, but got value: %v", val)
		}

		// Verify key2 still exists
		if val, ok := msg.Properties().Get("key2"); !ok || val != "value2" {
			t.Errorf("Expected key2='value2', got %v (ok=%v)", val, ok)
		}
	})

	t.Run("PropertiesSharedWithCopiedMessage", func(t *testing.T) {
		original := message.New("original_payload",
			message.WithProperty[string]("initial", "value"),
		)

		// Copy the message
		copied := message.Copy(original, "copied_payload")

		// Set property on original message
		original.Properties().Set("from_original", "orig_val")

		// Verify copied message sees the same property (properties are shared)
		if val, ok := copied.Properties().Get("from_original"); !ok || val != "orig_val" {
			t.Errorf("Expected copied message to see property set on original, got %v (ok=%v)", val, ok)
		}

		// Set property on copied message
		copied.Properties().Set("from_copied", "copy_val")

		// Verify original message sees the property (properties are shared)
		if val, ok := original.Properties().Get("from_copied"); !ok || val != "copy_val" {
			t.Errorf("Expected original message to see property set on copied, got %v (ok=%v)", val, ok)
		}
	})
}

// Helper function to check if a string contains a substring
func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && containsStringHelper(s, substr))
}

func containsStringHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// TestMessage_NewWithOptions tests the new functional options API
func TestMessage_NewWithOptions(t *testing.T) {
	t.Parallel()

	t.Run("MinimalMessage", func(t *testing.T) {
		msg := message.New(42)

		if msg.Payload() != 42 {
			t.Errorf("Expected payload 42, got %v", msg.Payload())
		}

		if msg.Properties() == nil {
			t.Error("Expected properties to be initialized")
		}
	})

	t.Run("WithContext", func(t *testing.T) {
		deadline := time.Now().Add(1 * time.Hour)
		ctx, cancel := context.WithDeadline(context.Background(), deadline)
		defer cancel()

		msg := message.New("payload", message.WithContext[string](ctx))

		if msgDeadline, ok := msg.Deadline(); !ok || msgDeadline != deadline {
			t.Errorf("Expected deadline %v, got %v (ok=%v)", deadline, msgDeadline, ok)
		}
	})

	t.Run("WithAcking", func(t *testing.T) {
		var ackCalled, nackCalled bool

		msg := message.New(42,
			message.WithAcking[int](
				func() { ackCalled = true },
				func(error) { nackCalled = true },
			),
		)

		if !msg.Ack() {
			t.Error("Expected Ack to return true")
		}

		if !ackCalled {
			t.Error("Expected ack callback to be called")
		}

		if nackCalled {
			t.Error("Expected nack callback NOT to be called")
		}
	})

	t.Run("WithID", func(t *testing.T) {
		msg := message.New("payload", message.WithID[string]("msg-001"))

		if msg.ID() != "msg-001" {
			t.Errorf("Expected ID 'msg-001', got %q", msg.ID())
		}

		// Also test via Properties
		if msg.Properties().ID() != "msg-001" {
			t.Errorf("Expected ID 'msg-001' via Properties, got %q", msg.Properties().ID())
		}
	})

	t.Run("WithCorrelationID", func(t *testing.T) {
		msg := message.New("payload", message.WithCorrelationID[string]("corr-123"))

		if msg.CorrelationID() != "corr-123" {
			t.Errorf("Expected CorrelationID 'corr-123', got %q", msg.CorrelationID())
		}
	})

	t.Run("WithProperty", func(t *testing.T) {
		msg := message.New("payload",
			message.WithProperty[string]("tenant", "acme-corp"),
			message.WithProperty[string]("priority", 5),
		)

		if val, ok := msg.Properties().Get("tenant"); !ok || val != "acme-corp" {
			t.Errorf("Expected tenant='acme-corp', got %v (ok=%v)", val, ok)
		}

		if val, ok := msg.Properties().Get("priority"); !ok || val != 5 {
			t.Errorf("Expected priority=5, got %v (ok=%v)", val, ok)
		}
	})

	t.Run("CombinedOptions", func(t *testing.T) {
		deadline := time.Now().Add(1 * time.Hour)
		ctx, cancel := context.WithDeadline(context.Background(), deadline)
		defer cancel()

		var ackCalled bool

		msg := message.New("order-data",
			message.WithContext[string](ctx),
			message.WithAcking[string](
				func() { ackCalled = true },
				func(error) {},
			),
			message.WithID[string]("msg-001"),
			message.WithCorrelationID[string]("order-123"),
			message.WithProperty[string]("tenant", "acme"),
			message.WithProperty[string]("source", "orders-queue"),
		)

		// Verify payload
		if msg.Payload() != "order-data" {
			t.Errorf("Expected payload 'order-data', got %v", msg.Payload())
		}

		// Verify ID
		if msg.ID() != "msg-001" {
			t.Errorf("Expected ID 'msg-001', got %q", msg.ID())
		}

		// Verify CorrelationID
		if msg.CorrelationID() != "order-123" {
			t.Errorf("Expected CorrelationID 'order-123', got %q", msg.CorrelationID())
		}

		// Verify custom properties
		if val, ok := msg.Properties().Get("tenant"); !ok || val != "acme" {
			t.Errorf("Expected tenant='acme', got %v (ok=%v)", val, ok)
		}

		if val, ok := msg.Properties().Get("source"); !ok || val != "orders-queue" {
			t.Errorf("Expected source='orders-queue', got %v (ok=%v)", val, ok)
		}

		// Verify deadline
		if msgDeadline, ok := msg.Deadline(); !ok || msgDeadline != deadline {
			t.Errorf("Expected deadline %v, got %v (ok=%v)", deadline, msgDeadline, ok)
		}

		// Verify acking
		if !msg.Ack() {
			t.Error("Expected Ack to return true")
		}

		if !ackCalled {
			t.Error("Expected ack callback to be called")
		}
	})

	t.Run("WithProperties", func(t *testing.T) {
		msg := message.New("payload",
			message.WithID[string]("msg-001"),
			message.WithProperty[string]("initial", "value"),
			message.WithProperties[string](map[string]any{
				"tenant":   "acme-corp",
				"priority": 5,
			}),
			message.WithCorrelationID[string]("corr-123"),
		)

		// Verify that ID set before WithProperties is preserved
		if msg.ID() != "msg-001" {
			t.Errorf("Expected ID 'msg-001', got %q", msg.ID())
		}

		// Verify that property set before WithProperties is preserved
		if val, ok := msg.Properties().Get("initial"); !ok || val != "value" {
			t.Errorf("Expected initial='value', got %v (ok=%v)", val, ok)
		}

		// Verify that properties from WithProperties are added
		if val, ok := msg.Properties().Get("tenant"); !ok || val != "acme-corp" {
			t.Errorf("Expected tenant='acme-corp', got %v (ok=%v)", val, ok)
		}

		if val, ok := msg.Properties().Get("priority"); !ok || val != 5 {
			t.Errorf("Expected priority=5, got %v (ok=%v)", val, ok)
		}

		// Verify that CorrelationID set after WithProperties is preserved
		if msg.CorrelationID() != "corr-123" {
			t.Errorf("Expected CorrelationID 'corr-123', got %q", msg.CorrelationID())
		}

		// Count all properties
		count := 0
		msg.Properties().Range(func(key string, value any) bool {
			count++
			return true
		})

		// Should have: initial, tenant, priority, ID, CorrelationID = 5 properties
		if count != 5 {
			t.Errorf("Expected 5 properties total, got %d", count)
		}
	})
}

// TestMessage_TypedAccessors tests the typed accessor methods
func TestMessage_TypedAccessors(t *testing.T) {
	t.Parallel()

	t.Run("GettersViaOptions", func(t *testing.T) {
		now := time.Now()
		msg := message.New(42,
			message.WithID[int]("msg-001"),
			message.WithCorrelationID[int]("corr-123"),
			message.WithProperty[int](message.PropCreatedAt, now),
			message.WithProperty[int](message.PropRetryCount, 3),
		)

		// Test ID
		if msg.ID() != "msg-001" {
			t.Errorf("Expected ID 'msg-001', got %q", msg.ID())
		}

		// Test CorrelationID
		if msg.CorrelationID() != "corr-123" {
			t.Errorf("Expected CorrelationID 'corr-123', got %q", msg.CorrelationID())
		}

		// Test CreatedAt
		if !msg.CreatedAt().Equal(now) {
			t.Errorf("Expected CreatedAt %v, got %v", now, msg.CreatedAt())
		}

		// Test RetryCount
		if msg.RetryCount() != 3 {
			t.Errorf("Expected RetryCount 3, got %d", msg.RetryCount())
		}
	})

	t.Run("SetPropertiesViaSet", func(t *testing.T) {
		msg := message.New(42)

		// Set reserved properties via Properties.Set() (only during creation or special cases)
		// Note: In production, reserved properties should be set via functional options
		now := time.Now()
		msg.Properties().Set(message.PropID, "msg-001")
		msg.Properties().Set(message.PropCorrelationID, "corr-123")
		msg.Properties().Set(message.PropCreatedAt, now)
		msg.Properties().Set(message.PropRetryCount, 3)

		// Get via message shortcuts
		if msg.ID() != "msg-001" {
			t.Errorf("Expected ID 'msg-001', got %q", msg.ID())
		}

		if msg.CorrelationID() != "corr-123" {
			t.Errorf("Expected CorrelationID 'corr-123', got %q", msg.CorrelationID())
		}

		if !msg.CreatedAt().Equal(now) {
			t.Errorf("Expected CreatedAt %v, got %v", now, msg.CreatedAt())
		}

		if msg.RetryCount() != 3 {
			t.Errorf("Expected RetryCount 3, got %d", msg.RetryCount())
		}
	})

	t.Run("IncrementRetryCount", func(t *testing.T) {
		msg := message.New(42)

		count := msg.Properties().IncrementRetryCount()
		if count != 1 {
			t.Errorf("Expected IncrementRetryCount to return 1, got %d", count)
		}

		if msg.RetryCount() != 1 {
			t.Errorf("Expected RetryCount 1, got %d", msg.RetryCount())
		}

		count = msg.Properties().IncrementRetryCount()
		if count != 2 {
			t.Errorf("Expected IncrementRetryCount to return 2, got %d", count)
		}
	})
}

// TestMessageProperties_ConcurrentIncrementRetryCount verifies that IncrementRetryCount
// is properly atomic and all concurrent increments are counted without race conditions.
func TestMessageProperties_ConcurrentIncrementRetryCount(t *testing.T) {
	t.Parallel()

	msg := message.New("payload")

	const numGoroutines = 100
	const incrementsPerGoroutine = 100
	const expectedTotal = numGoroutines * incrementsPerGoroutine

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	// Launch goroutines that concurrently increment retry count
	for range numGoroutines {
		go func() {
			defer wg.Done()
			for range incrementsPerGoroutine {
				msg.Properties().IncrementRetryCount()
			}
		}()
	}

	// Wait for all goroutines to complete
	wg.Wait()

	// Verify final count - all increments should be counted
	finalCount := msg.RetryCount()
	if finalCount != expectedTotal {
		t.Errorf("Expected final retry count to be %d, got %d (lost %d increments)",
			expectedTotal, finalCount, expectedTotal-finalCount)
	}
}
