package middleware

import (
	"context"

	"github.com/fxsml/gopipe"
)

// UseCancel creates a middleware that adds cancellation function to a processor.
// The middleware passes through Process calls unchanged while extending the cancellation path.
func UseCancel[In, Out any](cancel gopipe.CancelFunc[In]) MiddlewareFunc[In, Out] {
	return func(next gopipe.Processor[In, Out]) gopipe.Processor[In, Out] {
		return gopipe.NewProcessor(
			func(ctx context.Context, in In) (Out, error) {
				return next.Process(ctx, in)
			},
			func(in In, err error) {
				next.Cancel(in, err)
				cancel(in, err)
			})
	}
}
