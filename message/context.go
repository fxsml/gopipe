package message

import "context"

type contextKey string

const (
	attributesKey contextKey = "message.attributes"
	messageKey    contextKey = "message.message"
	rawMessageKey contextKey = "message.raw_message"
)

// contextWithAttributes stores attributes in context.
func contextWithAttributes(ctx context.Context, attrs Attributes) context.Context {
	return context.WithValue(ctx, attributesKey, attrs)
}

// AttributesFromContext retrieves message attributes from context.
// Returns nil if no attributes are present.
func AttributesFromContext(ctx context.Context) Attributes {
	v := ctx.Value(attributesKey)
	if v == nil {
		return nil
	}
	attrs, _ := v.(Attributes)
	return attrs
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
