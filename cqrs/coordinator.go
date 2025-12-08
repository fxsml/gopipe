package cqrs

import (
	"context"

	"github.com/fxsml/gopipe/message"
)

// SagaCoordinator defines workflow logic for saga patterns.
// It receives events and returns commands to trigger the next saga steps.
//
// This interface separates workflow coordination from event side effects:
//   - Event handlers perform side effects (emails, logging, analytics)
//   - Saga coordinators define workflow steps (what happens next)
//
// Example:
//
//	type OrderSagaCoordinator struct {
//	    marshaler Marshaler
//	}
//
//	func (s *OrderSagaCoordinator) OnEvent(ctx context.Context, msg *message.Message) ([]*message.Message, error) {
//	    subject, _ := msg.Properties.Subject()
//
//	    switch subject {
//	    case "OrderCreated":
//	        var evt OrderCreated
//	        s.marshaler.Unmarshal(msg.Payload, &evt)
//
//	        // Workflow logic: what commands to trigger next?
//	        return s.createCommands(
//	            ChargePayment{OrderID: evt.ID, Amount: evt.Amount},
//	            ReserveInventory{OrderID: evt.ID},
//	        )
//
//	    case "PaymentCharged":
//	        return s.createCommands(ShipOrder{...})
//
//	    case "OrderShipped":
//	        return nil, nil  // Terminal event
//	    }
//	}
type SagaCoordinator interface {
	// OnEvent handles an event and returns commands for the next saga steps.
	// Returns nil, nil for terminal events or events that don't trigger commands.
	OnEvent(ctx context.Context, msg *message.Message) ([]*message.Message, error)
}
