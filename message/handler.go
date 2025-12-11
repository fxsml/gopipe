package message

import (
	"context"
)

// Handler processes messages matching specific attributes.
type Handler interface {
	Handle(ctx context.Context, msg *Message) ([]*Message, error)
	Match(attrs Attributes) bool
}

type handler struct {
	handle func(ctx context.Context, msg *Message) ([]*Message, error)
	match  func(attrs Attributes) bool
}

func (h *handler) Handle(ctx context.Context, msg *Message) ([]*Message, error) {
	return h.handle(ctx, msg)
}

func (h *handler) Match(attrs Attributes) bool {
	return h.match(attrs)
}

// NewHandler creates a handler from message processing and matching functions.
func NewHandler(
	handle func(ctx context.Context, msg *Message) ([]*Message, error),
	match func(attrs Attributes) bool,
) Handler {
	return &handler{
		handle: handle,
		match:  match,
	}
}

// Matcher matches message attributes.
type Matcher func(Attributes) bool

// Match combines multiple matchers with AND logic.
func Match(matchers ...Matcher) Matcher {
	return func(attrs Attributes) bool {
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
	return func(attrs Attributes) bool {
		s, _ := attrs.Subject()
		return s == subject
	}
}

// MatchType matches messages with the specified type.
func MatchType(msgType string) Matcher {
	return func(attrs Attributes) bool {
		t, _ := attrs.Type()
		return t == msgType
	}
}

// MatchAttribute matches messages where the attribute equals the given value.
func MatchAttribute(key string, value any) Matcher {
	return func(attrs Attributes) bool {
		v, ok := attrs[key]
		if !ok {
			return false
		}
		return v == value
	}
}

// MatchHasAttribute matches messages that have the specified attribute key.
func MatchHasAttribute(key string) Matcher {
	return func(attrs Attributes) bool {
		_, ok := attrs[key]
		return ok
	}
}
