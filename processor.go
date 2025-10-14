package gopipe

import (
	"context"
)

// ProcessFunc is the function used by Processor.Process.
type ProcessFunc[In, Out any] func(context.Context, In) (Out, error)

// CancelFunc is the function used by Processor.Cancel.
type CancelFunc[In any] func(In, error)

// Processor combines processing and cancellation logic into a single abstraction.
// This abstraction allows controlling and manipulating the flow of data and errors.
type Processor[In, Out any] interface {
	// Process processes a single item with context awareness.
	Process(context.Context, In) (Out, error)
	// Cancel handles errors when processing fails.
	Cancel(In, error)
}

type processor[In, Out any] struct {
	process ProcessFunc[In, Out]
	cancel  CancelFunc[In]
}

func (p *processor[In, Out]) Process(ctx context.Context, in In) (Out, error) {
	return p.process(ctx, in)
}

func (p *processor[In, Out]) Cancel(in In, err error) {
	p.cancel(in, err)
}

// NewProcessor creates a new Processor with the given process and cancel functions.
func NewProcessor[In, Out any](process ProcessFunc[In, Out], cancel CancelFunc[In]) Processor[In, Out] {
	return &processor[In, Out]{
		process: process,
		cancel:  cancel,
	}
}
