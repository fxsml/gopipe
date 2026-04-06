package message

import (
	"context"
	"time"
)

type contextKey string

const (
	messageKey    contextKey = "message.message"
	rawMessageKey contextKey = "message.raw_message"
)

// messageContext is a custom context that reports message expiry as deadline
// without creating timers or goroutines. It delegates Done() and Err() to the
// parent context, so the deadline is only enforced if the parent enforces it.
//
// This design avoids resource leaks while still allowing handlers to query
// the effective deadline via ctx.Deadline().
type messageContext struct {
	context.Context
	msg    any // *Message or *RawMessage
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
		if msg, ok := c.msg.(*Message); ok {
			return msg
		}
		return nil
	case rawMessageKey:
		if msg, ok := c.msg.(*RawMessage); ok {
			return msg
		}
		return nil
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

// RawMessageFromContext retrieves the RawMessage from context.
// Returns nil if no raw message is present.
func RawMessageFromContext(ctx context.Context) *RawMessage {
	v := ctx.Value(rawMessageKey)
	if v == nil {
		return nil
	}
	msg, _ := v.(*RawMessage)
	return msg
}
