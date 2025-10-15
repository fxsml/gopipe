package gopipe

import "context"

// Transform applies handle to each value from in and sends the result to the
// returned channel. The returned channel is closed after in is closed.
func Transform[In, Out any](
	in <-chan In,
	handle func(In) Out,
) <-chan Out {
	out := make(chan Out)

	go func() {
		defer close(out)
		for val := range in {
			out <- handle(val)
		}
	}()

	return out
}

// NewTransformPipe creates a Pipe that transforms each input into exactly one output.
// Unlike NewProcessPipe, this always produces exactly one output for each successful input.
// The handle function receives a context and input item, and returns a single output or an error.
func NewTransformPipe[In, Out any](
	handle func(context.Context, In) (Out, error),
	opts ...Option[In, Out],
) Pipe[In, Out] {
	proc := NewProcessor(func(ctx context.Context, in In) ([]Out, error) {
		out, err := handle(ctx, in)
		if err != nil {
			return nil, err
		}
		return []Out{out}, nil
	}, nil)
	return NewPipe(NoopPreProcessorFunc[In], proc, opts...)
}
