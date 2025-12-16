//go:build ignore

// Example demonstrating type-agnostic middleware chains.
//
// Problem: Current middleware requires [In, Out] type parameters, making
// reusable chains verbose:
//
//     productionMiddleware := Chain(
//         Recover[Order, ShippingCmd](...),  // Type params everywhere!
//         Retry[Order, ShippingCmd](...),
//         Metrics[Order, ShippingCmd](...),
//     )
//     // Can ONLY use with [Order, ShippingCmd] pipes!
//
// Solution: Store middleware configs in a chain, instantiate with types later:
//
//     chain := NewChain(
//         RecoverConfig{...},   // No type params!
//         RetryConfig{...},
//         MetricsConfig{...},
//     )
//
//     // Apply to any pipe - types specified at application time
//     orderPipe := NewProcessPipe(orderHandler, nil, config, Apply[Order, Cmd](chain))
//     paymentPipe := NewProcessPipe(paymentHandler, nil, config, Apply[Payment, Receipt](chain))
//
// The chain is defined ONCE and reused across pipes with DIFFERENT types!
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
// TYPE-AGNOSTIC MIDDLEWARE CHAIN
// =============================================================================

// MiddlewareBuilder is a type-erased function that builds typed middleware.
// It's stored in the chain and called with type parameters when needed.
type MiddlewareBuilder func() any // Returns Middleware[In, Out] at runtime

// MiddlewareChain holds middleware configs for later typed instantiation.
type MiddlewareChain struct {
	builders []MiddlewareBuilder
}

// NewChain creates a chain from middleware configs.
// Configs are processed in order (first = outermost).
func NewChain(configs ...any) MiddlewareChain {
	chain := MiddlewareChain{
		builders: make([]MiddlewareBuilder, 0, len(configs)),
	}
	for _, cfg := range configs {
		chain.builders = append(chain.builders, configToBuilder(cfg))
	}
	return chain
}

// Apply instantiates the chain with specific type parameters.
// This is where the types are "locked in" for a specific pipe.
// Note: Go doesn't allow type parameters on methods, so this is a function.
func Apply[In, Out any](c MiddlewareChain) Middleware[In, Out] {
	middlewares := make([]Middleware[In, Out], len(c.builders))
	for i, builder := range c.builders {
		// Each builder returns Middleware[In, Out]
		mw := builder()
		// Type assert to the specific middleware type
		middlewares[i] = instantiate[In, Out](mw)
	}
	return Chain(middlewares...)
}

// configToBuilder converts a config struct to a type-erased builder.
func configToBuilder(cfg any) MiddlewareBuilder {
	switch c := cfg.(type) {
	case RecoverConfig:
		return func() any { return recoverBuilder{c} }
	case RetryConfig:
		return func() any { return retryBuilder{c} }
	case TimeoutConfig:
		return func() any { return timeoutBuilder{c} }
	case LoggingConfig:
		return func() any { return loggingBuilder{c} }
	case MetricsConfig:
		return func() any { return metricsBuilder{c} }
	default:
		panic(fmt.Sprintf("unknown middleware config type: %T", cfg))
	}
}

// instantiate converts a builder result to typed middleware.
func instantiate[In, Out any](builder any) Middleware[In, Out] {
	switch b := builder.(type) {
	case recoverBuilder:
		return Recover[In, Out](b.cfg)
	case retryBuilder:
		return Retry[In, Out](b.cfg)
	case timeoutBuilder:
		return Timeout[In, Out](b.cfg)
	case loggingBuilder:
		return Logging[In, Out](b.cfg)
	case metricsBuilder:
		return Metrics[In, Out](b.cfg)
	default:
		panic(fmt.Sprintf("unknown builder type: %T", builder))
	}
}

// Builder types (carry config for deferred instantiation)
type recoverBuilder struct{ cfg RecoverConfig }
type retryBuilder struct{ cfg RetryConfig }
type timeoutBuilder struct{ cfg TimeoutConfig }
type loggingBuilder struct{ cfg LoggingConfig }
type metricsBuilder struct{ cfg MetricsConfig }

// =============================================================================
// ALTERNATIVE: FLUENT BUILDER PATTERN
// =============================================================================

// ChainBuilder provides a fluent API for building middleware chains.
type ChainBuilder struct {
	chain MiddlewareChain
}

// Build starts a new chain builder.
func Build() *ChainBuilder {
	return &ChainBuilder{chain: MiddlewareChain{}}
}

func (b *ChainBuilder) WithRecover(cfg RecoverConfig) *ChainBuilder {
	b.chain.builders = append(b.chain.builders, func() any { return recoverBuilder{cfg} })
	return b
}

func (b *ChainBuilder) WithRetry(cfg RetryConfig) *ChainBuilder {
	b.chain.builders = append(b.chain.builders, func() any { return retryBuilder{cfg} })
	return b
}

func (b *ChainBuilder) WithTimeout(cfg TimeoutConfig) *ChainBuilder {
	b.chain.builders = append(b.chain.builders, func() any { return timeoutBuilder{cfg} })
	return b
}

func (b *ChainBuilder) WithLogging(cfg LoggingConfig) *ChainBuilder {
	b.chain.builders = append(b.chain.builders, func() any { return loggingBuilder{cfg} })
	return b
}

func (b *ChainBuilder) WithMetrics(cfg MetricsConfig) *ChainBuilder {
	b.chain.builders = append(b.chain.builders, func() any { return metricsBuilder{cfg} })
	return b
}

// Chain returns the built chain for use with Apply.
func (b *ChainBuilder) Chain() MiddlewareChain {
	return b.chain
}

// =============================================================================
// MIDDLEWARE IMPLEMENTATIONS (same as before, simplified)
// =============================================================================

type Processor[In, Out any] interface {
	Process(context.Context, In) ([]Out, error)
	Drop(In, error)
}

type Middleware[In, Out any] func(Processor[In, Out]) Processor[In, Out]

func Chain[In, Out any](mw ...Middleware[In, Out]) Middleware[In, Out] {
	return func(next Processor[In, Out]) Processor[In, Out] {
		for i := len(mw) - 1; i >= 0; i-- {
			next = mw[i](next)
		}
		return next
	}
}

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

// --- Recover ---

type RecoverConfig struct {
	OnPanic func(input any, panicValue any, stack string)
}

func Recover[In, Out any](cfg RecoverConfig) Middleware[In, Out] {
	return func(next Processor[In, Out]) Processor[In, Out] {
		return &processor[In, Out]{
			process: func(ctx context.Context, in In) (out []Out, err error) {
				defer func() {
					if r := recover(); r != nil {
						if cfg.OnPanic != nil {
							cfg.OnPanic(in, r, string(debug.Stack()))
						}
						err = fmt.Errorf("panic: %v", r)
					}
				}()
				return next.Process(ctx, in)
			},
			drop: next.Drop,
		}
	}
}

// --- Timeout ---

type TimeoutConfig struct {
	Duration time.Duration
}

func Timeout[In, Out any](cfg TimeoutConfig) Middleware[In, Out] {
	if cfg.Duration <= 0 {
		return func(next Processor[In, Out]) Processor[In, Out] { return next }
	}
	return func(next Processor[In, Out]) Processor[In, Out] {
		return &processor[In, Out]{
			process: func(ctx context.Context, in In) ([]Out, error) {
				ctx, cancel := context.WithTimeout(ctx, cfg.Duration)
				defer cancel()
				return next.Process(ctx, in)
			},
			drop: next.Drop,
		}
	}
}

// --- Retry ---

type RetryConfig struct {
	MaxAttempts int
	Backoff     func(attempt int) time.Duration
	ShouldRetry func(error) bool
}

func Retry[In, Out any](cfg RetryConfig) Middleware[In, Out] {
	if cfg.MaxAttempts == 0 {
		cfg.MaxAttempts = 3
	}
	if cfg.Backoff == nil {
		cfg.Backoff = func(int) time.Duration { return 100 * time.Millisecond }
	}
	if cfg.ShouldRetry == nil {
		cfg.ShouldRetry = func(error) bool { return true }
	}
	return func(next Processor[In, Out]) Processor[In, Out] {
		return &processor[In, Out]{
			process: func(ctx context.Context, in In) ([]Out, error) {
				var lastErr error
				for attempt := 1; attempt <= cfg.MaxAttempts; attempt++ {
					out, err := next.Process(ctx, in)
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
			},
			drop: next.Drop,
		}
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

func Logging[In, Out any](cfg LoggingConfig) Middleware[In, Out] {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	return func(next Processor[In, Out]) Processor[In, Out] {
		return &processor[In, Out]{
			process: func(ctx context.Context, in In) ([]Out, error) {
				start := time.Now()
				out, err := next.Process(ctx, in)
				if err != nil {
					cfg.Logger.Error("failed", "duration", time.Since(start), "error", err)
				} else if cfg.Verbose {
					cfg.Logger.Info("success", "duration", time.Since(start), "outputs", len(out))
				}
				return out, err
			},
			drop: func(in In, err error) {
				if cfg.OnDrop {
					cfg.Logger.Warn("dropped", "type", fmt.Sprintf("%T", in), "error", err)
				}
				next.Drop(in, err)
			},
		}
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

func Metrics[In, Out any](cfg MetricsConfig) Middleware[In, Out] {
	return func(next Processor[In, Out]) Processor[In, Out] {
		return &processor[In, Out]{
			process: func(ctx context.Context, in In) ([]Out, error) {
				start := time.Now()
				out, err := next.Process(ctx, in)
				cfg.Recorder.RecordProcessed(time.Since(start), err == nil)
				return out, err
			},
			drop: func(in In, err error) {
				cfg.Recorder.RecordDropped(errors.Is(err, ErrCanceled))
				next.Drop(in, err)
			},
		}
	}
}

// Helpers
var ErrCanceled = errors.New("canceled")

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

// Domain types - completely different types
type Order struct {
	ID     string
	Amount float64
}

type Payment struct {
	OrderID string
	Method  string
}

type ShippingCmd struct {
	OrderID string
}

type Receipt struct {
	PaymentID string
}

// Simple metrics recorder
type simpleMetrics struct {
	processed atomic.Int64
	succeeded atomic.Int64
	failed    atomic.Int64
	dropped   atomic.Int64
}

func (m *simpleMetrics) RecordProcessed(d time.Duration, success bool) {
	m.processed.Add(1)
	if success {
		m.succeeded.Add(1)
	} else {
		m.failed.Add(1)
	}
}

func (m *simpleMetrics) RecordDropped(isCancel bool) {
	m.dropped.Add(1)
}

func (m *simpleMetrics) Print(name string) {
	fmt.Printf("  %s: processed=%d (ok=%d, fail=%d) dropped=%d\n",
		name, m.processed.Load(), m.succeeded.Load(), m.failed.Load(), m.dropped.Load())
}

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Shared metrics recorders
	orderMetrics := &simpleMetrics{}
	paymentMetrics := &simpleMetrics{}

	// =========================================================================
	// OPTION 1: NewChain (variadic configs)
	// =========================================================================
	// Define ONCE - no type parameters!
	productionChain := NewChain(
		RecoverConfig{
			OnPanic: func(input any, val any, stack string) {
				slog.Error("PANIC", "value", val)
			},
		},
		RetryConfig{
			MaxAttempts: 2,
			Backoff:     ExponentialBackoff(50*time.Millisecond, 2, time.Second),
		},
		TimeoutConfig{
			Duration: 200 * time.Millisecond,
		},
		LoggingConfig{
			Logger: slog.Default(),
			OnDrop: true,
		},
	)

	// =========================================================================
	// OPTION 2: Fluent Builder
	// =========================================================================
	// Alternative syntax - same result
	_ = Build().
		WithRecover(RecoverConfig{}).
		WithRetry(RetryConfig{MaxAttempts: 3}).
		WithTimeout(TimeoutConfig{Duration: time.Second}).
		WithLogging(LoggingConfig{OnDrop: true})

	// =========================================================================
	// USE WITH DIFFERENT TYPES
	// =========================================================================

	// Order processor [Order, ShippingCmd]
	orderProc := Apply[Order, ShippingCmd](productionChain)(&processor[Order, ShippingCmd]{
		process: func(ctx context.Context, order Order) ([]ShippingCmd, error) {
			orderMetrics.RecordProcessed(0, true) // Inline metrics for demo
			if order.Amount < 10 {
				return nil, errors.New("amount too low")
			}
			time.Sleep(30 * time.Millisecond)
			return []ShippingCmd{{OrderID: order.ID}}, nil
		},
		drop: func(Order, error) {},
	})

	// Payment processor [Payment, Receipt] - DIFFERENT TYPES, SAME CHAIN!
	paymentProc := Apply[Payment, Receipt](productionChain)(&processor[Payment, Receipt]{
		process: func(ctx context.Context, payment Payment) ([]Receipt, error) {
			paymentMetrics.RecordProcessed(0, true)
			time.Sleep(20 * time.Millisecond)
			return []Receipt{{PaymentID: payment.OrderID + "-receipt"}}, nil
		},
		drop: func(Payment, error) {},
	})

	// =========================================================================
	// PROCESS ITEMS
	// =========================================================================
	var wg sync.WaitGroup

	// Process orders
	orders := []Order{
		{ID: "ORD-1", Amount: 100},
		{ID: "ORD-2", Amount: 5}, // Will fail validation
		{ID: "ORD-3", Amount: 200},
	}

	fmt.Println("=== Processing Orders ===")
	for _, order := range orders {
		wg.Add(1)
		go func(o Order) {
			defer wg.Done()
			cmds, err := orderProc.Process(ctx, o)
			if err != nil {
				fmt.Printf("❌ Order %s failed: %v\n", o.ID, err)
			} else {
				fmt.Printf("✓ Order %s → %d shipping commands\n", o.ID, len(cmds))
			}
		}(order)
	}

	// Process payments
	payments := []Payment{
		{OrderID: "ORD-1", Method: "card"},
		{OrderID: "ORD-3", Method: "paypal"},
	}

	fmt.Println("\n=== Processing Payments ===")
	for _, payment := range payments {
		wg.Add(1)
		go func(p Payment) {
			defer wg.Done()
			receipts, err := paymentProc.Process(ctx, p)
			if err != nil {
				fmt.Printf("❌ Payment %s failed: %v\n", p.OrderID, err)
			} else {
				fmt.Printf("✓ Payment %s → %d receipts\n", p.OrderID, len(receipts))
			}
		}(payment)
	}

	wg.Wait()

	// =========================================================================
	// DEMONSTRATE TYPE INFERENCE
	// =========================================================================
	fmt.Println("\n=== Type Inference Demo ===")
	fmt.Println("Same chain used for:")
	fmt.Println("  - [Order, ShippingCmd]")
	fmt.Println("  - [Payment, Receipt]")
	fmt.Println("Chain defined ONCE without type parameters!")
}
