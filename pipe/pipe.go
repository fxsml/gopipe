package pipe

import (
	"context"
	"sync"
	"time"

	"github.com/fxsml/gopipe/channel"
	"github.com/fxsml/gopipe/pipe/middleware"
)

// Pipe represents a complete processing pipeline that transforms input values to output values.
// It combines preprocessing with a ProcessFunc and configuration.
type Pipe[In, Out any] interface {
	// Pipe begins processing items from the input channel and returns a channel for outputs.
	// Processing continues until the input channel is closed or the context is canceled.
	// Returns ErrAlreadyStarted if the pipe has already been started.
	Pipe(ctx context.Context, in <-chan In) (<-chan Out, error)
}

type appliedPipe[In, Inter, Out any] struct {
	pipeA Pipe[In, Inter]
	pipeB Pipe[Inter, Out]
}

func (p *appliedPipe[In, Inter, Out]) Pipe(ctx context.Context, in <-chan In) (<-chan Out, error) {
	inter, err := p.pipeA.Pipe(ctx, in)
	if err != nil {
		return nil, err
	}
	return p.pipeB.Pipe(ctx, inter)
}

// Apply combines two Pipes into one, connecting the output of the first to the input of the second.
// The resulting Pipe takes inputs of type In and produces outputs of type Out.
func Apply[In, Inter, Out any](a Pipe[In, Inter], b Pipe[Inter, Out]) Pipe[In, Out] {
	return &appliedPipe[In, Inter, Out]{
		pipeA: a,
		pipeB: b,
	}
}

// NewFilterPipe creates a Pipe that selectively passes through inputs based on a predicate function.
// If the handle function returns true, the input is passed through; if false, the input is discarded.
// If the handle function returns an error, processing for that item stops and the error is handled.
// Use ApplyMiddleware on the returned *ProcessPipe to add middleware.
func NewFilterPipe[In any](
	handle func(context.Context, In) (bool, error),
	cfg Config,
) *ProcessPipe[In, In] {
	fn := func(ctx context.Context, in In) ([]In, error) {
		ok, err := handle(ctx, in)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, nil
		}
		return []In{in}, nil
	}
	return NewProcessPipe(fn, cfg)
}

// NewProcessPipe creates a Pipe that can transform each input into multiple outputs.
// Unlike NewTransformPipe, this can produce zero, one, or many outputs for each input.
// The handle function receives a context and input item, and returns a slice of outputs or an error.
// Use ApplyMiddleware on the returned *ProcessPipe to add middleware.
func NewProcessPipe[In, Out any](
	handle func(context.Context, In) ([]Out, error),
	cfg Config,
) *ProcessPipe[In, Out] {
	return &ProcessPipe[In, Out]{
		handle: handle,
		cfg:    cfg,
	}
}

// NewTransformPipe creates a Pipe that transforms each input into exactly one output.
// Unlike NewProcessPipe, this always produces exactly one output for each successful input.
// The handle function receives a context and input item, and returns a single output or an error.
// Use ApplyMiddleware on the returned *ProcessPipe to add middleware.
func NewTransformPipe[In, Out any](
	handle func(context.Context, In) (Out, error),
	cfg Config,
) *ProcessPipe[In, Out] {
	fn := func(ctx context.Context, in In) ([]Out, error) {
		out, err := handle(ctx, in)
		if err != nil {
			return nil, err
		}
		return []Out{out}, nil
	}
	return NewProcessPipe(fn, cfg)
}

// NewSinkPipe creates a Pipe that applies handle to each value from in.
// The returned channel is closed after in is closed and all values are processed.
// Use ApplyMiddleware on the returned *ProcessPipe to add middleware.
func NewSinkPipe[In any](
	handle func(context.Context, In) error,
	cfg Config,
) *ProcessPipe[In, struct{}] {
	fn := func(ctx context.Context, in In) ([]struct{}, error) {
		return nil, handle(ctx, in)
	}
	return NewProcessPipe(fn, cfg)
}

// ProcessPipe is a Pipe that processes individual items using a ProcessFunc.
type ProcessPipe[In, Out any] struct {
	handle ProcessFunc[In, Out]
	cfg    Config
	mw     []middleware.Middleware[In, Out]

	mu      sync.Mutex
	started bool
}

// Pipe begins processing items from the input channel.
// Returns ErrAlreadyStarted if the pipe has already been started.
func (p *ProcessPipe[In, Out]) Pipe(ctx context.Context, in <-chan In) (<-chan Out, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.started {
		return nil, ErrAlreadyStarted
	}
	p.started = true
	handle := applyMiddleware(p.handle, p.mw)
	return startProcessing(ctx, in, handle, p.cfg), nil
}

// ApplyMiddleware adds middleware to the processing chain.
// Middleware is applied in the order it is added.
// Returns ErrAlreadyStarted if the pipe has already been started.
func (p *ProcessPipe[In, Out]) ApplyMiddleware(mw ...middleware.Middleware[In, Out]) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.started {
		return ErrAlreadyStarted
	}
	p.mw = append(p.mw, mw...)
	return nil
}

// BatchConfig configures behavior of a BatchPipe.
type BatchConfig struct {
	// Config is the configuration for the underlying process pipe.
	Config Config

	// MaxSize is the maximum number of items in a batch.
	// When this size is reached, the batch is sent for processing.
	// Default is 1.
	MaxSize int

	// MaxDuration is the maximum duration to wait before sending a batch for processing.
	// If this duration elapses since the first item in the batch, the batch is sent for processing.
	// Default is 1 second.
	MaxDuration time.Duration
}

// NewBatchPipe creates a Pipe that groups inputs into batches before processing.
// Each batch is processed as a whole by the handle function, which can return multiple outputs.
// Batches are created when either maxSize items are collected or maxDuration elapses since the first item.
// Use ApplyMiddleware on the returned *BatchPipe to add middleware.
func NewBatchPipe[In, Out any](
	handle func(context.Context, []In) ([]Out, error),
	cfg BatchConfig,
) *BatchPipe[In, Out] {
	return &BatchPipe[In, Out]{
		handle: handle,
		cfg:    cfg,
	}
}

// BatchPipe is a Pipe that collects items into batches before processing.
type BatchPipe[In, Out any] struct {
	handle func(context.Context, []In) ([]Out, error)
	cfg    BatchConfig
	mw     []middleware.Middleware[[]In, Out]

	mu      sync.Mutex
	started bool
}

// Pipe begins collecting items into batches and processing them.
// Returns ErrAlreadyStarted if the pipe has already been started.
func (p *BatchPipe[In, Out]) Pipe(ctx context.Context, in <-chan In) (<-chan Out, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.started {
		return nil, ErrAlreadyStarted
	}
	p.started = true
	if p.cfg.MaxSize <= 0 {
		p.cfg.MaxSize = 1
	}
	if p.cfg.MaxDuration <= 0 {
		p.cfg.MaxDuration = time.Second
	}
	batchChan := channel.Collect(in, p.cfg.MaxSize, p.cfg.MaxDuration)
	handle := applyMiddleware(p.handle, p.mw)
	return startProcessing(ctx, batchChan, handle, p.cfg.Config), nil
}

// ApplyMiddleware adds middleware to the processing chain.
// Middleware is applied in the order it is added.
// Returns ErrAlreadyStarted if the pipe has already been started.
func (p *BatchPipe[In, Out]) ApplyMiddleware(mw ...middleware.Middleware[[]In, Out]) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.started {
		return ErrAlreadyStarted
	}
	p.mw = append(p.mw, mw...)
	return nil
}

func applyMiddleware[In, Out any](fn ProcessFunc[In, Out], mw []middleware.Middleware[In, Out]) ProcessFunc[In, Out] {
	for i := len(mw) - 1; i >= 0; i-- {
		fn = ProcessFunc[In, Out](mw[i](middleware.ProcessFunc[In, Out](fn)))
	}
	return fn
}
