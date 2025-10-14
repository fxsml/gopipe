package gopipe

import (
	"context"
)

// Process applies process function to each item from in concurrently.
// Successful results are sent to the returned channel; failures invoke cancel.
// Behavior is configurable with options. The returned channel is closed
// after processing finishes.
func Process[In, Out any](
	ctx context.Context,
	in <-chan In,
	proc Processor[In, Out],
	opts ...Option,
) <-chan Out {
	return pipe(ctx, in, proc, opts...)
}
