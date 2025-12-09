package cqrs

import (
	"reflect"

	"github.com/fxsml/gopipe/message"
)

// CreateCommand creates a command message from a command struct.
//
// This helper is useful for:
//   - Saga coordinators creating commands
//   - Initial command creation in application code
//   - Testing
//
// Parameters:
//   - marshaler: Used to serialize the command and extract its type name
//   - cmd: The command struct (e.g., CreateOrder{...})
//   - props: Optional properties to set on the message
//
// The function automatically sets:
//   - Subject from marshaler.Name(cmd)
//   - Type to "command"
//
// Example:
//
//	// In a saga coordinator
//	func (s *OrderSagaCoordinator) OnEvent(ctx, msg) ([]*message.Message, error) {
//	    return []*message.Message{
//	        CreateCommand(s.marshaler, ChargePayment{OrderID: "123", Amount: 100},
//	            message.Properties{
//	                message.PropCorrelationID: msg.Properties.CorrelationID(),
//	            },
//	        ),
//	    }, nil
//	}
//
//	// Creating initial command
//	cmd := CreateCommand(marshaler, CreateOrder{ID: "order-1", Amount: 100},
//	    message.Properties{
//	        message.PropCorrelationID: "corr-123",
//	    },
//	)
func CreateCommand(marshaler Marshaler, cmd any, props message.Properties) *message.Message {
	payload, _ := marshaler.Marshal(cmd)

	if props == nil {
		props = message.Properties{}
	}

	// Extract type name from the command
	t := reflect.TypeOf(cmd)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	props[message.PropSubject] = t.Name()
	props[message.PropType] = t.Name()
	props["type"] = "command"

	return message.New(payload, props)
}

// CreateCommands creates multiple command messages from command structs.
// This is a convenience function for saga coordinators that need to create multiple commands.
//
// Parameters:
//   - marshaler: Used to serialize commands
//   - correlationID: Correlation ID to propagate to all commands (optional, can be empty)
//   - cmds: Variable number of command structs
//
// Example:
//
//	func (s *OrderSagaCoordinator) OnEvent(ctx, msg) ([]*message.Message, error) {
//	    corrID, _ := msg.Properties.CorrelationID()
//
//	    return CreateCommands(s.marshaler, corrID,
//	        ChargePayment{OrderID: evt.ID, Amount: evt.Amount},
//	        ReserveInventory{OrderID: evt.ID, SKU: "SKU-123"},
//	    ), nil
//	}
func CreateCommands(marshaler Marshaler, correlationID string, cmds ...any) []*message.Message {
	msgs := make([]*message.Message, 0, len(cmds))

	for _, cmd := range cmds {
		props := message.Properties{}
		if correlationID != "" {
			props[message.PropCorrelationID] = correlationID
		}

		msgs = append(msgs, CreateCommand(marshaler, cmd, props))
	}

	return msgs
}
