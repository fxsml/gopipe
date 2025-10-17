package gopipe

import "context"

// Sink applies handle to each value from in.
// The returned channel is closed after in is closed and all values are processed.
func Sink[T any](
	in <-chan T,
	handle func(T),
) <-chan struct{} {
	done := make(chan struct{})

	go func() {
		defer close(done)
		for val := range in {
			handle(val)
		}
	}()

	return done
}

// NewSinkPipe creates a Pipe that applies handle to each value from in.
func NewSinkPipe[In any](
	handle func(context.Context, In) error,
	opts ...Option[In, struct{}],
) Pipe[In, struct{}] {
	proc := NewProcessor(func(ctx context.Context, in In) ([]struct{}, error) {
		return nil, handle(ctx, in)
	}, nil)
	return NewPipe(NoopPreProcessorFunc[In], proc, opts...)
}
