package cqrs

import (
	"reflect"

	"github.com/fxsml/gopipe/message"
)

// Matcher is an alias for message.Matcher for backward compatibility.
type Matcher = message.Matcher

// Re-export basic matchers from message package for backward compatibility.
var (
	Match             = message.Match
	MatchSubject      = message.MatchSubject
	MatchType         = message.MatchType
	MatchAttribute    = message.MatchAttribute
	MatchHasAttribute = message.MatchHasAttribute
)

// MatchTypeName matches messages where type equals the reflected type name of T.
// This is useful for CQRS handlers that match based on command/event struct types.
func MatchTypeName[T any]() Matcher {
	typeName := typeNameOf[T]()
	return func(attrs message.Attributes) bool {
		t, _ := attrs.Type()
		return t == typeName
	}
}

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
