# Cancel Path Refactoring Analysis

**Date:** 2025-12-16
**Status:** Analysis
**Supersedes:** Task 0.2 in layer-0-foundation-cleanup.md

## Executive Summary

This document critically evaluates refactoring options for gopipe's cancel path. The goal is to balance idiomatic Go (explicit over implicit), ease of use, and production-readiness while maintaining resilience against deadlocks.

**Key Insight:** The cancel goroutine is a resilience mechanism that prevents deadlocks when context is canceled while input channel still has items. This must be preserved.

## Current Architecture

### Cancel Goroutine in StartProcessor

```go
// processor.go:151-159
wgCancel := sync.WaitGroup{}
wgCancel.Add(1)
go func() {
    <-ctx.Done()
    for val := range in {
        proc.Cancel(val, newErrCancel(ctx.Err()))
    }
    wgCancel.Done()
}()
```

**Purpose:**
1. **Resilience:** Drains remaining items when context is canceled
2. **Deadlock prevention:** Input channel consumers don't block indefinitely
3. **Observability:** Enables logging/metrics of canceled items

### Current Processor Interface

```go
type Processor[In, Out any] interface {
    Process(context.Context, In) ([]Out, error)
    Cancel(In, error)
}
```

### Current Pipe Constructors

```go
// All pipes pass nil for cancel, getting a no-op
proc := NewProcessor(handle, nil)  // cancel defaults to no-op
```

**Problem:** The explicit cancel path is masked - users don't see it, can't intercept it, and canceled items are silently discarded.

### Current Options

```go
// WithCancel adds additional handlers, but still requires knowing about it
gopipe.WithCancel[In, Out](func(in In, err error) {
    log.Printf("canceled: %v", err)
})
```

---

## Options Analysis

### Option 1: Remove Cancel from Processor Interface

**Approach:** Simplify Processor to single-method interface.

```go
type Processor[In, Out any] interface {
    Process(context.Context, In) ([]Out, error)
}

// ProcessFunc implements Processor
type ProcessFunc[In, Out any] func(context.Context, In) ([]Out, error)

// StartProcessor handles errors via config callback
func StartProcessor[In, Out any](
    ctx context.Context,
    in <-chan In,
    proc Processor[In, Out],
    config ProcessorConfig,  // includes OnError callback
) <-chan Out
```

**Config with unified error handling:**
```go
type ProcessorConfig struct {
    Concurrency int
    Buffer      int
    OnError     func(input any, err error, isCancel bool)  // unified
}
```

**Pros:**
- Simplest Processor interface
- Easiest middleware implementation (one method to wrap)
- Clear separation: Processor processes, config handles errors

**Cons:**
- No middleware interception of cancel path
- Can't differentiate cancel vs failure in middleware layer
- Loses explicit cancel handling pattern
- `OnError` callback with `any` loses type safety

**Middleware Impact:**
```go
// Simple - only wraps Process
func WithRetry[In, Out any](cfg RetryConfig) Middleware[In, Out] {
    return func(next Processor[In, Out]) Processor[In, Out] {
        return ProcessFunc[In, Out](func(ctx context.Context, in In) ([]Out, error) {
            // retry logic
            return next.Process(ctx, in)
        })
    }
}
```

**Rating:** ⭐⭐ - Too simplified, loses observability

---

### Option 2: Explicit Cancel Parameter in Pipe Constructors

**Approach:** Add optional cancel function as explicit parameter.

```go
// New API - cancel is explicit but optional (can be nil)
func NewProcessPipe[In, Out any](
    handle func(context.Context, In) ([]Out, error),
    cancel func(In, error),  // explicit, nil = no-op
    opts ...Option[In, Out],
) Pipe[In, Out]
```

**Usage:**
```go
// Simple case - no cancel handling
pipe := NewProcessPipe(handler, nil)

// With cancel logging
pipe := NewProcessPipe(handler, func(in Order, err error) {
    log.Printf("order %s canceled: %v", in.ID, err)
})

// With metrics
pipe := NewProcessPipe(handler, func(in Order, err error) {
    metrics.CanceledTotal.Inc()
})
```

**Pros:**
- Explicit (idiomatic Go)
- Type-safe cancel handler (knows `In` type)
- Clear separation: handler for success, cancel for failure
- No hidden behavior

**Cons:**
- Extra parameter in all pipe constructors
- Logger as parameter violates separation of concerns
- Users must remember to add cancel handler for observability

**Middleware Impact:** Unchanged - middleware wraps both Process and Cancel.

**Rating:** ⭐⭐⭐ - Good but logging as parameter feels wrong

---

### Option 3: ProcessorConfig with OnDrop Callback

**Approach:** Move drop handling to ProcessorConfig struct with unified callback.

```go
type ProcessorConfig struct {
    Concurrency int
    Buffer      int

    // Unified drop handler (both errors and cancellations)
    OnDrop func(input any, err error)
}

// Pipe constructors unchanged
func NewProcessPipe[In, Out any](
    handle func(context.Context, In) ([]Out, error),
    config ProcessorConfig,  // non-generic config
    middleware ...Middleware[In, Out],
) Pipe[In, Out]
```

**Usage:**
```go
pipe := NewProcessPipe(handler, ProcessorConfig{
    Concurrency: 4,
    OnDrop: func(input any, err error) {
        if errors.Is(err, ErrCancel) {
            log.Printf("canceled: %v", err)
        } else {
            log.Printf("failed: %v", err)
        }
    },
})
```

**Pros:**
- Convention over configuration (nil = no-op)
- Non-generic config can be reused
- Single handler simplifies API
- Aligns with messaging where everything is `*Message`

**Cons:**
- Loses type safety (`any` requires assertion)
- Drop handling separate from middleware chain
- Can't compose drop handlers via middleware

**Middleware Impact:** Middleware only wraps Process. Drop handling is separate.

**Rating:** ⭐⭐⭐ - Good balance but loses type safety

---

### Option 4: Keep Current Interface, Add Cancel Middleware

**Approach:** Keep Processor.Cancel(), provide middleware for common cases.

```go
// Processor unchanged
type Processor[In, Out any] interface {
    Process(context.Context, In) ([]Out, error)
    Cancel(In, error)
}

// New middleware for cancel handling
func WithCancelHandler[In, Out any](
    handler func(In, error),
) Middleware[In, Out] {
    return func(next Processor[In, Out]) Processor[In, Out] {
        return NewProcessor(
            next.Process,
            func(in In, err error) {
                handler(in, err)
                next.Cancel(in, err)
            },
        )
    }
}

// Convenience middleware
func WithCancelLogging[In, Out any](logger Logger) Middleware[In, Out]
func WithCancelMetrics[In, Out any](collector MetricsCollector) Middleware[In, Out]
```

**Usage:**
```go
pipe := NewProcessPipe(handler, ProcessorConfig{Concurrency: 4},
    WithCancelLogging[Order, ShippingCmd](logger),
    WithCancelMetrics[Order, ShippingCmd](metrics),
)
```

**Pros:**
- Type-safe (middleware knows types)
- Composable (chain multiple handlers)
- Explicit middleware application
- Reusable middleware implementations

**Cons:**
- Requires understanding middleware for basic logging
- More verbose for simple cases
- Generic parameters on middleware

**Middleware Impact:** Full - both Process and Cancel wrapped.

**Rating:** ⭐⭐⭐⭐ - Powerful but verbose

---

### Option 5: Hybrid - Explicit Drop + Config Default (RECOMMENDED)

**Approach:** Combine explicit drop handler in constructor with config fallback.

```go
// Processor keeps Cancel method (consider renaming to Drop)
type Processor[In, Out any] interface {
    Process(context.Context, In) ([]Out, error)
    Cancel(In, error)  // Handles both errors and cancellations
}

// ProcessorConfig provides defaults
type ProcessorConfig struct {
    Concurrency int
    Buffer      int

    // Unified drop handler (fallback if no explicit handler provided)
    // Handles BOTH errors and cancellations - error type distinguishes cause
    OnDrop func(input any, err error)
}

// Pipe constructors: explicit drop parameter
func NewProcessPipe[In, Out any](
    handle func(context.Context, In) ([]Out, error),
    drop func(In, error),  // nil = use config.OnDrop or no-op
    config ProcessorConfig,
    middleware ...Middleware[In, Out],
) Pipe[In, Out]
```

**Usage Examples:**

```go
// 1. Simple - no drop handling (silent discard)
pipe := NewProcessPipe(handler, nil, ProcessorConfig{})

// 2. Type-safe drop handler
pipe := NewProcessPipe(handler, func(order Order, err error) {
    log.Printf("order %s dropped: %v", order.ID, err)
}, ProcessorConfig{})

// 3. Shared config fallback (non-generic, reusable)
sharedConfig := ProcessorConfig{
    Concurrency: 4,
    OnDrop: func(input any, err error) {
        if errors.Is(err, ErrCancel) {
            slog.Warn("canceled", "error", err)
        } else {
            slog.Error("failed", "error", err)
        }
    },
}
orderPipe := NewProcessPipe(orderHandler, nil, sharedConfig)
paymentPipe := NewProcessPipe(paymentHandler, nil, sharedConfig)  // Same config!

// 4. With middleware (advanced)
pipe := NewProcessPipe(handler, nil, sharedConfig,
    middleware.WithDropMetrics[Order, ShippingCmd](collector),
)
```

**Pros:**
- Explicit (drop parameter visible in signature)
- Convention over configuration (nil = use defaults)
- Type-safe when explicit handler provided
- Fallback to config for generic handling (shared across types)
- Single handler simplifies mental model
- Middleware for advanced composition

**Cons:**
- Three-level fallback (explicit > config > no-op) adds complexity
- Extra parameter in constructors

**Rating:** ⭐⭐⭐⭐⭐ - Best balance of explicitness, safety, and pragmatism

---

### Option 6: Default Cancel Middleware in Pipes

**Approach:** Pipes automatically apply a cancel logging middleware.

```go
func NewProcessPipe[In, Out any](
    handle func(context.Context, In) ([]Out, error),
    config ProcessorConfig,
    middleware ...Middleware[In, Out],
) Pipe[In, Out] {
    // Default cancel logging middleware prepended
    allMiddleware := append(
        []Middleware[In, Out]{defaultCancelLogMiddleware[In, Out]()},
        middleware...,
    )
    // ...
}
```

**Pros:**
- Zero-config observability
- Users don't need to think about cancel handling

**Cons:**
- Implicit behavior (not idiomatic Go)
- Opinionated about logging
- Hard to disable
- Couples pipe to logging

**Rating:** ⭐⭐ - Too implicit, violates Go conventions

---

## Middleware Implementation Comparison

### With Cancel on Processor (Options 2, 4, 5)

```go
func WithTimeout[In, Out any](d time.Duration) Middleware[In, Out] {
    return func(next Processor[In, Out]) Processor[In, Out] {
        return NewProcessor(
            func(ctx context.Context, in In) ([]Out, error) {
                ctx, cancel := context.WithTimeout(ctx, d)
                defer cancel()
                return next.Process(ctx, in)
            },
            func(in In, err error) {
                // Can wrap cancel too if needed
                next.Cancel(in, err)
            },
        )
    }
}
```

### Without Cancel on Processor (Options 1, 3)

```go
func WithTimeout[In, Out any](d time.Duration) Middleware[In, Out] {
    return func(next Processor[In, Out]) Processor[In, Out] {
        return ProcessFunc[In, Out](func(ctx context.Context, in In) ([]Out, error) {
            ctx, cancel := context.WithTimeout(ctx, d)
            defer cancel()
            return next.Process(ctx, in)
        })
    }
}
```

**Observation:** Without Cancel on Processor, middleware is simpler. However, we lose the ability to intercept cancellation in middleware (e.g., for metrics, logging, or error routing).

---

## Impact on Subsequent Tasks

### Task 0.3: Middleware Package Consolidation

| Option | Middleware Complexity | Cancel Middleware Needed |
|--------|----------------------|--------------------------|
| 1 (Remove Cancel) | Simple | No |
| 2 (Explicit Param) | Moderate | No (handled at pipe level) |
| 3 (Config Callback) | Simple | No |
| 4 (Cancel Middleware) | Full | Yes (core feature) |
| 5 (Hybrid) | Moderate | Optional (for advanced cases) |
| 6 (Default Middleware) | Moderate | Yes (implicit) |

**Recommendation for Middleware Package:**

With **Option 5**, the middleware package should include:

```
middleware/
├── middleware.go       # Core types, Chain()
├── timeout.go          # Context timeout
├── retry.go            # Retry logic
├── metrics.go          # Metrics collection
├── logging.go          # Logging (Process only)
├── recover.go          # Panic recovery
├── cancel/             # Cancel-specific (optional)
│   ├── logging.go      # WithCancelLogging
│   ├── metrics.go      # WithCancelMetrics
│   └── routing.go      # WithCancelRouting (for DLQ)
└── message/            # Message-specific
    ├── correlation.go
    └── validation.go
```

---

## Recommendation: Option 5 (Hybrid)

### Rationale

1. **Explicit is idiomatic:** Go prefers explicit over implicit. The cancel parameter makes the cancel path visible.

2. **Convention over configuration:** `nil` cancel means "use defaults" - simple cases remain simple.

3. **Type safety when it matters:** Explicit handler is type-safe. Config fallback accepts `any` for generic cases.

4. **Separation of concerns:** Logging should be middleware, not built into pipes. But having a fallback callback in config allows simple cases without middleware.

5. **Resilience preserved:** The cancel goroutine remains, preventing deadlocks.

6. **Production-ready:** Advanced users can compose cancel handlers via middleware.

### Recommended API

```go
// processor.go - Processor interface unchanged
type Processor[In, Out any] interface {
    Process(context.Context, In) ([]Out, error)
    Cancel(In, error)  // Consider renaming to Drop()
}

// errors.go - Sentinel errors distinguish cause
var (
    ErrCancel  = errors.New("gopipe: canceled")   // Item drained after ctx.Done()
    ErrFailure = errors.New("gopipe: processing failed")  // Process() returned error
)

// processor_config.go - Non-generic config
type ProcessorConfig struct {
    // Worker pool
    Concurrency int  // Default: 1
    Buffer      int  // Default: 0

    // Unified drop handler (handles BOTH errors and cancellations)
    // The error type distinguishes the cause:
    //   - errors.Is(err, ErrCancel)  -> context was canceled
    //   - errors.Is(err, ErrFailure) -> Process() returned error
    OnDrop func(input any, err error)

    // Cleanup
    CleanupTimeout time.Duration
}

// Default config
var DefaultProcessorConfig = ProcessorConfig{
    Concurrency:    1,
    CleanupTimeout: 30 * time.Second,
}

// pipe.go - Explicit drop handler parameter
func NewProcessPipe[In, Out any](
    process func(context.Context, In) ([]Out, error),
    drop func(In, error),  // nil = use config.OnDrop or no-op
    config ProcessorConfig,
    middleware ...Middleware[In, Out],
) Pipe[In, Out]

func NewTransformPipe[In, Out any](
    transform func(context.Context, In) (Out, error),
    drop func(In, error),
    config ProcessorConfig,
    middleware ...Middleware[In, Out],
) Pipe[In, Out]

// Similar for NewFilterPipe, NewSinkPipe, NewBatchPipe
```

### Why Single Handler (`OnDrop`) Instead of Separate Handlers?

1. **Existing convention**: gopipe already uses `Cancel()` for both error paths
2. **Error type distinguishes cause**: `errors.Is(err, ErrCancel)` vs `errors.Is(err, ErrFailure)`
3. **Simpler API**: One callback instead of two
4. **DRY**: Common logic (logging, metrics) doesn't need duplication

### Naming Consideration

The current name `Cancel` is confusing because it handles both cancellations AND errors. Options:

| Name | Pros | Cons |
|------|------|------|
| `Cancel` | Existing convention | Misleading (handles errors too) |
| `OnDrop` | Clear intent (item dropped) | New term to learn |
| `OnReject` | Implies validation | Doesn't fit cancellation |
| `OnDiscard` | Clear | Verbose |

**Recommendation:** Rename to `Drop` / `OnDrop` - it accurately describes what happens: the item is dropped from the pipeline, regardless of reason.

---

## Working Example

A complete runnable example is available at: `docs/plans/cancel-path-refactoring/main.go`

### Key Concept: Single Drop Handler

The key insight is that **one handler handles both errors and cancellations**. The error type distinguishes the cause:

```go
// ProcessorConfig with unified OnDrop handler
sharedConfig := ProcessorConfig{
    Concurrency: 2,
    Buffer:      10,

    // OnDrop handles BOTH errors AND cancellations
    OnDrop: func(input any, err error) {
        if errors.Is(err, ErrCancel) {
            // Item was in channel when context canceled (drained)
            slog.Warn("item canceled", "type", fmt.Sprintf("%T", input), "error", err)
        } else if errors.Is(err, ErrFailure) {
            // Process() returned an error
            slog.Error("item failed", "type", fmt.Sprintf("%T", input), "error", err)
        }
    },
}
```

### Example 1: Shared Config Across Multiple Processors

```go
// PIPE 1: Order -> ShippingCommand (uses OnDrop fallback)
orderPipe := NewProcessPipe(
    func(ctx context.Context, order Order) ([]ShippingCommand, error) {
        if order.Amount < 50 {
            return nil, fmt.Errorf("amount too low: %.2f", order.Amount)
        }
        return []ShippingCommand{{OrderID: order.ID}}, nil
    },
    nil,          // No explicit handler -> uses sharedConfig.OnDrop
    sharedConfig, // Reused!
)

// PIPE 2: Payment -> Notification (uses same OnDrop fallback)
paymentPipe := NewProcessPipe(
    func(ctx context.Context, payment Payment) ([]Notification, error) {
        return []Notification{{Message: payment.Status}}, nil
    },
    nil,          // No explicit handler -> uses sharedConfig.OnDrop
    sharedConfig, // Same config reused!
)

// PIPE 3: Order -> ShippingCommand (with EXPLICIT type-safe handler)
criticalOrderPipe := NewProcessPipe(
    func(ctx context.Context, order Order) ([]ShippingCommand, error) {
        return []ShippingCommand{{OrderID: order.ID}}, nil
    },
    func(order Order, err error) {
        // TYPE-SAFE: Can access order.ID, order.Amount directly
        slog.Error("CRITICAL: VIP order dropped",
            "orderID", order.ID,
            "amount", order.Amount,
            "isCancel", errors.Is(err, ErrCancel),
            "error", err,
        )
    },
    sharedConfig, // Still uses config for Concurrency, Buffer
)
```

### Example Output

```
✓ Shipping: {OrderID:ORD-1 Address:123 Main St}
✓ Critical: {OrderID:VIP-1 Address:VIP Address}
✓ Notification: {UserID:user-123 Message:Payment ORD-1: completed}

// Validation error (ErrFailure):
ERROR item failed type=main.Order input="{ID:ORD-2 Amount:25}" error="gopipe: processing failed: amount too low: 25.00"

// Context deadline exceeded during processing (ErrFailure):
ERROR item failed type=main.Order input="{ID:ORD-5 Amount:400}" error="gopipe: processing failed: context deadline exceeded"

// Items drained from channel after ctx.Done() (ErrCancel):
WARN item canceled type=main.Order input="{ID:ORD-7 Amount:600}" error="gopipe: canceled: context deadline exceeded"
WARN item canceled type=main.Order input="{ID:ORD-8 Amount:700}" error="gopipe: canceled: context deadline exceeded"

=== Metrics ===
Total dropped:    6
  - Canceled:     3  (items drained after ctx.Done)
  - Failed:       3  (Process returned error)
```

### Example 2: Fallback Resolution Order

```go
// Resolution order for drop handling:
//
// 1. Explicit drop parameter (type-safe)
// 2. Config.OnDrop fallback (any)
// 3. No-op (silent discard)

// Case 1: Explicit handler takes precedence
pipe := NewProcessPipe(handler,
    func(in Order, err error) { /* USED */ },
    ProcessorConfig{
        OnDrop: func(in any, err error) { /* IGNORED */ },
    },
)

// Case 2: Config fallback when no explicit handler
pipe := NewProcessPipe(handler,
    nil,  // No explicit handler
    ProcessorConfig{
        OnDrop: func(in any, err error) { /* USED */ },
    },
)

// Case 3: Silent discard (backwards compatible, not recommended)
pipe := NewProcessPipe(handler,
    nil,
    ProcessorConfig{}, // No OnDrop either -> no-op
)
```

### Example 3: Production Setup with Middleware

```go
// For production: combine shared config with type-safe middleware

sharedConfig := ProcessorConfig{
    Concurrency: 8,
    Buffer:      1000,
    OnDrop: func(input any, err error) {
        // Fallback: generic structured logging
        slog.Warn("dropped", "type", fmt.Sprintf("%T", input), "error", err)
    },
}

// High-value orders get additional type-safe handling via middleware
criticalPipe := NewProcessPipe(
    orderHandler,
    nil,  // Use config fallback for basic logging
    sharedConfig,
    // Middleware adds type-safe metrics on top
    WithDropHandler[Order, ShippingCommand](func(order Order, err error) {
        if order.Amount > 1000 {
            alerting.SendPagerDuty("High-value order dropped: " + order.ID)
        }
        metrics.DroppedOrderValue.Add(order.Amount)
    }),
)
```

### Key Points

1. **Single handler**: `OnDrop` handles both errors and cancellations
2. **Error type distinguishes cause**: `errors.Is(err, ErrCancel)` vs `errors.Is(err, ErrFailure)`
3. **Non-generic config**: Same `ProcessorConfig` works for any type
4. **Type-safe when needed**: Explicit handler parameter gets the actual type
5. **Shared observability**: All pipes use the same drop handling logic

---

## Migration Path

### From Current API

```go
// Current (verbose, generic options)
pipe := gopipe.NewProcessPipe(
    handler,
    gopipe.WithConcurrency[Order, ShippingCmd](4),
    gopipe.WithCancel[Order, ShippingCmd](func(in Order, err error) {
        log.Printf("dropped: %v", err)
    }),
)

// New (Option 5 - explicit drop handler, non-generic config)
pipe := gopipe.NewProcessPipe(
    handler,
    func(in Order, err error) {  // Type-safe drop handler
        log.Printf("dropped: %v", err)
    },
    gopipe.ProcessorConfig{Concurrency: 4},
)

// Or with shared config fallback
pipe := gopipe.NewProcessPipe(
    handler,
    nil,  // Use config.OnDrop
    sharedConfig,
)
```

### Backward Compatibility

```go
// deprecated.go

// Deprecated: Use NewProcessPipe with ProcessorConfig instead.
func NewProcessPipeWithOptions[In, Out any](
    handle func(context.Context, In) ([]Out, error),
    opts ...Option[In, Out],
) Pipe[In, Out] {
    config := parseConfigFromOptions(opts)
    return NewProcessPipe(handle, nil, config)
}
```

---

## Decision Matrix

| Criteria | Opt 1 | Opt 2 | Opt 3 | Opt 4 | Opt 5 | Opt 6 |
|----------|-------|-------|-------|-------|-------|-------|
| Idiomatic Go (explicit) | ⚪ | ✅ | ⚪ | ✅ | ✅ | ❌ |
| Type safety | ❌ | ✅ | ❌ | ✅ | ✅ | ⚪ |
| Ease of use | ✅ | ⚪ | ✅ | ⚪ | ✅ | ✅ |
| Middleware composability | ❌ | ⚪ | ❌ | ✅ | ✅ | ⚪ |
| Separation of concerns | ⚪ | ⚪ | ⚪ | ✅ | ✅ | ❌ |
| Production readiness | ⚪ | ✅ | ⚪ | ✅ | ✅ | ⚪ |

**Legend:** ✅ Strong | ⚪ Moderate | ❌ Weak

---

## Conclusion

**Recommendation:** Option 5 (Hybrid - Explicit Cancel + Config Default)

This approach:
1. Preserves the cancel goroutine for resilience
2. Makes cancel handling explicit in pipe constructors
3. Provides type-safe cancel handlers when explicit
4. Falls back to config for generic cases
5. Supports middleware composition for advanced scenarios
6. Separates logging concerns from core pipe logic

The slight increase in API complexity is justified by the clarity, type safety, and flexibility gained.

## Next Steps

1. Update ADR 0026 to reflect this decision
2. Update Task 0.2 in layer-0-foundation-cleanup.md
3. Implement ProcessorConfig struct
4. Update pipe constructors with explicit cancel parameter
5. Create cancel middleware in middleware/cancel/ package
6. Add backward compatibility wrappers
7. Update tests and documentation

## Related Documents

- [ADR 0026: Pipe and Processor Simplification](../adr/0026-pipe-processor-simplification.md)
- [Layer 0: Foundation Cleanup](layer-0-foundation-cleanup.md)
- [Architecture Roadmap](architecture-roadmap.md)
