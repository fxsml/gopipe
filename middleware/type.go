package middleware

import (
	"context"

	"github.com/fxsml/gopipe"
	"github.com/fxsml/gopipe/message"
)

// MessageType returns middleware that sets the type property on all output messages.
//
// This middleware is useful for ensuring all output messages have a consistent type,
// such as "event", "command", or "query".
//
// Example:
//
//	router := message.NewRouter(
//	    message.RouterConfig{
//	        Middleware: []gopipe.MiddlewareFunc[*message.Message, *message.Message]{
//	            middleware.MessageType("event"),
//	        },
//	    },
//	    handlers...,
//	)
func MessageType(msgType string) gopipe.MiddlewareFunc[*message.Message, *message.Message] {
	return NewMessageMiddleware(
		func(ctx context.Context, msg *message.Message, next func() ([]*message.Message, error)) ([]*message.Message, error) {
			results, err := next()
			if err != nil {
				return results, err
			}

			for _, outMsg := range results {
				if outMsg.Properties == nil {
					outMsg.Properties = make(message.Properties)
				}
				outMsg.Properties["type"] = msgType
			}

			return results, nil
		},
	)
}
