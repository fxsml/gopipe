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
		WithType(),
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

	// Verify PropType is set to the event type name
	propType, ok := outMsg.Attributes.Type()
	if !ok {
		t.Fatal("PropType not set in output message")
	}

	expectedType := "TestEvent"
	if propType != expectedType {
		t.Errorf("expected PropType=%q, got %q", expectedType, propType)
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

// TestMatchTypeName_UsesPropType verifies that MatchTypeName
// matches against the PropType property
func TestMatchTypeName_UsesPropType(t *testing.T) {
	tests := []struct {
		name       string
		properties message.Attributes
		matches    bool
	}{
		{
			name: "matches when PropType equals type name",
			properties: message.Attributes{
				message.AttrType: "TestEvent",
			},
			matches: true,
		},
		{
			name: "does not match when PropType differs",
			properties: message.Attributes{
				message.AttrType: "OtherEvent",
			},
			matches: false,
		},
		{
			name:       "does not match when PropType is missing",
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

	matcher := MatchTypeName[TestEvent]()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := matcher(tt.properties)
			if result != tt.matches {
				t.Errorf("expected match=%v, got %v for properties %+v", tt.matches, result, tt.properties)
			}
		})
	}
}

// TestNewCommandHandler_WithMultipleEvents verifies PropType
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

	// Verify PropType is set on all output messages
	for i, outMsg := range outputMsgs {
		propType, ok := outMsg.Attributes.Type()
		if !ok {
			t.Errorf("message %d: PropType not set", i)
			continue
		}

		if propType != "TestEvent" {
			t.Errorf("message %d: expected PropType=%q, got %q", i, "TestEvent", propType)
		}
	}
}
