//go:build ignore

// Example demonstrating shared middleware for message pipelines.
//
// For message-based pipelines, all processors use *message.Message as both
// input and output. This means a single typed middleware chain can be
// defined once and reused across all message processors.
//
// Run with: go run main.go
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"
)

// =============================================================================
// SIMULATED MESSAGE TYPE (normally from github.com/fxsml/gopipe/message)
// =============================================================================

type Message struct {
	ID      string
	Payload []byte
	attrs   map[string]string
}

func NewMessage(id string, payload []byte) *Message {
	return &Message{ID: id, Payload: payload, attrs: make(map[string]string)}
}

func (m *Message) SetAttr(key, value string) { m.attrs[key] = value }
func (m *Message) GetAttr(key string) string { return m.attrs[key] }

// =============================================================================
// CORE TYPES (from gopipe)
// =============================================================================

type Processor[In, Out any] interface {
	Process(context.Context, In) ([]Out, error)
	Drop(In, error)
}

type Middleware[In, Out any] func(Processor[In, Out]) Processor[In, Out]

// Chain combines multiple middleware. First is outermost.
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

// =============================================================================
// MESSAGE MIDDLEWARE TYPE ALIAS
// =============================================================================

// MessageMiddleware is middleware for message processors.
// Since all message processors use *Message → *Message, we can define
// a shared chain once and reuse it.
type MessageMiddleware = Middleware[*Message, *Message]

// MessageProcessor is a processor for messages.
type MessageProcessor = Processor[*Message, *Message]

// =============================================================================
// MIDDLEWARE IMPLEMENTATIONS
// =============================================================================

type RecoverConfig struct {
	OnPanic func(msg *Message, panicValue any, stack string)
}

func Recover(cfg RecoverConfig) MessageMiddleware {
	return func(next MessageProcessor) MessageProcessor {
		return &processor[*Message, *Message]{
			process: func(ctx context.Context, msg *Message) (out []*Message, err error) {
				defer func() {
					if r := recover(); r != nil {
						if cfg.OnPanic != nil {
							cfg.OnPanic(msg, r, string(debug.Stack()))
						}
						err = fmt.Errorf("panic: %v", r)
					}
				}()
				return next.Process(ctx, msg)
			},
			drop: next.Drop,
		}
	}
}

type TimeoutConfig struct {
	Duration time.Duration
}

func Timeout(cfg TimeoutConfig) MessageMiddleware {
	return func(next MessageProcessor) MessageProcessor {
		return &processor[*Message, *Message]{
			process: func(ctx context.Context, msg *Message) ([]*Message, error) {
				ctx, cancel := context.WithTimeout(ctx, cfg.Duration)
				defer cancel()
				return next.Process(ctx, msg)
			},
			drop: next.Drop,
		}
	}
}

type RetryConfig struct {
	MaxAttempts int
	Backoff     func(attempt int) time.Duration
	ShouldRetry func(error) bool
}

func Retry(cfg RetryConfig) MessageMiddleware {
	if cfg.MaxAttempts == 0 {
		cfg.MaxAttempts = 3
	}
	if cfg.Backoff == nil {
		cfg.Backoff = func(int) time.Duration { return 100 * time.Millisecond }
	}
	if cfg.ShouldRetry == nil {
		cfg.ShouldRetry = func(error) bool { return true }
	}
	return func(next MessageProcessor) MessageProcessor {
		return &processor[*Message, *Message]{
			process: func(ctx context.Context, msg *Message) ([]*Message, error) {
				var lastErr error
				for attempt := 1; attempt <= cfg.MaxAttempts; attempt++ {
					out, err := next.Process(ctx, msg)
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

type LoggingConfig struct {
	Logger  *slog.Logger
	OnDrop  bool
	Verbose bool
}

func Logging(cfg LoggingConfig) MessageMiddleware {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	return func(next MessageProcessor) MessageProcessor {
		return &processor[*Message, *Message]{
			process: func(ctx context.Context, msg *Message) ([]*Message, error) {
				start := time.Now()
				out, err := next.Process(ctx, msg)
				if err != nil {
					cfg.Logger.Error("message processing failed",
						"msg_id", msg.ID,
						"duration", time.Since(start),
						"error", err,
					)
				} else if cfg.Verbose {
					cfg.Logger.Debug("message processed",
						"msg_id", msg.ID,
						"duration", time.Since(start),
						"output_count", len(out),
					)
				}
				return out, err
			},
			drop: func(msg *Message, err error) {
				if cfg.OnDrop {
					cfg.Logger.Warn("message dropped",
						"msg_id", msg.ID,
						"error", err,
					)
				}
				next.Drop(msg, err)
			},
		}
	}
}

type MetricsConfig struct {
	Recorder MetricsRecorder
}

type MetricsRecorder interface {
	RecordProcessed(msgID string, duration time.Duration, success bool)
	RecordDropped(msgID string, isCancel bool)
}

func Metrics(cfg MetricsConfig) MessageMiddleware {
	return func(next MessageProcessor) MessageProcessor {
		return &processor[*Message, *Message]{
			process: func(ctx context.Context, msg *Message) ([]*Message, error) {
				start := time.Now()
				out, err := next.Process(ctx, msg)
				cfg.Recorder.RecordProcessed(msg.ID, time.Since(start), err == nil)
				return out, err
			},
			drop: func(msg *Message, err error) {
				cfg.Recorder.RecordDropped(msg.ID, errors.Is(err, context.Canceled))
				next.Drop(msg, err)
			},
		}
	}
}

// =============================================================================
// EXAMPLE USAGE
// =============================================================================

// SimpleMetrics implements MetricsRecorder
type SimpleMetrics struct {
	processed, succeeded, failed, dropped atomic.Int64
}

func (m *SimpleMetrics) RecordProcessed(msgID string, d time.Duration, success bool) {
	m.processed.Add(1)
	if success {
		m.succeeded.Add(1)
	} else {
		m.failed.Add(1)
	}
}

func (m *SimpleMetrics) RecordDropped(msgID string, isCancel bool) {
	m.dropped.Add(1)
}

func main() {
	ctx := context.Background()
	metrics := &SimpleMetrics{}

	// =========================================================================
	// SHARED MIDDLEWARE CHAIN
	// =========================================================================
	// Define ONCE - works for ALL message processors because they all use
	// *Message → *Message
	productionMiddleware := Chain(
		// Outermost: panic recovery
		Recover(RecoverConfig{
			OnPanic: func(msg *Message, val any, stack string) {
				slog.Error("PANIC in message processing", "msg_id", msg.ID, "panic", val)
			},
		}),
		// Logging
		Logging(LoggingConfig{
			Logger:  slog.Default(),
			OnDrop:  true,
			Verbose: false,
		}),
		// Metrics
		Metrics(MetricsConfig{
			Recorder: metrics,
		}),
		// Timeout
		Timeout(TimeoutConfig{
			Duration: 5 * time.Second,
		}),
		// Innermost: retry
		Retry(RetryConfig{
			MaxAttempts: 2,
			Backoff:     func(attempt int) time.Duration { return 50 * time.Millisecond },
		}),
	)

	// =========================================================================
	// PROCESSOR 1: Order handler
	// =========================================================================
	orderHandler := productionMiddleware(&processor[*Message, *Message]{
		process: func(ctx context.Context, msg *Message) ([]*Message, error) {
			// Simulate order processing
			if msg.GetAttr("amount") == "0" {
				return nil, errors.New("invalid order amount")
			}
			// Create shipping command
			result := NewMessage(msg.ID+"-ship", []byte("ship order"))
			result.SetAttr("order_id", msg.ID)
			return []*Message{result}, nil
		},
		drop: func(msg *Message, err error) {},
	})

	// =========================================================================
	// PROCESSOR 2: Payment handler (SAME middleware chain!)
	// =========================================================================
	paymentHandler := productionMiddleware(&processor[*Message, *Message]{
		process: func(ctx context.Context, msg *Message) ([]*Message, error) {
			// Simulate payment processing
			result := NewMessage(msg.ID+"-receipt", []byte("payment confirmed"))
			result.SetAttr("payment_id", msg.ID)
			return []*Message{result}, nil
		},
		drop: func(msg *Message, err error) {},
	})

	// =========================================================================
	// PROCESSOR 3: Notification handler (SAME middleware chain!)
	// =========================================================================
	notificationHandler := productionMiddleware(&processor[*Message, *Message]{
		process: func(ctx context.Context, msg *Message) ([]*Message, error) {
			// Simulate notification
			fmt.Printf("📧 Notification sent for: %s\n", msg.ID)
			return nil, nil // Sink - no output
		},
		drop: func(msg *Message, err error) {},
	})

	// =========================================================================
	// PROCESS MESSAGES
	// =========================================================================
	var wg sync.WaitGroup

	// Orders
	fmt.Println("=== Processing Orders ===")
	orders := []*Message{
		NewMessage("order-1", []byte(`{"item": "book"}`)),
		NewMessage("order-2", []byte(`{"item": "laptop"}`)),
		NewMessage("order-3", []byte(`{"item": "phone"}`)),
	}
	orders[1].SetAttr("amount", "0") // This will fail

	for _, msg := range orders {
		wg.Add(1)
		go func(m *Message) {
			defer wg.Done()
			results, err := orderHandler.Process(ctx, m)
			if err != nil {
				fmt.Printf("❌ Order %s failed: %v\n", m.ID, err)
			} else {
				fmt.Printf("✓ Order %s → %d results\n", m.ID, len(results))
			}
		}(msg)
	}

	// Payments
	fmt.Println("\n=== Processing Payments ===")
	payments := []*Message{
		NewMessage("payment-1", []byte(`{"amount": 100}`)),
		NewMessage("payment-2", []byte(`{"amount": 200}`)),
	}

	for _, msg := range payments {
		wg.Add(1)
		go func(m *Message) {
			defer wg.Done()
			results, err := paymentHandler.Process(ctx, m)
			if err != nil {
				fmt.Printf("❌ Payment %s failed: %v\n", m.ID, err)
			} else {
				fmt.Printf("✓ Payment %s → %d results\n", m.ID, len(results))
			}
		}(msg)
	}

	// Notifications
	fmt.Println("\n=== Sending Notifications ===")
	notifications := []*Message{
		NewMessage("notif-1", []byte("Welcome!")),
		NewMessage("notif-2", []byte("Order shipped!")),
	}

	for _, msg := range notifications {
		wg.Add(1)
		go func(m *Message) {
			defer wg.Done()
			_, err := notificationHandler.Process(ctx, m)
			if err != nil {
				fmt.Printf("❌ Notification %s failed: %v\n", m.ID, err)
			}
		}(msg)
	}

	wg.Wait()

	// =========================================================================
	// SUMMARY
	// =========================================================================
	fmt.Println("\n=== Summary ===")
	fmt.Printf("Processed: %d (ok=%d, fail=%d), Dropped: %d\n",
		metrics.processed.Load(),
		metrics.succeeded.Load(),
		metrics.failed.Load(),
		metrics.dropped.Load(),
	)
	fmt.Println("\nKey points:")
	fmt.Println("  - ONE middleware chain defined")
	fmt.Println("  - Used by THREE different handlers (order, payment, notification)")
	fmt.Println("  - All use *Message → *Message, so chain is reusable")
}
