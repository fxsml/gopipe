package middleware

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/fxsml/gopipe/message"
)

// ErrMissingRequiredAttr is returned when a required CloudEvents attribute is missing.
var ErrMissingRequiredAttr = errors.New("missing required attribute")

// ValidateRequired checks that all required CloudEvents attributes are set.
// Required per CloudEvents 1.0 spec: id, type, source, specversion.
func ValidateRequired() message.Middleware {
	return func(next message.ProcessFunc) message.ProcessFunc {
		return func(ctx context.Context, msg *message.Message) ([]*message.Message, error) {
			var missing []string
			if msg.ID() == "" {
				missing = append(missing, message.AttrID)
			}
			if msg.Type() == "" {
				missing = append(missing, message.AttrType)
			}
			if msg.Source() == "" {
				missing = append(missing, message.AttrSource)
			}
			if msg.SpecVersion() == "" {
				missing = append(missing, message.AttrSpecVersion)
			}
			if len(missing) > 0 {
				return nil, fmt.Errorf("%w: %s", ErrMissingRequiredAttr, strings.Join(missing, ", "))
			}
			return next(ctx, msg)
		}
	}
}
