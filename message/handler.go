package message

import (
	"context"
	"fmt"
	"maps"
	"reflect"
	"time"
)

// CommandHandlerConfig configures a command handler.
type CommandHandlerConfig struct {
	Source     string          // required, CE source attribute
	Naming     EventTypeNaming // optional, derives CE types for input and output
	Attributes Attributes      // optional, merged into all output messages

	// Marshaler unmarshals raw input Data and marshals output Data
	// (default NewJSONMarshaler()). Ignored if DisableMarshaler is true.
	Marshaler Marshaler
	// DisableMarshaler opts this handler out of marshal-by-default: input
	// Data is used as-is (typed-through) instead of unmarshaled from raw
	// []byte, and output Data is left typed instead of marshaled to bytes.
	DisableMarshaler bool
	// Subject, if set, derives the CE subject attribute from each typed
	// output event, before marshaling.
	Subject func(data any) string
}

// Handler processes messages of a specific CE type.
type Handler interface {
	// EventType returns the CE type this handler processes.
	EventType() string

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
// EventTypeNaming derives EventType from T. If nil, DefaultNaming is used.
func NewHandler[T any](
	fn func(ctx context.Context, msg *Message) ([]*Message, error),
	naming EventTypeNaming,
) Handler {
	if naming == nil {
		naming = DefaultNaming
	}
	var zero T
	t := reflect.TypeOf(zero)
	return &handler[T]{
		eventType: naming.EventType(t),
		fn:        fn,
	}
}

func (h *handler[T]) EventType() string {
	return h.eventType
}

func (h *handler[T]) Handle(ctx context.Context, msg *Message) ([]*Message, error) {
	return h.fn(ctx, msg)
}

// commandHandler wraps a command function with convention-based output.
type commandHandler[C, E any] struct {
	eventType        string
	source           string
	naming           EventTypeNaming
	attrs            Attributes
	marshaler        Marshaler
	disableMarshaler bool
	subject          func(data any) string
	fn               func(ctx context.Context, cmd C) ([]E, error)
}

// NewCommandHandler creates a handler that receives commands directly.
// Config provides Source and EventTypeNaming for deriving CE types.
// If Naming is nil, DefaultNaming is used.
//
// By default (DisableMarshaler: false), input Data is unmarshaled from raw
// []byte using cfg.Marshaler (default NewJSONMarshaler()), and output Data
// is marshaled to raw []byte the same way. Set DisableMarshaler: true to
// operate typed-through instead: input Data is used as-is, output Data is
// left typed.
func NewCommandHandler[C, E any](
	fn func(ctx context.Context, cmd C) ([]E, error),
	cfg CommandHandlerConfig,
) Handler {
	naming := cfg.Naming
	if naming == nil {
		naming = DefaultNaming
	}
	marshaler := cfg.Marshaler
	if marshaler == nil {
		marshaler = NewJSONMarshaler()
	}
	var zeroC C
	t := reflect.TypeOf(zeroC)
	return &commandHandler[C, E]{
		eventType:        naming.EventType(t),
		source:           cfg.Source,
		naming:           naming,
		attrs:            cfg.Attributes,
		marshaler:        marshaler,
		disableMarshaler: cfg.DisableMarshaler,
		subject:          cfg.Subject,
		fn:               fn,
	}
}

func (h *commandHandler[C, E]) EventType() string {
	return h.eventType
}

func (h *commandHandler[C, E]) Handle(ctx context.Context, msg *Message) ([]*Message, error) {
	cmd, err := h.command(msg)
	if err != nil {
		return nil, err
	}

	events, err := h.fn(ctx, cmd)
	if err != nil {
		return nil, err
	}

	var zeroE E
	eventType := h.naming.EventType(reflect.TypeOf(zeroE))

	outputs := make([]*Message, len(events))
	for i, event := range events {
		attrs := Attributes{
			AttrID:          NewID(),
			AttrSpecVersion: "1.0",
			AttrType:        eventType,
			AttrSource:      h.source,
			AttrTime:        time.Now().UTC(),
		}
		maps.Copy(attrs, h.attrs)

		if h.subject != nil {
			attrs[AttrSubject] = h.subject(event)
		}

		data, err := h.output(event, attrs)
		if err != nil {
			return nil, err
		}
		outputs[i] = New(data, attrs, nil)
	}

	return outputs, nil
}

// command extracts the typed command from msg.Data, unmarshaling from raw
// []byte unless disableMarshaler is set.
func (h *commandHandler[C, E]) command(msg *Message) (C, error) {
	var cmd C
	if h.disableMarshaler {
		switch v := msg.Data.(type) {
		case *C:
			return *v, nil
		case C:
			return v, nil
		default:
			return cmd, fmt.Errorf("%w: got %T, want %T", ErrCommandDataMismatch, msg.Data, cmd)
		}
	}

	raw, ok := msg.Raw()
	if !ok {
		return cmd, fmt.Errorf("%w: want raw []byte, got %T", ErrUnexpectedDataType, msg.Data)
	}
	if err := h.marshaler.Unmarshal(raw, &cmd); err != nil {
		return cmd, err
	}
	return cmd, nil
}

// output produces the Data value for an output message, marshaling event to
// raw []byte (and setting attrs' AttrDataContentType) unless disableMarshaler
// is set.
func (h *commandHandler[C, E]) output(event E, attrs Attributes) (any, error) {
	if h.disableMarshaler {
		return event, nil
	}
	encoded, err := h.marshaler.Marshal(event)
	if err != nil {
		return nil, err
	}
	attrs[AttrDataContentType] = h.marshaler.DataContentType()
	return encoded, nil
}

// Verify handlers implement Handler.
var (
	_ Handler = (*handler[any])(nil)
	_ Handler = (*commandHandler[any, any])(nil)
)
