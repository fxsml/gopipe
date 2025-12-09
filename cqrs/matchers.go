package cqrs

import (
	"reflect"

	"github.com/fxsml/gopipe/message"
)

// Matcher is a function that matches message properties.
type Matcher func(message.Properties) bool

// ============================================================================
// Matcher Combinators
// ============================================================================

// Match returns a matcher that combines multiple matchers with AND logic.
// All provided matchers must return true for the combined matcher to return true.
//
// Example:
//
//	matcher := cqrs.Match(
//	    cqrs.MatchSubject("CreateOrder"),
//	    cqrs.MatchType("command"),
//	)
//	// Matches messages with subject="CreateOrder" AND type="command"
func Match(matchers ...Matcher) Matcher {
	return func(prop message.Properties) bool {
		for _, matcher := range matchers {
			if !matcher(prop) {
				return false
			}
		}
		return true
	}
}

// ============================================================================
// Single-Responsibility Matchers
// ============================================================================

// MatchSubject returns a matcher that matches messages with the specified subject.
//
// Example:
//
//	matcher := cqrs.MatchSubject("CreateOrder")
//	// Matches messages with subject="CreateOrder"
func MatchSubject(subject string) Matcher {
	return func(prop message.Properties) bool {
		propSubject, _ := prop.Subject()
		return propSubject == subject
	}
}

// MatchType returns a matcher that matches messages with the specified type property.
//
// Example:
//
//	matcher := cqrs.MatchType("command")
//	// Matches messages with type="command"
func MatchType(msgType string) Matcher {
	return func(prop message.Properties) bool {
		propType, _ := prop["type"].(string)
		return propType == msgType
	}
}

// MatchProperty returns a matcher that matches messages where the specified property
// equals the given value.
//
// Example:
//
//	matcher := cqrs.MatchProperty("priority", "high")
//	// Matches messages with priority="high"
func MatchProperty(key string, value any) Matcher {
	return func(prop message.Properties) bool {
		propValue, ok := prop[key]
		if !ok {
			return false
		}
		return propValue == value
	}
}

// MatchTypeName returns a matcher that matches messages where the "type" property
// equals the reflected type name of T.
//
// This is useful for automatic type-based routing.
//
// Example:
//
//	type CreateOrder struct { ... }
//
//	matcher := cqrs.MatchTypeName[CreateOrder]()
//	// Matches messages with type="CreateOrder"
func MatchTypeName[T any]() Matcher {
	typeName := typeNameOf[T]()
	return func(prop message.Properties) bool {
		propType, _ := prop["type"].(string)
		return propType == typeName
	}
}

// MatchHasProperty returns a matcher that matches messages that have the specified
// property key (regardless of value).
//
// Example:
//
//	matcher := cqrs.MatchHasProperty("correlation-id")
//	// Matches messages that have a "correlation-id" property
func MatchHasProperty(key string) Matcher {
	return func(prop message.Properties) bool {
		_, ok := prop[key]
		return ok
	}
}

// ============================================================================
// Property Transformers
// ============================================================================

// PropagateCorrelation returns a property transformation function that
// propagates the correlation ID from input to output.
//
// Example:
//
//	props := cqrs.PropagateCorrelation(inProps, evt)
//	// Output properties will have correlation ID if present in input
func PropagateCorrelation[T any](inProp message.Properties, out T) message.Properties {
	props := message.Properties{}
	if corrID, ok := inProp.CorrelationID(); ok {
		props[message.PropCorrelationID] = corrID
	}
	return props
}

// WithType returns a property transformation function that sets the message type.
//
// Example:
//
//	props := cqrs.WithType("event")(inProps, evt)
//	// Output properties will have type="event"
func WithType(msgType string) func(message.Properties, any) message.Properties {
	return func(inProp message.Properties, out any) message.Properties {
		props := message.Properties{
			"type": msgType,
		}
		if corrID, ok := inProp.CorrelationID(); ok {
			props[message.PropCorrelationID] = corrID
		}
		return props
	}
}

// WithSubject returns a property transformation function that sets the subject.
//
// Example:
//
//	props := cqrs.WithSubject("OrderCreated")(inProps, evt)
//	// Output properties will have subject="OrderCreated"
func WithSubject(subject string) func(message.Properties, any) message.Properties {
	return func(inProp message.Properties, out any) message.Properties {
		props := message.Properties{
			message.PropSubject: subject,
		}
		if corrID, ok := inProp.CorrelationID(); ok {
			props[message.PropCorrelationID] = corrID
		}
		return props
	}
}

// WithSubjectAndType returns a property transformation function that sets
// both subject and type.
//
// Example:
//
//	props := cqrs.WithSubjectAndType("OrderCreated", "event")(inProps, evt)
//	// Output properties will have subject="OrderCreated" and type="event"
func WithSubjectAndType(subject, msgType string) func(message.Properties, any) message.Properties {
	return func(inProp message.Properties, out any) message.Properties {
		props := message.Properties{
			message.PropSubject: subject,
			"type":              msgType,
		}
		if corrID, ok := inProp.CorrelationID(); ok {
			props[message.PropCorrelationID] = corrID
		}
		return props
	}
}

// WithTypeAndName returns a property transformation function that sets the type
// and uses the reflected type name of T as the subject.
//
// Example:
//
//	type OrderCreated struct { ... }
//
//	props := cqrs.WithTypeAndName[OrderCreated]("event")(inProps, evt)
//	// Output properties will have subject="OrderCreated" and type="event"
func WithTypeAndName[T any](msgType string) func(message.Properties, T) message.Properties {
	typeName := typeNameOf[T]()
	return func(inProp message.Properties, out T) message.Properties {
		props := message.Properties{
			message.PropSubject: typeName,
			"type":              msgType,
		}
		if corrID, ok := inProp.CorrelationID(); ok {
			props[message.PropCorrelationID] = corrID
		}
		return props
	}
}

// ============================================================================
// Helpers
// ============================================================================

// typeNameOf returns the type name of T, stripping pointer indirection.
func typeNameOf[T any]() string {
	var zero T
	t := reflect.TypeOf(zero)
	if t == nil {
		return ""
	}
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	return t.Name()
}
