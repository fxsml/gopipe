package gopipe

import (
	"context"
	"time"

	"github.com/fxsml/gopipe/channel"
)

// PreProcessorFunc transforms a channel of one type into a channel of another type.
// Used to prepare inputs before they reach the main Processor in a Pipe.
type PreProcessorFunc[Pre, In any] func(in <-chan Pre) <-chan In

// NoopPreProcessorFunc returns the input channel unchanged.
// Used as a default preprocessor when no transformation is needed.
func NoopPreProcessorFunc[In any](in <-chan In) <-chan In {
	return in
}

// Pipe represents a complete processing pipeline that transforms input values to output values.
// It combines preprocessing with a Processor and optional configuration.
type Pipe[Pre, Out any] interface {
	// Start begins processing items from the input channel and returns a channel for outputs.
	// Processing continues until the input channel is closed or the context is canceled.
	Start(ctx context.Context, pre <-chan Pre) <-chan Out
}

type pipe[Pre, In, Out any] struct {
	preProc PreProcessorFunc[Pre, In]
	proc    Processor[In, Out]
	opts    []Option[In, Out]
}

func (p *pipe[Pre, In, Out]) Start(ctx context.Context, pre <-chan Pre) <-chan Out {
	return startProcessor(ctx, p.preProc(pre), p.proc, p.opts)
}

// NewPipe creates a new pipeline that preprocesses inputs and then processes them.
// The pipeline behavior can be customized with options.
func NewPipe[Pre, In, Out any](
	preProc PreProcessorFunc[Pre, In],
	proc Processor[In, Out],
	opts ...Option[In, Out],
) Pipe[Pre, Out] {
	return &pipe[Pre, In, Out]{
		preProc: preProc,
		proc:    proc,
		opts:    opts,
	}
}

// NewBatchPipe creates a Pipe that groups inputs into batches before processing.
// Each batch is processed as a whole by the handle function, which can return multiple outputs.
// Batches are created when either maxSize items are collected or maxDuration elapses since the first item.
func NewBatchPipe[In any, Out any](
	handle func(context.Context, []In) ([]Out, error),
	maxSize int,
	maxDuration time.Duration,
	opts ...Option[[]In, Out],
) Pipe[In, Out] {
	proc := NewProcessor(handle, nil)
	return NewPipe(func(pre <-chan In) <-chan []In {
		return channel.Collect(pre, maxSize, maxDuration)
	}, proc, opts...)
}

// NewFilterPipe creates a Pipe that selectively passes through inputs based on a predicate function.
// If the handle function returns true, the input is passed through; if false, the input is discarded.
// If the handle function returns an error, processing for that item stops and the error is handled.
func NewFilterPipe[In any](
	handle func(context.Context, In) (bool, error),
	opts ...Option[In, In],
) Pipe[In, In] {
	proc := NewProcessor(func(ctx context.Context, in In) ([]In, error) {
		ok, err := handle(ctx, in)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, nil
		}
		return []In{in}, nil
	}, nil)
	return NewPipe(NoopPreProcessorFunc[In], proc, opts...)
}

// NewProcessPipe creates a Pipe that can transform each input into multiple outputs.
// Unlike NewTransformPipe, this can produce zero, one, or many outputs for each input.
// The handle function receives a context and input item, and returns a slice of outputs or an error.
func NewProcessPipe[In, Out any](
	handle func(context.Context, In) ([]Out, error),
	opts ...Option[In, Out],
) Pipe[In, Out] {
	proc := NewProcessor(handle, nil)
	return NewPipe(NoopPreProcessorFunc[In], proc, opts...)
}

// NewTransformPipe creates a Pipe that transforms each input into exactly one output.
// Unlike NewProcessPipe, this always produces exactly one output for each successful input.
// The handle function receives a context and input item, and returns a single output or an error.
func NewTransformPipe[In, Out any](
	handle func(context.Context, In) (Out, error),
	opts ...Option[In, Out],
) Pipe[In, Out] {
	proc := NewProcessor(func(ctx context.Context, in In) ([]Out, error) {
		out, err := handle(ctx, in)
		if err != nil {
			return nil, err
		}
		return []Out{out}, nil
	}, nil)
	return NewPipe(NoopPreProcessorFunc[In], proc, opts...)
}
