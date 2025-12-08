package main

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/fxsml/gopipe/message"
)

// TestCommandHandlerAcking verifies that command handlers:
// 1. Ack the input message when successful
// 2. Do NOT propagate acking to output messages
// 3. Do propagate correlation IDs
func TestCommandHandlerAcking(t *testing.T) {
	marshaler := JSONMarshaler{}

	var ackCalled, nackCalled bool
	inputAck := func() { ackCalled = true }
	inputNack := func(err error) { nackCalled = true }

	// Command handler that produces events
	handler := NewCommandHandler(
		"CreateOrder",
		marshaler,
		func(ctx context.Context, cmd CreateOrder) ([]OrderCreated, error) {
			return []OrderCreated{{
				ID:         cmd.ID,
				CustomerID: cmd.CustomerID,
				Amount:     cmd.Amount,
			}}, nil
		},
	)

	// Create input message with acking
	cmdPayload, _ := json.Marshal(CreateOrder{
		ID:         "order-123",
		CustomerID: "customer-456",
		Amount:     100,
	})

	inputMsg := message.NewWithAcking(
		cmdPayload,
		message.Properties{
			message.PropSubject:       "CreateOrder",
			message.PropCorrelationID: "corr-abc",
			"type":                    "command",
		},
		inputAck,
		inputNack,
	)

	// Execute handler
	ctx := context.Background()
	outputMsgs, err := handler.Handle(ctx, inputMsg)

	// Assertions
	if err != nil {
		t.Fatalf("Handler failed: %v", err)
	}

	// ✅ ASSERT: Input message was acked
	if !ackCalled {
		t.Error("Expected input message to be acked")
	}

	if nackCalled {
		t.Error("Input message should not be nacked")
	}

	// ✅ ASSERT: Got output messages
	if len(outputMsgs) != 1 {
		t.Fatalf("Expected 1 output message, got %d", len(outputMsgs))
	}

	outputMsg := outputMsgs[0]

	// ✅ ASSERT: Output message does NOT have acking
	// We test this by trying to ack - it should return false (no acking)
	if outputMsg.Ack() {
		t.Error("Output message should NOT have acking (Ack() should return false)")
	}

	// ✅ ASSERT: Correlation ID was propagated
	outputCorrID, ok := outputMsg.Properties.CorrelationID()
	if !ok {
		t.Error("Output message should have correlation ID")
	}

	if outputCorrID != "corr-abc" {
		t.Errorf("Expected correlation ID 'corr-abc', got '%s'", outputCorrID)
	}

	// ✅ ASSERT: Output is an event
	msgType, _ := outputMsg.Properties["type"].(string)
	if msgType != "event" {
		t.Errorf("Expected output type 'event', got '%s'", msgType)
	}

	subject, _ := outputMsg.Properties.Subject()
	if subject != "OrderCreated" {
		t.Errorf("Expected subject 'OrderCreated', got '%s'", subject)
	}
}

// TestCommandHandlerNacking verifies that command handlers
// nack the input message on error
func TestCommandHandlerNacking(t *testing.T) {
	marshaler := JSONMarshaler{}

	var ackCalled, nackCalled bool
	var nackErr error

	inputAck := func() { ackCalled = true }
	inputNack := func(err error) {
		nackCalled = true
		nackErr = err
	}

	// Command handler that fails
	handler := NewCommandHandler(
		"CreateOrder",
		marshaler,
		func(ctx context.Context, cmd CreateOrder) ([]OrderCreated, error) {
			return nil, &testError{msg: "insufficient funds"}
		},
	)

	cmdPayload, _ := json.Marshal(CreateOrder{
		ID:     "order-123",
		Amount: 100,
	})

	inputMsg := message.NewWithAcking(
		cmdPayload,
		message.Properties{
			message.PropSubject: "CreateOrder",
			"type":              "command",
		},
		inputAck,
		inputNack,
	)

	// Execute handler
	ctx := context.Background()
	outputMsgs, err := handler.Handle(ctx, inputMsg)

	// Assertions
	if err == nil {
		t.Fatal("Expected handler to return error")
	}

	// ✅ ASSERT: Input message was nacked
	if !nackCalled {
		t.Error("Expected input message to be nacked")
	}

	if ackCalled {
		t.Error("Input message should not be acked")
	}

	// ✅ ASSERT: Nack error was passed through
	if nackErr == nil {
		t.Error("Expected nack error to be set")
	}

	// ✅ ASSERT: No output messages on error
	if outputMsgs != nil && len(outputMsgs) > 0 {
		t.Error("Expected no output messages on error")
	}
}

// TestSagaCoordinatorDoesNotCopyAcking verifies that saga coordinator
// creates commands without acking (internal processing)
func TestSagaCoordinatorDoesNotCopyAcking(t *testing.T) {
	marshaler := JSONMarshaler{}
	coordinator := &OrderSagaCoordinator{marshaler: marshaler}

	// Create event with acking
	eventPayload, _ := json.Marshal(OrderCreated{
		ID:         "order-123",
		CustomerID: "customer-456",
		Amount:     100,
	})

	var ackCalled bool
	eventMsg := message.NewWithAcking(
		eventPayload,
		message.Properties{
			message.PropSubject:       "OrderCreated",
			message.PropCorrelationID: "corr-xyz",
			"type":                    "event",
		},
		func() { ackCalled = true },
		func(error) {},
	)

	// Execute saga coordinator
	ctx := context.Background()
	outputCmds, err := coordinator.OnEvent(ctx, eventMsg)

	if err != nil {
		t.Fatalf("Saga coordinator failed: %v", err)
	}

	// ✅ ASSERT: Got output commands
	if len(outputCmds) < 1 {
		t.Fatal("Expected at least 1 output command")
	}

	// ✅ ASSERT: Output commands do NOT have acking
	for i, cmd := range outputCmds {
		if cmd.Ack() {
			t.Errorf("Output command %d should NOT have acking", i)
		}
	}

	// ✅ ASSERT: Correlation ID propagated
	for i, cmd := range outputCmds {
		corrID, ok := cmd.Properties.CorrelationID()
		if !ok {
			t.Errorf("Output command %d should have correlation ID", i)
		}

		if corrID != "corr-xyz" {
			t.Errorf("Output command %d: expected correlation ID 'corr-xyz', got '%s'", i, corrID)
		}
	}

	// ✅ ASSERT: Event message was NOT acked by coordinator
	// (Coordinator doesn't ack - that's done by event processor's router)
	if ackCalled {
		t.Error("Event message should not be acked by saga coordinator")
	}
}

type testError struct {
	msg string
}

func (e *testError) Error() string {
	return e.msg
}
