package middleware

import (
	"context"

	"github.com/fxsml/gopipe"
	"github.com/fxsml/gopipe/message"
)

// MessageCorrelation returns middleware that propagates correlation ID from input to output messages.
//
// This middleware ensures that correlation IDs are automatically carried forward from input messages
// to all output messages, enabling request tracing across message processing pipelines.
//
// Example:
//
//	router := message.NewRouter(
//	    message.RouterConfig{
//	        Middleware: []gopipe.MiddlewareFunc[*message.Message, *message.Message]{
//	            middleware.MessageCorrelation(),
//	        },
//	    },
//	    handlers...,
//	)
func MessageCorrelation() gopipe.MiddlewareFunc[*message.Message, *message.Message] {
	return NewMessageMiddleware(
		func(ctx context.Context, msg *message.Message, next func() ([]*message.Message, error)) ([]*message.Message, error) {
			results, err := next()
			if err != nil {
				return results, err
			}

			// Propagate correlation ID to all output messages
			if corrID, ok := msg.Properties.CorrelationID(); ok {
				for _, outMsg := range results {
					if outMsg.Properties == nil {
						outMsg.Properties = make(message.Properties)
					}
					// Always overwrite correlation ID
					outMsg.Properties[message.PropCorrelationID] = corrID
				}
			}

			return results, nil
		},
	)
}
