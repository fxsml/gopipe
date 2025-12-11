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

	props := message.Attributes{
		message.AttrDeadline: deadline,
		"key":                "value",
	}
	msg := message.NewWithAcking("payload", props,
		func() { ackCalled = true },
		func(error) { nackCalled = true })

	if val, ok := msg.Attributes["key"]; !ok || val != "value" {
		t.Errorf("Expected properties key='value', got %v", val)
	}

	if msg.Data != "payload" {
		t.Errorf("Expected Payload 'payload', got %v", msg.Data)
	}

	if msgDeadline, ok := msg.Attributes.Deadline(); !ok || msgDeadline != deadline {
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
	msg := message.NewWithAcking(42, nil,
		func() { ackCount++ },
		func(error) {})

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
	msg := message.NewWithAcking(42, nil,
		func() {},
		func(err error) {
			nackCount++
			capturedErr = err
		})

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

	props := message.Attributes{
		message.AttrDeadline: deadline,
		"key":                "value",
	}

	original := message.NewWithAcking("original", props,
		func() { ackCalled = true },
		func(error) {})

	// Copy with new payload
	copy := message.Copy(original, "copied")

	// Type assert the payload
	if copy.Data != "copied" {
		t.Errorf("Expected Payload 'copied', got %v", copy.Data)
	}

	if val, ok := copy.Attributes["key"]; !ok || val != "value" {
		t.Errorf("Expected properties key='value', got %v", val)
	}

	if copyDeadline, ok := copy.Attributes.Deadline(); !ok || copyDeadline != deadline {
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
		msg := message.New("string payload", nil)
		if msg.Data != "string payload" {
			t.Errorf("Expected 'string payload', got %v", msg.Data)
		}
	})

	t.Run("Int", func(t *testing.T) {
		msg := message.New(42, nil)
		if msg.Data != 42 {
			t.Errorf("Expected 42, got %v", msg.Data)
		}
	})

	t.Run("Struct", func(t *testing.T) {
		type Data struct {
			Field string
		}
		msg := message.New(Data{Field: "test"}, nil)
		if msg.Data != (Data{Field: "test"}) {
			t.Errorf("Expected Data{Field: 'test'}, got %v", msg.Data)
		}
	})
}

func TestMessage_MultiStageAcking(t *testing.T) {
	t.Parallel()

	var ackCalled bool
	// Create shared acking for 2 messages
	acking := message.NewAcking(
		func() { ackCalled = true },
		func(error) {},
		2)

	// Create two messages sharing the same acking
	msg1 := message.NewWithSharedAcking(42, nil, acking)
	msg2 := message.NewWithSharedAcking(43, nil, acking)

	// First ack should not trigger callback
	if !msg1.Ack() {
		t.Error("Expected first Ack to return true")
	}

	if ackCalled {
		t.Error("Expected ack callback NOT to be called after first ack")
	}

	// Second ack should trigger callback
	if !msg2.Ack() {
		t.Error("Expected second Ack to return true")
	}

	if !ackCalled {
		t.Error("Expected ack callback to be called after second ack")
	}
}

func TestMessage_SharedAcking_Nack(t *testing.T) {
	t.Parallel()

	var ackCalled bool
	var nackCalled bool
	var nackErr error

	// Create shared acking for 3 messages
	acking := message.NewAcking(
		func() { ackCalled = true },
		func(err error) { nackCalled = true; nackErr = err },
		3)

	msg1 := message.NewWithSharedAcking(1, nil, acking)
	msg2 := message.NewWithSharedAcking(2, nil, acking)
	msg3 := message.NewWithSharedAcking(3, nil, acking)

	// First message acks
	msg1.Ack()
	if ackCalled || nackCalled {
		t.Error("No callback should be called after 1/3 acks")
	}

	// Second message nacks - should trigger nack immediately
	testErr := errors.New("processing failed")
	msg2.Nack(testErr)

	if !nackCalled {
		t.Error("Expected nack callback to be called immediately")
	}
	if nackErr != testErr {
		t.Errorf("Expected nack error %v, got %v", testErr, nackErr)
	}
	if ackCalled {
		t.Error("Ack callback should not be called after nack")
	}

	// Third message tries to ack - should fail since already nacked
	if msg3.Ack() {
		t.Error("Expected Ack to return false after nack")
	}
}

func TestNewAcking_Validation(t *testing.T) {
	t.Parallel()

	t.Run("zero count returns nil", func(t *testing.T) {
		acking := message.NewAcking(func() {}, func(error) {}, 0)
		if acking != nil {
			t.Error("Expected nil for zero count")
		}
	})

	t.Run("negative count returns nil", func(t *testing.T) {
		acking := message.NewAcking(func() {}, func(error) {}, -1)
		if acking != nil {
			t.Error("Expected nil for negative count")
		}
	})

	t.Run("nil ack returns nil", func(t *testing.T) {
		acking := message.NewAcking(nil, func(error) {}, 1)
		if acking != nil {
			t.Error("Expected nil for nil ack callback")
		}
	})

	t.Run("nil nack returns nil", func(t *testing.T) {
		acking := message.NewAcking(func() {}, nil, 1)
		if acking != nil {
			t.Error("Expected nil for nil nack callback")
		}
	})

	t.Run("valid params returns non-nil", func(t *testing.T) {
		acking := message.NewAcking(func() {}, func(error) {}, 1)
		if acking == nil {
			t.Error("Expected non-nil for valid params")
		}
	})
}

// NOTE: Concurrent access test removed - Properties is now a plain map[string]any
// without thread-safety. Users must provide their own synchronization if needed.
// See ADR 0002: Remove Properties Thread-Safety

func TestMessage_PropertiesSetAfterCreation(t *testing.T) {
	t.Parallel()

	t.Run("SetPropertiesOnNilProperties", func(t *testing.T) {
		// Create minimal message - properties are always created
		msg := message.New("payload", nil)

		// Properties should exist
		if msg.Attributes == nil {
			t.Error("Expected Properties to be non-nil")
		}
	})

	t.Run("SetPropertiesOnEmptyProperties", func(t *testing.T) {
		msg := message.New("payload", nil)

		// Set properties after message creation
		msg.Attributes["key1"] = "value1"
		msg.Attributes["key2"] = 42
		msg.Attributes["key3"] = true

		// Verify all properties were set correctly
		if val, ok := msg.Attributes["key1"]; !ok || val != "value1" {
			t.Errorf("Expected key1='value1', got %v (ok=%v)", val, ok)
		}

		if val, ok := msg.Attributes["key2"]; !ok || val != 42 {
			t.Errorf("Expected key2=42, got %v (ok=%v)", val, ok)
		}

		if val, ok := msg.Attributes["key3"]; !ok || val != true {
			t.Errorf("Expected key3=true, got %v (ok=%v)", val, ok)
		}
	})

	t.Run("MixPreAndPostCreationProperties", func(t *testing.T) {
		props := message.Attributes{
			"initial1": "value1",
			"initial2": "value2",
		}
		msg := message.New("payload", props)

		// Verify initial properties exist
		if val, ok := msg.Attributes["initial1"]; !ok || val != "value1" {
			t.Errorf("Expected initial1='value1', got %v (ok=%v)", val, ok)
		}

		// Add more properties after message creation
		msg.Attributes["added1"] = "new_value1"
		msg.Attributes["added2"] = 100

		// Verify all properties are accessible
		if val, ok := msg.Attributes["initial1"]; !ok || val != "value1" {
			t.Errorf("Expected initial1='value1', got %v", val)
		}

		if val, ok := msg.Attributes["initial2"]; !ok || val != "value2" {
			t.Errorf("Expected initial2='value2', got %v", val)
		}

		if val, ok := msg.Attributes["added1"]; !ok || val != "new_value1" {
			t.Errorf("Expected added1='new_value1', got %v", val)
		}

		if val, ok := msg.Attributes["added2"]; !ok || val != 100 {
			t.Errorf("Expected added2=100, got %v", val)
		}

		// Verify properties count
		count := 0
		for range msg.Attributes {
			count++
		}

		if count != 4 {
			t.Errorf("Expected 4 properties total, got %d", count)
		}
	})

	t.Run("UpdateExistingProperty", func(t *testing.T) {
		props := message.Attributes{
			"key": "original",
		}
		msg := message.New("payload", props)

		// Verify original value
		if val, ok := msg.Attributes["key"]; !ok || val != "original" {
			t.Errorf("Expected key='original', got %v", val)
		}

		// Update the property after message creation
		msg.Attributes["key"] = "updated"

		// Verify updated value
		if val, ok := msg.Attributes["key"]; !ok || val != "updated" {
			t.Errorf("Expected key='updated', got %v", val)
		}
	})

	t.Run("DeletePropertyAfterCreation", func(t *testing.T) {
		props := message.Attributes{
			"key1": "value1",
			"key2": "value2",
		}
		msg := message.New("payload", props)

		// Delete a property after message creation
		delete(msg.Attributes, "key1")

		// Verify key1 is deleted
		if val, ok := msg.Attributes["key1"]; ok {
			t.Errorf("Expected key1 to be deleted, but got value: %v", val)
		}

		// Verify key2 still exists
		if val, ok := msg.Attributes["key2"]; !ok || val != "value2" {
			t.Errorf("Expected key2='value2', got %v (ok=%v)", val, ok)
		}
	})

	t.Run("PropertiesSharedWithCopiedMessage", func(t *testing.T) {
		props := message.Attributes{
			"initial": "value",
		}
		original := message.New("original_payload", props)

		// Copy the message
		copied := message.Copy(original, "copied_payload")

		// Set property on original message
		original.Attributes["from_original"] = "orig_val"

		// Verify copied message sees the same property (properties are shared)
		if val, ok := copied.Attributes["from_original"]; !ok || val != "orig_val" {
			t.Errorf("Expected copied message to see property set on original, got %v (ok=%v)", val, ok)
		}

		// Set property on copied message
		copied.Attributes["from_copied"] = "copy_val"

		// Verify original message sees the property (properties are shared)
		if val, ok := original.Attributes["from_copied"]; !ok || val != "copy_val" {
			t.Errorf("Expected original message to see property set on copied, got %v (ok=%v)", val, ok)
		}
	})
}

func TestMessage_NewWithOptions(t *testing.T) {
	t.Parallel()

	t.Run("MinimalMessage", func(t *testing.T) {
		msg := message.New(42, nil)

		if msg.Data != 42 {
			t.Errorf("Expected payload 42, got %v", msg.Data)
		}

		if msg.Attributes == nil {
			t.Error("Expected properties to be initialized")
		}
	})

	t.Run("WithDeadline", func(t *testing.T) {
		deadline := time.Now().Add(1 * time.Hour)
		props := message.Attributes{
			message.AttrDeadline: deadline,
		}
		msg := message.New("payload", props)

		if msgDeadline, ok := msg.Attributes.Deadline(); !ok || msgDeadline != deadline {
			t.Errorf("Expected deadline %v, got %v (ok=%v)", deadline, msgDeadline, ok)
		}
	})

	t.Run("WithAcking", func(t *testing.T) {
		var ackCalled, nackCalled bool
		msg := message.NewWithAcking(42, nil,
			func() { ackCalled = true },
			func(error) { nackCalled = true })

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
		props := message.Attributes{
			message.AttrID: "msg-001",
		}
		msg := message.New("payload", props)

		if id, ok := msg.Attributes.ID(); !ok || id != "msg-001" {
			t.Errorf("Expected ID 'msg-001', got %q (ok=%v)", id, ok)
		}
	})

	t.Run("WithCorrelationID", func(t *testing.T) {
		props := message.Attributes{
			message.AttrCorrelationID: "corr-123",
		}
		msg := message.New("payload", props)

		if id, ok := msg.Attributes.CorrelationID(); !ok || id != "corr-123" {
			t.Errorf("Expected CorrelationID 'corr-123', got %q (ok=%v)", id, ok)
		}
	})

	t.Run("WithProperty", func(t *testing.T) {
		props := message.Attributes{
			"tenant":   "acme-corp",
			"priority": 5,
		}
		msg := message.New("payload", props)

		if val, ok := msg.Attributes["tenant"]; !ok || val != "acme-corp" {
			t.Errorf("Expected tenant='acme-corp', got %v (ok=%v)", val, ok)
		}

		if val, ok := msg.Attributes["priority"]; !ok || val != 5 {
			t.Errorf("Expected priority=5, got %v (ok=%v)", val, ok)
		}
	})

	t.Run("CombinedOptions", func(t *testing.T) {
		deadline := time.Now().Add(1 * time.Hour)
		var ackCalled bool

		props := message.Attributes{
			message.AttrDeadline:      deadline,
			message.AttrID:            "msg-001",
			message.AttrCorrelationID: "order-123",
			"tenant":                  "acme",
			"source":                  "orders-queue",
		}
		msg := message.NewWithAcking("order-data", props,
			func() { ackCalled = true },
			func(error) {})

		// Verify payload
		if msg.Data != "order-data" {
			t.Errorf("Expected payload 'order-data', got %v", msg.Data)
		}

		// Verify ID
		if id, ok := msg.Attributes.ID(); !ok || id != "msg-001" {
			t.Errorf("Expected ID 'msg-001', got %q", id)
		}

		// Verify CorrelationID
		if corrID, ok := msg.Attributes.CorrelationID(); !ok || corrID != "order-123" {
			t.Errorf("Expected CorrelationID 'order-123', got %q", corrID)
		}

		// Verify custom properties
		if val, ok := msg.Attributes["tenant"]; !ok || val != "acme" {
			t.Errorf("Expected tenant='acme', got %v (ok=%v)", val, ok)
		}

		if val, ok := msg.Attributes["source"]; !ok || val != "orders-queue" {
			t.Errorf("Expected source='orders-queue', got %v (ok=%v)", val, ok)
		}

		// Verify deadline
		if msgDeadline, ok := msg.Attributes.Deadline(); !ok || msgDeadline != deadline {
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
		props := message.Attributes{
			message.AttrID:            "msg-001",
			"initial":                 "value",
			"tenant":                  "acme-corp",
			"priority":                5,
			message.AttrCorrelationID: "corr-123",
		}
		msg := message.New("payload", props)

		// Verify that ID is set
		if id, ok := msg.Attributes.ID(); !ok || id != "msg-001" {
			t.Errorf("Expected ID 'msg-001', got %q", id)
		}

		// Verify that initial property is set
		if val, ok := msg.Attributes["initial"]; !ok || val != "value" {
			t.Errorf("Expected initial='value', got %v (ok=%v)", val, ok)
		}

		// Verify that properties are added
		if val, ok := msg.Attributes["tenant"]; !ok || val != "acme-corp" {
			t.Errorf("Expected tenant='acme-corp', got %v (ok=%v)", val, ok)
		}

		if val, ok := msg.Attributes["priority"]; !ok || val != 5 {
			t.Errorf("Expected priority=5, got %v (ok=%v)", val, ok)
		}

		// Verify that CorrelationID is set
		if corrID, ok := msg.Attributes.CorrelationID(); !ok || corrID != "corr-123" {
			t.Errorf("Expected CorrelationID 'corr-123', got %q", corrID)
		}

		// Count all properties
		count := 0
		for range msg.Attributes {
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
		props := message.Attributes{
			message.AttrID:            "msg-001",
			message.AttrCorrelationID: "corr-123",
			message.AttrTime:     now,
		}
		msg := message.New(42, props)

		// Test ID
		if id, ok := msg.Attributes.ID(); !ok || id != "msg-001" {
			t.Errorf("Expected ID 'msg-001', got %q (ok=%v)", id, ok)
		}

		// Test CorrelationID
		if id, ok := msg.Attributes.CorrelationID(); !ok || id != "corr-123" {
			t.Errorf("Expected CorrelationID 'corr-123', got %q (ok=%v)", id, ok)
		}

		// Test EventTime
		if ts, ok := msg.Attributes.EventTime(); !ok || !ts.Equal(now) {
			t.Errorf("Expected EventTime %v, got %v (ok=%v)", now, ts, ok)
		}
	})

	t.Run("SetAttributesViaMap", func(t *testing.T) {
		msg := message.New(42, nil)

		// Set reserved attributes via map access
		now := time.Now()
		msg.Attributes[message.AttrID] = "msg-001"
		msg.Attributes[message.AttrCorrelationID] = "corr-123"
		msg.Attributes[message.AttrTime] = now

		// Get via pass-through functions
		if id, ok := msg.Attributes.ID(); !ok || id != "msg-001" {
			t.Errorf("Expected ID 'msg-001', got %q (ok=%v)", id, ok)
		}

		if id, ok := msg.Attributes.CorrelationID(); !ok || id != "corr-123" {
			t.Errorf("Expected CorrelationID 'corr-123', got %q (ok=%v)", id, ok)
		}

		if ts, ok := msg.Attributes.EventTime(); !ok || !ts.Equal(now) {
			t.Errorf("Expected EventTime %v, got %v (ok=%v)", now, ts, ok)
		}
	})
}
