// Package middleware provides reusable middleware for message processing pipelines.
package middleware

import (
	"context"

	"github.com/fxsml/gopipe/message"
	"github.com/fxsml/gopipe/pipe/middleware"
)

// NewMessageMiddleware creates router middleware from a handler-style function.
//
// This is a convenience wrapper that converts a simple handler-style function
// into gopipe middleware, making it easier to write middleware without
// directly dealing with the ProcessFunc type.
func NewMessageMiddleware(
	fn func(ctx context.Context, msg *message.Message, next func() ([]*message.Message, error)) ([]*message.Message, error),
) middleware.Middleware[*message.Message, *message.Message] {
	return func(next middleware.ProcessFunc[*message.Message, *message.Message]) middleware.ProcessFunc[*message.Message, *message.Message] {
		return func(ctx context.Context, msg *message.Message) ([]*message.Message, error) {
			return fn(ctx, msg, func() ([]*message.Message, error) {
				return next(ctx, msg)
			})
		}
	}
}
