# Plan 0001: Message Engine Implementation

**Status:** Proposed
**Related ADRs:** [0019](../adr/0019-remove-sender-receiver.md), [0020](../adr/0020-message-engine-architecture.md), [0021](../adr/0021-codec-marshaling-pattern.md), [0022](../adr/0022-message-package-redesign.md)
**Depends On:** [Plan 0002](0002-marshaler.md) (Marshaler)

## Overview

Implement a Message Engine that orchestrates message flow with marshaling at boundaries and type-based routing.

## Design Principles

1. **Reuse `pipe` and `channel` packages** - Build on existing foundation
2. **Engine implements `Pipe()` signature** - Consistent with pipe package
3. **Router is internal** - Not exposed as public type
4. **Minimal Phase 1** - Single path, then iterate

## Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                         message.Engine                           │
│                                                                  │
│  Subscribers ──┐                                                 │
│                ├──> pipe.Merger ──> route() ──> handlers ──> out │
│  Generators ───┘         ↑                           │           │
│                          │                           │           │
│                          └─── internal loopback ─────┘           │
└─────────────────────────────────────────────────────────────────┘
```

## Phase 1: Minimal Engine

Single subscriber, handlers, output channel. No internal loopback.

### Core Types

```go
// Handler processes messages of a specific type
type Handler interface {
    EventType() reflect.Type
    Handle(ctx context.Context, event any) ([]*Message, error)
}

// NewHandler creates a typed handler
func NewHandler[T any](fn func(ctx context.Context, event T) ([]*Message, error)) Handler

// Subscriber provides messages from external source
type Subscriber interface {
    Subscribe(ctx context.Context) (<-chan *Message, error)
}

// Publisher consumes messages to external destination
type Publisher interface {
    Publish(ctx context.Context, msgs <-chan *Message) error
}
```

### Engine

```go
type Engine struct {
    marshaler Marshaler
    handlers  map[reflect.Type][]Handler  // internal router

    mu      sync.Mutex
    started bool
}

type EngineConfig struct {
    Marshaler Marshaler
}

func NewEngine(cfg EngineConfig) *Engine

func (e *Engine) AddHandler(name string, h Handler) error

// Pipe processes messages: unmarshal → route → handle → marshal
// Implements the pipe.Pipe pattern
func (e *Engine) Pipe(ctx context.Context, in <-chan *Message) (<-chan *Message, error)
```

### Usage

```go
// Create engine
engine := message.NewEngine(message.EngineConfig{
    Marshaler: message.NewJSONMarshaler(),
})

// Add handlers (type registry auto-built from handlers)
engine.AddHandler("process-order", message.NewHandler(
    func(ctx context.Context, order OrderCreated) ([]*Message, error) {
        return []*Message{
            message.New(OrderShipped{OrderID: order.ID}, nil),
        }, nil
    },
))

// Wire with subscriber
subscriber := ce.NewSubscriber(cloudEventsClient)  // from message/cloudevents
msgs, _ := subscriber.Subscribe(ctx)

// Process through engine
out, _ := engine.Pipe(ctx, msgs)

// Publish results
publisher := ce.NewPublisher(cloudEventsClient)
publisher.Publish(ctx, out)
```

### Implementation Details

**route() function (internal):**
```go
func (e *Engine) route(ctx context.Context, msg *Message) ([]*Message, error) {
    // Get CE type from attributes
    ceType, _ := msg.Attributes.Type()

    // Lookup Go type via marshaler
    goType, ok := e.marshaler.TypeFor(ceType)
    if !ok {
        return nil, fmt.Errorf("unknown type: %s", ceType)
    }

    // Unmarshal to typed value
    event, err := e.marshaler.Unmarshal(msg.Data, ceType)
    if err != nil {
        return nil, err
    }

    // Get handlers for this type
    handlers := e.handlers[goType]
    if len(handlers) == 0 {
        return nil, fmt.Errorf("no handler for type: %s", goType)
    }

    // Execute handlers
    var results []*Message
    for _, h := range handlers {
        out, err := h.Handle(ctx, event)
        if err != nil {
            return nil, err
        }
        results = append(results, out...)
    }

    return results, nil
}
```

**Pipe() implementation:**
```go
func (e *Engine) Pipe(ctx context.Context, in <-chan *Message) (<-chan *Message, error) {
    e.mu.Lock()
    if e.started {
        e.mu.Unlock()
        return nil, ErrAlreadyStarted
    }
    e.started = true
    e.mu.Unlock()

    // Auto-build type registry from handlers
    e.buildTypeRegistry()

    // Use pipe.ProcessPipe for processing with middleware support
    pp := pipe.NewProcessPipe(e.route, pipe.Config{})
    return pp.Pipe(ctx, in)
}
```

## Phase 2: Multiple Subscribers

Add `pipe.Merger` for dynamic fan-in:

```go
type Engine struct {
    // ... existing fields
    merger *pipe.Merger[*Message]
}

func (e *Engine) AddSubscriber(name string, sub Subscriber) error {
    ch, err := sub.Subscribe(ctx)
    if err != nil {
        return err
    }
    e.merger.Add(ch)
    return nil
}
```

## Phase 3: Internal Loopback

Route handler outputs back to input when destination is `gopipe://`:

```go
func (e *Engine) Pipe(ctx context.Context, in <-chan *Message) (<-chan *Message, error) {
    // ... setup

    // Split output: internal goes back to merger, external goes to out
    internal, external := channel.Route(processed, func(msg *Message) int {
        if isInternal(msg) { return 0 }
        return 1
    }, 2)

    // Feed internal back to merger
    e.merger.Add(internal)

    return external, nil
}
```

## Files to Create/Modify

- `message/engine.go` - Engine implementation
- `message/handler.go` - Keep Handler interface, add NewHandler[T]
- `message/errors.go` - Add ErrAlreadyStarted

## Test Plan

1. Single handler receives typed data
2. Multiple handlers for same type all execute
3. Unknown type returns error
4. Type registry auto-built from handlers
5. Pipe() returns ErrAlreadyStarted on second call

## Acceptance Criteria

- [ ] Engine.Pipe() works with single subscriber
- [ ] Handlers receive typed data (not []byte)
- [ ] Type registry auto-generated from handlers
- [ ] Uses pipe.ProcessPipe for processing
- [ ] Tests pass (`make test`)
- [ ] Build passes (`make build && make vet`)
