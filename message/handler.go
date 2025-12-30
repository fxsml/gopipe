package message

import (
	"context"
	"crypto/rand"
	"fmt"
	"reflect"
	"time"
)

// Handler processes messages of a specific CE type.
type Handler interface {
	// EventType returns the CE type this handler processes.
	EventType() string

	// NewInput creates a new instance for unmarshaling input data.
	NewInput() any

	// Handle processes a message and returns output messages.
	Handle(ctx context.Context, msg *Message) ([]*Message, error)
}

// handler wraps a typed handler function.
type handler[T any] struct {
	eventType string
	fn        func(ctx context.Context, msg *Message) ([]*Message, error)
}

// NewHandler creates a handler from a typed function.
// The generic type T is used for unmarshaling and event type derivation.
// NamingStrategy derives EventType from T.
func NewHandler[T any](
	fn func(ctx context.Context, msg *Message) ([]*Message, error),
	naming NamingStrategy,
) Handler {
	var zero T
	t := reflect.TypeOf(zero)
	return &handler[T]{
		eventType: naming.TypeName(t),
		fn:        fn,
	}
}

func (h *handler[T]) EventType() string {
	return h.eventType
}

func (h *handler[T]) NewInput() any {
	return new(T)
}

func (h *handler[T]) Handle(ctx context.Context, msg *Message) ([]*Message, error) {
	return h.fn(ctx, msg)
}

// commandHandler wraps a command function with convention-based output.
type commandHandler[C, E any] struct {
	eventType string
	source    string
	naming    NamingStrategy
	fn        func(ctx context.Context, cmd C) ([]E, error)
}

// NewCommandHandler creates a handler that receives commands directly.
// Config provides Source and NamingStrategy for deriving CE types.
func NewCommandHandler[C, E any](
	fn func(ctx context.Context, cmd C) ([]E, error),
	cfg CommandHandlerConfig,
) Handler {
	var zeroC C
	t := reflect.TypeOf(zeroC)
	return &commandHandler[C, E]{
		eventType: cfg.Naming.TypeName(t),
		source:    cfg.Source,
		naming:    cfg.Naming,
		fn:        fn,
	}
}

func (h *commandHandler[C, E]) EventType() string {
	return h.eventType
}

func (h *commandHandler[C, E]) NewInput() any {
	return new(C)
}

func (h *commandHandler[C, E]) Handle(ctx context.Context, msg *Message) ([]*Message, error) {
	var cmd C
	if msg.Data != nil {
		if v, ok := msg.Data.(*C); ok {
			cmd = *v
		} else if v, ok := msg.Data.(C); ok {
			cmd = v
		}
	}

	// Store attributes in context for handler access
	ctx = contextWithAttributes(ctx, msg.Attributes)

	events, err := h.fn(ctx, cmd)
	if err != nil {
		return nil, err
	}

	var zeroE E
	eventType := h.naming.TypeName(reflect.TypeOf(zeroE))

	outputs := make([]*Message, len(events))
	for i, event := range events {
		outputs[i] = New[any](event, Attributes{
			"id":          newUUID(),
			"specversion": "1.0",
			"type":        eventType,
			"source":      h.source,
			"time":        time.Now().UTC().Format(time.RFC3339),
		})
	}

	return outputs, nil
}

// newUUID generates a UUID v4 string using crypto/rand.
func newUUID() string {
	var u [16]byte
	rand.Read(u[:])
	u[6] = (u[6] & 0x0f) | 0x40 // version 4
	u[8] = (u[8] & 0x3f) | 0x80 // variant 10
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", u[0:4], u[4:6], u[6:8], u[8:10], u[10:16])
}

// Verify handlers implement Handler.
var (
	_ Handler = (*handler[any])(nil)
	_ Handler = (*commandHandler[any, any])(nil)
)
