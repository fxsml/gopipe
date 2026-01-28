package middleware

import (
	"context"

	"github.com/fxsml/gopipe/message"
)

// AutoAck returns middleware that automatically acks messages on success
// and nacks on error. Use this to restore automatic acking behavior
// for handlers that don't need custom ack timing.
//
// Example usage:
//
//	engine.Use(middleware.AutoAck())
func AutoAck() message.Middleware {
	return func(next message.ProcessFunc) message.ProcessFunc {
		return func(ctx context.Context, msg *message.Message) ([]*message.Message, error) {
			results, err := next(ctx, msg)
			if err != nil {
				msg.Nack(err)
				return nil, err
			}
			msg.Ack()
			return results, nil
		}
	}
}
