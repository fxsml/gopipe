package message

import (
	"context"
	"fmt"

	"github.com/fxsml/gopipe/pipe"
	"github.com/fxsml/gopipe/pipe/middleware"
)

// UnmarshalPipe converts a Message with raw []byte Data into a Message with
// typed Data, using a registry and marshaler. Fails with ErrDataNotRaw if
// Data is not raw when received. Automatically nacks messages on errors and
// provides consistent logging.
type UnmarshalPipe struct {
	inner *pipe.ProcessPipe[*Message, *Message]
}

// NewUnmarshalPipe creates a pipe that unmarshals raw Message Data into typed Data.
// Uses registry to create typed instances for unmarshaling.
// Messages are automatically nacked on unmarshal failures.
func NewUnmarshalPipe(registry InputRegistry, marshaler Marshaler, cfg PipeConfig) *UnmarshalPipe {
	cfg = cfg.parse()

	p := &UnmarshalPipe{}
	p.inner = pipe.NewProcessPipe(func(ctx context.Context, msg *Message) ([]*Message, error) {
		raw, ok := msg.Raw()
		if !ok {
			return nil, fmt.Errorf("%w: got %T", ErrDataNotRaw, msg.Data)
		}

		instance := registry.NewInput(msg.Type())
		if instance == nil {
			return nil, ErrUnknownType
		}

		if err := marshaler.Unmarshal(raw, instance); err != nil {
			return nil, err
		}

		msg.Data = instance
		return []*Message{msg}, nil
	}, pipe.Config{
		BufferSize:      cfg.Pool.BufferSize,
		Concurrency:     cfg.Pool.Workers,
		ProcessTimeout:  cfg.ProcessTimeout,
		ShutdownTimeout: cfg.ShutdownTimeout,
		Metrics:         cfg.Metrics,
		Labels:          cfg.Labels,
		ErrorHandler: func(in any, err error) {
			msg := in.(*Message)
			msg.Nack(err)
			cfg.Logger.Error("Unmarshal failed",
				"component", "unmarshal",
				"error", err,
				"attributes", msg.Attributes)
			if cfg.ErrorHandler != nil {
				cfg.ErrorHandler(msg, err)
			}
		},
	})
	return p
}

// Pipe starts the unmarshal pipeline.
func (p *UnmarshalPipe) Pipe(ctx context.Context, in <-chan *Message) (<-chan *Message, error) {
	return p.inner.Pipe(ctx, in)
}

// Stats returns a point-in-time snapshot of the output buffer depth and capacity.
// Use with a pull-based metrics backend (e.g. OTel observable gauge).
func (p *UnmarshalPipe) Stats() pipe.Stats {
	return p.inner.Stats()
}

// Use adds middleware to the unmarshal processing chain.
// Middleware is applied in the order it is added.
// Returns ErrAlreadyStarted if the pipe has already been started.
func (p *UnmarshalPipe) Use(mw ...Middleware) error {
	converted := make([]middleware.Middleware[*Message, *Message], len(mw))
	for i, m := range mw {
		converted[i] = adaptMiddleware(m)
	}
	return p.inner.Use(converted...)
}

// MarshalPipe converts a Message with typed Data into a Message with raw
// []byte Data, using a marshaler. Fails with ErrDataNotTyped if Data is
// already raw when received. Automatically nacks messages on errors and
// provides consistent logging.
type MarshalPipe struct {
	inner     *pipe.ProcessPipe[*Message, *Message]
	marshaler Marshaler
}

// NewMarshalPipe creates a pipe that marshals typed Message Data into raw Data.
// Messages are automatically nacked on marshal failures.
func NewMarshalPipe(marshaler Marshaler, cfg PipeConfig) *MarshalPipe {
	cfg = cfg.parse()

	p := &MarshalPipe{marshaler: marshaler}
	p.inner = pipe.NewProcessPipe(func(ctx context.Context, msg *Message) ([]*Message, error) {
		if _, ok := msg.Raw(); ok {
			return nil, ErrDataNotTyped
		}

		data, err := marshaler.Marshal(msg.Data)
		if err != nil {
			return nil, err
		}

		if msg.Attributes == nil {
			msg.Attributes = make(Attributes)
		}
		msg.Attributes[AttrDataContentType] = marshaler.DataContentType()
		msg.Data = data

		return []*Message{msg}, nil
	}, pipe.Config{
		BufferSize:      cfg.Pool.BufferSize,
		Concurrency:     cfg.Pool.Workers,
		ProcessTimeout:  cfg.ProcessTimeout,
		ShutdownTimeout: cfg.ShutdownTimeout,
		Metrics:         cfg.Metrics,
		Labels:          cfg.Labels,
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
func (p *MarshalPipe) Pipe(ctx context.Context, in <-chan *Message) (<-chan *Message, error) {
	return p.inner.Pipe(ctx, in)
}

// Stats returns a point-in-time snapshot of the output buffer depth and capacity.
// Use with a pull-based metrics backend (e.g. OTel observable gauge).
func (p *MarshalPipe) Stats() pipe.Stats {
	return p.inner.Stats()
}

// Use adds middleware to the marshal processing chain.
// Middleware is applied in the order it is added.
// Returns ErrAlreadyStarted if the pipe has already been started.
func (p *MarshalPipe) Use(mw ...Middleware) error {
	converted := make([]middleware.Middleware[*Message, *Message], len(mw))
	for i, m := range mw {
		converted[i] = adaptMiddleware(m)
	}
	return p.inner.Use(converted...)
}

// adaptMiddleware converts a message.Middleware into the equivalent
// pipe/middleware.Middleware[*Message, *Message] instantiation, so
// UnmarshalPipe/MarshalPipe can accept the same middleware type Router uses
// while still delegating to pipe.ProcessPipe's own Use() and started tracking.
func adaptMiddleware(mw Middleware) middleware.Middleware[*Message, *Message] {
	return func(next middleware.ProcessFunc[*Message, *Message]) middleware.ProcessFunc[*Message, *Message] {
		return middleware.ProcessFunc[*Message, *Message](mw(ProcessFunc(next)))
	}
}
