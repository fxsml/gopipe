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

	// ProcessTimeout sets a per-message processing deadline (default: 0, no timeout).
	// If > 0, each handler invocation is wrapped with a timeout context.
	// During normal operation, handlers are cancelled if they exceed ProcessTimeout.
	// During shutdown grace period, handlers continue executing and ProcessTimeout
	// remains enforced independently (handlers can still timeout during grace period).
	// On forced shutdown (grace period expired), all handlers are cancelled immediately
	// regardless of their remaining ProcessTimeout.
	// This ensures ShutdownTimeout remains a hard deadline while allowing
	// per-message timeouts during normal operation and grace periods.
	ProcessTimeout time.Duration

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
	//   - Active handler contexts are canceled immediately
	//   - Workers blocked sending output call ErrorHandler with ErrShutdownDropped
	//   - Workers waiting for input exit immediately (remaining buffered input is abandoned)
	ShutdownTimeout time.Duration

	// Metrics receives observability events (optional, nil = no overhead).
	Metrics Metrics

	// Labels provides static identifiers attached to every metrics event.
	// Set once at configuration time, same for all operations.
	// Common keys: "stage", "pipeline", "handler", "service"
	Labels map[string]string

	// LabelFunc extracts dynamic labels from each processed value.
	// Called after a value is received, before RecordProcessing and RecordWait (WaitOpSend).
	// Returned labels are merged with Labels, with LabelFunc values taking precedence on conflicts.
	// Only called when Metrics is non-nil. Nil means no dynamic labels.
	LabelFunc func(val any) map[string]string
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
// On forced shutdown, active handler contexts are canceled, workers blocked on output sends
// call ErrorHandler with ErrShutdownDropped, and workers waiting for input exit immediately
// (remaining buffered input is abandoned). The output channel is closed when all workers exit.
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

	// Create shutdown context that's only cancelled on forced shutdown
	// This ensures handlers continue running during grace period
	shutdownCtx, shutdownCancel := context.WithCancel(context.Background())

	var wg sync.WaitGroup
	wg.Add(cfg.Concurrency)
	for range cfg.Concurrency {
		go func() {
			defer wg.Done()
			for {
				// Track time blocked waiting for input.
				var receiveStart time.Time
				if cfg.Metrics != nil {
					receiveStart = time.Now()
				}

				select {
				case <-done:
					// Forced shutdown - exit immediately without draining
					// (draining blocks if input channel is still open)
					return
				case val, ok := <-in:
					if !ok {
						return
					}
					if cfg.Metrics != nil {
						cfg.Metrics.RecordWait(ctx, WaitInfo{
							Labels:    cfg.Labels,
							Operation: WaitOpReceive,
							Duration:  time.Since(receiveStart),
						})
					}

					// Compute dynamic labels once per value, before RecordProcessing and send waits.
					// Receive waits use static cfg.Labels only (no value available yet).
					labels := mergeLabels(cfg.Labels, cfg.LabelFunc, val)

					// Process message in anonymous function to ensure defer executes per-message
					func() {
						// Create handler context with timeout if configured
						// Derive from shutdownCtx (not parent ctx) to ensure handlers
						// continue during grace period and are only cancelled on forced shutdown
						handlerCtx := shutdownCtx
						var cancel context.CancelFunc
						if cfg.ProcessTimeout > 0 {
							handlerCtx, cancel = context.WithTimeout(shutdownCtx, cfg.ProcessTimeout)
							defer cancel()
						}

						var processStart time.Time
						if cfg.Metrics != nil {
							processStart = time.Now()
						}

						res, err := fn(handlerCtx, val)

						if cfg.Metrics != nil {
							cfg.Metrics.RecordProcessing(ctx, ProcessingInfo{
								Labels:      labels,
								Duration:    time.Since(processStart),
								Error:       err,
								OutputCount: len(res),
							})
						}

						if err != nil {
							cfg.ErrorHandler(val, err)
						} else {
							for _, r := range res {
								var sendStart time.Time
								if cfg.Metrics != nil {
									sendStart = time.Now()
								}

								select {
								case out <- r:
									if cfg.Metrics != nil {
										cfg.Metrics.RecordWait(ctx, WaitInfo{
											Labels:    labels,
											Operation: WaitOpSend,
											Duration:  time.Since(sendStart),
										})
									}
								case <-done:
									// Forced shutdown - report current input and exit
									cfg.ErrorHandler(val, ErrShutdownDropped)
									return
								}
							}
						}
					}()
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
					shutdownCancel() // Cancel handler contexts
				}
			} else {
				// No grace period - force shutdown immediately
				close(done)
				shutdownCancel() // Cancel handler contexts immediately
			}
		case <-wgDone:
			// Workers finished naturally (input closed)
		}
		<-wgDone

		// Cancel shutdown context after all workers exit (idempotent - may already
		// be cancelled from forced shutdown path above, but safe to call again)
		shutdownCancel()

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
