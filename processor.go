package gopipe

import (
	"context"
	"sync"
	"time"
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
	return p.process(ctx, in)
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

// WithConcurrency sets worker count for concurrent processing.
func WithConcurrency[In, Out any](concurrency int) Option[In, Out] {
	return func(cfg *config[In, Out]) {
		if concurrency > 0 {
			cfg.concurrency = concurrency
		}
	}
}

// WithBuffer sets output channel buffer size.
func WithBuffer[In, Out any](buffer int) Option[In, Out] {
	return func(cfg *config[In, Out]) {
		if buffer > 0 {
			cfg.buffer = buffer
		}
	}
}

// WithCancel provides an additional cancel function to the processor.
func WithCancel[In, Out any](cancel func(In, error)) Option[In, Out] {
	return func(cfg *config[In, Out]) {
		cfg.cancel = append(cfg.cancel, cancel)
	}
}

// WithCleanup adds a cleanup function to be called when processing is complete.
// If timeout is greater than zero, context will be canceled after the timeout duration.
func WithCleanup[In, Out any](cleanup func(ctx context.Context), timeout time.Duration) Option[In, Out] {
	return func(cfg *config[In, Out]) {
		cfg.cleanup = cleanup
		cfg.cleanupTimeout = timeout
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
	proc = c.apply(proc)

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
						proc.Cancel(val, newErrFailure(err))
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
			proc.Cancel(val, newErrCancel(ctx.Err()))
		}
		wgCancel.Done()
	}()

	go func() {
		wgProcess.Wait()
		cancel()
		wgCancel.Wait()

		if c.cleanup != nil {
			cleanupCtx := context.Background()
			if c.cleanupTimeout > 0 {
				var cancel context.CancelFunc
				cleanupCtx, cancel = context.WithTimeout(context.Background(), c.cleanupTimeout)
				defer cancel()
			}
			c.cleanup(cleanupCtx)
		}

		close(out)
	}()

	return out
}
