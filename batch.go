package gopipe

import (
	"context"
	"time"
)

// Batch groups values from in into batches and processes each batch using
// the process function. Batches are created when either maxSize is reached
// or maxDuration elapses. Successful results are sent to the returned channel;
// failures invoke cancel. An additional process stage can be chained for result
// transformation. Behavior is configurable with options. The returned channel
// is closed after processing finishes.
func Batch[In, Out any](
	ctx context.Context,
	in <-chan In,
	proc Processor[[]In, []Out],
	maxSize int,
	maxDuration time.Duration,
	opts ...Option,
) <-chan Out {

	out := pipe(
		ctx,
		Collect(in, maxSize, maxDuration),
		proc,
		opts...)

	return Split(out)
}
