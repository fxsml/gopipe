package middleware

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/fxsml/gopipe/message"
)

// ErrMessageExpired is returned when a message's expirytime has passed.
var ErrMessageExpired = errors.New("message expired")

// Deadline enforces processing deadlines based on the expirytime extension.
// Sets a context deadline and returns ErrMessageExpired if the message expires.
// Messages without expirytime pass through unchanged.
func Deadline() message.Middleware {
	return func(next message.ProcessFunc) message.ProcessFunc {
		return func(ctx context.Context, msg *message.Message) ([]*message.Message, error) {
			expiry := msg.ExpiryTime()
			if expiry.IsZero() {
				return next(ctx, msg)
			}

			if time.Now().After(expiry) {
				return nil, ErrMessageExpired
			}

			ctx, cancel := context.WithDeadline(ctx, expiry)
			defer cancel()

			outputs, err := next(ctx, msg)
			expired := time.Now().After(expiry)
			if err != nil && errors.Is(err, context.DeadlineExceeded) && expired {
				return nil, fmt.Errorf("%w: %w", ErrMessageExpired, err)
			}
			return outputs, err
		}
	}
}
