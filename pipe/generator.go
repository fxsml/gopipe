package pipe

import (
	"context"
	"sync"

	"github.com/fxsml/gopipe/channel"
	"github.com/fxsml/gopipe/pipe/middleware"
)

// Generator produces a stream of values using a provided function.
type Generator[Out any] interface {
	// Generate returns a channel that emits generated values until context cancellation or error.
	// Returns ErrAlreadyStarted if the generator has already been started.
	Generate(ctx context.Context) (<-chan Out, error)
}

// GeneratePipe is a concrete Generator implementation that supports method chaining.
type GeneratePipe[Out any] struct {
	fn  ProcessFunc[struct{}, Out]
	cfg Config
	mw  []middleware.Middleware[struct{}, Out]

	mu      sync.Mutex
	started bool
	outCh   <-chan Out
}

// Generate returns a channel that emits generated values until context cancellation or error.
// Returns ErrAlreadyStarted if the generator has already been started.
func (g *GeneratePipe[Out]) Generate(ctx context.Context) (<-chan Out, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.started {
		return nil, ErrAlreadyStarted
	}
	g.started = true
	fn := applyMiddleware(g.fn, g.mw)
	out := startProcessing(ctx, channel.FromFunc(ctx, func() struct{} {
		return struct{}{}
	}), fn, g.cfg)
	g.outCh = out
	return out, nil
}

// Stats returns a point-in-time snapshot of the output buffer depth and capacity.
// Returns a zero Stats if the generator has not been started.
// Use with a pull-based metrics backend (e.g. OTel observable gauge) to avoid
// recording on every send.
func (g *GeneratePipe[Out]) Stats() Stats {
	g.mu.Lock()
	ch := g.outCh
	g.mu.Unlock()
	if ch == nil {
		return Stats{}
	}
	return Stats{
		Depth:    len(ch),
		Capacity: cap(ch),
	}
}

// Use adds middleware to the processing chain.
// Middleware is applied in the order it is added.
// Returns ErrAlreadyStarted if the generator has already been started.
func (g *GeneratePipe[Out]) Use(mw ...middleware.Middleware[struct{}, Out]) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.started {
		return ErrAlreadyStarted
	}
	g.mw = append(g.mw, mw...)
	return nil
}

// NewGenerator creates a GeneratePipe that produces values using the provided handle function.
// The handle function is called repeatedly until context cancellation.
// Call Use on the returned *GeneratePipe to add middleware.
func NewGenerator[Out any](
	handle func(context.Context) ([]Out, error),
	cfg Config,
) *GeneratePipe[Out] {
	fn := func(ctx context.Context, _ struct{}) ([]Out, error) {
		return handle(ctx)
	}
	return &GeneratePipe[Out]{
		fn:  fn,
		cfg: cfg,
	}
}
