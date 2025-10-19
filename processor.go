package gopipe

import (
	"context"
	"sync"
)

// ProcessFunc is the function used by Processor.Process.
type ProcessFunc[In, Out any] func(context.Context, In) ([]Out, error)

// CancelFunc is the function used by Processor.Cancel.
type CancelFunc[In any] func(In, error)

// Processor combines processing and cancellation logic into a single abstraction.
// This abstraction allows controlling and manipulating the flow of data and errors.
type Processor[In, Out any] interface {
	// Process processes a single input item with context awareness.
	// It transforms the input into zero or more output items, or returns an error.
	Process(context.Context, In) ([]Out, error)
	// Cancel handles errors when processing fails.
	Cancel(In, error)
}

type processor[In, Out any] struct {
	process ProcessFunc[In, Out]
	cancel  CancelFunc[In]
}

func (p *processor[In, Out]) Process(ctx context.Context, in In) ([]Out, error) {
	out, err := p.process(ctx, in)
	if err != nil {
		return nil, newErrFailure(err)
	}
	return out, nil
}

func (p *processor[In, Out]) Cancel(in In, err error) {
	p.cancel(in, err)
}

// NewProcessor creates a new Processor with the provided process and cancel functions.
//
// Panics if process is nil.
// If cancel is nil, a no-op function is used.
func NewProcessor[In, Out any](
	process ProcessFunc[In, Out],
	cancel CancelFunc[In],
) Processor[In, Out] {
	if process == nil {
		panic("ProcessFunc cannot be nil")
	}
	if cancel == nil {
		cancel = func(in In, err error) {}
	}
	return &processor[In, Out]{
		process: process,
		cancel:  cancel,
	}
}

// StartProcessor processes items from the input channel using the provided processor
// and returns a channel that will receive the processed outputs.
//
// Processing will continue until the input channel is closed or the context is canceled.
// The output channel is closed when processing is complete.
// Behavior can be customized with options.
func StartProcessor[In, Out any](
	ctx context.Context,
	in <-chan In,
	proc Processor[In, Out],
	opts ...Option[In, Out],
) <-chan Out {
	return startProcessor(ctx, in, proc, opts)
}

func startProcessor[In, Out any](
	ctx context.Context,
	in <-chan In,
	proc Processor[In, Out],
	opts []Option[In, Out],
) <-chan Out {
	mainCtx, mainCancel := context.WithCancel(ctx)
	middlewareCtx, middlewareCancel := context.WithCancel(context.Background())

	c := parseConfig(opts)
	proc = c.apply(middlewareCtx, proc)

	out := make(chan Out, c.buffer)

	var wg sync.WaitGroup
	wg.Add(c.concurrency)
	for i := 0; i < c.concurrency; i++ {
		go func() {
			defer wg.Done()
			for {
				select {
				case <-mainCtx.Done():
					return
				default:
				}

				select {
				case <-mainCtx.Done():
					return
				case val, ok := <-in:
					if !ok {
						return
					}
					if res, err := proc.Process(mainCtx, val); err != nil {
						proc.Cancel(val, err)
					} else {
						for _, r := range res {
							out <- r
						}
					}
				}
			}
		}()
	}

	// Start draining as soon as the parent context is cancelled.
	wgDrain := sync.WaitGroup{}
	wgDrain.Add(1)
	go func() {
		<-mainCtx.Done()
		for val := range in {
			proc.Cancel(val, mainCtx.Err())
		}
		wgDrain.Done()
	}()

	go func() {
		wg.Wait()
		close(out)
		mainCancel()
		wgDrain.Wait()
		middlewareCancel()
	}()

	return out
}
