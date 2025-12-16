// Example demonstrating Option 5: Hybrid approach with shared non-generic config
//
// This example shows the PROPOSED API - it demonstrates what the refactored
// gopipe would look like. The key insight is that a single "drop handler"
// (currently called Cancel in gopipe) handles both:
//   - Processing errors (Process returned error)
//   - Context cancellation (context was canceled)
//
// The error type (ErrCancel vs ErrFailure) distinguishes the cause.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"
)

// =============================================================================
// PROPOSED API TYPES (would live in gopipe package)
// =============================================================================

// ProcessorConfig is non-generic - can be shared across processors of any type.
type ProcessorConfig struct {
	Concurrency int
	Buffer      int

	// OnDrop is called when an item cannot proceed through the pipeline.
	// This handles BOTH processing errors AND context cancellation.
	// The error type distinguishes the cause:
	//   - errors.Is(err, ErrCancel) -> context was canceled
	//   - errors.Is(err, ErrFailure) -> Process() returned an error
	//
	// Receives `any` because config is non-generic and shared across types.
	// This is the FALLBACK when no explicit drop handler is provided.
	OnDrop func(input any, err error)
}

// Sentinel errors to distinguish drop causes
var (
	ErrCancel  = errors.New("gopipe: canceled")
	ErrFailure = errors.New("gopipe: processing failed")
)

// =============================================================================
// PROPOSED PIPE CONSTRUCTOR (simplified for demo)
// =============================================================================

// Pipe represents a processing pipeline
type Pipe[In, Out any] struct {
	process func(context.Context, In) ([]Out, error)
	drop    func(In, error) // explicit, type-safe drop handler (may be nil)
	config  ProcessorConfig
}

// NewProcessPipe creates a pipe with explicit drop handler parameter.
//
// Parameters:
//   - process: the processing function
//   - drop: explicit drop handler (nil = use config.OnDrop fallback)
//   - config: shared, non-generic configuration
func NewProcessPipe[In, Out any](
	process func(context.Context, In) ([]Out, error),
	drop func(In, error), // explicit, may be nil
	config ProcessorConfig,
) *Pipe[In, Out] {
	return &Pipe[In, Out]{
		process: process,
		drop:    drop,
		config:  config,
	}
}

// Start begins processing items from the input channel.
func (p *Pipe[In, Out]) Start(ctx context.Context, in <-chan In) <-chan Out {
	ctx, cancel := context.WithCancel(ctx)

	concurrency := max(p.config.Concurrency, 1)

	out := make(chan Out, p.config.Buffer)

	// Resolve drop handler: explicit > config.OnDrop > no-op
	dropHandler := p.resolveDropHandler()

	var wgProcess sync.WaitGroup
	wgProcess.Add(concurrency)

	for range concurrency {
		go func() {
			defer wgProcess.Done()
			for {
				select {
				case <-ctx.Done():
					return
				default:
				}

				select {
				case <-ctx.Done():
					return
				case val, ok := <-in:
					if !ok {
						return
					}
					results, err := p.process(ctx, val)
					if err != nil {
						// Processing failed - call drop handler with ErrFailure
						dropHandler(val, fmt.Errorf("%w: %w", ErrFailure, err))
					} else {
						for _, r := range results {
							select {
							case out <- r:
							case <-ctx.Done():
								return
							}
						}
					}
				}
			}
		}()
	}

	// Cancel goroutine: drains remaining items on context cancellation
	// This is the RESILIENCE mechanism - prevents deadlocks
	var wgCancel sync.WaitGroup
	wgCancel.Add(1)
	go func() {
		defer wgCancel.Done()
		<-ctx.Done()
		for val := range in {
			// Context canceled - call drop handler with ErrCancel
			dropHandler(val, fmt.Errorf("%w: %w", ErrCancel, ctx.Err()))
		}
	}()

	go func() {
		wgProcess.Wait()
		cancel()
		wgCancel.Wait()
		close(out)
	}()

	return out
}

// resolveDropHandler returns the effective drop handler based on precedence:
// 1. Explicit drop parameter (type-safe)
// 2. Config.OnDrop fallback (any)
// 3. No-op (silent discard)
func (p *Pipe[In, Out]) resolveDropHandler() func(In, error) {
	// Priority 1: Explicit type-safe handler
	if p.drop != nil {
		return p.drop
	}

	// Priority 2: Config fallback (wraps any -> In)
	if p.config.OnDrop != nil {
		return func(in In, err error) {
			p.config.OnDrop(in, err) // in is implicitly converted to any
		}
	}

	// Priority 3: No-op
	return func(In, error) {}
}

// =============================================================================
// DOMAIN TYPES
// =============================================================================

type Order struct {
	ID     string
	Amount float64
}

type Payment struct {
	OrderID string
	Status  string
}

type ShippingCommand struct {
	OrderID string
	Address string
}

type Notification struct {
	UserID  string
	Message string
}

// =============================================================================
// EXAMPLE USAGE
// =============================================================================

func main() {
	// Short timeout to trigger cancellation
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Millisecond)
	defer cancel()

	// =========================================================================
	// SHARED CONFIG: Non-generic, reusable across ALL processors
	// =========================================================================
	sharedConfig := ProcessorConfig{
		Concurrency: 2,
		Buffer:      10,

		// OnDrop handles BOTH errors and cancellations
		// The error type tells you which:
		//   errors.Is(err, ErrCancel)  -> context canceled
		//   errors.Is(err, ErrFailure) -> process returned error
		OnDrop: func(input any, err error) {
			if errors.Is(err, ErrCancel) {
				slog.Warn("item canceled",
					"type", fmt.Sprintf("%T", input),
					"input", input,
					"error", err,
				)
				canceledTotal.Add(1)
			} else if errors.Is(err, ErrFailure) {
				slog.Error("item failed",
					"type", fmt.Sprintf("%T", input),
					"input", input,
					"error", err,
				)
				failedTotal.Add(1)
			}
			droppedTotal.Add(1)
		},
	}

	// =========================================================================
	// PIPE 1: Order -> ShippingCommand (uses OnDrop fallback)
	// =========================================================================
	orderPipe := NewProcessPipe(
		func(ctx context.Context, order Order) ([]ShippingCommand, error) {
			// Simulate work
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(50 * time.Millisecond):
			}

			// Simulate validation error for low amounts
			if order.Amount < 50 {
				return nil, fmt.Errorf("amount too low: %.2f", order.Amount)
			}

			return []ShippingCommand{{
				OrderID: order.ID,
				Address: "123 Main St",
			}}, nil
		},
		nil,          // No explicit handler -> uses sharedConfig.OnDrop
		sharedConfig, // Reused!
	)

	// =========================================================================
	// PIPE 2: Payment -> Notification (uses OnDrop fallback)
	// =========================================================================
	paymentPipe := NewProcessPipe(
		func(ctx context.Context, payment Payment) ([]Notification, error) {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(50 * time.Millisecond):
			}
			return []Notification{{
				UserID:  "user-123",
				Message: fmt.Sprintf("Payment %s: %s", payment.OrderID, payment.Status),
			}}, nil
		},
		nil,          // No explicit handler -> uses sharedConfig.OnDrop
		sharedConfig, // Same config reused!
	)

	// =========================================================================
	// PIPE 3: Order -> ShippingCommand (with EXPLICIT type-safe handler)
	// =========================================================================
	criticalOrderPipe := NewProcessPipe(
		func(ctx context.Context, order Order) ([]ShippingCommand, error) {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(50 * time.Millisecond):
			}
			return []ShippingCommand{{OrderID: order.ID, Address: "VIP Address"}}, nil
		},
		func(order Order, err error) {
			// TYPE-SAFE: We know it's an Order
			// Can access order.ID, order.Amount directly
			slog.Error("CRITICAL: VIP order dropped",
				"orderID", order.ID,
				"amount", order.Amount,
				"isCancel", errors.Is(err, ErrCancel),
				"isFailure", errors.Is(err, ErrFailure),
				"error", err,
			)
			criticalDrops.Add(1)
		},
		sharedConfig, // Still uses config for Concurrency, Buffer
	)

	// =========================================================================
	// CREATE INPUT CHANNELS
	// =========================================================================
	orders := make(chan Order, 20)
	go func() {
		defer close(orders)
		for _, o := range []Order{
			{ID: "ORD-1", Amount: 100},
			{ID: "ORD-2", Amount: 25}, // Will fail validation
			{ID: "ORD-3", Amount: 200},
			{ID: "ORD-4", Amount: 300},
			{ID: "ORD-5", Amount: 400},
			{ID: "ORD-6", Amount: 500},
			{ID: "ORD-7", Amount: 600},
			{ID: "ORD-8", Amount: 700}, // May be canceled (still in channel when ctx done)
			{ID: "ORD-9", Amount: 800}, // May be canceled (still in channel when ctx done)
		} {
			orders <- o // Buffer is large enough, won't block
		}
	}()

	payments := make(chan Payment, 10)
	go func() {
		defer close(payments)
		for _, p := range []Payment{
			{OrderID: "ORD-1", Status: "completed"},
			{OrderID: "ORD-2", Status: "pending"},
			{OrderID: "ORD-3", Status: "completed"}, // May be canceled
		} {
			select {
			case payments <- p:
			case <-ctx.Done():
				return
			}
		}
	}()

	criticalOrders := make(chan Order, 10)
	go func() {
		defer close(criticalOrders)
		for _, o := range []Order{
			{ID: "VIP-1", Amount: 10000},
			{ID: "VIP-2", Amount: 20000}, // May be canceled
		} {
			select {
			case criticalOrders <- o:
			case <-ctx.Done():
				return
			}
		}
	}()

	// =========================================================================
	// START PIPELINES
	// =========================================================================
	shippingCmds := orderPipe.Start(ctx, orders)
	notifications := paymentPipe.Start(ctx, payments)
	criticalShipping := criticalOrderPipe.Start(ctx, criticalOrders)

	// =========================================================================
	// COLLECT RESULTS
	// =========================================================================
	var wg sync.WaitGroup
	wg.Add(3)

	go func() {
		defer wg.Done()
		for cmd := range shippingCmds {
			fmt.Printf("✓ Shipping: %+v\n", cmd)
		}
	}()

	go func() {
		defer wg.Done()
		for notif := range notifications {
			fmt.Printf("✓ Notification: %+v\n", notif)
		}
	}()

	go func() {
		defer wg.Done()
		for cmd := range criticalShipping {
			fmt.Printf("✓ Critical: %+v\n", cmd)
		}
	}()

	wg.Wait()

	// =========================================================================
	// PRINT METRICS
	// =========================================================================
	fmt.Println()
	fmt.Println("=== Metrics ===")
	fmt.Printf("Total dropped:    %d\n", droppedTotal.Load())
	fmt.Printf("  - Canceled:     %d\n", canceledTotal.Load())
	fmt.Printf("  - Failed:       %d\n", failedTotal.Load())
	fmt.Printf("Critical drops:   %d\n", criticalDrops.Load())
}

// Metrics
var (
	droppedTotal  atomic.Int64
	canceledTotal atomic.Int64
	failedTotal   atomic.Int64
	criticalDrops atomic.Int64
)
