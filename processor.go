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
	ctx, cancel := context.WithCancel(ctx)

	c := parseConfig(opts)
	proc = c.applyMiddleware(proc)

	out := make(chan Out, c.buffer)

	var wgProcess sync.WaitGroup
	wgProcess.Add(c.concurrency)
	for range c.concurrency {
		go func() {
			defer wgProcess.Done()
			for {
				select {
				case <-ctx.Done():
					return
				default:
				}

				select {
				case <-ctx.Done():
					return
				case val, ok := <-in:
					if !ok {
						return
					}
					if res, err := proc.Process(ctx, val); err != nil {
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

	wgCancel := sync.WaitGroup{}
	wgCancel.Add(1)
	go func() {
		<-ctx.Done()
		for val := range in {
			proc.Cancel(val, ctx.Err())
		}
		wgCancel.Done()
	}()

	go func() {
		wgProcess.Wait()
		cancel()
		wgCancel.Wait()
		close(out)
	}()

	return out
}
