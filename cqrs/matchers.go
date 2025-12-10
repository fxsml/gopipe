package cqrs

import (
	"reflect"

	"github.com/fxsml/gopipe/message"
)

// Matcher matches message attributes.
type Matcher func(message.Attributes) bool

// Match combines multiple matchers with AND logic.
func Match(matchers ...Matcher) Matcher {
	return func(attrs message.Attributes) bool {
		for _, matcher := range matchers {
			if !matcher(attrs) {
				return false
			}
		}
		return true
	}
}

// MatchSubject matches messages with the specified subject.
func MatchSubject(subject string) Matcher {
	return func(attrs message.Attributes) bool {
		s, _ := attrs.Subject()
		return s == subject
	}
}

// MatchType matches messages with the specified type.
func MatchType(msgType string) Matcher {
	return func(attrs message.Attributes) bool {
		t, _ := attrs["type"].(string)
		return t == msgType
	}
}

// MatchAttribute matches messages where the attribute equals the given value.
func MatchAttribute(key string, value any) Matcher {
	return func(attrs message.Attributes) bool {
		v, ok := attrs[key]
		if !ok {
			return false
		}
		return v == value
	}
}

// MatchTypeName matches messages where type equals the reflected type name of T.
func MatchTypeName[T any]() Matcher {
	typeName := typeNameOf[T]()
	return func(attrs message.Attributes) bool {
		t, _ := attrs.Type()
		return t == typeName
	}
}

// MatchHasAttribute matches messages that have the specified attribute key.
func MatchHasAttribute(key string) Matcher {
	return func(attrs message.Attributes) bool {
		_, ok := attrs[key]
		return ok
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
