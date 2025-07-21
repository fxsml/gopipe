package gopipe

import (
	"context"
	"fmt"
	"sync"
	"time"
)

var (
	// ErrProcessBatch is returned when a batch process function returns an error.
	ErrProcessBatch = fmt.Errorf("gopipe: process batch")
	// ErrProcessBatchResult is returned when a batch process function returns an error for a specific item.
	ErrProcessBatchResult = fmt.Errorf("gopipe: process batch result")
)

// BatchResult is a generic type that holds both the processed value and any error
// that occurred during the processing of that specific item within a batch.
//
// This is provided as a convenience feature and can be used as a default batch result implementation.
// Users can also implement their own custom batch result types to suit individual needs and
// prevent dependency lock-in. The ProcessBatch function allows for any batch result type
// as long as it can be processed by the resultHandler.
type BatchResult[T any] struct {
	Val T     // The processed value
	Err error // Any error that occurred during processing
}

// NewBatchResult creates a new BatchResult instance with the provided value and error.
// This is a convenience function for creating BatchResult instances with type inference.
func NewBatchResult[T any](val T, err error) BatchResult[T] {
	return BatchResult[T]{Val: val, Err: err}
}

// HandleBatchResult is a utility function that extracts the value and error from a BatchResult.
// It can be used as the default resultHandler in ProcessBatch when no additional processing is needed.
func HandleBatchResult[T any](res BatchResult[T]) (T, error) {
	return res.Val, res.Err
}

// BatchResultHandler is a function type that processes a single batch result and returns
// an output value or an error. This allows transforming or validating individual batch results
// before sending them to the output channel.
//
// The BatchResult type parameter can be the provided BatchResult[T] convenience type or any custom
// implementation. This design gives you the flexibility to work with your own domain-specific
// batch result types, helping to prevent dependency lock-in.
type BatchResultHandler[BatchResult, Out any] func(BatchResult) (Out, error)

// ProcessBatchHandler is a function type that processes a batch of input values and returns
// a slice of batch result objects or an error. If an error is returned, the entire batch is considered
// failed and no individual results are processed.
//
// The BatchResult type parameter can be the provided BatchResult[T] convenience type or any custom
// implementation that suits your specific needs. This flexibility allows you to avoid dependency
// lock-in by using your own domain-specific batch result types.
type ProcessBatchHandler[In, BatchResult any] func(context.Context, []In) ([]BatchResult, error)

// ProcessBatch creates a pipeline stage that collects input values into batches, processes them
// using the provided handlers, and sends the results to the output channel.
//
// ProcessBatch uses two handlers:
//   - processHandler: Processes an entire batch of input values and returns individual batch results
//   - resultHandler: Processes each individual batch result to produce final output values
//
// While the library provides BatchResult as a convenience type, ProcessBatch is designed to work with
// any custom batch result implementation. Users can create their own batch result types that better
// suit their specific requirements, which helps prevent dependency lock-in. The generic type parameters
// allow for this flexibility without requiring adherence to a specific interface.
//
// Items are collected into batches based on two triggers:
//   - maxSize: When the number of collected items reaches this threshold, a batch is processed
//   - maxDuration: When this duration elapses since the last batch, any collected items are processed
//
// If the processHandler returns an error, the entire batch is considered failed and reported
// to the error handler. Individual item errors can be returned in the BatchResult.Err field,
// which will be handled by the resultHandler.
//
// ProcessBatch features:
//   - Efficient batching with size and time-based triggers
//   - Concurrent batch processing with configurable number of workers
//   - Configurable output channel buffer size
//   - Custom error handling at both batch and item levels
//   - Context-based cancellation
//   - Optional processing timeouts
//
// Example:
//
//	handler := func(ctx context.Context, batch []int) ([]gopipe.BatchResult[int], error) {
//	    results := make([]gopipe.BatchResult[int], len(batch))
//	    for i, v := range batch {
//	        if v%4 == 0 {
//	            results[i] = gopipe.NewBatchResult(0, fmt.Errorf("invalid value: %d", v))
//	        } else {
//	            results[i] = gopipe.NewBatchResult(v*10, nil)
//	        }
//	    }
//	    return results, nil
//	}
//
//	out := gopipe.ProcessBatch(
//	    ctx, inputChan, handler, gopipe.HandleBatchResult,
//	    10, 500*time.Millisecond, gopipe.WithConcurrency(3),
//	)
//
// The output channel is automatically closed when all inputs have been processed
// or when the context is cancelled.
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
