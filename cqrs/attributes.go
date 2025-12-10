package cqrs

import (
	"reflect"

	"github.com/fxsml/gopipe/message"
)

// AttributeProvider is a function that provides message attributes based on
// the value being marshaled.
//
// AttributeProviders work at the type level, extracting attributes from the value itself.
// For input attribute propagation (like correlation IDs), use message-level middleware instead.
//
// This follows the same composable pattern as Matchers, allowing you to:
// - Access typed data via type assertions
// - Combine multiple providers
// - Create custom attribute logic
//
// Example:
//
//	provider := CombineAttrs(
//	    WithType("OrderCreated"),
//	    WithSource("/orders/api"),
//	)
type AttributeProvider func(v any) message.Attributes

// ============================================================================
// Attribute Provider Combinators
// ============================================================================

// CombineAttrs combines multiple attribute providers into one.
// Later providers can overwrite attributes from earlier ones.
//
// Example:
//
//	provider := CombineAttrs(
//	    WithTypeName(),
//	    WithSource("/orders/api"),
//	)
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

// WithType returns an attribute provider that sets the event type.
// Per CloudEvents, type describes what kind of event occurred.
//
// Example:
//
//	marshaler := NewJSONCommandMarshaler(
//	    WithType("OrderCreated"),
//	)
func WithType(eventType string) AttributeProvider {
	return func(v any) message.Attributes {
		return message.Attributes{message.AttrType: eventType}
	}
}

// WithSubject returns an attribute provider that sets a static subject.
//
// Example:
//
//	marshaler := NewJSONCommandMarshaler(
//	    WithSubject("order/ORD-001"),
//	)
func WithSubject(subject string) AttributeProvider {
	return func(v any) message.Attributes {
		return message.Attributes{message.AttrSubject: subject}
	}
}

// WithTypeName returns an attribute provider that sets AttrType to the
// reflected type name of the value.
//
// This is automatically applied by NewJSONCommandMarshaler if no providers are specified.
//
// Example:
//
//	marshaler := NewJSONCommandMarshaler(
//	    WithTypeName(),
//	)
//	// For OrderCreated{}, sets AttrType = "OrderCreated"
func WithTypeName() AttributeProvider {
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

// WithSubjectFromTypeName returns an attribute provider that sets both
// AttrSubject and AttrType to the reflected type name of the value.
//
// This is useful for event-driven systems where the subject should match the event type.
//
// Example:
//
//	marshaler := NewJSONCommandMarshaler(
//	    WithSubjectFromTypeName(),
//	)
//	// For OrderCreated{}, sets both:
//	// - AttrSubject = "OrderCreated"
//	// - AttrType = "OrderCreated"
func WithSubjectFromTypeName() AttributeProvider {
	return func(v any) message.Attributes {
		t := reflect.TypeOf(v)
		if t == nil {
			return message.Attributes{}
		}
		if t.Kind() == reflect.Ptr {
			t = t.Elem()
		}
		typeName := t.Name()
		return message.Attributes{
			message.AttrSubject: typeName,
			message.AttrType:    typeName,
		}
	}
}

// WithAttribute returns an attribute provider that sets a specific attribute key-value pair.
//
// Example:
//
//	marshaler := NewJSONCommandMarshaler(
//	    WithAttribute("priority", "high"),
//	)
func WithAttribute(key string, value any) AttributeProvider {
	return func(v any) message.Attributes {
		return message.Attributes{key: value}
	}
}

// WithTopic returns an attribute provider that sets the pub/sub topic for message routing.
//
// The topic attribute is used by publishers to determine the destination topic for messages.
// Empty string is a valid topic value representing the default topic. This attribute is
// intended for routing only and should not be forwarded to the broker by senders.
//
// Example:
//
//	marshaler := NewJSONCommandMarshaler(
//	    WithTopic("orders/created"),
//	)
func WithTopic(topic string) AttributeProvider {
	return func(v any) message.Attributes {
		return message.Attributes{message.AttrTopic: topic}
	}
}

// WithSource returns an attribute provider that sets the event source URI.
// Per CloudEvents, source identifies the context in which an event happened.
//
// Example:
//
//	marshaler := NewJSONCommandMarshaler(
//	    WithSource("/orders/api"),
//	)
func WithSource(source string) AttributeProvider {
	return func(v any) message.Attributes {
		return message.Attributes{message.AttrSource: source}
	}
}

// ============================================================================
// Custom Attribute Provider Example
// ============================================================================

// Example of a custom attribute provider that accesses typed data
// to build a dynamic subject:
//
//	type ArticleEvent interface {
//	    GetArticleID() string
//	    GetSalesChannel() string
//	}
//
//	func WithArticleSubject() AttributeProvider {
//	    return func(v any) message.Attributes {
//	        if article, ok := v.(ArticleEvent); ok {
//	            subject := fmt.Sprintf("article/%s;sales-channel/%s",
//	                article.GetArticleID(),
//	                article.GetSalesChannel())
//	            return message.Attributes{message.AttrSubject: subject}
//	        }
//	        return message.Attributes{}
//	    }
//	}
//
//	marshaler := NewJSONCommandMarshaler(
//	    WithTypeName(),
//	    WithArticleSubject(),
//	)
//
// Note: For correlation ID propagation, use message-level middleware instead:
//
//	router := message.NewRouter(
//	    message.RouterConfig{
//	        Middleware: []gopipe.MiddlewareFunc[*message.Message, *message.Message]{
//	            middleware.MessageCorrelation(),  // Propagate correlation IDs
//	        },
//	    },
//	    handlers...,
//	)
