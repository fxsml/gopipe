//go:build ignore

// Example demonstrating extensible type-agnostic middleware chains.
//
// Problem: Previous approach required `instantiate` to know all middleware types.
// This doesn't allow custom middleware.
//
// Solution: Use a shared builder signature that operates on type-erased functions.
// Each middleware returns a MiddlewareBuilder that can be instantiated with any types.
//
//     // Middleware returns a builder (no type params)
//     func Retry(cfg RetryConfig) MiddlewareBuilder
//
//     // Builder has shared signature, can be chained
//     chain := Chain(
//         Recover(RecoverConfig{...}),
//         Retry(RetryConfig{...}),
//         MyCustomMiddleware(MyConfig{...}),  // Works with custom middleware!
//     )
//
//     // Apply with specific types
//     mw := Apply[Order, Cmd](chain)
//
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
// CORE TYPES
// =============================================================================

// Processor interface (typed)
type Processor[In, Out any] interface {
	Process(context.Context, In) ([]Out, error)
	Drop(In, error)
}

// Middleware wraps a typed Processor
type Middleware[In, Out any] func(Processor[In, Out]) Processor[In, Out]

// =============================================================================
// TYPE-ERASED BUILDER PATTERN
// =============================================================================

// WrapperFunc is the shared signature for all middleware.
// It receives:
//   - process: calls next.Process (type-erased)
//   - drop: calls next.Drop (type-erased)
//
// It returns wrapped versions of these functions.
// This signature is the SAME for all middleware, enabling chaining.
type WrapperFunc func(
	process func(context.Context, any) ([]any, error),
	drop func(any, error),
) (
	wrappedProcess func(context.Context, any) ([]any, error),
	wrappedDrop func(any, error),
)

// MiddlewareBuilder holds a WrapperFunc for deferred typed instantiation.
type MiddlewareBuilder struct {
	name string      // For debugging
	wrap WrapperFunc // The actual wrapping logic
}

// Chain combines multiple builders into one.
func Chain(builders ...MiddlewareBuilder) MiddlewareBuilder {
	return MiddlewareBuilder{
		name: "chain",
		wrap: func(process func(context.Context, any) ([]any, error), drop func(any, error)) (func(context.Context, any) ([]any, error), func(any, error)) {
			// Apply in reverse order (last builder wraps innermost)
			for i := len(builders) - 1; i >= 0; i-- {
				process, drop = builders[i].wrap(process, drop)
			}
			return process, drop
		},
	}
}

// Apply instantiates a builder with specific types.
// This is where types are "locked in" for a specific pipe.
func Apply[In, Out any](b MiddlewareBuilder) Middleware[In, Out] {
	return func(next Processor[In, Out]) Processor[In, Out] {
		// Convert typed processor to type-erased functions
		processAny := func(ctx context.Context, in any) ([]any, error) {
			out, err := next.Process(ctx, in.(In))
			// Convert []Out to []any
			result := make([]any, len(out))
			for i, o := range out {
				result[i] = o
			}
			return result, err
		}
		dropAny := func(in any, err error) {
			next.Drop(in.(In), err)
		}

		// Apply the wrapper
		wrappedProcess, wrappedDrop := b.wrap(processAny, dropAny)

		// Convert back to typed processor
		return &processor[In, Out]{
			process: func(ctx context.Context, in In) ([]Out, error) {
				result, err := wrappedProcess(ctx, in)
				// Convert []any to []Out
				out := make([]Out, len(result))
				for i, r := range result {
					out[i] = r.(Out)
				}
				return out, err
			},
			drop: func(in In, err error) {
				wrappedDrop(in, err)
			},
		}
	}
}

// =============================================================================
// STANDARD MIDDLEWARE (return MiddlewareBuilder, not Middleware[In, Out])
// =============================================================================

// --- Recover ---

type RecoverConfig struct {
	OnPanic func(input any, panicValue any, stack string)
}

func Recover(cfg RecoverConfig) MiddlewareBuilder {
	return MiddlewareBuilder{
		name: "recover",
		wrap: func(process func(context.Context, any) ([]any, error), drop func(any, error)) (func(context.Context, any) ([]any, error), func(any, error)) {
			return func(ctx context.Context, in any) (out []any, err error) {
				defer func() {
					if r := recover(); r != nil {
						if cfg.OnPanic != nil {
							cfg.OnPanic(in, r, string(debug.Stack()))
						}
						err = fmt.Errorf("panic: %v", r)
					}
				}()
				return process(ctx, in)
			}, drop
		},
	}
}

// --- Timeout ---

type TimeoutConfig struct {
	Duration time.Duration
}

func Timeout(cfg TimeoutConfig) MiddlewareBuilder {
	return MiddlewareBuilder{
		name: "timeout",
		wrap: func(process func(context.Context, any) ([]any, error), drop func(any, error)) (func(context.Context, any) ([]any, error), func(any, error)) {
			if cfg.Duration <= 0 {
				return process, drop
			}
			return func(ctx context.Context, in any) ([]any, error) {
				ctx, cancel := context.WithTimeout(ctx, cfg.Duration)
				defer cancel()
				return process(ctx, in)
			}, drop
		},
	}
}

// --- Retry ---

type RetryConfig struct {
	MaxAttempts int
	Backoff     func(attempt int) time.Duration
	ShouldRetry func(error) bool
}

func Retry(cfg RetryConfig) MiddlewareBuilder {
	if cfg.MaxAttempts == 0 {
		cfg.MaxAttempts = 3
	}
	if cfg.Backoff == nil {
		cfg.Backoff = func(int) time.Duration { return 100 * time.Millisecond }
	}
	if cfg.ShouldRetry == nil {
		cfg.ShouldRetry = func(error) bool { return true }
	}

	return MiddlewareBuilder{
		name: "retry",
		wrap: func(process func(context.Context, any) ([]any, error), drop func(any, error)) (func(context.Context, any) ([]any, error), func(any, error)) {
			return func(ctx context.Context, in any) ([]any, error) {
				var lastErr error
				for attempt := 1; attempt <= cfg.MaxAttempts; attempt++ {
					out, err := process(ctx, in)
					if err == nil {
						return out, nil
					}
					lastErr = err
					if !cfg.ShouldRetry(err) || attempt >= cfg.MaxAttempts {
						break
					}
					select {
					case <-ctx.Done():
						return nil, ctx.Err()
					case <-time.After(cfg.Backoff(attempt)):
					}
				}
				return nil, lastErr
			}, drop
		},
	}
}

// --- Logging ---

type LoggingConfig struct {
	Logger  Logger
	OnDrop  bool
	Verbose bool
}

type Logger interface {
	Debug(msg string, args ...any)
	Info(msg string, args ...any)
	Warn(msg string, args ...any)
	Error(msg string, args ...any)
}

func Logging(cfg LoggingConfig) MiddlewareBuilder {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	return MiddlewareBuilder{
		name: "logging",
		wrap: func(process func(context.Context, any) ([]any, error), drop func(any, error)) (func(context.Context, any) ([]any, error), func(any, error)) {
			wrappedProcess := func(ctx context.Context, in any) ([]any, error) {
				start := time.Now()
				out, err := process(ctx, in)
				if err != nil {
					cfg.Logger.Error("failed", "duration", time.Since(start), "error", err)
				} else if cfg.Verbose {
					cfg.Logger.Info("success", "duration", time.Since(start), "outputs", len(out))
				}
				return out, err
			}
			wrappedDrop := drop
			if cfg.OnDrop {
				wrappedDrop = func(in any, err error) {
					cfg.Logger.Warn("dropped", "type", fmt.Sprintf("%T", in), "error", err)
					drop(in, err)
				}
			}
			return wrappedProcess, wrappedDrop
		},
	}
}

// --- Metrics ---

type MetricsConfig struct {
	Recorder MetricsRecorder
}

type MetricsRecorder interface {
	RecordProcessed(duration time.Duration, success bool)
	RecordDropped(isCancel bool)
}

func Metrics(cfg MetricsConfig) MiddlewareBuilder {
	return MiddlewareBuilder{
		name: "metrics",
		wrap: func(process func(context.Context, any) ([]any, error), drop func(any, error)) (func(context.Context, any) ([]any, error), func(any, error)) {
			return func(ctx context.Context, in any) ([]any, error) {
				start := time.Now()
				out, err := process(ctx, in)
				cfg.Recorder.RecordProcessed(time.Since(start), err == nil)
				return out, err
			}, func(in any, err error) {
				cfg.Recorder.RecordDropped(errors.Is(err, ErrCanceled))
				drop(in, err)
			}
		},
	}
}

// =============================================================================
// CUSTOM MIDDLEWARE EXAMPLE
// =============================================================================

// CircuitBreakerConfig - example of custom middleware
type CircuitBreakerConfig struct {
	Threshold   int           // Failures before opening
	ResetAfter  time.Duration // Time before trying again
	OnStateChange func(open bool)
}

// CircuitBreaker - custom middleware using the SAME builder pattern
func CircuitBreaker(cfg CircuitBreakerConfig) MiddlewareBuilder {
	var failures atomic.Int32
	var lastFailure atomic.Int64

	return MiddlewareBuilder{
		name: "circuit-breaker",
		wrap: func(process func(context.Context, any) ([]any, error), drop func(any, error)) (func(context.Context, any) ([]any, error), func(any, error)) {
			return func(ctx context.Context, in any) ([]any, error) {
				// Check if circuit is open
				if int(failures.Load()) >= cfg.Threshold {
					lastFail := time.Unix(0, lastFailure.Load())
					if time.Since(lastFail) < cfg.ResetAfter {
						return nil, errors.New("circuit breaker open")
					}
					// Reset - try again
					failures.Store(0)
					if cfg.OnStateChange != nil {
						cfg.OnStateChange(false)
					}
				}

				out, err := process(ctx, in)
				if err != nil {
					if failures.Add(1) == int32(cfg.Threshold) {
						lastFailure.Store(time.Now().UnixNano())
						if cfg.OnStateChange != nil {
							cfg.OnStateChange(true)
						}
					}
				} else {
					failures.Store(0)
				}
				return out, err
			}, drop
		},
	}
}

// =============================================================================
// HELPERS
// =============================================================================

var ErrCanceled = errors.New("canceled")

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

func ExponentialBackoff(initial time.Duration, factor float64, max time.Duration) func(int) time.Duration {
	return func(attempt int) time.Duration {
		d := time.Duration(float64(initial) * math.Pow(factor, float64(attempt-1)))
		if max > 0 && d > max {
			d = max
		}
		jitter := 1.0 + (rand.Float64()*0.2 - 0.1)
		return time.Duration(float64(d) * jitter)
	}
}

// =============================================================================
// EXAMPLE USAGE
// =============================================================================

type Order struct {
	ID     string
	Amount float64
}

type Payment struct {
	OrderID string
	Method  string
}

type ShippingCmd struct{ OrderID string }
type Receipt struct{ PaymentID string }

type simpleMetrics struct {
	processed, succeeded, failed, dropped atomic.Int64
}

func (m *simpleMetrics) RecordProcessed(d time.Duration, success bool) {
	m.processed.Add(1)
	if success {
		m.succeeded.Add(1)
	} else {
		m.failed.Add(1)
	}
}

func (m *simpleMetrics) RecordDropped(isCancel bool) { m.dropped.Add(1) }

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	metrics := &simpleMetrics{}

	// =========================================================================
	// DEFINE CHAIN ONCE - includes standard AND custom middleware
	// =========================================================================
	productionChain := Chain(
		// Standard middleware
		Recover(RecoverConfig{
			OnPanic: func(input any, val any, stack string) {
				slog.Error("PANIC", "value", val)
			},
		}),
		Logging(LoggingConfig{
			Logger: slog.Default(),
			OnDrop: true,
		}),
		Metrics(MetricsConfig{
			Recorder: metrics,
		}),
		Timeout(TimeoutConfig{
			Duration: 200 * time.Millisecond,
		}),
		// CUSTOM middleware - same API!
		CircuitBreaker(CircuitBreakerConfig{
			Threshold:  3,
			ResetAfter: time.Second,
			OnStateChange: func(open bool) {
				if open {
					slog.Warn("circuit breaker OPENED")
				} else {
					slog.Info("circuit breaker closed")
				}
			},
		}),
		Retry(RetryConfig{
			MaxAttempts: 2,
			Backoff:     ExponentialBackoff(10*time.Millisecond, 2, time.Second),
		}),
	)

	// =========================================================================
	// APPLY TO DIFFERENT TYPES
	// =========================================================================

	// Order processor [Order, ShippingCmd]
	orderProc := Apply[Order, ShippingCmd](productionChain)(&processor[Order, ShippingCmd]{
		process: func(ctx context.Context, order Order) ([]ShippingCmd, error) {
			if order.Amount < 10 {
				return nil, errors.New("amount too low")
			}
			time.Sleep(20 * time.Millisecond)
			return []ShippingCmd{{OrderID: order.ID}}, nil
		},
		drop: func(Order, error) {},
	})

	// Payment processor [Payment, Receipt] - SAME CHAIN, DIFFERENT TYPES!
	paymentProc := Apply[Payment, Receipt](productionChain)(&processor[Payment, Receipt]{
		process: func(ctx context.Context, p Payment) ([]Receipt, error) {
			time.Sleep(10 * time.Millisecond)
			return []Receipt{{PaymentID: p.OrderID + "-receipt"}}, nil
		},
		drop: func(Payment, error) {},
	})

	// =========================================================================
	// PROCESS
	// =========================================================================
	var wg sync.WaitGroup

	fmt.Println("=== Processing Orders ===")
	for _, order := range []Order{
		{ID: "ORD-1", Amount: 100},
		{ID: "ORD-2", Amount: 5}, // Will fail
		{ID: "ORD-3", Amount: 200},
	} {
		wg.Add(1)
		go func(o Order) {
			defer wg.Done()
			cmds, err := orderProc.Process(ctx, o)
			if err != nil {
				fmt.Printf("❌ Order %s: %v\n", o.ID, err)
			} else {
				fmt.Printf("✓ Order %s → %d commands\n", o.ID, len(cmds))
			}
		}(order)
	}

	fmt.Println("\n=== Processing Payments ===")
	for _, payment := range []Payment{
		{OrderID: "ORD-1", Method: "card"},
		{OrderID: "ORD-3", Method: "paypal"},
	} {
		wg.Add(1)
		go func(p Payment) {
			defer wg.Done()
			receipts, err := paymentProc.Process(ctx, p)
			if err != nil {
				fmt.Printf("❌ Payment %s: %v\n", p.OrderID, err)
			} else {
				fmt.Printf("✓ Payment %s → %d receipts\n", p.OrderID, len(receipts))
			}
		}(payment)
	}

	wg.Wait()

	fmt.Println("\n=== Summary ===")
	fmt.Printf("Processed: %d (ok=%d, fail=%d), Dropped: %d\n",
		metrics.processed.Load(), metrics.succeeded.Load(),
		metrics.failed.Load(), metrics.dropped.Load())
	fmt.Println("\nKey points:")
	fmt.Println("  - Chain defined ONCE with no type parameters")
	fmt.Println("  - Includes CUSTOM middleware (CircuitBreaker)")
	fmt.Println("  - Applied to [Order, ShippingCmd] AND [Payment, Receipt]")
	fmt.Println("  - All middleware use the same WrapperFunc signature")
}
