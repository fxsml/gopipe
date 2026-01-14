package middleware

import (
	"context"

	"github.com/fxsml/gopipe/message"
)

// subjecter is satisfied by types with a Subject method.
// Uses duck typing - users implement Subject() string on their types.
type subjecter interface {
	Subject() string
}

// Subject sets AttrSubject on output messages from data implementing Subject().
// If the output message's Data has a Subject() string method, its return value
// is used as the CloudEvents subject attribute.
func Subject() message.Middleware {
	return func(next message.ProcessFunc) message.ProcessFunc {
		return func(ctx context.Context, msg *message.Message) ([]*message.Message, error) {
			outputs, err := next(ctx, msg)
			if err != nil {
				return nil, err
			}

			for _, out := range outputs {
				if s, ok := out.Data.(subjecter); ok {
					if out.Attributes == nil {
						out.Attributes = make(message.Attributes)
					}
					out.Attributes[message.AttrSubject] = s.Subject()
				}
			}

			return outputs, nil
		}
	}
}
