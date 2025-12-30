# State: Marshal/Unmarshal Pipe Components

**Status:** Analysis
**Related:** Plan 0008 (Release Review)

## Problem Statement

Currently `marshal` and `unmarshal` are private Engine methods. Exposing them as reusable pipe components would:
1. Allow standalone use (without Engine)
2. Enable custom pipelines
3. Support parallelization via pipe.Config.Concurrency

### The Registry Problem

Unmarshal needs to know **which type to unmarshal into**:

```go
// Current implementation
func (e *Engine) unmarshal(in <-chan *RawMessage) <-chan *Message {
    return channel.Process(in, func(raw *RawMessage) []*Message {
        ceType := raw.Attributes["type"].(string)

        // Registry lookup - needs handler map
        entry, ok := e.router.handler(ceType)
        if !ok {
            return nil // ErrNoHandler
        }

        instance := entry.handler.NewInput() // Creates typed instance
        e.marshaler.Unmarshal(raw.Data, instance)
        // ...
    })
}
```

The question: How to decouple this from the Router/Engine?

---

## Option 1: TypeRegistry Interface

Separate the type-to-factory mapping from handlers:

```go
// TypeRegistry maps CE types to factory functions.
type TypeRegistry interface {
    // NewInstance creates a new instance for the given CE type.
    // Returns nil if type is not registered.
    NewInstance(ceType string) any
}

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
