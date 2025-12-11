package cqrs

import (
	"reflect"

	"github.com/fxsml/gopipe/message"
)

// AttributeProvider extracts message attributes from a value being marshaled.
type AttributeProvider func(v any) message.Attributes

// ============================================================================
// Attribute Provider Combinators
// ============================================================================

// CombineAttrs combines multiple attribute providers.
// Later providers overwrite earlier ones.
func CombineAttrs(providers ...AttributeProvider) AttributeProvider {
	return func(v any) message.Attributes {
		result := make(message.Attributes)
		for _, provider := range providers {
			for k, val := range provider(v) {
				result[k] = val
			}
		}
		return result
	}
}

// ============================================================================
// Built-in Attribute Providers
// ============================================================================

// WithTypeOf sets AttrType to the reflected type name of the value.
// Automatically applied by NewJSONCommandMarshaler if no providers specified.
func WithTypeOf() AttributeProvider {
	return func(v any) message.Attributes {
		t := reflect.TypeOf(v)
		if t == nil {
			return message.Attributes{}
		}
		if t.Kind() == reflect.Ptr {
			t = t.Elem()
		}
		return message.Attributes{message.AttrType: t.Name()}
	}
}

func WithGenericTypeOf[T any]() AttributeProvider {
	typeName := GenericTypeOf[T]()
	return func(v any) message.Attributes {
		return message.Attributes{message.AttrType: typeName}
	}
}

// WithType sets a static type attribute.
func WithType(msgType string) AttributeProvider {
	return func(v any) message.Attributes {
		return message.Attributes{message.AttrType: msgType}
	}
}

// WithSubject sets a static subject attribute.
func WithSubject(subject string) AttributeProvider {
	return func(v any) message.Attributes {
		return message.Attributes{message.AttrSubject: subject}
	}
}

// WithAttribute sets a custom attribute key-value pair.
func WithAttribute(key string, value any) AttributeProvider {
	return func(v any) message.Attributes {
		return message.Attributes{key: value}
	}
}

// WithTopic sets the pub/sub topic for message routing.
func WithTopic(topic string) AttributeProvider {
	return func(v any) message.Attributes {
		return message.Attributes{message.AttrTopic: topic}
	}
}

// WithSource sets the CloudEvents source URI.
func WithSource(source string) AttributeProvider {
	return func(v any) message.Attributes {
		return message.Attributes{message.AttrSource: source}
	}
}
