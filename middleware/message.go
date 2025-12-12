// Package middleware provides reusable middleware for message processing pipelines.
package middleware

import (
	"context"

	"github.com/fxsml/gopipe"
	"github.com/fxsml/gopipe/message"
)

// NewMessageMiddleware creates router middleware from a handler-style function.
//
// This is a convenience wrapper that converts a simple handler-style function
// into gopipe middleware, making it easier to write middleware without
// directly dealing with the Processor interface.
func NewMessageMiddleware(
	fn func(ctx context.Context, msg *message.Message, next func() ([]*message.Message, error)) ([]*message.Message, error),
) gopipe.MiddlewareFunc[*message.Message, *message.Message] {
	return func(proc gopipe.Processor[*message.Message, *message.Message]) gopipe.Processor[*message.Message, *message.Message] {
		return gopipe.NewProcessor(
			func(ctx context.Context, msg *message.Message) ([]*message.Message, error) {
				return fn(ctx, msg, func() ([]*message.Message, error) {
					return proc.Process(ctx, msg)
				})
			},
			func(msg *message.Message, err error) {
				proc.Cancel(msg, err)
			},
		)
	}
}
