package message

import (
	"context"

	"github.com/fxsml/gopipe/pipe"
	"github.com/fxsml/gopipe/pipe/middleware"
)

// UnmarshalPipe converts RawMessage to Message using a registry and marshaler.
// Automatically nacks messages on errors and provides consistent logging.
type UnmarshalPipe struct {
	inner *pipe.ProcessPipe[*RawMessage, *Message]
}

// NewUnmarshalPipe creates a pipe that unmarshals RawMessage to Message.
// Uses registry to create typed instances for unmarshaling.
// Messages are automatically nacked on unmarshal failures.
func NewUnmarshalPipe(registry InputRegistry, marshaler Marshaler, cfg PipeConfig) *UnmarshalPipe {
	cfg = cfg.parse()

	p := &UnmarshalPipe{}
	p.inner = pipe.NewProcessPipe(func(ctx context.Context, raw *RawMessage) ([]*Message, error) {
		instance := registry.NewInput(raw.Type())
		if instance == nil {
			return nil, ErrUnknownType
		}

		if err := marshaler.Unmarshal(raw.Data, instance); err != nil {
			return nil, err
		}

		return []*Message{{
			Data:       instance,
			Attributes: raw.Attributes,
			acking:     raw.acking,
		}}, nil
	}, pipe.Config{
		BufferSize:      cfg.Pool.BufferSize,
		Concurrency:     cfg.Pool.Workers,
		ShutdownTimeout: cfg.ShutdownTimeout,
		ErrorHandler: func(in any, err error) {
			raw := in.(*RawMessage)
			raw.Nack(err)
			cfg.Logger.Error("Unmarshal failed",
				"component", "unmarshal",
				"error", err,
				"attributes", raw.Attributes)
			if cfg.ErrorHandler != nil {
				// Wrap RawMessage as Message for consistent error handler signature
				cfg.ErrorHandler(&Message{
					Data:       raw.Data,
					Attributes: raw.Attributes,
					acking:     raw.acking,
				}, err)
			}
		},
	})
	return p
}

// Pipe starts the unmarshal pipeline.
func (p *UnmarshalPipe) Pipe(ctx context.Context, in <-chan *RawMessage) (<-chan *Message, error) {
	return p.inner.Pipe(ctx, in)
}

// Use adds middleware to the unmarshal processing chain.
// Middleware is applied in the order it is added.
// Returns ErrAlreadyStarted if the pipe has already been started.
func (p *UnmarshalPipe) Use(mw ...middleware.Middleware[*RawMessage, *Message]) error {
	return p.inner.Use(mw...)
}

// MarshalPipe converts Message to RawMessage using a marshaler.
// Automatically nacks messages on errors and provides consistent logging.
type MarshalPipe struct {
	inner     *pipe.ProcessPipe[*Message, *RawMessage]
	marshaler Marshaler
}

// NewMarshalPipe creates a pipe that marshals Message to RawMessage.
// Messages are automatically nacked on marshal failures.
func NewMarshalPipe(marshaler Marshaler, cfg PipeConfig) *MarshalPipe {
	cfg = cfg.parse()

	p := &MarshalPipe{marshaler: marshaler}
	p.inner = pipe.NewProcessPipe(func(ctx context.Context, msg *Message) ([]*RawMessage, error) {
		data, err := marshaler.Marshal(msg.Data)
		if err != nil {
			return nil, err
		}

		attrs := msg.Attributes
		if attrs == nil {
			attrs = make(Attributes)
		}
		attrs[AttrDataContentType] = marshaler.DataContentType()

		return []*RawMessage{{
			Data:       data,
			Attributes: attrs,
			acking:     msg.acking,
		}}, nil
	}, pipe.Config{
		BufferSize:      cfg.Pool.BufferSize,
		Concurrency:     cfg.Pool.Workers,
		ShutdownTimeout: cfg.ShutdownTimeout,
		ErrorHandler: func(in any, err error) {
			msg := in.(*Message)
			msg.Nack(err)
			cfg.Logger.Error("Marshal failed",
				"component", "marshal",
				"error", err,
				"attributes", msg.Attributes)
			if cfg.ErrorHandler != nil {
				cfg.ErrorHandler(msg, err)
			}
		},
	})
	return p
}

// Pipe starts the marshal pipeline.
func (p *MarshalPipe) Pipe(ctx context.Context, in <-chan *Message) (<-chan *RawMessage, error) {
	return p.inner.Pipe(ctx, in)
}

// Use adds middleware to the marshal processing chain.
// Middleware is applied in the order it is added.
// Returns ErrAlreadyStarted if the pipe has already been started.
func (p *MarshalPipe) Use(mw ...middleware.Middleware[*Message, *RawMessage]) error {
	return p.inner.Use(mw...)
}
