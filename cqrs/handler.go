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
//   - handle: Business logic function that processes the command and returns events
//   - marshaler: Used to serialize/deserialize commands and events
//   - match: Function to match incoming messages (e.g., by subject and type)
//   - props: Function to transform input properties into output properties
//
// The returned handler:
//   - Unmarshals the command from message payload
//   - Calls the business logic function
//   - Marshals resulting events into output messages
//   - Applies property transformation for output messages
//   - Acks the input message when complete
//   - Nacks the input message on error
//
// Example:
//
//	createOrderHandler := NewCommandHandler(
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
//	    marshaler,
//	    message.MatchSubjectAndType("CreateOrder", "command"),
//	    message.PropagateCorrelationWithType("event"),
//	)
func NewCommandHandler[Cmd, Evt any](
	handle func(ctx context.Context, cmd Cmd) ([]Evt, error),
	marshaler Marshaler,
	match func(prop message.Properties) bool,
	props func(inProp message.Properties, evt Evt) message.Properties,
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
				payload, err := marshaler.Marshal(evt)
				if err != nil {
					msg.Nack(err)
					return nil, err
				}

				outProps := props(msg.Properties, evt)
				outMsgs = append(outMsgs, message.New(payload, outProps))
			}

			// Ack when complete
			msg.Ack()
			return outMsgs, nil
		},
		match,
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
//   - handle: Side effect function that processes the event
//   - marshaler: Used to deserialize events
//   - match: Function to match incoming messages (e.g., by subject and type)
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
//	    func(ctx context.Context, evt OrderCreated) error {
//	        // Side effect: send email
//	        return emailService.SendOrderConfirmation(evt.CustomerID, evt.ID)
//	    },
//	    marshaler,
//	    message.MatchSubjectAndType("OrderCreated", "event"),
//	)
func NewEventHandler[Evt any](
	handle func(ctx context.Context, evt Evt) error,
	marshaler Marshaler,
	match func(prop message.Properties) bool,
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
		match,
	)
}
