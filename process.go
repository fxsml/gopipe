package gopipe

import "context"

// Process maps each value from in to zero or more values using handle.
// The returned channel is closed after in is closed.
func Process[In, Out any](
	in <-chan In,
	handle func(In) []Out,
) <-chan Out {
	out := make(chan Out)

	go func() {
		defer close(out)
		for val := range in {
			for _, item := range handle(val) {
				out <- item
			}
		}
	}()

	return out
}

// NewProcessPipe creates a Pipe that can transform each input into multiple outputs.
// Unlike NewTransformPipe, this can produce zero, one, or many outputs for each input.
// The handle function receives a context and input item, and returns a slice of outputs or an error.
func NewProcessPipe[In, Out any](
	handle func(context.Context, In) ([]Out, error),
	opts ...Option[In, Out],
) Pipe[In, Out] {
	proc := NewProcessor(handle, nil)
	return NewPipe(NoopPreProcessorFunc[In], proc, opts...)
}
