//go:build ignore

// Example demonstrating the proposed middleware architecture.
//
// This example shows how middleware would work after refactoring:
// - All middleware in middleware/ package
// - Explicit middleware composition via Chain()
// - No implicit behavior (logging is opt-in)
// - Reusable middleware chains across processors
//
// Run with: go run main.go
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"math/rand/v2"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"
)

// =============================================================================
// PROPOSED MIDDLEWARE PACKAGE TYPES
// =============================================================================

// Processor interface (from gopipe - unchanged)
type Processor[In, Out any] interface {
	Process(context.Context, In) ([]Out, error)
	Drop(In, error) // Renamed from Cancel
}

// Middleware wraps a Processor with additional behavior.
type Middleware[In, Out any] func(Processor[In, Out]) Processor[In, Out]

// Chain combines multiple middleware into one.
// Execution order: first middleware is outermost.
func Chain[In, Out any](mw ...Middleware[In, Out]) Middleware[In, Out] {
	return func(next Processor[In, Out]) Processor[In, Out] {
		for i := len(mw) - 1; i >= 0; i-- {
			next = mw[i](next)
		}
		return next
	}
}

// =============================================================================
// MIDDLEWARE IMPLEMENTATIONS
// =============================================================================

// --- Timeout Middleware ---

type TimeoutConfig struct {
	Duration         time.Duration
	PropagateContext bool
}

func Timeout[In, Out any](cfg TimeoutConfig) Middleware[In, Out] {
	if cfg.Duration <= 0 {
		return func(next Processor[In, Out]) Processor[In, Out] { return next }
	}
	return func(next Processor[In, Out]) Processor[In, Out] {
		return &processor[In, Out]{
			process: func(ctx context.Context, in In) ([]Out, error) {
				if !cfg.PropagateContext {
					ctx = context.Background()
				}
				ctx, cancel := context.WithTimeout(ctx, cfg.Duration)
				defer cancel()
				return next.Process(ctx, in)
			},
			drop: next.Drop,
		}
	}
}

// --- Retry Middleware ---

type RetryConfig struct {
	ShouldRetry func(error) bool
	Backoff     func(attempt int) time.Duration
	MaxAttempts int
	Timeout     time.Duration
}

var DefaultRetryConfig = RetryConfig{
	ShouldRetry: func(error) bool { return true },
	Backoff:     ConstantBackoff(time.Second, 0.2),
	MaxAttempts: 3,
	Timeout:     time.Minute,
}

func Retry[In, Out any](cfg RetryConfig) Middleware[In, Out] {
	if cfg.ShouldRetry == nil {
		cfg.ShouldRetry = DefaultRetryConfig.ShouldRetry
	}
	if cfg.Backoff == nil {
		cfg.Backoff = DefaultRetryConfig.Backoff
	}
	if cfg.MaxAttempts == 0 {
		cfg.MaxAttempts = DefaultRetryConfig.MaxAttempts
	}

	return func(next Processor[In, Out]) Processor[In, Out] {
		return &processor[In, Out]{
			process: func(ctx context.Context, in In) ([]Out, error) {
				start := time.Now()
				var lastErr error
				for attempt := 1; ; attempt++ {
					out, err := next.Process(ctx, in)
					if err == nil {
						return out, nil
					}
					lastErr = err

					if !cfg.ShouldRetry(err) {
						return nil, fmt.Errorf("not retryable: %w", err)
					}
					if cfg.MaxAttempts > 0 && attempt >= cfg.MaxAttempts {
						return nil, fmt.Errorf("max attempts (%d) reached: %w", cfg.MaxAttempts, lastErr)
					}
					if cfg.Timeout > 0 && time.Since(start) >= cfg.Timeout {
						return nil, fmt.Errorf("timeout reached: %w", lastErr)
					}

					select {
					case <-ctx.Done():
						return nil, ctx.Err()
					case <-time.After(cfg.Backoff(attempt)):
					}
				}
			},
			drop: next.Drop,
		}
	}
}

func ConstantBackoff(delay time.Duration, jitter float64) func(int) time.Duration {
	return func(attempt int) time.Duration {
		return applyJitter(delay, jitter)
	}
}

func ExponentialBackoff(initial time.Duration, factor float64, maxDelay time.Duration, jitter float64) func(int) time.Duration {
	return func(attempt int) time.Duration {
		backoff := time.Duration(float64(initial) * math.Pow(factor, float64(attempt-1)))
		if maxDelay > 0 && backoff > maxDelay {
			backoff = maxDelay
		}
		return applyJitter(backoff, jitter)
	}
}

func applyJitter(d time.Duration, jitter float64) time.Duration {
	if jitter <= 0 {
		return d
	}
	if jitter > 1 {
		jitter = 1
	}
	factor := 1.0 + (rand.Float64()*2*jitter - jitter)
	return time.Duration(float64(d) * factor)
}

// --- Recover Middleware ---

type RecoverConfig struct {
	OnPanic func(input any, panicValue any, stack string)
}

func Recover[In, Out any](cfg RecoverConfig) Middleware[In, Out] {
	return func(next Processor[In, Out]) Processor[In, Out] {
		return &processor[In, Out]{
			process: func(ctx context.Context, in In) (out []Out, err error) {
				defer func() {
					if r := recover(); r != nil {
						stack := string(debug.Stack())
						if cfg.OnPanic != nil {
							cfg.OnPanic(in, r, stack)
						}
						err = fmt.Errorf("panic recovered: %v", r)
					}
				}()
				return next.Process(ctx, in)
			},
			drop: next.Drop,
		}
	}
}

// --- Logging Middleware ---

type Logger interface {
	Debug(msg string, args ...any)
	Info(msg string, args ...any)
	Warn(msg string, args ...any)
	Error(msg string, args ...any)
}

type LoggingConfig struct {
	Logger      Logger
	LogSuccess  bool
	LogDuration bool
	OnDrop      bool
}

func Logging[In, Out any](cfg LoggingConfig) Middleware[In, Out] {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	return func(next Processor[In, Out]) Processor[In, Out] {
		return &processor[In, Out]{
			process: func(ctx context.Context, in In) ([]Out, error) {
				start := time.Now()
				out, err := next.Process(ctx, in)
				duration := time.Since(start)

				if err != nil {
					cfg.Logger.Error("processing failed",
						"duration", duration,
						"error", err,
					)
				} else if cfg.LogSuccess {
					args := []any{}
					if cfg.LogDuration {
						args = append(args, "duration", duration)
					}
					args = append(args, "output_count", len(out))
					cfg.Logger.Info("processing succeeded", args...)
				}
				return out, err
			},
			drop: func(in In, err error) {
				if cfg.OnDrop {
					cfg.Logger.Warn("item dropped",
						"type", fmt.Sprintf("%T", in),
						"error", err,
					)
				}
				next.Drop(in, err)
			},
		}
	}
}

// --- Metrics Middleware ---

type MetricsRecorder interface {
	RecordProcessed(duration time.Duration, success bool)
	RecordInFlight(delta int)
	RecordDropped(isCancel bool)
}

type MetricsConfig struct {
	Recorder MetricsRecorder
}

func Metrics[In, Out any](cfg MetricsConfig) Middleware[In, Out] {
	var inFlight atomic.Int32
	return func(next Processor[In, Out]) Processor[In, Out] {
		return &processor[In, Out]{
			process: func(ctx context.Context, in In) ([]Out, error) {
				cfg.Recorder.RecordInFlight(int(inFlight.Add(1)))
				defer func() { cfg.Recorder.RecordInFlight(int(inFlight.Add(-1))) }()

				start := time.Now()
				out, err := next.Process(ctx, in)
				cfg.Recorder.RecordProcessed(time.Since(start), err == nil)
				return out, err
			},
			drop: func(in In, err error) {
				isCancel := errors.Is(err, ErrCanceled)
				cfg.Recorder.RecordDropped(isCancel)
				next.Drop(in, err)
			},
		}
	}
}

// =============================================================================
// HELPER TYPES (simulating gopipe internals)
// =============================================================================

var (
	ErrCanceled = errors.New("gopipe: canceled")
	ErrFailed   = errors.New("gopipe: processing failed")
)

type processor[In, Out any] struct {
	process func(context.Context, In) ([]Out, error)
	drop    func(In, error)
}

func (p *processor[In, Out]) Process(ctx context.Context, in In) ([]Out, error) {
	return p.process(ctx, in)
}

func (p *processor[In, Out]) Drop(in In, err error) {
	if p.drop != nil {
		p.drop(in, err)
	}
}

// =============================================================================
// EXAMPLE USAGE
// =============================================================================

type Order struct {
	ID     string
	Amount float64
}

type ShippingCmd struct {
	OrderID string
	Address string
}

// SimpleMetrics implements MetricsRecorder
type SimpleMetrics struct {
	processed     atomic.Int64
	succeeded     atomic.Int64
	failed        atomic.Int64
	dropped       atomic.Int64
	droppedCancel atomic.Int64
}

func (m *SimpleMetrics) RecordProcessed(duration time.Duration, success bool) {
	m.processed.Add(1)
	if success {
		m.succeeded.Add(1)
	} else {
		m.failed.Add(1)
	}
}

func (m *SimpleMetrics) RecordInFlight(delta int) {}

func (m *SimpleMetrics) RecordDropped(isCancel bool) {
	m.dropped.Add(1)
	if isCancel {
		m.droppedCancel.Add(1)
	}
}

func (m *SimpleMetrics) Print() {
	fmt.Printf("\n=== Metrics ===\n")
	fmt.Printf("Processed:       %d\n", m.processed.Load())
	fmt.Printf("  Succeeded:     %d\n", m.succeeded.Load())
	fmt.Printf("  Failed:        %d\n", m.failed.Load())
	fmt.Printf("Dropped:         %d\n", m.dropped.Load())
	fmt.Printf("  Canceled:      %d\n", m.droppedCancel.Load())
}

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	metrics := &SimpleMetrics{}

	// =========================================================================
	// PRODUCTION MIDDLEWARE CHAIN (reusable across processors)
	// =========================================================================
	productionMiddleware := Chain(
		// Outermost: panic recovery
		Recover[Order, ShippingCmd](RecoverConfig{
			OnPanic: func(input any, panicValue any, stack string) {
				slog.Error("PANIC", "value", panicValue, "input", input)
			},
		}),
		// Logging
		Logging[Order, ShippingCmd](LoggingConfig{
			Logger:      slog.Default(),
			LogSuccess:  false, // Only log errors
			LogDuration: true,
			OnDrop:      true,
		}),
		// Metrics
		Metrics[Order, ShippingCmd](MetricsConfig{
			Recorder: metrics,
		}),
		// Timeout per item
		Timeout[Order, ShippingCmd](TimeoutConfig{
			Duration:         100 * time.Millisecond,
			PropagateContext: true,
		}),
		// Innermost: retry
		Retry[Order, ShippingCmd](RetryConfig{
			MaxAttempts: 2,
			Backoff:     ConstantBackoff(10*time.Millisecond, 0.1),
			ShouldRetry: func(err error) bool {
				// Don't retry validation errors
				return !errors.Is(err, errValidation)
			},
		}),
	)

	// =========================================================================
	// CREATE PROCESSOR WITH MIDDLEWARE
	// =========================================================================
	var attemptCount atomic.Int32

	baseProcessor := &processor[Order, ShippingCmd]{
		process: func(ctx context.Context, order Order) ([]ShippingCmd, error) {
			attempt := attemptCount.Add(1)

			// Simulate validation error
			if order.Amount < 50 {
				return nil, fmt.Errorf("%w: amount too low: %.2f", errValidation, order.Amount)
			}

			// Simulate transient error (succeeds on retry)
			if order.ID == "RETRY-1" && attempt <= 1 {
				return nil, errors.New("transient error")
			}

			// Simulate slow processing
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(30 * time.Millisecond):
			}

			return []ShippingCmd{{
				OrderID: order.ID,
				Address: "123 Main St",
			}}, nil
		},
		drop: func(order Order, err error) {
			// Default drop handler (could be from ProcessorConfig.OnDrop)
		},
	}

	// Apply middleware chain
	proc := productionMiddleware(baseProcessor)

	// =========================================================================
	// PROCESS ORDERS
	// =========================================================================
	orders := []Order{
		{ID: "ORD-1", Amount: 100},
		{ID: "ORD-2", Amount: 25},   // Validation error (not retried)
		{ID: "RETRY-1", Amount: 75}, // Transient error (retried)
		{ID: "ORD-3", Amount: 150},
		{ID: "ORD-4", Amount: 200},
	}

	var wg sync.WaitGroup
	results := make(chan ShippingCmd, len(orders))

	for _, order := range orders {
		wg.Add(1)
		go func(o Order) {
			defer wg.Done()
			out, err := proc.Process(ctx, o)
			if err != nil {
				proc.Drop(o, fmt.Errorf("%w: %w", ErrFailed, err))
				return
			}
			for _, cmd := range out {
				results <- cmd
			}
		}(order)
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	// Collect results
	fmt.Println("=== Results ===")
	for cmd := range results {
		fmt.Printf("✓ Shipping: %+v\n", cmd)
	}

	// Print metrics
	metrics.Print()
}

var errValidation = errors.New("validation error")
