package message

import (
	"context"

	"github.com/fxsml/gopipe"
)

// NewMiddleware creates router middleware from a handler-style function.
//
// This is a convenience wrapper that converts a simple handler-style function
// into gopipe middleware, making it easier for users to write middleware without
// directly dealing with the Processor interface.
//
// The provided function receives:
// - ctx: The context for the current message processing
// - msg: The message being processed
// - next: A function to call the next middleware/handler in the chain
//
// Example:
//
//	loggingMiddleware := message.NewMiddleware(
//	    func(ctx context.Context, msg *message.Message, next func() ([]*message.Message, error)) ([]*message.Message, error) {
//	        log.Printf("Processing message")
//	        result, err := next()
//	        log.Printf("Completed: error=%v", err != nil)
//	        return result, err
//	    },
//	)
//
//	router := message.NewRouter(
//	    message.RouterConfig{
//	        Middleware: []gopipe.MiddlewareFunc[*message.Message, *message.Message]{
//	            loggingMiddleware,
//	        },
//	    },
//	    handlers...,
//	)
func NewMiddleware(
	fn func(ctx context.Context, msg *Message, next func() ([]*Message, error)) ([]*Message, error),
) gopipe.MiddlewareFunc[*Message, *Message] {
	return func(proc gopipe.Processor[*Message, *Message]) gopipe.Processor[*Message, *Message] {
		return gopipe.NewProcessor(
			func(ctx context.Context, msg *Message) ([]*Message, error) {
				return fn(ctx, msg, func() ([]*Message, error) {
					return proc.Process(ctx, msg)
				})
			},
			func(msg *Message, err error) {
				proc.Cancel(msg, err)
			},
		)
	}
}
