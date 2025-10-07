package gopipe

import (
	"context"
	"time"
)

// BatchItemResult contains the result of processing a single item within a batch.
type BatchItemResult[In, Out any] struct {
	In  In    // The original input value
	Out Out   // The processed value
	Err error // Any error that occurred during processing
}

// BatchResult is a collection of individual item results from batch processing.
type BatchResult[In, Out any] []BatchItemResult[In, Out]

// NewBatchResult creates a new BatchResult with pre-allocated capacity.
// This improves performance when building results for batches of known size.
func NewBatchResult[In, Out any](size int) BatchResult[In, Out] {
	return make([]BatchItemResult[In, Out], 0, size)
}

// AddSuccess appends a successful result with the provided output value.
// The input value is not stored for successful results.
func (r *BatchResult[In, Out]) AddSuccess(out Out) {
	*r = append(*r, BatchItemResult[In, Out]{Out: out})
}

// AddFailure appends a failed result with the provided input value and error.
// This allows tracking which inputs caused errors during batch processing.
func (r *BatchResult[In, Out]) AddFailure(in In, err error) {
	*r = append(*r, BatchItemResult[In, Out]{In: in, Err: err})
}

// Batch processes items in batches defined by maxSize or maxDuration.
// Process is called for each batch, returning BatchResult values.
// Successful items are forwarded to the output channel; errors invoke cancel.
// Behavior is configurable with options. The returned channel is closed
// after processing finishes.
func Batch[In, Out any](
	ctx context.Context,
	in <-chan In,
	process ProcessFunc[[]In, BatchResult[In, Out]],
	cancel CancelFunc[[]In],
	maxSize int,
	maxDuration time.Duration,
	opts ...Option,
) <-chan Out {

	out := Process(
		ctx,
		Collect(in, maxSize, maxDuration),
		processBatchResult(process, cancel),
		cancel,
		opts...)

	return Split(out)
}

func processBatchResult[In, Out any](
	process ProcessFunc[[]In, BatchResult[In, Out]],
	cancel CancelFunc[[]In],
) ProcessFunc[[]In, []Out] {
	return func(ctx context.Context, batch []In) ([]Out, error) {
		res, err := process(ctx, batch)
		if err != nil {
			return nil, err
		}
		outs := make([]Out, 0, len(res))
		for _, r := range res {
			if r.Err != nil {
				cancel([]In{r.In}, r.Err)
			} else {
				outs = append(outs, r.Out)
			}
		}
		return outs, nil
	}
}
