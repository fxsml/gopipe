# Named Pools for Per-Handler Concurrency Control

**Status:** Implemented

## Overview

Add named worker pools to the Router, allowing handlers to be assigned to pools with different concurrency limits. This enables resource-constrained handlers (e.g., those calling external APIs) to have lower concurrency than cache-only handlers.

## Problem

Previous state: Router had a single global `Concurrency` setting that applies to ALL handlers.

**Issue:** Different handlers have different resource constraints:
- ERP-bound handlers (slow, rate-limited external API): Need low concurrency (5-10)
- Cache-only handlers (fast, local Redis): Can handle high concurrency (100+)

With global concurrency, you must either:
- Set low concurrency → underutilize cache handlers
- Set high concurrency → overwhelm external APIs

## Why Router, Not Engine?

**Pools are a Router concern, not an Engine concern.**

| Component | Responsibility |
|-----------|---------------|
| **Engine** | Wiring (inputs → processing → outputs), lifecycle management, message tracking |
| **Router** | Handler dispatch, concurrency control, middleware |

Key observations:

1. **Engine doesn't know about handlers.** It calls `router.Pipe(ctx, in)` and receives an output channel. Engine shouldn't need to know which handlers exist or how they're grouped.

2. **Concurrency is already Router's domain.** Router controls worker concurrency today. Pools are a natural extension.

3. **Handler registration is Router's job.** `Router.AddHandler()` already exists. Pool assignment belongs with handler registration.

4. **Clean interface preserved.** Engine continues to call `router.Pipe()` without changes. Pool management is an internal Router detail.

5. **Simpler shutdown.** Router already manages its `ProcessPipe` lifecycle. Multiple pools = multiple ProcessPipes, same pattern.

Adding pools to Engine would couple it to handler details and complicate shutdown coordination with message tracking and loopback handling.

## Solution: Named Pools in Router

Introduce named pools with independent concurrency settings. Handlers are assigned to pools via named methods.

```
Router
  ├── Pool "default" (concurrency: 50)
  │     ├── product-cache-handler
  │     └── session-cache-handler
  │
  └── Pool "erp" (concurrency: 5)
        ├── prices-handler
        ├── inventory-handler
        └── delivery-times-handler
```

## API Design

### PoolConfig

```go
// PoolConfig configures a worker pool.
type PoolConfig struct {
    // Workers is the number of concurrent workers (default: 1).
    Workers int
    // BufferSize is the output channel buffer (0 = inherit from RouterConfig).
    BufferSize int
}
```

### RouterConfig

```go
type RouterConfig struct {
    // BufferSize is the output channel buffer size (default: 100).
    BufferSize int
    // Pool configures the default pool (default: 1 worker).
    Pool PoolConfig
    // ErrorHandler is called on processing errors (default: no-op, errors logged via Logger).
    ErrorHandler ErrorHandler
    // Logger for router events (default: slog.Default()).
    Logger Logger
}
```

The default pool is configured via `RouterConfig.Pool`. This makes default pool configuration explicit and consistent with named pools.

### Router Methods

```go
// AddPoolWithConfig creates a named worker pool.
// Returns error if:
//   - name is empty
//   - name already exists
//   - router already started
func (r *Router) AddPoolWithConfig(name string, cfg PoolConfig) error

// AddHandler registers a handler to the default pool.
// Signature unchanged from current API - full backward compatibility.
func (r *Router) AddHandler(name string, matcher Matcher, h Handler) error

// AddHandlerToPool registers a handler to a named pool.
// First three parameters match AddHandler for symmetry.
// Returns error if:
//   - pool does not exist
//   - handler event type already registered
//   - router already started
func (r *Router) AddHandlerToPool(name string, matcher Matcher, h Handler, pool string) error
```

The "default" pool is created from `RouterConfig.Pool`. It cannot be overridden via `AddPoolWithConfig("default", ...)`.

### Engine Passthrough Methods

Engine exposes pool methods as passthroughs to Router for convenience:

```go
// AddPoolWithConfig creates a named worker pool.
// Must be called before Start().
func (e *Engine) AddPoolWithConfig(name string, cfg PoolConfig) error

// AddHandlerToPool registers a handler to a named pool.
// Must be called before Start().
func (e *Engine) AddHandlerToPool(name string, matcher Matcher, h Handler, pool string) error
```

### EngineConfig

```go
type EngineConfig struct {
    Marshaler Marshaler
    // BufferSize is the engine buffer size for merger and distributor (default: 100).
    BufferSize int
    // RouterBufferSize is the router's internal buffer (0 = inherit BufferSize).
    RouterBufferSize int
    // RouterPool configures the default worker pool (Workers default: 1, BufferSize 0 = inherit RouterBufferSize).
    RouterPool PoolConfig
    ErrorHandler ErrorHandler
    Logger Logger
    ShutdownTimeout time.Duration
}
```

**Inheritance chain:** `BufferSize` → `RouterBufferSize` → `RouterPool.BufferSize`

## Usage Examples

### Using Router Directly

```go
router := message.NewRouter(message.RouterConfig{
    BufferSize: 100,
    Pool: message.PoolConfig{
        Workers: 50,  // Default pool: 50 workers
    },
})

// Create a low-concurrency pool for ERP-bound handlers
router.AddPoolWithConfig("erp", message.PoolConfig{
    Workers: 5,
})

// ERP handlers → "erp" pool (5 concurrent)
router.AddHandlerToPool("prices", nil, pricesHandler, "erp")
router.AddHandlerToPool("inventory", nil, inventoryHandler, "erp")

// Cache handlers → default pool (50 concurrent)
router.AddHandler("product-cache", nil, productCacheHandler)
router.AddHandler("session-cache", nil, sessionCacheHandler)
```

### Using Engine (Recommended)

```go
engine := message.NewEngine(message.EngineConfig{
    BufferSize: 100,
    RouterPool: message.PoolConfig{Workers: 50},
})

// Create pools via Engine passthrough
engine.AddPoolWithConfig("erp", message.PoolConfig{Workers: 5})

// Register handlers via Engine
engine.AddHandlerToPool("prices", nil, pricesHandler, "erp")
engine.AddHandler("product-cache", nil, productCacheHandler)
```

## Internal Architecture

Router internally composes existing pipe package primitives:

| Primitive | Purpose |
|-----------|---------|
| `Distributor` | Routes input to pool channels by event type |
| `ProcessPipe` | Per-pool concurrent processing |
| `Merger` | Combines pool outputs into single output channel |

### Single-Pool Optimization

For the common case of a single pool (no custom pools added), Router uses a simple `ProcessPipe` directly, avoiding Distributor/Merger overhead:

```go
// Single pool: simple ProcessPipe
if !hasMultiplePools {
    return r.pipeSinglePool(ctx, in, fn)
}
// Multiple pools: Distributor + ProcessPipes + Merger
return r.pipeMultiPool(ctx, in, fn)
```

### Multi-Pool Architecture

```
                    ┌───────────────────────────────────────────────────┐
                    │                     Router                         │
                    │                                                    │
in ──► Distributor ─┼──► pool "default" ProcessPipe ──┐                 │
       (by type)    │                                  ├──► Merger ──► out
                    │──► pool "erp" ProcessPipe ──────┘                 │
                    │                                                    │
                    └───────────────────────────────────────────────────┘
```

### Data Structures

```go
type Router struct {
    // ...existing fields...

    pools    map[string]poolEntry   // "default" always exists
    handlers map[string]handlerEntry // eventType → handler
}

type poolEntry struct {
    cfg PoolConfig
}

type handlerEntry struct {
    name    string
    matcher Matcher
    handler Handler
    pool    string  // pool assignment
}
```

### Why Pipe Primitives?

1. **Proven & tested** - Distributor, ProcessPipe, Merger handle edge cases
2. **Shutdown is automatic** - Each primitive manages its own graceful drain
3. **Autoscale-ready** - ProcessPipe already supports `Config.Autoscale`
4. **No custom worker management** - ProcessPipe handles all concurrency
5. **Composition over custom code** - Router just wires primitives together

## Shutdown Flow

Shutdown leverages existing pipe primitives - no custom coordination:

```
1. ctx.Done() triggered
   │
2. Distributor stops reading, closes pool output channels
   │
3. Each ProcessPipe drains buffered messages
   │
4. ProcessPipes close their outputs when done
   │
5. Merger collects final outputs, closes when all inputs closed
   │
6. Router.Pipe() output closes
```

Each component handles its own graceful shutdown. The composition "just works".

## Error Handling

| Method | Error Condition | Error |
|--------|-----------------|-------|
| `AddPoolWithConfig` | Empty name | `ErrPoolNameEmpty` |
| `AddPoolWithConfig` | Name exists | `ErrPoolExists` |
| `AddPoolWithConfig` | Router started | `ErrAlreadyStarted` |
| `AddHandlerToPool` | Pool not found | `ErrPoolNotFound` |
| `AddHandlerToPool` | Type registered | `ErrHandlerExists` |
| `AddHandlerToPool` | Router started | `ErrAlreadyStarted` |

## Files Modified

| File | Changes |
|------|---------|
| `message/router.go` | Add pools map, PoolConfig, AddPoolWithConfig(), AddHandlerToPool(), pipeSinglePool(), pipeMultiPool() |
| `message/errors.go` | Add ErrPoolNameEmpty, ErrPoolExists, ErrPoolNotFound, ErrHandlerExists |
| `message/engine.go` | Add passthrough methods, EngineConfig.RouterBufferSize/RouterPool |
| `message/router_test.go` | Pool unit tests, multi-pool routing tests, concurrency verification tests |
| `message/engine_test.go` | TestEngine_RouterPool |
| `message/engine_bench_test.go` | Fix MultiStep benchmark to use unique event types |

## Testing

### Unit Tests

- `TestRouter_AddPoolWithConfig` - validation (empty name, duplicates, after started)
- `TestRouter_AddHandlerToPool` - validation (pool exists, type not registered)
- `TestRouter_PoolConfig_Defaults` - default values

### Integration Tests

- `TestRouter_MultiPool` - messages route to correct pool by event type

### Concurrency Tests

- `TestRouter_MultiPool_Concurrency` - verifies pools with different worker counts execute with different concurrency levels

## Future Enhancements (Out of Scope)

- **Autoscaling per pool**: Add `MaxWorkers` field for dynamic scaling
- **Pool metrics**: Expose active/total workers per pool
- **Dynamic pool resize**: Change concurrency at runtime
- **Pool priorities**: Higher priority pools get resources first

## Backward Compatibility

Fully backward compatible:
- `AddHandler()` unchanged, uses default pool
- `EngineConfig.RouterPool` replaces `EngineConfig.Concurrency`
- Existing code works with minimal migration (`Concurrency: N` → `RouterPool: PoolConfig{Workers: N}`)
- Engine interface unchanged (passthrough methods are additions)

---

## Design Review Notes

### Bottleneck Analysis

**Distributor:** Routes all messages through single goroutine.
- Risk: Could bottleneck under very high throughput
- Mitigation: Only does map lookup + channel send (O(1))
- Verdict: **Low risk** - if proven problematic, can shard by adding multiple distributors

**Merger:** Collects outputs from all pools.
- Risk: Contention on merged output channel
- Mitigation: Existing `Merger` uses per-input goroutines
- Verdict: **Low risk** - proven pattern

### Alternative Considered: Engine-Based Pools

Originally proposed adding pools to Engine. Rejected because:
- Engine would need to know handler details (breaks separation)
- Complicates shutdown (message tracker, loopback interactions)
- Couples Engine to Router implementation details

Router-based pools using pipe primitives keeps the abstraction clean.

### Alternative Considered: Functional Options

Considered `AddHandler(..., WithPool("erp"))` pattern. Rejected in favor of named methods because:
- `AddHandlerToPool` has clearer intent
- Avoids complexity of options parsing
- Consistent with Go standard library patterns (e.g., `context.WithValue` vs methods)
