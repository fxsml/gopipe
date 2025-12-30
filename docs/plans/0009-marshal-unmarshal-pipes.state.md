# State: Marshal/Unmarshal Pipe Components

**Status:** Phase 1 Implemented (In Progress)
**Related:** Plan 0008 (Release Review)

---

## Part 1: Handler Interface Simplification

### Current Handler Interface

```go
type Handler interface {
    EventType() string                                           // Registration concern
    NewInput() any                                               // Unmarshaling concern
    Handle(ctx context.Context, msg *Message) ([]*Message, error) // Processing concern
}
```

**Problem:** Three concerns mixed in one interface:
1. **Registration** - What CE type to handle
2. **Unmarshaling** - How to create typed instance
3. **Processing** - What to do with the message

### Proposed Separation

```go
// TypeEntry handles registration and unmarshaling concerns.
// Used by TypeRegistry and Router for type lookup and instance creation.
type TypeEntry interface {
    EventType() string   // What CE type this handles
    NewInstance() any    // Factory for unmarshaling (renamed from NewInput)
}

// Handler handles processing concern only.
// Simpler interface - just processes messages.
type Handler interface {
    Handle(ctx context.Context, msg *Message) ([]*Message, error)
}

// TypeRegistry maps CE types to TypeEntry.
type TypeRegistry interface {
    Lookup(ceType string) (TypeEntry, bool)
    Register(entry TypeEntry) error
}
```

### RegistryHandler - Combines Both

For convenience, a combined interface for the common case:

```go
// RegistryHandler combines TypeEntry and Handler.
// Equivalent to current Handler interface.
type RegistryHandler interface {
    TypeEntry
    Handler
}

// Verify commandHandler implements RegistryHandler
var _ RegistryHandler = (*commandHandler[any, any])(nil)
```

---

## Implications Analysis

### 1. Router Changes

**Current:**
```go
func (r *Router) AddHandler(h Handler, cfg HandlerConfig) error {
    r.handlers[h.EventType()] = handlerEntry{handler: h, config: cfg}
}
```

**New Options:**

**Option A: Keep AddHandler for RegistryHandler**
```go
// For types that implement both TypeEntry and Handler
func (r *Router) AddHandler(h RegistryHandler, cfg HandlerConfig) error {
    r.Register(h)  // TypeEntry part
    r.handlers[h.EventType()] = handlerEntry{handler: h, config: cfg}
}
```

**Option B: Separate registration**
```go
// Register type info and handler separately
func (r *Router) Register(entry TypeEntry, h Handler, cfg HandlerConfig) error {
    r.typeRegistry.Register(entry)
    r.handlers[entry.EventType()] = handlerEntry{handler: h, config: cfg}
}

// Convenience for RegistryHandler
func (r *Router) AddHandler(h RegistryHandler, cfg HandlerConfig) error {
    return r.Register(h, h, cfg)
}
```

**Option C: Functional registration**
```go
// Most flexible - no interfaces needed for simple cases
func (r *Router) Handle(
    eventType string,
    factory func() any,
    handler func(context.Context, *Message) ([]*Message, error),
    cfg HandlerConfig,
) error
```

### 2. TypeRegistry Implementation

```go
// Router implements TypeRegistry
type Router struct {
    handlers map[string]handlerEntry
    types    map[string]TypeEntry  // Separate type registry
}

func (r *Router) Lookup(ceType string) (TypeEntry, bool) {
    entry, ok := r.types[ceType]
    return entry, ok
}

func (r *Router) Register(entry TypeEntry) error {
    r.types[entry.EventType()] = entry
    return nil
}
```

### 3. Unmarshal Pipe Uses TypeRegistry

```go
func NewUnmarshalPipe(
    registry TypeRegistry,
    marshaler Marshaler,
    cfg pipe.Config,
) *pipe.ProcessPipe[*RawMessage, *Message] {
    return pipe.NewProcessPipe(func(ctx context.Context, raw *RawMessage) ([]*Message, error) {
        ceType := raw.Attributes["type"].(string)

        entry, ok := registry.Lookup(ceType)
        if !ok {
            return nil, ErrNoHandler
        }

        instance := entry.NewInstance()
        if err := marshaler.Unmarshal(raw.Data, instance); err != nil {
            return nil, err
        }

        return []*Message{{Data: instance, Attributes: raw.Attributes}}, nil
    }, cfg)
}
```

### 4. Handler Implementations

**NewCommandHandler stays similar but implements RegistryHandler:**
```go
type commandHandler[C, E any] struct {
    eventType string
    source    string
    naming    NamingStrategy
    fn        func(ctx context.Context, cmd C) ([]E, error)
}

// TypeEntry methods
func (h *commandHandler[C, E]) EventType() string { return h.eventType }
func (h *commandHandler[C, E]) NewInstance() any  { return new(C) }

// Handler method
func (h *commandHandler[C, E]) Handle(ctx context.Context, msg *Message) ([]*Message, error) {
    // ... same as current
}
```

**Simple handler without type info:**
```go
// For cases where you just want to handle, not register types
simpleHandler := message.HandlerFunc(func(ctx context.Context, msg *Message) ([]*Message, error) {
    // process message
    return nil, nil
})

// Register with explicit type info
router.Register(
    message.TypeOf[OrderCommand](message.KebabNaming),
    simpleHandler,
    message.HandlerConfig{},
)
```

---

## Benefits of Separation

### 1. More Modular

```go
// Reuse same handler for multiple types
genericLogger := message.HandlerFunc(logMessage)

router.Register(TypeOf[EventA](), genericLogger, cfg)
router.Register(TypeOf[EventB](), genericLogger, cfg)
router.Register(TypeOf[EventC](), genericLogger, cfg)
```

### 2. Type Registration Without Handler

```go
// Register type for unmarshaling only (validation, logging)
router.RegisterType(TypeOf[AuditEvent]())

// Later add handler
router.SetHandler("audit.event", auditHandler, cfg)
```

### 3. Cleaner Standalone Pipes

```go
// TypeRegistry can be built without handlers
registry := message.NewMapRegistry()
registry.Register(TypeOf[Order]())
registry.Register(TypeOf[User]())

// UnmarshalPipe works independently
pipe := message.NewUnmarshalPipe(registry, marshaler, cfg)
```

### 4. More Idiomatic Go

Single-method interfaces are preferred in Go:
```go
type Handler interface {
    Handle(ctx context.Context, msg *Message) ([]*Message, error)
}

// Like http.Handler, io.Reader, etc.
```

### 5. HandlerFunc Pattern

```go
// Like http.HandlerFunc
type HandlerFunc func(context.Context, *Message) ([]*Message, error)

func (f HandlerFunc) Handle(ctx context.Context, msg *Message) ([]*Message, error) {
    return f(ctx, msg)
}
```

---

## Helper: TypeOf

Convenience for creating TypeEntry from Go type:

```go
// TypeOf creates a TypeEntry for a Go type using the given naming strategy.
func TypeOf[T any](naming NamingStrategy) TypeEntry {
    return &typeEntry[T]{naming: naming}
}

type typeEntry[T any] struct {
    naming NamingStrategy
}

func (e *typeEntry[T]) EventType() string {
    var zero T
    return e.naming.TypeName(reflect.TypeOf(zero))
}

func (e *typeEntry[T]) NewInstance() any {
    return new(T)
}

// Usage
router.Register(
    message.TypeOf[OrderCommand](message.KebabNaming),  // TypeEntry
    orderHandler,                                        // Handler
    message.HandlerConfig{},
)
```

---

## Migration Path

### Phase 1: Add new interfaces (non-breaking)
- Add `TypeEntry` interface
- Add `TypeRegistry` interface
- Add `RegistryHandler` as alias for combined interface
- Add `HandlerFunc` type
- Add `TypeOf[T]()` helper
- Existing code continues to work

### Phase 2: Update Router (non-breaking)
- Router implements `TypeRegistry`
- Add `router.Register(TypeEntry, Handler, cfg)` method
- Keep `router.AddHandler(RegistryHandler, cfg)` for compatibility

### Phase 3: Rename in existing types (breaking)
- Rename `NewInput()` to `NewInstance()` in Handler implementations
- Update `Handler` interface to just `Handle()`
- Update `commandHandler` to implement `RegistryHandler`

---

## Summary

| Aspect | Current | Proposed |
|--------|---------|----------|
| Handler interface | 3 methods | 1 method (Handle) |
| Type registration | Via Handler | Via TypeEntry |
| Unmarshaling factory | Handler.NewInput() | TypeEntry.NewInstance() |
| Router registration | AddHandler(Handler) | Register(TypeEntry, Handler) |
| Standalone unmarshal | Needs Handler | Needs TypeEntry only |
| Code reuse | Limited | Handler reusable across types |
| Go idiom alignment | Mixed | Single-method interface |

**Recommendation:** Proceed with this separation. It leads to more modular, idiomatic code and cleanly solves the registry problem for standalone pipes.

// UnmarshalPipe creates a pipe that unmarshals RawMessage to Message.
func NewUnmarshalPipe(
    registry TypeRegistry,
    marshaler Marshaler,
    cfg pipe.Config,
) *pipe.ProcessPipe[*RawMessage, *Message]

// Usage
registry := message.NewTypeRegistry()
registry.Register("order.command", func() any { return new(OrderCommand) })

pipe := message.NewUnmarshalPipe(registry, marshaler, pipe.Config{
    Concurrency: 4,
})
```

**Pros:**
- Clean separation of concerns
- Registry reusable across components
- Handler doesn't need to implement NewInput()

**Cons:**
- New concept to learn (TypeRegistry)
- Duplication if also registering handlers
- Need to keep registry and handlers in sync

---

## Option 2: Router Implements TypeRegistry

Router already has the handler map. Make it implement TypeRegistry:

```go
// Router implements TypeRegistry via its handler map.
func (r *Router) NewInstance(ceType string) any {
    entry, ok := r.handler(ceType)
    if !ok {
        return nil
    }
    return entry.handler.NewInput()
}

// UnmarshalPipe uses Router as registry
pipe := message.NewUnmarshalPipe(router, marshaler, cfg)
```

**Pros:**
- No new concept - Router is the registry
- Handler.NewInput() still used
- Natural integration with Engine

**Cons:**
- Couples UnmarshalPipe to Router
- Can't use UnmarshalPipe without Router

---

## Option 3: Handler-Based Factory

Pass handlers directly, derive registry internally:

```go
// NewUnmarshalPipe creates unmarshal pipe from handlers.
func NewUnmarshalPipe(
    handlers []Handler,
    marshaler Marshaler,
    cfg pipe.Config,
) *pipe.ProcessPipe[*RawMessage, *Message]

// Usage
pipe := message.NewUnmarshalPipe(
    []Handler{orderHandler, userHandler},
    marshaler,
    pipe.Config{Concurrency: 4},
)
```

**Pros:**
- Simple API
- Reuses existing Handler interface
- No new concepts

**Cons:**
- Need to pass all handlers
- Changes require recreating pipe

---

## Option 4: Generic Unmarshal (No Registry)

Unmarshal to `map[string]any` or keep as `json.RawMessage`:

```go
// GenericUnmarshalPipe unmarshals to map[string]any.
// Handlers receive untyped data and do their own type assertion.
func NewGenericUnmarshalPipe(
    marshaler Marshaler,
    cfg pipe.Config,
) *pipe.ProcessPipe[*RawMessage, *Message]

// Handler deals with map[string]any
func (h *handler) Handle(ctx context.Context, msg *Message) ([]*Message, error) {
    data := msg.Data.(map[string]any)
    cmd := OrderCommand{
        ID: data["id"].(string),
    }
    // ...
}
```

**Pros:**
- No registry needed
- Simplest implementation
- Works with any message type

**Cons:**
- Loses type safety
- Handler code more complex
- Worse performance (extra conversion)

---

## TransformPipe vs ProcessPipe

### Current: channel.Process (1:N)

```go
channel.Process(in, func(raw *RawMessage) []*Message {
    // Returns nil on error (0 outputs)
    // Returns []*Message{msg} on success (1 output)
})
```

### Proposed: pipe.ProcessPipe with Transform semantics

```go
// Could add NewTransformPipe to pipe package
func NewTransformPipe[In, Out any](
    fn func(context.Context, In) (Out, error),
    cfg Config,
) *ProcessPipe[In, Out]

// Internally wraps to ProcessFunc
func(ctx context.Context, in In) ([]Out, error) {
    out, err := fn(ctx, in)
    if err != nil {
        return nil, err
    }
    return []Out{out}, nil
}
```

### Benefits of ProcessPipe over channel.Process

| Feature | channel.Process | pipe.ProcessPipe |
|---------|-----------------|------------------|
| Concurrency | ❌ Single goroutine | ✅ Configurable |
| Error handling | Manual (return nil) | ✅ Config.ErrorHandler |
| Middleware | ❌ No | ✅ ApplyMiddleware |
| Context | ❌ No | ✅ Passed to handler |
| Buffer size | ❌ Fixed | ✅ Configurable |

**Recommendation:** Use `pipe.ProcessPipe` (or add `NewTransformPipe` convenience).

---

## Proposed Public API

### Types

```go
// TypeRegistry maps CE types to instance factories.
type TypeRegistry interface {
    NewInstance(ceType string) any
}

// MapRegistry is a simple map-based TypeRegistry.
type MapRegistry map[string]func() any

func (r MapRegistry) NewInstance(ceType string) any {
    if factory, ok := r[ceType]; ok {
        return factory()
    }
    return nil
}

// Router implements TypeRegistry (uses handler.NewInput)
var _ TypeRegistry = (*Router)(nil)
```

### Pipes

```go
// NewUnmarshalPipe creates a pipe that unmarshals RawMessage to Message.
// Uses registry to create typed instances for unmarshaling.
// Returns error via cfg.ErrorHandler if type not in registry or unmarshal fails.
func NewUnmarshalPipe(
    registry TypeRegistry,
    marshaler Marshaler,
    cfg pipe.Config,
) *pipe.ProcessPipe[*RawMessage, *Message]

// NewMarshalPipe creates a pipe that marshals Message to RawMessage.
// No registry needed - marshals msg.Data directly.
func NewMarshalPipe(
    marshaler Marshaler,
    cfg pipe.Config,
) *pipe.ProcessPipe[*Message, *RawMessage]
```

### Usage Examples

#### Standalone (without Engine)

```go
// Create registry
registry := message.MapRegistry{
    "order.command": func() any { return new(OrderCommand) },
    "user.command":  func() any { return new(UserCommand) },
}

// Create pipes
unmarshal := message.NewUnmarshalPipe(registry, marshaler, pipe.Config{
    Concurrency: 4,
    ErrorHandler: func(in any, err error) {
        log.Printf("unmarshal error: %v", err)
    },
})

marshal := message.NewMarshalPipe(marshaler, pipe.Config{
    Concurrency: 4,
})

// Use in pipeline
rawIn := make(chan *message.RawMessage)
typed, _ := unmarshal.Pipe(ctx, rawIn)
// ... process typed messages ...
rawOut, _ := marshal.Pipe(ctx, processed)
```

#### With Engine (using Router as registry)

```go
engine := message.NewEngine(cfg)
engine.AddHandler(orderHandler, message.HandlerConfig{})

// Router implements TypeRegistry
unmarshal := message.NewUnmarshalPipe(
    engine.Router(), // exposed method
    marshaler,
    pipe.Config{Concurrency: 4},
)
```

---

## Engine Integration

### Current Engine

```go
type Engine struct {
    router      *Router
    merger      *pipe.Merger[*Message]
    distributor *pipe.Distributor[*Message]
    // ...
}

// Private methods
func (e *Engine) unmarshal(in <-chan *RawMessage) <-chan *Message
func (e *Engine) marshal(in <-chan *Message) <-chan *RawMessage
```

### Proposed Engine

```go
type Engine struct {
    router      *Router
    merger      *pipe.Merger[*Message]
    distributor *pipe.Distributor[*Message]
    // ...
}

// Expose router for use as TypeRegistry
func (e *Engine) Router() *Router { return e.router }

// Internal: use public pipes
func (e *Engine) unmarshal(in <-chan *RawMessage) <-chan *Message {
    p := NewUnmarshalPipe(e.router, e.marshaler, pipe.Config{
        ErrorHandler: func(in any, err error) {
            raw := in.(*RawMessage)
            e.handleRawError(raw.Attributes, err)
        },
    })
    out, _ := p.Pipe(context.Background(), in)
    return out
}
```

---

## Implementation Checklist

### pipe package

- [ ] Add `NewTransformPipe` convenience constructor (optional)
- [ ] Ensure ProcessPipe supports 1:1 semantics cleanly

### message package

- [ ] Add `TypeRegistry` interface
- [ ] Add `MapRegistry` implementation
- [ ] Make `Router` implement `TypeRegistry`
- [ ] Add `NewUnmarshalPipe` function
- [ ] Add `NewMarshalPipe` function
- [ ] Expose `Engine.Router()` method
- [ ] Refactor Engine to use public pipes internally
- [ ] Add tests for standalone pipe usage
- [ ] Update documentation

---

## Questions to Resolve

1. **Should we add NewTransformPipe to pipe package?**
   - ProcessPipe works but semantically 1:N
   - TransformPipe would be cleaner for 1:1

2. **Default concurrency for marshal/unmarshal?**
   - 1 (current) or match CPU count?

3. **Error semantics: skip or propagate?**
   - Current: skip (return nil)
   - Could: propagate to error handler and skip

4. **Should MapRegistry be thread-safe?**
   - sync.RWMutex or immutable after creation?

---

## Recommendation

**Option 2 (Router implements TypeRegistry)** with **TypeRegistry interface** for flexibility:

1. Define `TypeRegistry` interface
2. `Router` implements it via `handler.NewInput()`
3. Provide `MapRegistry` for standalone use
4. Use `pipe.ProcessPipe` for parallelization support
5. Engine uses public pipes internally

---

## Implementation Status

### Phase 1: Completed ✅

**Date:** 2025-12-30

Added the following non-breaking changes:

#### New Types in `message/registry.go`

```go
// TypeEntry - Type information for CE message handling
type TypeEntry interface {
    EventType() string   // CE type this handles
    NewInstance() any    // Factory for unmarshaling
}

// TypeRegistry - Maps CE types to TypeEntry
type TypeRegistry interface {
    Lookup(ceType string) (TypeEntry, bool)
    Register(entry TypeEntry) error
}

// RegistryHandler - Combines TypeEntry and Handler
type RegistryHandler interface {
    TypeEntry
    Handler
}

// HandlerFunc - Adapter for functions (like http.HandlerFunc)
type HandlerFunc func(ctx context.Context, msg *Message) ([]*Message, error)

// TypeOf[T] - Creates TypeEntry from Go type
func TypeOf[T any](naming NamingStrategy) TypeEntry

// MapRegistry - Simple map-based TypeRegistry
type MapRegistry map[string]func() any
```

#### Handler Updates

- Added `NewInstance()` method to `handler[T]` and `commandHandler[C, E]`
- Handlers now implement both `Handler` and `TypeEntry` interfaces
- Existing `NewInput()` method retained for backward compatibility

#### Router Updates

- `Router` now implements `TypeRegistry` interface
- Added `Lookup(ceType string) (TypeEntry, bool)` method
- Added `Register(entry TypeEntry) error` method

#### New Error

- `ErrNotAHandler` - Returned when `Router.Register` is called with a TypeEntry that doesn't implement Handler

#### Tests

- Added `message/registry_test.go` with tests for:
  - `TypeOf[T]` helper
  - `MapRegistry` implementation
  - `HandlerFunc` adapter
  - `Router` as `TypeRegistry`
  - Handler `TypeEntry` compliance

### Remaining Phases

**Phase 2: Update Router API (non-breaking)** - Not yet started
- Add `router.Register(TypeEntry, Handler, cfg)` method variant

**Phase 3: Rename in existing types (breaking)** - Not yet started
- Rename `NewInput()` to `NewInstance()` in Handler interface
- Update `Handler` interface to just `Handle()`
