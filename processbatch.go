package gopipeline

import (
	"context"
	"fmt"
	"sync"
	"time"
)

var (
	// ErrProcessBatch is returned when a batch process function returns an error.
	ErrProcessBatch = fmt.Errorf("gopipeline: process batch")
	// ErrProcessBatchRes is returned when a batch process function returns an error for a specific item.
	ErrProcessBatchRes = fmt.Errorf("gopipeline: process batch result")
)

type BatchRes[T any] interface {
	Val() T
	Err() error
}

type batchRes[T any] struct {
	val T
	err error
}

func (r batchRes[T]) Val() T {
	return r.val
}

func (r batchRes[T]) Err() error {
	return r.err
}

func NewBatchRes[T any](val T, err error) BatchRes[T] {
	return batchRes[T]{val: val, err: err}
}

type ProcessBatchHandler[In, Out any] func(context.Context, []In) ([]BatchRes[Out], error)

func ProcessBatch[In, Out any](
	ctx context.Context,
	in <-chan In,
	handler ProcessBatchHandler[In, Out],
	maxSize int,
	maxDuration time.Duration,
	opts ...Option,
) <-chan Out {
	cfg := defaultConfig
	for _, opt := range opts {
		opt(&cfg)
	}

	out := make(chan Out, cfg.buffer)
	batchChan := make(chan []In, cfg.concurrency)

	// Batch collector goroutine
	go func() {
		batch := make([]In, 0, maxSize)
		ticker := time.NewTicker(maxDuration)

		defer func() {
			ticker.Stop()
			close(batchChan)
		}()

		for {
			select {
			case <-ctx.Done():
				return
			case val, ok := <-in:
				if !ok {
					if len(batch) > 0 {
						batchChan <- batch
					}
					return
				}
				batch = append(batch, val)
				if len(batch) >= maxSize {
					batchChan <- batch
					batch = make([]In, 0, maxSize)
					ticker.Reset(maxDuration)
				}
			case <-ticker.C:
				if len(batch) > 0 {
					batchChan <- batch
					batch = make([]In, 0, maxSize)
				}
			}
		}
	}()

	// Worker goroutines for processing batches
	var wg sync.WaitGroup
	wg.Add(cfg.concurrency)
	for range cfg.concurrency {
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case batch, ok := <-batchChan:
					if !ok {
						return
					}
					ctxProcessBatch, cancel := cfg.ctx(ctx)
					batchRes, err := handler(ctxProcessBatch, batch)
					if err != nil {
						cfg.err(batch, fmt.Errorf("%w: %w", ErrProcessBatch, err))
						cancel()
						continue
					}
					for i, res := range batchRes {
						if res.Err() != nil {
							val := any(nil)
							if len(batch) == len(batchRes) {
								val = batch[i]
							}
							cfg.err(val, fmt.Errorf("%w: %w", ErrProcessBatchRes, res.Err()))
							continue
						}
						select {
						case out <- res.Val():
						case <-ctxProcessBatch.Done():
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
