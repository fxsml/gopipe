package cqrs

import (
	"context"
	"testing"

	"github.com/fxsml/gopipe/message"
)

type TestCommand struct {
	ID   string
	Name string
}

type TestEvent struct {
	ID        string
	Name      string
	Processed bool
}

// TestNewCommandHandler_SetsType verifies that NewCommandHandler
// automatically sets attributes using marshaler.Attributes()
func TestNewCommandHandler_SetsType(t *testing.T) {
	marshaler := NewJSONCommandMarshaler(
		WithTypeOf(),
	)

	// Create command handler
	handler := NewCommandHandler(
		func(ctx context.Context, cmd TestCommand) ([]TestEvent, error) {
			return []TestEvent{{
				ID:        cmd.ID,
				Name:      cmd.Name,
				Processed: true,
			}}, nil
		},
		Match(MatchSubject("TestCommand"), MatchType("command")),
		marshaler,
	)

	// Create input message
	cmdPayload, _ := marshaler.Marshal(TestCommand{
		ID:   "test-123",
		Name: "test command",
	})
	inputMsg := message.New(cmdPayload, message.Attributes{
		message.AttrSubject:       "TestCommand",
		"type":                    "command",
		message.AttrCorrelationID: "corr-456",
	})

	// Process message
	ctx := context.Background()
	outputMsgs, err := handler.Handle(ctx, inputMsg)

	// Verify no error
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify output messages
	if len(outputMsgs) != 1 {
		t.Fatalf("expected 1 output message, got %d", len(outputMsgs))
	}

	outMsg := outputMsgs[0]

	// Verify Type attribute is set to the event type name
	eventType, ok := outMsg.Attributes.Type()
	if !ok {
		t.Fatal("Type attribute not set in output message")
	}

	expectedType := "TestEvent"
	if eventType != expectedType {
		t.Errorf("expected Type=%q, got %q", expectedType, eventType)
	}

	// Note: Correlation ID propagation is now handled by message-level middleware,
	// not by the marshaler. So we don't expect it to be propagated here.

	// Verify payload can be unmarshaled
	var evt TestEvent
	if err := marshaler.Unmarshal(outMsg.Data, &evt); err != nil {
		t.Fatalf("failed to unmarshal output: %v", err)
	}

	if evt.ID != "test-123" || evt.Name != "test command" || !evt.Processed {
		t.Errorf("unexpected event data: %+v", evt)
	}
}

// TestMatchTypeName_UsesType verifies that MatchTypeName
// matches against the Type attribute
func TestMatchTypeName_UsesType(t *testing.T) {
	tests := []struct {
		name       string
		properties message.Attributes
		matches    bool
	}{
		{
			name: "matches when Type equals type name",
			properties: message.Attributes{
				message.AttrType: "TestEvent",
			},
			matches: true,
		},
		{
			name: "does not match when Type differs",
			properties: message.Attributes{
				message.AttrType: "OtherEvent",
			},
			matches: false,
		},
		{
			name:       "does not match when Type is missing",
			properties: message.Attributes{},
			matches:    false,
		},
		{
			name: "matches when type property equals type name",
			properties: message.Attributes{
				"type": "TestEvent",
			},
			matches: true,
		},
		{
			name: "does not match when type is generic category",
			properties: message.Attributes{
				message.AttrType: "event", // Generic category, not the actual type
			},
			matches: false,
		},
	}

	matcher := MatchGenericTypeOf[TestEvent]()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := matcher(tt.properties)
			if result != tt.matches {
				t.Errorf("expected match=%v, got %v for properties %+v", tt.matches, result, tt.properties)
			}
		})
	}
}

// TestNewCommandHandler_WithMultipleEvents verifies Type attribute
// is set correctly for multiple output events
func TestNewCommandHandler_WithMultipleEvents(t *testing.T) {
	marshaler := NewJSONCommandMarshaler()

	// Create command handler that returns multiple events
	handler := NewCommandHandler(
		func(ctx context.Context, cmd TestCommand) ([]TestEvent, error) {
			return []TestEvent{
				{ID: "evt-1", Name: "event 1", Processed: true},
				{ID: "evt-2", Name: "event 2", Processed: true},
				{ID: "evt-3", Name: "event 3", Processed: true},
			}, nil
		},
		Match(MatchSubject("TestCommand")),
		marshaler,
	)

	// Create input message
	cmdPayload, _ := marshaler.Marshal(TestCommand{ID: "cmd-1", Name: "test"})
	inputMsg := message.New(cmdPayload, message.Attributes{
		message.AttrSubject: "TestCommand",
	})

	// Process message
	ctx := context.Background()
	outputMsgs, err := handler.Handle(ctx, inputMsg)

	// Verify no error
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify output messages
	if len(outputMsgs) != 3 {
		t.Fatalf("expected 3 output messages, got %d", len(outputMsgs))
	}

	// Verify Type attribute is set on all output messages
	for i, outMsg := range outputMsgs {
		eventType, ok := outMsg.Attributes.Type()
		if !ok {
			t.Errorf("message %d: Type attribute not set", i)
			continue
		}

		if eventType != "TestEvent" {
			t.Errorf("message %d: expected Type=%q, got %q", i, "TestEvent", eventType)
		}
	}
}

// TestNewCommandHandler_UnmarshalError verifies error handling when
// command unmarshal fails
func TestNewCommandHandler_UnmarshalError(t *testing.T) {
	marshaler := NewJSONCommandMarshaler()

	var nackCalled bool
	var nackErr error

	handler := NewCommandHandler(
		func(ctx context.Context, cmd TestCommand) ([]TestEvent, error) {
			t.Fatal("handler should not be called on unmarshal error")
			return nil, nil
		},
		Match(MatchSubject("TestCommand")),
		marshaler,
	)

	// Create message with invalid JSON
	inputMsg := message.NewWithAcking(
		[]byte(`invalid json`),
		message.Attributes{message.AttrSubject: "TestCommand"},
		func() { t.Error("ack should not be called on error") },
		func(err error) { nackCalled = true; nackErr = err },
	)

	// Process message
	ctx := context.Background()
	outputMsgs, err := handler.Handle(ctx, inputMsg)

	// Verify error returned
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	// Verify nack was called
	if !nackCalled {
		t.Error("nack should have been called")
	}

	// Verify nack error matches returned error
	if nackErr != err {
		t.Errorf("nack error %v should match returned error %v", nackErr, err)
	}

	// Verify no output messages
	if outputMsgs != nil {
		t.Errorf("expected nil output, got %v", outputMsgs)
	}
}

// TestNewEventHandler_UnmarshalError verifies error handling when
// event unmarshal fails
func TestNewEventHandler_UnmarshalError(t *testing.T) {
	marshaler := NewJSONCommandMarshaler()

	var nackCalled bool

	handler := NewEventHandler(
		func(ctx context.Context, evt TestEvent) error {
			t.Fatal("handler should not be called on unmarshal error")
			return nil
		},
		Match(MatchSubject("TestEvent")),
		marshaler,
	)

	// Create message with invalid JSON
	inputMsg := message.NewWithAcking(
		[]byte(`{invalid`),
		message.Attributes{message.AttrSubject: "TestEvent"},
		func() { t.Error("ack should not be called on error") },
		func(err error) { nackCalled = true },
	)

	// Process message
	ctx := context.Background()
	outputMsgs, err := handler.Handle(ctx, inputMsg)

	// Verify error returned
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	// Verify nack was called
	if !nackCalled {
		t.Error("nack should have been called")
	}

	// Verify no output messages
	if outputMsgs != nil {
		t.Errorf("expected nil output, got %v", outputMsgs)
	}
}

// TestNewEventHandler_Success verifies successful event handling
func TestNewEventHandler_Success(t *testing.T) {
	marshaler := NewJSONCommandMarshaler()

	var handlerCalled bool
	var ackCalled bool

	handler := NewEventHandler(
		func(ctx context.Context, evt TestEvent) error {
			handlerCalled = true
			if evt.ID != "evt-123" {
				t.Errorf("expected ID evt-123, got %s", evt.ID)
			}
			return nil
		},
		Match(MatchSubject("TestEvent")),
		marshaler,
	)

	// Create valid message
	payload, _ := marshaler.Marshal(TestEvent{ID: "evt-123", Name: "test event"})
	inputMsg := message.NewWithAcking(
		payload,
		message.Attributes{message.AttrSubject: "TestEvent"},
		func() { ackCalled = true },
		func(err error) { t.Errorf("nack should not be called: %v", err) },
	)

	// Process message
	ctx := context.Background()
	outputMsgs, err := handler.Handle(ctx, inputMsg)

	// Verify no error
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify handler was called
	if !handlerCalled {
		t.Error("handler should have been called")
	}

	// Verify ack was called
	if !ackCalled {
		t.Error("ack should have been called")
	}

	// Verify no output messages (event handlers don't return messages)
	if outputMsgs != nil {
		t.Errorf("expected nil output, got %v", outputMsgs)
	}
}
