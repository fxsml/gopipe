package gopipe

import (
	"context"
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
