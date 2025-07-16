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
	// ErrProcessBatchResult is returned when a batch process function returns an error for a specific item.
	ErrProcessBatchResult = fmt.Errorf("gopipeline: process batch result")
)

type BatchResult[T any] struct {
	Val T
	Err error
}

func NewBatchResult[T any](val T, err error) BatchResult[T] {
	return BatchResult[T]{Val: val, Err: err}
}

func HandleBatchResult[T any](res BatchResult[T]) (T, error) {
	return res.Val, res.Err
}

type BatchResultHandler[BatchResult, Out any] func(BatchResult) (Out, error)

type ProcessBatchHandler[In, BatchResult any] func(context.Context, []In) ([]BatchResult, error)

func ProcessBatch[In, BatchResult, Out any](
	ctx context.Context,
	in <-chan In,
	processHandler ProcessBatchHandler[In, BatchResult],
	resultHandler BatchResultHandler[BatchResult, Out],
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
					batchResult, err := processHandler(ctxProcessBatch, batch)
					if err != nil {
						cfg.err(batch, fmt.Errorf("%w: %w", ErrProcessBatch, err))
						cancel()
						continue
					}
					for i, res := range batchResult {
						o, err := resultHandler(res)
						if err != nil {
							val := any(nil)
							if len(batch) == len(batchResult) {
								val = batch[i]
							}
							cfg.err(val, fmt.Errorf("%w: %w", ErrProcessBatchResult, err))
							continue
						}
						select {
						case out <- o:
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
