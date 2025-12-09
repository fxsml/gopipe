package cqrs

import (
	"reflect"

	"github.com/fxsml/gopipe/message"
)

// PropertyProvider is a function that provides message properties based on
// the value being marshaled.
//
// PropertyProviders work at the type level, extracting properties from the value itself.
// For input property propagation (like correlation IDs), use message-level middleware instead.
//
// This follows the same composable pattern as Matchers, allowing you to:
// - Access typed data via type assertions
// - Combine multiple providers
// - Create custom property logic
//
// Example:
//
//	provider := CombineProps(
//	    WithType("event"),
//	    WithTypeName(),
//	)
type PropertyProvider func(v any) message.Properties

// ============================================================================
// Property Provider Combinators
// ============================================================================

// CombineProps combines multiple property providers into one.
// Later providers can overwrite properties from earlier ones.
//
// Example:
//
//	provider := CombineProps(
//	    WithType("event"),
//	    WithTypeName(),
//	)
func CombineProps(providers ...PropertyProvider) PropertyProvider {
	return func(v any) message.Properties {
		result := make(message.Properties)
		for _, provider := range providers {
			for k, val := range provider(v) {
				result[k] = val
			}
		}
		return result
	}
}

// ============================================================================
// Built-in Property Providers
// ============================================================================

// WithType returns a property provider that sets the generic message type
// (e.g., "command", "event", "query").
//
// Example:
//
//	marshaler := NewJSONCommandMarshaler(
//	    WithType("event"),
//	)
func WithType(msgType string) PropertyProvider {
	return func(v any) message.Properties {
		return message.Properties{"type": msgType}
	}
}

// WithSubject returns a property provider that sets a static subject.
//
// Example:
//
//	marshaler := NewJSONCommandMarshaler(
//	    WithSubject("OrderEvents"),
//	)
func WithSubject(subject string) PropertyProvider {
	return func(v any) message.Properties {
		return message.Properties{message.PropSubject: subject}
	}
}

// WithTypeName returns a property provider that sets PropType to the
// reflected type name of the value.
//
// This is automatically applied by NewJSONCommandMarshaler if no providers are specified.
//
// Example:
//
//	marshaler := NewJSONCommandMarshaler(
//	    WithTypeName(),
//	)
//	// For OrderCreated{}, sets PropType = "OrderCreated"
func WithTypeName() PropertyProvider {
	return func(v any) message.Properties {
		t := reflect.TypeOf(v)
		if t == nil {
			return message.Properties{}
		}
		if t.Kind() == reflect.Ptr {
			t = t.Elem()
		}
		return message.Properties{message.PropType: t.Name()}
	}
}

// WithSubjectFromTypeName returns a property provider that sets both
// PropSubject and PropType to the reflected type name of the value.
//
// This is useful for event-driven systems where the subject should match the event type.
//
// Example:
//
//	marshaler := NewJSONCommandMarshaler(
//	    WithType("event"),
//	    WithSubjectFromTypeName(),
//	)
//	// For OrderCreated{}, sets both:
//	// - PropSubject = "OrderCreated"
//	// - PropType = "OrderCreated"
func WithSubjectFromTypeName() PropertyProvider {
	return func(v any) message.Properties {
		t := reflect.TypeOf(v)
		if t == nil {
			return message.Properties{}
		}
		if t.Kind() == reflect.Ptr {
			t = t.Elem()
		}
		typeName := t.Name()
		return message.Properties{
			message.PropSubject: typeName,
			message.PropType:    typeName,
		}
	}
}

// WithProperty returns a property provider that sets a specific property key-value pair.
//
// Example:
//
//	marshaler := NewJSONCommandMarshaler(
//	    WithProperty("priority", "high"),
//	)
func WithProperty(key string, value any) PropertyProvider {
	return func(v any) message.Properties {
		return message.Properties{key: value}
	}
}

// ============================================================================
// Custom Property Provider Example
// ============================================================================

// Example of a custom property provider that accesses typed data
// to build a dynamic subject:
//
//	type ArticleEvent interface {
//	    GetArticleID() string
//	    GetSalesChannel() string
//	}
//
//	func WithArticleSubject() PropertyProvider {
//	    return func(v any) message.Properties {
//	        if article, ok := v.(ArticleEvent); ok {
//	            subject := fmt.Sprintf("article/%s;sales-channel/%s",
//	                article.GetArticleID(),
//	                article.GetSalesChannel())
//	            return message.Properties{message.PropSubject: subject}
//	        }
//	        return message.Properties{}
//	    }
//	}
//
//	marshaler := NewJSONCommandMarshaler(
//	    WithType("event"),
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
