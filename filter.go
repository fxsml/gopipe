package gopipe

import "context"

// Filter passes through values from in for which handle returns true.
// The returned channel is closed after in is closed.
func Filter[T any](
	in <-chan T,
	handle func(T) bool,
) <-chan T {
	out := make(chan T)

	go func() {
		defer close(out)
		for val := range in {
			if handle(val) {
				out <- val
			}
		}
	}()

	return out
}

// NewFilterPipe creates a Pipe that selectively passes through inputs based on a predicate function.
// If the handle function returns true, the input is passed through; if false, the input is discarded.
// If the handle function returns an error, processing for that item stops and the error is handled.
func NewFilterPipe[In any](
	handle func(context.Context, In) (bool, error),
	opts ...Option[In, In],
) Pipe[In, In] {
	proc := NewProcessor(func(ctx context.Context, in In) ([]In, error) {
		ok, err := handle(ctx, in)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, nil
		}
		return []In{in}, nil
	}, nil)
	return NewPipe(NoopPreProcessorFunc[In], proc, opts...)
}
