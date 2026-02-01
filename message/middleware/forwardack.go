package middleware

import (
	"context"

	"github.com/fxsml/gopipe/message"
)

// ForwardAck returns middleware that forwards acknowledgment to output messages.
// The input message is acked when ALL output messages are acked.
// If ANY output message is nacked, the input is immediately nacked.
//
// Use this for event sourcing patterns where a command should only be acked
// after all resulting events are successfully processed downstream.
//
// Important:
//   - Do not use with AutoAck; they are mutually exclusive
//   - Handlers should not manually ack the input message
//   - All outputs must eventually be acked or nacked; dropped outputs will
//     prevent the input from being acked (distributor auto-nacks unrouted messages)
//
// Example usage:
//
//	engine.Use(middleware.ForwardAck())
func ForwardAck() message.Middleware {
	return func(next message.ProcessFunc) message.ProcessFunc {
		return func(ctx context.Context, msg *message.Message) ([]*message.Message, error) {
			outputs, err := next(ctx, msg)
			if err != nil {
				msg.Nack(err)
				return nil, err
			}

			// No outputs - ack input immediately
			if len(outputs) == 0 {
				msg.Ack()
				return outputs, nil
			}

			// Check if input was already acked (handler acked manually)
			if msg.AckState() != message.AckPending {
				// Already acked/nacked - don't set up forwarding
				return outputs, nil
			}

			// Create shared acking: input acked when all outputs acked
			shared := message.NewSharedAcking(
				func() { msg.Ack() },
				func(e error) { msg.Nack(e) },
				len(outputs),
			)

			// Replace each output's acking with the shared acking
			for _, out := range outputs {
				out.Acking = shared
			}

			return outputs, nil
		}
	}
}
