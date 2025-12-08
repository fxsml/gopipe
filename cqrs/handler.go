package cqrs

import (
	"context"

	"github.com/fxsml/gopipe/message"
)

// NewCommandHandler creates a message.Handler that processes commands and returns events.
//
// Command handlers follow the pattern: Command → Business Logic → Events
//
// Type Parameters:
//   - Cmd: The command struct type (e.g., CreateOrder)
//   - Evt: The event struct type that results from the command (e.g., OrderCreated)
//
// Parameters:
//   - cmdName: The command name for routing (should match Marshaler.Name(Cmd))
//   - marshaler: Used to serialize/deserialize commands and events
//   - handle: Business logic function that processes the command and returns events
//
// The returned handler:
//   - Unmarshals the command from message payload
//   - Calls the business logic function
//   - Marshals resulting events into output messages
//   - Propagates correlation IDs for tracing
//   - Acks the input message when complete
//   - Nacks the input message on error
//
// Example:
//
//	createOrderHandler := NewCommandHandler(
//	    "CreateOrder",
//	    marshaler,
//	    func(ctx context.Context, cmd CreateOrder) ([]OrderCreated, error) {
//	        // Save to database
//	        if err := saveOrder(cmd); err != nil {
//	            return nil, err
//	        }
//
//	        // Return events
//	        return []OrderCreated{{
//	            ID:         cmd.ID,
//	            CustomerID: cmd.CustomerID,
//	            Amount:     cmd.Amount,
//	            CreatedAt:  time.Now(),
//	        }}, nil
//	    },
//	)
func NewCommandHandler[Cmd, Evt any](
	cmdName string,
	marshaler Marshaler,
	handle func(ctx context.Context, cmd Cmd) ([]Evt, error),
) message.Handler {
	return message.NewHandler(
		func(ctx context.Context, msg *message.Message) ([]*message.Message, error) {
			// Unmarshal command
			var cmd Cmd
			if err := marshaler.Unmarshal(msg.Payload, &cmd); err != nil {
				msg.Nack(err)
				return nil, err
			}

			// Execute business logic
			events, err := handle(ctx, cmd)
			if err != nil {
				msg.Nack(err)
				return nil, err
			}

			// Marshal events into output messages
			var outMsgs []*message.Message
			for _, evt := range events {
				evtName := marshaler.Name(evt)
				payload, err := marshaler.Marshal(evt)
				if err != nil {
					msg.Nack(err)
					return nil, err
				}

				props := message.Properties{
					message.PropSubject: evtName,
					"type":              "event",
				}

				// Propagate correlation ID for tracing
				if corrID, ok := msg.Properties.CorrelationID(); ok {
					props[message.PropCorrelationID] = corrID
				}

				outMsgs = append(outMsgs, message.New(payload, props))
			}

			// Ack when complete
			msg.Ack()
			return outMsgs, nil
		},
		func(prop message.Properties) bool {
			// Route by subject and type
			subject, _ := prop.Subject()
			msgType, _ := prop["type"].(string)
			return subject == cmdName && msgType == "command"
		},
	)
}

// NewEventHandler creates a message.Handler that processes events and performs side effects.
//
// Event handlers follow the pattern: Event → Side Effects
//
// Unlike command handlers, event handlers:
//   - Do NOT return output messages (return nil, nil)
//   - Perform side effects like sending emails, logging, analytics
//   - Are decoupled from workflow logic (use SagaCoordinator for workflows)
//
// Type Parameters:
//   - Evt: The event struct type (e.g., OrderCreated)
//
// Parameters:
//   - evtName: The event name for routing (should match Marshaler.Name(Evt))
//   - marshaler: Used to deserialize events
//   - handle: Side effect function that processes the event
//
// The returned handler:
//   - Unmarshals the event from message payload
//   - Calls the side effect function
//   - Acks the input message when complete
//   - Nacks the input message on error
//   - Returns nil, nil (no output messages)
//
// Example:
//
//	emailHandler := NewEventHandler(
//	    "OrderCreated",
//	    marshaler,
//	    func(ctx context.Context, evt OrderCreated) error {
//	        // Side effect: send email
//	        return emailService.SendOrderConfirmation(evt.CustomerID, evt.ID)
//	    },
//	)
func NewEventHandler[Evt any](
	evtName string,
	marshaler Marshaler,
	handle func(ctx context.Context, evt Evt) error,
) message.Handler {
	return message.NewHandler(
		func(ctx context.Context, msg *message.Message) ([]*message.Message, error) {
			// Unmarshal event
			var evt Evt
			if err := marshaler.Unmarshal(msg.Payload, &evt); err != nil {
				msg.Nack(err)
				return nil, err
			}

			// Execute side effect
			if err := handle(ctx, evt); err != nil {
				msg.Nack(err)
				return nil, err
			}

			// Ack when complete
			msg.Ack()
			return nil, nil // No output messages
		},
		func(prop message.Properties) bool {
			// Route by subject and type
			subject, _ := prop.Subject()
			msgType, _ := prop["type"].(string)
			return subject == evtName && msgType == "event"
		},
	)
}
