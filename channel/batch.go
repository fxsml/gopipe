package channel

import (
	"time"
)

// Batch groups values from in into batches and processes each batch using
// the process function. Batches are created when either maxSize is reached
// or maxDuration elapses. Successful results are sent to the returned channel;
// failures invoke cancel. An additional process stage can be chained for result
// transformation. Behavior is configurable with options. The returned channel
// is closed after processing finishes.
func Batch[In, Out any](
	in <-chan In,
	handle func([]In) []Out,
	maxSize int,
	maxDuration time.Duration,
) <-chan Out {
	return Flatten(Transform(Collect(in, maxSize, maxDuration), handle))
}
