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
	process ProcessFunc[[]In, []Out],
	cancel CancelFunc[[]In],
	maxSize int,
	maxDuration time.Duration,
	opts ...Option,
) <-chan Out {

	// Read options to see if metrics are configured so we can observe batch sizes.
	c := defaultConfig()
	for _, opt := range opts {
		opt(&c)
	}

	// Wrap process to emit batch-size metric if available.
	if m := c.metrics; m != nil {
		process = func(ctx context.Context, batch []In) ([]Out, error) {
			m.ObserveBatchSize(len(batch))
			return process(ctx, batch)
		}
	}

	out := Process(
		ctx,
		Collect(in, maxSize, maxDuration),
		process,
		cancel,
		opts...)

	return Split(out)
}
