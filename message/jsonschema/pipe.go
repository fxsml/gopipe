package jsonschema

import (
	"context"

	"github.com/fxsml/gopipe/message"
	"github.com/fxsml/gopipe/pipe/middleware"
)

// UnmarshalPipe is a convenience wrapper that combines type registration
// and schema validation into a single component.
//
// It reuses message.UnmarshalPipe internally, passing the Marshaler as both
// the InputRegistry (for type creation) and Marshaler (for validation).
type UnmarshalPipe struct {
	inner *message.UnmarshalPipe
}

// NewUnmarshalPipe creates a validating unmarshal pipe.
// The Marshaler serves dual purpose: creating typed instances via NewInput()
// and validating them via Unmarshal().
//
// This is equivalent to:
//
//	message.NewUnmarshalPipe(marshaler, marshaler, cfg)
//
// But with a cleaner API that makes the dual nature explicit.
func NewUnmarshalPipe(m *Marshaler, cfg message.PipeConfig) *UnmarshalPipe {
	return &UnmarshalPipe{
		inner: message.NewUnmarshalPipe(m, m, cfg),
	}
}

// Pipe starts the unmarshal pipeline.
func (p *UnmarshalPipe) Pipe(ctx context.Context, in <-chan *message.RawMessage) (<-chan *message.Message, error) {
	return p.inner.Pipe(ctx, in)
}

// Use adds middleware to the unmarshal processing chain.
// Middleware is applied in the order it is added.
// Returns ErrAlreadyStarted if the pipe has already been started.
func (p *UnmarshalPipe) Use(mw ...middleware.Middleware[*message.RawMessage, *message.Message]) error {
	return p.inner.Use(mw...)
}

// MarshalPipe is a convenience wrapper for marshaling with schema validation.
//
// It reuses message.MarshalPipe internally, validating messages before
// marshaling them to bytes.
type MarshalPipe struct {
	inner *message.MarshalPipe
}

// NewMarshalPipe creates a validating marshal pipe.
// Messages are validated against their registered schemas before marshaling.
//
// This is equivalent to:
//
//	message.NewMarshalPipe(marshaler, cfg)
//
// But makes it explicit that schema validation is involved.
func NewMarshalPipe(m *Marshaler, cfg message.PipeConfig) *MarshalPipe {
	return &MarshalPipe{
		inner: message.NewMarshalPipe(m, cfg),
	}
}

// Pipe starts the marshal pipeline.
func (p *MarshalPipe) Pipe(ctx context.Context, in <-chan *message.Message) (<-chan *message.RawMessage, error) {
	return p.inner.Pipe(ctx, in)
}

// Use adds middleware to the marshal processing chain.
// Middleware is applied in the order it is added.
// Returns ErrAlreadyStarted if the pipe has already been started.
func (p *MarshalPipe) Use(mw ...middleware.Middleware[*message.Message, *message.RawMessage]) error {
	return p.inner.Use(mw...)
}
