package message

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/fxsml/gopipe/pipe"
)

// UnmarshalPipe converts a Message with raw []byte Data into a Message with
// typed Data, using a registry and marshaler. Fails with ErrDataNotRaw if
// Data is not raw when received. Automatically nacks messages on errors and
// provides consistent logging.
type UnmarshalPipe struct {
	cfg       PipeConfig
	registry  InputRegistry
	marshaler Marshaler

	mu         sync.Mutex
	middleware []Middleware
	started    bool
	inner      atomic.Pointer[pipe.ProcessPipe[*Message, *Message]]
}

// NewUnmarshalPipe creates a pipe that unmarshals raw Message Data into typed Data.
// Uses registry to create typed instances for unmarshaling.
// Messages are automatically nacked on unmarshal failures.
func NewUnmarshalPipe(registry InputRegistry, marshaler Marshaler, cfg PipeConfig) *UnmarshalPipe {
	return &UnmarshalPipe{
		cfg:       cfg.parse(),
		registry:  registry,
		marshaler: marshaler,
	}
}

func (p *UnmarshalPipe) process(ctx context.Context, msg *Message) ([]*Message, error) {
	raw, ok := msg.Raw()
	if !ok {
		return nil, fmt.Errorf("%w: got %T", ErrDataNotRaw, msg.Data)
	}

	instance := p.registry.NewInput(msg.Type())
	if instance == nil {
		return nil, ErrUnknownType
	}

	if err := p.marshaler.Unmarshal(raw, instance); err != nil {
		return nil, err
	}

	msg.Data = instance
	return []*Message{msg}, nil
}

// Pipe starts the unmarshal pipeline.
func (p *UnmarshalPipe) Pipe(ctx context.Context, in <-chan *Message) (<-chan *Message, error) {
	p.mu.Lock()
	if p.started {
		p.mu.Unlock()
		return nil, ErrAlreadyStarted
	}
	p.started = true

	fn := ProcessFunc(p.process)
	for i := len(p.middleware) - 1; i >= 0; i-- {
		fn = p.middleware[i](fn)
	}
	p.mu.Unlock()

	inner := pipe.NewProcessPipe(fn, pipe.Config{
		BufferSize:      p.cfg.Pool.BufferSize,
		Concurrency:     p.cfg.Pool.Workers,
		ProcessTimeout:  p.cfg.ProcessTimeout,
		ShutdownTimeout: p.cfg.ShutdownTimeout,
		Metrics:         p.cfg.Metrics,
		Labels:          p.cfg.Labels,
		ErrorHandler: func(in any, err error) {
			msg := in.(*Message)
			msg.Nack(err)
			p.cfg.Logger.Error("Unmarshal failed",
				"component", "unmarshal",
				"error", err,
				"attributes", msg.Attributes)
			p.cfg.ErrorHandler(msg, err)
		},
	})
	p.inner.Store(inner)
	return inner.Pipe(ctx, in)
}

// Stats returns a point-in-time snapshot of the output buffer depth and capacity.
// Use with a pull-based metrics backend (e.g. OTel observable gauge).
func (p *UnmarshalPipe) Stats() pipe.Stats {
	if inner := p.inner.Load(); inner != nil {
		return inner.Stats()
	}
	return pipe.Stats{}
}

// Use adds middleware to the unmarshal processing chain.
// Middleware is applied in the order it is added.
// Returns ErrAlreadyStarted if the pipe has already been started.
func (p *UnmarshalPipe) Use(mw ...Middleware) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.started {
		return ErrAlreadyStarted
	}
	p.middleware = append(p.middleware, mw...)
	return nil
}

// MarshalPipe converts a Message with typed Data into a Message with raw
// []byte Data, using a marshaler. Fails with ErrDataNotTyped if Data is
// already raw when received. Automatically nacks messages on errors and
// provides consistent logging.
type MarshalPipe struct {
	cfg       PipeConfig
	marshaler Marshaler

	mu         sync.Mutex
	middleware []Middleware
	started    bool
	inner      atomic.Pointer[pipe.ProcessPipe[*Message, *Message]]
}

// NewMarshalPipe creates a pipe that marshals typed Message Data into raw Data.
// Messages are automatically nacked on marshal failures.
func NewMarshalPipe(marshaler Marshaler, cfg PipeConfig) *MarshalPipe {
	return &MarshalPipe{
		cfg:       cfg.parse(),
		marshaler: marshaler,
	}
}

func (p *MarshalPipe) process(ctx context.Context, msg *Message) ([]*Message, error) {
	if _, ok := msg.Raw(); ok {
		return nil, ErrDataNotTyped
	}

	data, err := p.marshaler.Marshal(msg.Data)
	if err != nil {
		return nil, err
	}

	if msg.Attributes == nil {
		msg.Attributes = make(Attributes)
	}
	msg.Attributes[AttrDataContentType] = p.marshaler.DataContentType()
	msg.Data = data
	return []*Message{msg}, nil
}

// Pipe starts the marshal pipeline.
func (p *MarshalPipe) Pipe(ctx context.Context, in <-chan *Message) (<-chan *Message, error) {
	p.mu.Lock()
	if p.started {
		p.mu.Unlock()
		return nil, ErrAlreadyStarted
	}
	p.started = true

	fn := ProcessFunc(p.process)
	for i := len(p.middleware) - 1; i >= 0; i-- {
		fn = p.middleware[i](fn)
	}
	p.mu.Unlock()

	inner := pipe.NewProcessPipe(fn, pipe.Config{
		BufferSize:      p.cfg.Pool.BufferSize,
		Concurrency:     p.cfg.Pool.Workers,
		ProcessTimeout:  p.cfg.ProcessTimeout,
		ShutdownTimeout: p.cfg.ShutdownTimeout,
		Metrics:         p.cfg.Metrics,
		Labels:          p.cfg.Labels,
		ErrorHandler: func(in any, err error) {
			msg := in.(*Message)
			msg.Nack(err)
			p.cfg.Logger.Error("Marshal failed",
				"component", "marshal",
				"error", err,
				"attributes", msg.Attributes)
			p.cfg.ErrorHandler(msg, err)
		},
	})
	p.inner.Store(inner)
	return inner.Pipe(ctx, in)
}

// Stats returns a point-in-time snapshot of the output buffer depth and capacity.
// Use with a pull-based metrics backend (e.g. OTel observable gauge).
func (p *MarshalPipe) Stats() pipe.Stats {
	if inner := p.inner.Load(); inner != nil {
		return inner.Stats()
	}
	return pipe.Stats{}
}

// Use adds middleware to the marshal processing chain.
// Middleware is applied in the order it is added.
// Returns ErrAlreadyStarted if the pipe has already been started.
func (p *MarshalPipe) Use(mw ...Middleware) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.started {
		return ErrAlreadyStarted
	}
	p.middleware = append(p.middleware, mw...)
	return nil
}
