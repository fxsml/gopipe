package middleware

import (
	"context"
	"testing"

	"github.com/fxsml/gopipe"
	"github.com/fxsml/gopipe/message"
)

// TestNewMessageMiddleware_ModificationBeforeNext verifies that modifications
// to the input message before calling next() are propagated to the next processor.
func TestNewMessageMiddleware_ModificationBeforeNext(t *testing.T) {
	// Create a test message
	msg := message.New([]byte("original payload"), message.Attributes{
		"original-key": "original-value",
	})

	// Track what the inner processor receives
	var receivedMsg *message.Message
	var receivedPayload []byte
	var receivedProperty string

	// Create an inner processor that records what it receives
	innerProc := gopipe.NewProcessor(
		func(ctx context.Context, m *message.Message) ([]*message.Message, error) {
			receivedMsg = m
			receivedPayload = m.Data
			receivedProperty, _ = m.Attributes["modified-key"].(string)
			return []*message.Message{m}, nil
		},
		func(m *message.Message, err error) {},
	)

	// Create middleware that modifies the message before calling next()
	middleware := NewMessageMiddleware(
		func(ctx context.Context, m *message.Message, next func() ([]*message.Message, error)) ([]*message.Message, error) {
			// Modify payload
			m.Data = []byte("modified payload")

			// Add a new property
			if m.Attributes == nil {
				m.Attributes = make(message.Attributes)
			}
			m.Attributes["modified-key"] = "modified-value"

			// Call next with the modified message
			return next()
		},
	)

	// Apply middleware to inner processor
	wrappedProc := middleware(innerProc)

	// Process the message
	ctx := context.Background()
	results, err := wrappedProc.Process(ctx, msg)

	// Verify no error
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify results were returned
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	// Verify the inner processor received the modified message
	if receivedMsg == nil {
		t.Fatal("inner processor did not receive message")
	}

	// Verify payload was modified
	expectedPayload := "modified payload"
	actualPayload := string(receivedPayload)
	if actualPayload != expectedPayload {
		t.Errorf("expected payload %q, got %q", expectedPayload, actualPayload)
	}

	// Verify property was added
	expectedProperty := "modified-value"
	if receivedProperty != expectedProperty {
		t.Errorf("expected property %q, got %q", expectedProperty, receivedProperty)
	}

	// Verify original property is still present
	originalValue, ok := receivedMsg.Attributes["original-key"].(string)
	if !ok || originalValue != "original-value" {
		t.Errorf("original property was lost or modified: got %v", originalValue)
	}
}

// TestNewMessageMiddleware_MultipleModifications verifies that multiple
// middleware can each modify the message and all modifications are propagated.
func TestNewMessageMiddleware_MultipleModifications(t *testing.T) {
	// Create a test message
	msg := message.New([]byte("initial"), message.Attributes{})

	// Track what the inner processor receives
	var receivedMsg *message.Message

	// Create an inner processor
	innerProc := gopipe.NewProcessor(
		func(ctx context.Context, m *message.Message) ([]*message.Message, error) {
			receivedMsg = m
			return []*message.Message{m}, nil
		},
		func(m *message.Message, err error) {},
	)

	// Create first middleware that adds property "step1"
	middleware1 := NewMessageMiddleware(
		func(ctx context.Context, m *message.Message, next func() ([]*message.Message, error)) ([]*message.Message, error) {
			if m.Attributes == nil {
				m.Attributes = make(message.Attributes)
			}
			m.Attributes["step1"] = "completed"
			return next()
		},
	)

	// Create second middleware that adds property "step2"
	middleware2 := NewMessageMiddleware(
		func(ctx context.Context, m *message.Message, next func() ([]*message.Message, error)) ([]*message.Message, error) {
			if m.Attributes == nil {
				m.Attributes = make(message.Attributes)
			}
			m.Attributes["step2"] = "completed"
			return next()
		},
	)

	// Create third middleware that adds property "step3"
	middleware3 := NewMessageMiddleware(
		func(ctx context.Context, m *message.Message, next func() ([]*message.Message, error)) ([]*message.Message, error) {
			if m.Attributes == nil {
				m.Attributes = make(message.Attributes)
			}
			m.Attributes["step3"] = "completed"
			return next()
		},
	)

	// Apply middleware in order: 1 -> 2 -> 3 -> inner
	wrappedProc := middleware1(middleware2(middleware3(innerProc)))

	// Process the message
	ctx := context.Background()
	_, err := wrappedProc.Process(ctx, msg)

	// Verify no error
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify the inner processor received all modifications
	if receivedMsg == nil {
		t.Fatal("inner processor did not receive message")
	}

	// Verify all properties were added
	expectedProperties := map[string]string{
		"step1": "completed",
		"step2": "completed",
		"step3": "completed",
	}

	for key, expectedValue := range expectedProperties {
		actualValue, ok := receivedMsg.Attributes[key].(string)
		if !ok {
			t.Errorf("property %q was not added", key)
			continue
		}
		if actualValue != expectedValue {
			t.Errorf("property %q: expected %q, got %q", key, expectedValue, actualValue)
		}
	}
}

// TestNewMessageMiddleware_ModificationAfterNext verifies that modifications
// to output messages after calling next() work correctly.
func TestNewMessageMiddleware_ModificationAfterNext(t *testing.T) {
	// Create a test message
	msg := message.New([]byte("input"), message.Attributes{})

	// Create an inner processor that returns a message
	innerProc := gopipe.NewProcessor(
		func(ctx context.Context, m *message.Message) ([]*message.Message, error) {
			outMsg := message.New([]byte("output"), message.Attributes{
				"inner": "value",
			})
			return []*message.Message{outMsg}, nil
		},
		func(m *message.Message, err error) {},
	)

	// Create middleware that modifies output messages
	middleware := NewMessageMiddleware(
		func(ctx context.Context, m *message.Message, next func() ([]*message.Message, error)) ([]*message.Message, error) {
			results, err := next()
			if err != nil {
				return results, err
			}

			// Modify all output messages
			for _, outMsg := range results {
				if outMsg.Attributes == nil {
					outMsg.Attributes = make(message.Attributes)
				}
				outMsg.Attributes["middleware"] = "modified"
			}

			return results, nil
		},
	)

	// Apply middleware
	wrappedProc := middleware(innerProc)

	// Process the message
	ctx := context.Background()
	results, err := wrappedProc.Process(ctx, msg)

	// Verify no error
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify results
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	outMsg := results[0]

	// Verify original property is present
	innerValue, ok := outMsg.Attributes["inner"].(string)
	if !ok || innerValue != "value" {
		t.Errorf("inner property was lost: got %v", innerValue)
	}

	// Verify middleware added property
	middlewareValue, ok := outMsg.Attributes["middleware"].(string)
	if !ok || middlewareValue != "modified" {
		t.Errorf("middleware property was not added: got %v", middlewareValue)
	}
}
