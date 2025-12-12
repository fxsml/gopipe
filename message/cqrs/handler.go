package cqrs

import (
	"context"

	"github.com/fxsml/gopipe/message"
)

// Handler is an alias for message.Handler for backward compatibility.
type Handler = message.Handler

// NewHandler is an alias for message.NewHandler for backward compatibility.
var NewHandler = message.NewHandler

// NewCommandHandler creates a command handler that processes commands and returns events.
// The handler unmarshals commands, executes business logic, and marshals resulting events.
func NewCommandHandler[Cmd, Evt any](
	handle func(ctx context.Context, cmd Cmd) ([]Evt, error),
	match message.Matcher,
	marshaler CommandMarshaler,
) Handler {
	return message.NewHandler(
		func(ctx context.Context, msg *message.Message) ([]*message.Message, error) {
			// Unmarshal command
			var cmd Cmd
			if err := marshaler.Unmarshal(msg.Data, &cmd); err != nil {
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

				// Use marshaler to provide attributes
				attrs := marshaler.Attributes(evt)
				outMsgs = append(outMsgs, message.New(payload, attrs))
			}

			// Ack when complete
			msg.Ack()
			return outMsgs, nil
		},
		match,
	)
}

// NewEventHandler creates an event handler that processes events and performs side effects.
// Event handlers do not return output messages.
func NewEventHandler[Evt any](
	handle func(ctx context.Context, evt Evt) error,
	match message.Matcher,
	marshaler EventMarshaler,
) Handler {
	return message.NewHandler(
		func(ctx context.Context, msg *message.Message) ([]*message.Message, error) {
			// Unmarshal event
			var evt Evt
			if err := marshaler.Unmarshal(msg.Data, &evt); err != nil {
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
