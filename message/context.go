package message

import "context"

type contextKey string

const attributesKey contextKey = "message.attributes"

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
