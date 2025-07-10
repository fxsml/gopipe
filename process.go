package gopipeline

import (
	"context"
	"fmt"
	"sync"
)

// ErrProcess is returned when a process function returns an error.
var ErrProcess = fmt.Errorf("gopipeline: process")

type ProcessHandler[In, Out any] func(context.Context, In) (Out, error)

func Process[In, Out any](
	ctx context.Context,
	in <-chan In,
	handler ProcessHandler[In, Out],
	opts ...Option,
) <-chan Out {
	cfg := defaultConfig
	for _, opt := range opts {
		opt(&cfg)
	}

	out := make(chan Out, cfg.buffer)

	var wg sync.WaitGroup
	wg.Add(cfg.concurrency)
	for range cfg.concurrency {
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case val, ok := <-in:
					if !ok {
						return
					}
					ctxProcess, cancel := cfg.ctx(ctx)
					if res, err := handler(ctxProcess, val); err != nil {
						cfg.err(val, fmt.Errorf("%w: %w", ErrProcess, err))
					} else {
						select {
						case out <- res:
						case <-ctx.Done():
						}
					}
					cancel()
				}
			}
		}()
	}

	go func() {
		wg.Wait()
		close(out)
	}()

	return out
}
