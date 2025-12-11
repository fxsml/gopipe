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
