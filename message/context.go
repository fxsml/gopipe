package message

import (
	"context"
	"time"
)

type contextKey string

const (
	messageKey contextKey = "message.message"
)

// messageContext is a custom context that reports message expiry as deadline
// without creating timers or goroutines. It delegates Done() and Err() to the
// parent context, so the deadline is only enforced if the parent enforces it.
//
// This design avoids resource leaks while still allowing handlers to query
// the effective deadline via ctx.Deadline().
type messageContext struct {
	context.Context
	msg    *Message
	expiry time.Time
}

// Deadline returns the earlier of parent deadline or message expiry.
func (c *messageContext) Deadline() (time.Time, bool) {
	parentDeadline, hasParent := c.Context.Deadline()

	if c.expiry.IsZero() {
		return parentDeadline, hasParent
	}

	if !hasParent || c.expiry.Before(parentDeadline) {
		return c.expiry, true
	}

	return parentDeadline, true
}

// Value returns message-specific values or delegates to parent.
// Locals (SetLocal/Local) are decoupled and not returned here.
func (c *messageContext) Value(key any) any {
	switch key {
	case messageKey:
		return c.msg
	default:
		return c.Context.Value(key)
	}
}

// MessageFromContext retrieves the Message from context.
// Returns nil if no message is present.
func MessageFromContext(ctx context.Context) *Message {
	v := ctx.Value(messageKey)
	if v == nil {
		return nil
	}
	msg, _ := v.(*Message)
	return msg
}

// FromContext is an alias for MessageFromContext.
func FromContext(ctx context.Context) *Message {
	return MessageFromContext(ctx)
}
