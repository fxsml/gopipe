package cqrs

import (
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
	MatchTopic        = message.MatchTopic
)

// MatchGenericTypeOf matches messages where type equals the reflected type name of T.
func MatchGenericTypeOf[T any]() Matcher {
	typeName := GenericTypeOf[T]()
	return func(attrs message.Attributes) bool {
		t, _ := attrs.Type()
		return t == typeName
	}
}
