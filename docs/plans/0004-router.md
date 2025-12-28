# Plan 0004: Router Extraction

**Status:** Proposed
**Related ADRs:** -
**Depends On:** [Plan 0001](0001-message-engine.md)

## Overview

Extract handler dispatch logic from Engine into a reusable Router component with a Pipe signature for composability.

## Motivation

1. **Separation of concerns** - Handler routing is distinct from Engine orchestration
2. **Composability** - Router with Pipe() can be used with `pipe.Apply()`
3. **Reusability** - Standalone multi-handler pipelines outside Engine
4. **Testability** - Router logic tested independently

## Design

### Router Component

```go
// Router dispatches messages to handlers by CE type.
// Implements Pipe signature for composability.
// Uses pipe.ProcessPipe internally for middleware, concurrency, and error handling.
type Router struct {
    handlers     map[string]handlerEntry
    errorHandler ErrorHandler
    pipeConfig   pipe.Config
}

type RouterConfig struct {
    ErrorHandler ErrorHandler
    // PipeConfig allows configuring the underlying ProcessPipe.
    // Buffer, Concurrency, Timeout, etc. can be set here.
    PipeConfig pipe.Config
}

// NewRouter creates a new message router.
func NewRouter(cfg RouterConfig) *Router

// AddHandler registers a handler for its CE type.
func (r *Router) AddHandler(h Handler, cfg HandlerConfig) error

// Pipe routes messages to handlers and returns outputs.
// Signature matches pipe.Pipe[*Message, *Message] for composability.
// Creates a new ProcessPipe on each call, allowing concurrent Pipe() calls.
func (r *Router) Pipe(ctx context.Context, in <-chan *Message) (<-chan *Message, error)
```

### Integration with Engine

```go
func NewEngine(cfg EngineConfig) *Engine {
    return &Engine{
        router: NewRouter(RouterConfig{
            ErrorHandler: cfg.ErrorHandler,
        }),
        // ...
    }
}

func (e *Engine) AddHandler(h Handler, cfg HandlerConfig) error {
    return e.router.AddHandler(h, cfg)
}

// In Start():
// Replace: handlerPipe := e.createHandlerPipe()
// With:    handled, _ := e.router.Pipe(ctx, typedMerged)
```

### Standalone Usage

```go
// Multi-handler pipeline outside Engine
router := message.NewRouter(message.RouterConfig{
    ErrorHandler: func(msg *message.Message, err error) {
        log.Error("routing error", "error", err)
    },
})

router.AddHandler(orderHandler, message.HandlerConfig{})
router.AddHandler(userHandler, message.HandlerConfig{})

// Use as pipe
out, _ := router.Pipe(ctx, inputCh)

// Or compose with other pipes
pipeline := pipe.Apply(unmarshalPipe.Pipe, router.Pipe)
```

## What This Is NOT

This is **not** the rejected PipeHandler approach:

| Rejected (Plan 0004 v1) | New Approach |
|-------------------------|--------------|
| `PipeHandler` interface handlers implement | Router is a component containing Handlers |
| `EventType()` on PipeHandler (awkward for Router) | No new interface - Router uses existing Handler |
| Interface bloat | Simple extraction of existing logic |

## Implementation

### Router Implementation

```go
type handlerEntry struct {
    handler Handler
    config  HandlerConfig
}

type Router struct {
    mu           sync.RWMutex
    handlers     map[string]handlerEntry
    errorHandler ErrorHandler
}

func NewRouter(cfg RouterConfig) *Router {
    eh := cfg.ErrorHandler
    if eh == nil {
        eh = func(msg *Message, err error) {
            slog.Error("router error", "error", err)
        }
    }
    return &Router{
        handlers:     make(map[string]handlerEntry),
        errorHandler: eh,
    }
}

func (r *Router) AddHandler(h Handler, cfg HandlerConfig) error {
    r.mu.Lock()
    defer r.mu.Unlock()
    r.handlers[h.EventType()] = handlerEntry{handler: h, config: cfg}
    return nil
}

func (r *Router) Pipe(ctx context.Context, in <-chan *Message) (<-chan *Message, error) {
    out := make(chan *Message, 100)

    go func() {
        defer close(out)
        for msg := range in {
            outputs, err := r.process(ctx, msg)
            if err != nil {
                r.errorHandler(msg, err)
                continue
            }
            for _, o := range outputs {
                select {
                case out <- o:
                case <-ctx.Done():
                    return
                }
            }
        }
    }()

    return out, nil
}

func (r *Router) process(ctx context.Context, msg *Message) ([]*Message, error) {
    ceType, _ := msg.Attributes["type"].(string)

    r.mu.RLock()
    entry, ok := r.handlers[ceType]
    r.mu.RUnlock()

    if !ok {
        return nil, ErrNoHandler
    }

    if entry.config.Matcher != nil && !entry.config.Matcher.Match(msg) {
        return nil, ErrHandlerRejected
    }

    outputs, err := entry.handler.Handle(ctx, msg)
    if err != nil {
        return nil, err
    }

    msg.Ack()
    return outputs, nil
}
```

### Engine Changes

1. Add `router *Router` field to Engine
2. Create Router in `NewEngine()`
3. Delegate `AddHandler()` to Router
4. Replace `createHandlerPipe()` with `router.Pipe()` in `Start()`
5. Remove `handlers map[string]handlerEntry` from Engine
6. Remove `createHandlerPipe()` method

## Migration

No public API changes. Engine.AddHandler() behavior unchanged.

## Test Plan

1. Existing engine tests pass unchanged
2. New tests for Router standalone usage
3. Test Router composition with pipe.Apply()
4. `make test && make build && make vet` passes

## Acceptance Criteria

- [ ] Router type with AddHandler() and Pipe()
- [ ] Engine delegates to Router internally
- [ ] Router usable standalone (without Engine)
- [ ] Router composable via pipe.Apply()
- [ ] All existing tests pass
- [ ] `make test && make build && make vet` passes
