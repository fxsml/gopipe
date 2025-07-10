package gopipeline

import (
	"context"
	"fmt"
	"sync"
)

// ErrFilter is returned when a filter function returns an error.
var ErrFilter = fmt.Errorf("gopipeline: filter")

type FilterHandler[In any] func(context.Context, In) (bool, error)

func Filter[T any](
	ctx context.Context,
	in <-chan T,
	handler FilterHandler[T],
	opts ...Option,
) <-chan T {
	cfg := defaultConfig
	for _, opt := range opts {
		opt(&cfg)
	}

	out := make(chan T, cfg.buffer)

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
					ctxFilter, cancel := cfg.ctx(ctx)
					if ok, err := handler(ctxFilter, val); err != nil {
						cfg.err(val, fmt.Errorf("%w: %w", ErrFilter, err))
					} else if ok {
						select {
						case out <- val:
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
