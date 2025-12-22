package middleware

import (
	"context"

	"github.com/fxsml/gopipe/message"
	"github.com/fxsml/gopipe/pipe/middleware"
)

// MessageCorrelation returns middleware that propagates correlation ID from input to output messages.
func MessageCorrelation() middleware.Middleware[*message.Message, *message.Message] {
	return NewMessageMiddleware(
		func(ctx context.Context, msg *message.Message, next func() ([]*message.Message, error)) ([]*message.Message, error) {
			results, err := next()
			if err != nil {
				return results, err
			}

			// Propagate correlation ID to all output messages
			if corrID, ok := msg.Attributes.CorrelationID(); ok {
				for _, outMsg := range results {
					if outMsg.Attributes == nil {
						outMsg.Attributes = make(message.Attributes)
					}
					// Always overwrite correlation ID
					outMsg.Attributes[message.AttrCorrelationID] = corrID
				}
			}

			return results, nil
		},
	)
}
