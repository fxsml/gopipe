package pipe

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// ProcessFunc is the core processing function signature.
// It takes a context and an input value, and returns a slice of output values or an error.
type ProcessFunc[In, Out any] func(ctx context.Context, in In) ([]Out, error)

// Config configures behavior of a Pipe.
type Config struct {
	// Concurrency sets the number of concurrent workers.
	// Default is 1.
	Concurrency int

	// BufferSize sets the output channel buffer size.
	// Default is 0 (unbuffered).
	BufferSize int

	// ErrorHandler is called when processing fails.
	// Default logs via slog.Error.
	ErrorHandler func(in any, err error)

	// CleanupHandler is called when processing is complete.
	CleanupHandler func(ctx context.Context)

	// CleanupTimeout sets the timeout for cleanup operations.
	CleanupTimeout time.Duration

	// ShutdownTimeout controls shutdown behavior on context cancellation.
	// If <= 0, forces immediate shutdown (no grace period).
	// If > 0, waits up to this duration for natural completion, then forces shutdown.
	// On forced shutdown:
	//   - Workers stop forwarding (escape blocked sends)
	//   - Workers drain remaining input, calling ErrorHandler for each
	//   - Workers exit when input closes
	ShutdownTimeout time.Duration
}

func (c Config) parse() Config {
	if c.Concurrency <= 0 {
		c.Concurrency = 1
	}
	if c.ErrorHandler == nil {
		c.ErrorHandler = func(in any, err error) {
			slog.Error("[GOPIPE] Processing failed", slog.Any("input", in), slog.Any("error", err))
		}
	}
	return c
}

// startProcessing is the internal processing engine used by all pipe types.
// It processes items from the input channel using the provided function
// and returns a channel that will receive the processed outputs.
//
// Processing continues until the input channel is closed or the context is canceled.
// On context cancellation, ShutdownTimeout controls the grace period before forced shutdown.
// On forced shutdown, workers stop forwarding and drain remaining input, calling
// ErrorHandler with ErrShutdownDropped for each drained message.
// Workers exit when input closes. The output channel is closed when processing is complete.
//
// This function does not apply middleware. Users should call Use
// on the pipe before calling Start to add middleware like retry, logging, etc.
func startProcessing[In, Out any](
	ctx context.Context,
	in <-chan In,
	fn ProcessFunc[In, Out],
	cfg Config,
) <-chan Out {
	cfg = cfg.parse()
	out := make(chan Out, cfg.BufferSize)
	done := make(chan struct{})

	var wg sync.WaitGroup
	wg.Add(cfg.Concurrency)
	for range cfg.Concurrency {
		go func() {
			defer wg.Done()
			for {
				select {
				case <-done:
					// Forced shutdown - exit immediately without draining
					// (draining blocks if input channel is still open)
					return
				case val, ok := <-in:
					if !ok {
						return
					}
					if res, err := fn(ctx, val); err != nil {
						cfg.ErrorHandler(val, err)
					} else {
						for _, r := range res {
							select {
							case out <- r:
							case <-done:
								// Forced shutdown - report current input and exit
								cfg.ErrorHandler(val, ErrShutdownDropped)
								return
							}
						}
					}
				}
			}
		}()
	}

	wgDone := make(chan struct{})
	go func() {
		wg.Wait()
		close(wgDone)
	}()

	go func() {
		// Wait for either context cancellation or natural completion
		select {
		case <-ctx.Done():
			if cfg.ShutdownTimeout > 0 {
				// Grace period - wait for natural completion or timeout
				select {
				case <-wgDone:
					// Workers finished naturally within grace period
				case <-time.After(cfg.ShutdownTimeout):
					// Force shutdown after grace period
					close(done)
				}
			} else {
				// No grace period - force shutdown immediately
				close(done)
			}
		case <-wgDone:
			// Workers finished naturally (input closed)
		}
		<-wgDone

		if cfg.CleanupHandler != nil {
			cleanupCtx := context.Background()
			if cfg.CleanupTimeout > 0 {
				var cancel context.CancelFunc
				cleanupCtx, cancel = context.WithTimeout(context.Background(), cfg.CleanupTimeout)
				defer cancel()
			}
			cfg.CleanupHandler(cleanupCtx)
		}

		close(out)
	}()

	return out
}
