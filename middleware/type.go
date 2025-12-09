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
	return func(proc gopipe.Processor[*message.Message, *message.Message]) gopipe.Processor[*message.Message, *message.Message] {
		return gopipe.NewProcessor(
			func(ctx context.Context, msg *message.Message) ([]*message.Message, error) {
				results, err := proc.Process(ctx, msg)
				if err != nil {
					return results, err
				}

				for _, outMsg := range results {
					if outMsg.Properties == nil {
						outMsg.Properties = make(message.Properties)
					}
					outMsg.Properties["type"] = msgType
				}

				return results, err
			},
			func(msg *message.Message, err error) {
				proc.Cancel(msg, err)
			},
		)
	}
}
