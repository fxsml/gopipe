package message_test

import (
	"errors"
	"testing"
	"time"

	"github.com/fxsml/gopipe/message"
)

func TestMessage_NewMessage(t *testing.T) {
	t.Parallel()

	deadline := time.Now().Add(1 * time.Hour)
	var ackCalled, nackCalled bool

	props := message.Properties{
		message.PropDeadline: deadline,
		"key":                "value",
	}
	acking := message.NewAcking(
		func() { ackCalled = true },
		func(error) { nackCalled = true },
	)

	msg := message.New("payload", props, acking)

	if val, ok := msg.Properties["key"]; !ok || val != "value" {
		t.Errorf("Expected properties key='value', got %v", val)
	}

	if msg.Payload != "payload" {
		t.Errorf("Expected Payload 'payload', got %v", msg.Payload)
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
	acking := message.NewAcking(
		func() { ackCount++ },
		func(error) {},
	)
	msg := message.New(42, nil, acking)

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
	acking := message.NewAcking(
		func() {},
		func(err error) {
			nackCount++
			capturedErr = err
		},
	)
	msg := message.New(42, nil, acking)

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
	var ackCalled bool

	props := message.Properties{
		message.PropDeadline: deadline,
		"key":                "value",
	}
	acking := message.NewAcking(
		func() { ackCalled = true },
		func(error) {},
	)

	original := message.New("original", props, acking)

	// Copy with new payload
	copy := message.Copy(original, "copied")

	// Type assert the payload
	if copy.Payload != "copied" {
		t.Errorf("Expected Payload 'copied', got %v", copy.Payload)
	}

	if val, ok := copy.Properties["key"]; !ok || val != "value" {
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
		msg := message.New("string payload", nil, nil)
		if msg.Payload != "string payload" {
			t.Errorf("Expected 'string payload', got %v", msg.Payload)
		}
	})

	t.Run("Int", func(t *testing.T) {
		msg := message.New(42, nil, nil)
		if msg.Payload != 42 {
			t.Errorf("Expected 42, got %v", msg.Payload)
		}
	})

	t.Run("Struct", func(t *testing.T) {
		type Data struct {
			Field string
		}
		msg := message.New(Data{Field: "test"}, nil, nil)
		if msg.Payload != (Data{Field: "test"}) {
			t.Errorf("Expected Data{Field: 'test'}, got %v", msg.Payload)
		}
	})
}

func TestMessage_MultiStageAcking(t *testing.T) {
	t.Parallel()

	var ackCalled bool
	acking := message.NewAcking(
		func() { ackCalled = true },
		func(error) {},
	)
	msg := message.New(42, nil, acking)

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

// NOTE: Concurrent access test removed - Properties is now a plain map[string]any
// without thread-safety. Users must provide their own synchronization if needed.
// See ADR 0002: Remove Properties Thread-Safety

func TestMessage_PropertiesSetAfterCreation(t *testing.T) {
	t.Parallel()

	t.Run("SetPropertiesOnNilProperties", func(t *testing.T) {
		// Create minimal message - properties are always created
		msg := message.New("payload", nil, nil)

		// Properties should exist
		if msg.Properties == nil {
			t.Error("Expected Properties to be non-nil")
		}
	})

	t.Run("SetPropertiesOnEmptyProperties", func(t *testing.T) {
		msg := message.New("payload", nil, nil)

		// Set properties after message creation
		msg.Properties["key1"] = "value1"
		msg.Properties["key2"] = 42
		msg.Properties["key3"] = true

		// Verify all properties were set correctly
		if val, ok := msg.Properties["key1"]; !ok || val != "value1" {
			t.Errorf("Expected key1='value1', got %v (ok=%v)", val, ok)
		}

		if val, ok := msg.Properties["key2"]; !ok || val != 42 {
			t.Errorf("Expected key2=42, got %v (ok=%v)", val, ok)
		}

		if val, ok := msg.Properties["key3"]; !ok || val != true {
			t.Errorf("Expected key3=true, got %v (ok=%v)", val, ok)
		}
	})

	t.Run("MixPreAndPostCreationProperties", func(t *testing.T) {
		props := message.Properties{
			"initial1": "value1",
			"initial2": "value2",
		}
		msg := message.New("payload", props, nil)

		// Verify initial properties exist
		if val, ok := msg.Properties["initial1"]; !ok || val != "value1" {
			t.Errorf("Expected initial1='value1', got %v (ok=%v)", val, ok)
		}

		// Add more properties after message creation
		msg.Properties["added1"] = "new_value1"
		msg.Properties["added2"] = 100

		// Verify all properties are accessible
		if val, ok := msg.Properties["initial1"]; !ok || val != "value1" {
			t.Errorf("Expected initial1='value1', got %v", val)
		}

		if val, ok := msg.Properties["initial2"]; !ok || val != "value2" {
			t.Errorf("Expected initial2='value2', got %v", val)
		}

		if val, ok := msg.Properties["added1"]; !ok || val != "new_value1" {
			t.Errorf("Expected added1='new_value1', got %v", val)
		}

		if val, ok := msg.Properties["added2"]; !ok || val != 100 {
			t.Errorf("Expected added2=100, got %v", val)
		}

		// Verify properties count
		count := 0
		for range msg.Properties {
			count++
		}

		if count != 4 {
			t.Errorf("Expected 4 properties total, got %d", count)
		}
	})

	t.Run("UpdateExistingProperty", func(t *testing.T) {
		props := message.Properties{
			"key": "original",
		}
		msg := message.New("payload", props, nil)

		// Verify original value
		if val, ok := msg.Properties["key"]; !ok || val != "original" {
			t.Errorf("Expected key='original', got %v", val)
		}

		// Update the property after message creation
		msg.Properties["key"] = "updated"

		// Verify updated value
		if val, ok := msg.Properties["key"]; !ok || val != "updated" {
			t.Errorf("Expected key='updated', got %v", val)
		}
	})

	t.Run("DeletePropertyAfterCreation", func(t *testing.T) {
		props := message.Properties{
			"key1": "value1",
			"key2": "value2",
		}
		msg := message.New("payload", props, nil)

		// Delete a property after message creation
		delete(msg.Properties, "key1")

		// Verify key1 is deleted
		if val, ok := msg.Properties["key1"]; ok {
			t.Errorf("Expected key1 to be deleted, but got value: %v", val)
		}

		// Verify key2 still exists
		if val, ok := msg.Properties["key2"]; !ok || val != "value2" {
			t.Errorf("Expected key2='value2', got %v (ok=%v)", val, ok)
		}
	})

	t.Run("PropertiesSharedWithCopiedMessage", func(t *testing.T) {
		props := message.Properties{
			"initial": "value",
		}
		original := message.New("original_payload", props, nil)

		// Copy the message
		copied := message.Copy(original, "copied_payload")

		// Set property on original message
		original.Properties["from_original"] = "orig_val"

		// Verify copied message sees the same property (properties are shared)
		if val, ok := copied.Properties["from_original"]; !ok || val != "orig_val" {
			t.Errorf("Expected copied message to see property set on original, got %v (ok=%v)", val, ok)
		}

		// Set property on copied message
		copied.Properties["from_copied"] = "copy_val"

		// Verify original message sees the property (properties are shared)
		if val, ok := original.Properties["from_copied"]; !ok || val != "copy_val" {
			t.Errorf("Expected original message to see property set on copied, got %v (ok=%v)", val, ok)
		}
	})
}

func TestMessage_NewWithOptions(t *testing.T) {
	t.Parallel()

	t.Run("MinimalMessage", func(t *testing.T) {
		msg := message.New(42, nil, nil)

		if msg.Payload != 42 {
			t.Errorf("Expected payload 42, got %v", msg.Payload)
		}

		if msg.Properties == nil {
			t.Error("Expected properties to be initialized")
		}
	})

	t.Run("WithDeadline", func(t *testing.T) {
		deadline := time.Now().Add(1 * time.Hour)
		props := message.Properties{
			message.PropDeadline: deadline,
		}
		msg := message.New("payload", props, nil)

		if msgDeadline, ok := msg.Deadline(); !ok || msgDeadline != deadline {
			t.Errorf("Expected deadline %v, got %v (ok=%v)", deadline, msgDeadline, ok)
		}
	})

	t.Run("WithAcking", func(t *testing.T) {
		var ackCalled, nackCalled bool
		acking := message.NewAcking(
			func() { ackCalled = true },
			func(error) { nackCalled = true },
		)
		msg := message.New(42, nil, acking)

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
		props := message.Properties{
			message.PropID: "msg-001",
		}
		msg := message.New("payload", props, nil)

		if id, ok := message.IDProps(msg.Properties); !ok || id != "msg-001" {
			t.Errorf("Expected ID 'msg-001', got %q (ok=%v)", id, ok)
		}
	})

	t.Run("WithCorrelationID", func(t *testing.T) {
		props := message.Properties{
			message.PropCorrelationID: "corr-123",
		}
		msg := message.New("payload", props, nil)

		if id, ok := message.CorrelationIDProps(msg.Properties); !ok || id != "corr-123" {
			t.Errorf("Expected CorrelationID 'corr-123', got %q (ok=%v)", id, ok)
		}
	})

	t.Run("WithProperty", func(t *testing.T) {
		props := message.Properties{
			"tenant":   "acme-corp",
			"priority": 5,
		}
		msg := message.New("payload", props, nil)

		if val, ok := msg.Properties["tenant"]; !ok || val != "acme-corp" {
			t.Errorf("Expected tenant='acme-corp', got %v (ok=%v)", val, ok)
		}

		if val, ok := msg.Properties["priority"]; !ok || val != 5 {
			t.Errorf("Expected priority=5, got %v (ok=%v)", val, ok)
		}
	})

	t.Run("CombinedOptions", func(t *testing.T) {
		deadline := time.Now().Add(1 * time.Hour)
		var ackCalled bool

		props := message.Properties{
			message.PropDeadline:      deadline,
			message.PropID:            "msg-001",
			message.PropCorrelationID: "order-123",
			"tenant":                  "acme",
			"source":                  "orders-queue",
		}
		acking := message.NewAcking(
			func() { ackCalled = true },
			func(error) {},
		)
		msg := message.New("order-data", props, acking)

		// Verify payload
		if msg.Payload != "order-data" {
			t.Errorf("Expected payload 'order-data', got %v", msg.Payload)
		}

		// Verify ID
		if id, ok := message.IDProps(msg.Properties); !ok || id != "msg-001" {
			t.Errorf("Expected ID 'msg-001', got %q", id)
		}

		// Verify CorrelationID
		if corrID, ok := message.CorrelationIDProps(msg.Properties); !ok || corrID != "order-123" {
			t.Errorf("Expected CorrelationID 'order-123', got %q", corrID)
		}

		// Verify custom properties
		if val, ok := msg.Properties["tenant"]; !ok || val != "acme" {
			t.Errorf("Expected tenant='acme', got %v (ok=%v)", val, ok)
		}

		if val, ok := msg.Properties["source"]; !ok || val != "orders-queue" {
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

	t.Run("WithMultipleProperties", func(t *testing.T) {
		props := message.Properties{
			message.PropID:      "msg-001",
			"initial":           "value",
			"tenant":            "acme-corp",
			"priority":          5,
			message.PropCorrelationID: "corr-123",
		}
		msg := message.New("payload", props, nil)

		// Verify that ID is set
		if id, ok := message.IDProps(msg.Properties); !ok || id != "msg-001" {
			t.Errorf("Expected ID 'msg-001', got %q", id)
		}

		// Verify that initial property is set
		if val, ok := msg.Properties["initial"]; !ok || val != "value" {
			t.Errorf("Expected initial='value', got %v (ok=%v)", val, ok)
		}

		// Verify that properties are added
		if val, ok := msg.Properties["tenant"]; !ok || val != "acme-corp" {
			t.Errorf("Expected tenant='acme-corp', got %v (ok=%v)", val, ok)
		}

		if val, ok := msg.Properties["priority"]; !ok || val != 5 {
			t.Errorf("Expected priority=5, got %v (ok=%v)", val, ok)
		}

		// Verify that CorrelationID is set
		if corrID, ok := message.CorrelationIDProps(msg.Properties); !ok || corrID != "corr-123" {
			t.Errorf("Expected CorrelationID 'corr-123', got %q", corrID)
		}

		// Count all properties
		count := 0
		for range msg.Properties {
			count++
		}

		// Should have: initial, tenant, priority, ID, CorrelationID = 5 properties
		if count != 5 {
			t.Errorf("Expected 5 properties total, got %d", count)
		}
	})
}

func TestMessage_TypedAccessors(t *testing.T) {
	t.Parallel()

	t.Run("GettersViaProperties", func(t *testing.T) {
		now := time.Now()
		props := message.Properties{
			message.PropID:            "msg-001",
			message.PropCorrelationID: "corr-123",
			message.PropCreatedAt:     now,
		}
		msg := message.New(42, props, nil)

		// Test ID
		if id, ok := message.IDProps(msg.Properties); !ok || id != "msg-001" {
			t.Errorf("Expected ID 'msg-001', got %q (ok=%v)", id, ok)
		}

		// Test CorrelationID
		if id, ok := message.CorrelationIDProps(msg.Properties); !ok || id != "corr-123" {
			t.Errorf("Expected CorrelationID 'corr-123', got %q (ok=%v)", id, ok)
		}

		// Test CreatedAt
		if ts, ok := message.CreatedAtProps(msg.Properties); !ok || !ts.Equal(now) {
			t.Errorf("Expected CreatedAt %v, got %v (ok=%v)", now, ts, ok)
		}
	})

	t.Run("SetPropertiesViaMap", func(t *testing.T) {
		msg := message.New(42, nil, nil)

		// Set reserved properties via map access
		now := time.Now()
		msg.Properties[message.PropID] = "msg-001"
		msg.Properties[message.PropCorrelationID] = "corr-123"
		msg.Properties[message.PropCreatedAt] = now

		// Get via pass-through functions
		if id, ok := message.IDProps(msg.Properties); !ok || id != "msg-001" {
			t.Errorf("Expected ID 'msg-001', got %q (ok=%v)", id, ok)
		}

		if id, ok := message.CorrelationIDProps(msg.Properties); !ok || id != "corr-123" {
			t.Errorf("Expected CorrelationID 'corr-123', got %q (ok=%v)", id, ok)
		}

		if ts, ok := message.CreatedAtProps(msg.Properties); !ok || !ts.Equal(now) {
			t.Errorf("Expected CreatedAt %v, got %v (ok=%v)", now, ts, ok)
		}
	})
}
