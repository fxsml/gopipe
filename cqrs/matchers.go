package cqrs

import (
	"reflect"

	"github.com/fxsml/gopipe/message"
)

// Matcher is a function that matches message properties.
type Matcher func(message.Attributes) bool

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
	return func(prop message.Attributes) bool {
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
	return func(prop message.Attributes) bool {
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
	return func(prop message.Attributes) bool {
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
	return func(prop message.Attributes) bool {
		propValue, ok := prop[key]
		if !ok {
			return false
		}
		return propValue == value
	}
}

// MatchTypeName returns a matcher that matches messages where the message_type property
// equals the reflected type name of T.
//
// This is useful for automatic type-based routing using the specific type name
// (e.g., "OrderCreated", "CreateOrder") rather than generic types ("command", "event").
//
// Example:
//
//	type CreateOrder struct { ... }
//
//	matcher := cqrs.MatchTypeName[CreateOrder]()
//	// Matches messages with message_type="CreateOrder"
func MatchTypeName[T any]() Matcher {
	typeName := typeNameOf[T]()
	return func(prop message.Attributes) bool {
		propType, _ := prop.Type()
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
	return func(prop message.Attributes) bool {
		_, ok := prop[key]
		return ok
	}
}

// ============================================================================
// Property Transformers (Deprecated)
// ============================================================================
//
// These functions are deprecated. Use AttributeProvider functions instead:
// - PropagateCorrelation() -> AttributeProvider in properties.go
// - WithType() -> AttributeProvider in properties.go
// - WithSubject() -> AttributeProvider in properties.go
// - WithSubjectAndType() -> CombineAttrs(WithSubject(), WithType())
// - WithTypeAndName[T]() -> CombineAttrs(WithTypeName(), WithType())

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
