package middleware

import (
	"context"

	"github.com/fxsml/gopipe/message"
)

// CorrelationID propagates the correlationid attribute from input to output messages.
func CorrelationID() message.Middleware {
	return func(next message.ProcessFunc) message.ProcessFunc {
		return func(ctx context.Context, msg *message.Message) ([]*message.Message, error) {
			correlationID, _ := msg.Attributes["correlationid"].(string)

			outputs, err := next(ctx, msg)
			if err != nil {
				return nil, err
			}

			if correlationID != "" {
				for _, out := range outputs {
					if out.Attributes == nil {
						out.Attributes = make(map[string]any)
					}
					out.Attributes["correlationid"] = correlationID
				}
			}

			return outputs, nil
		}
	}
}
