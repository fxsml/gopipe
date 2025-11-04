package gopipe

import (
	"context"

	"github.com/fxsml/gopipe/channel"
)

// Generator produces a stream of values using a provided function.
type Generator[Out any] interface {
	// Generate returns a channel that emits generated values until context cancellation or error.
	Generate(ctx context.Context) <-chan Out
}

type generator[Out any] struct {
	proc Processor[struct{}, Out]
	opts []Option[struct{}, Out]
}

func (g *generator[Out]) Generate(ctx context.Context) <-chan Out {
	return startProcessor(ctx, channel.FromFunc(ctx, func() struct{} {
		return struct{}{}
	}), g.proc, g.opts)
}

// NewGenerator creates a Generator that produces values using the provided handle function.
// The handle function is called repeatedly until context cancellation.
func NewGenerator[Out any](
	handle func(context.Context) ([]Out, error),
	opts ...Option[struct{}, Out],
) Generator[Out] {
	return &generator[Out]{
		proc: NewProcessor(
			func(ctx context.Context, _ struct{}) ([]Out, error) {
				return handle(ctx)
			},
			nil,
		),
		opts: opts,
	}
}
