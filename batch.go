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
	in <-chan In,
	handle func([]In) []Out,
	maxSize int,
	maxDuration time.Duration,
) <-chan Out {
	return Split(Transform(Collect(in, maxSize, maxDuration), handle))
}

// NewBatchPipe creates a Pipe that groups inputs into batches before processing.
// Each batch is processed as a whole by the handle function, which can return multiple outputs.
// Batches are created when either maxSize items are collected or maxDuration elapses since the first item.
func NewBatchPipe[In any, Out any](
	handle func(context.Context, []In) ([]Out, error),
	maxSize int,
	maxDuration time.Duration,
	opts ...Option[[]In, Out],
) Pipe[In, Out] {
	proc := NewProcessor(handle, nil)
	return NewPipe(func(pre <-chan In) <-chan []In {
		return Collect(pre, maxSize, maxDuration)
	}, proc, opts...)
}
