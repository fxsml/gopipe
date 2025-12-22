# ADR 0018: Interface Naming Conventions

**Date:** 2025-12-22
**Status:** Proposed

## Context

The library has inconsistent interface naming across packages. Go convention is `<Verb>er` interface with `<Verb>()` method (e.g., `io.Reader.Read()`), but current code mixes patterns:

| Package | Interface | Method | Issue |
|---------|-----------|--------|-------|
| `pipe` | `Pipe[Pre, Out]` | `Start()` | Method doesn't match interface name |
| `pipe` | `Generator[Out]` | `Generate()` | ✓ Consistent |
| `message` | `Subscriber` (struct) | `Subscribe()` | Not an interface |
| `message` | `Publisher` (struct) | `Publish()` | Not an interface |

## Decision

Adopt verb-based naming with clear intent per interface. Reserve `Start()` for orchestration types.

### Core Pattern

```
Interface Name = <Verb> or <Verb>er
Method Name    = <Verb>
```

### Interface Inventory by Package

#### pipe (generic, reusable)

```go
// Pipe transforms input channel to output channel
type Pipe[In, Out any] interface {
    Pipe(ctx context.Context, in <-chan In) (<-chan Out, error)
}

// Generator produces values without input
type Generator[Out any] interface {
    Generate(ctx context.Context) (<-chan Out, error)
}
```

#### message (message-specific)

```go
// Subscriber receives messages from external source (topic-based)
type Subscriber interface {
    Subscribe(ctx context.Context, topic string) (<-chan *Message, error)
}

// Generator produces messages internally (no topic)
type Generator interface {
    Generate(ctx context.Context) (<-chan *Message, error)
}

// Publisher sends messages to external destination
type Publisher interface {
    Publish(ctx context.Context, msgs <-chan *Message) error
}

// Handler processes a single message
type Handler interface {
    EventType() reflect.Type
    Handle(ctx context.Context, msg *Message) ([]*Message, error)
}

// Marshaler serializes/deserializes messages
type Marshaler interface {
    Marshal(v any) ([]byte, error)
    Unmarshal(data []byte, ceType string) (any, error)
    // ... type registry methods
}
```

#### Channel Operations (semantic verbs)

```go
// Merger combines multiple input channels into one output
// Inputs are configured at construction time
type Merger[T any] interface {
    Merge(ctx context.Context) (<-chan T, error)
}

// Distributor sends from one input to multiple outputs
// Outputs are configured at construction time
type Distributor[T any] interface {
    Distribute(ctx context.Context, in <-chan T) (<-chan struct{}, error)
}
```

Construction patterns (not part of interface):
```go
// Merger: inputs provided at construction
merger := NewMerger(ch1, ch2, ch3)
out, err := merger.Merge(ctx)

// Distributor: outputs provided at construction
dist := NewDistributor(out1, out2, out3)
done, err := dist.Distribute(ctx, in)
```

#### Orchestration (Start reserved)

```go
// Engine orchestrates message flow
// Returns done channel, closed when engine stops
type Engine interface {
    Start(ctx context.Context) (<-chan struct{}, error)
}

// Fan combines Merger and Distributor for bidirectional fan patterns
// Returns done channel, closed when fan stops
type Fan[T any] interface {
    Start(ctx context.Context) (<-chan struct{}, error)
}
```

**Method return patterns:**
- `Merge()` returns `(<-chan T, error)` - output channel IS the result
- `Distribute()` returns `(<-chan struct{}, error)` - done channel for completion
- `Start()` returns `(<-chan struct{}, error)` - orchestration completion signal

### Naming Decisions

| Concept | Interface | Method | Rationale |
|---------|-----------|--------|-----------|
| Channel transform | `Pipe` | `Pipe()` | Clear intent, Unix heritage |
| Value generation | `Generator` | `Generate()` | Standard pattern |
| Channel merging | `Merger` | `Merge()` | Semantic verb describes action |
| Channel distribution | `Distributor` | `Distribute()` | Semantic verb describes action |
| Topic subscription | `Subscriber` | `Subscribe()` | Standard pattern |
| Message publishing | `Publisher` | `Publish()` | Standard pattern |
| Message handling | `Handler` | `Handle()` | Standard pattern |
| Orchestration | `Engine`, `Fan` | `Start()` | Reserved for high-level composition |

### Why Merger/Distributor over FanIn/FanOut

The rename follows Go stdlib patterns where the interface name describes **what it does**:

1. **Semantic clarity** - `Merge()` and `Distribute()` are self-documenting verbs
2. **Pattern consistency** - follows `io.Reader.Read()`, `io.Writer.Write()`
3. **Reserved Start()** - keeps `Start()` for true orchestration (Engine, Fan)
4. **Fan as composition** - `Fan` combines Merger + Distributor, justifying `Start()`

### Why `Pipe.Pipe()`

The redundancy is a **feature**, not a bug:

1. **Crystal clear intent** - no ambiguity what it does
2. **Unix heritage** - "pipe" is both noun and verb
3. **Follows the pattern** - `Filter.Filter()`, `Route.Route()` are common
4. **Distinguishes from orchestration** - `Start()` reserved for Engine, Fan

### Breaking Change: `Pipe.Start()` → `Pipe.Pipe()`

```go
// Before
type Pipe[Pre, Out any] interface {
    Start(ctx context.Context, pre <-chan Pre) (<-chan Out, error)
}

// After
type Pipe[In, Out any] interface {
    Pipe(ctx context.Context, in <-chan In) (<-chan Out, error)
}
```

Also rename parameter `Pre` → `In` for clarity.

### Level Distinction

- **Item-level**: `ProcessFunc`, `TransformFunc`, `FilterFunc` (single item)
- **Channel-level**: `Pipe`, `Generator`, `Merger`, `Distributor`, `Subscriber`, `Publisher` (streams)
- **Orchestration**: `Engine`, `Fan` (composition of multiple channel-level components)

## Consequences

**Benefits:**
- Consistent naming across the library
- Clear intent per interface method
- `Start()` reserved for orchestration - clear hierarchy
- Self-documenting: `myPipe.Pipe(ctx, in)`, `merger.Merge(ctx)` are unambiguous
- Semantic verbs for channel operations improve discoverability

**Drawbacks:**
- Breaking change for `pipe.Pipe` interface users
- Breaking change: `FanIn` → `Merger`, `FanOut` → `Distributor`
- `Pipe.Pipe()` is redundant (by design)

## Migration

### Pipe interface
1. Rename `Start()` → `Pipe()` method
2. Rename parameter `Pre` → `In`

### Fan operations
1. Rename `FanIn` → `Merger`
2. Rename `FanIn.Start()` → `Merger.Merge()`
3. Rename `FanOut` → `Distributor`
4. Rename `FanOut.Start()` → `Distributor.Distribute()`
5. Add `Fan` type for combined fan-in/fan-out orchestration

## Links

- Related: ADR 0019 (Remove Sender/Receiver)
- Related: ADR 0020 (Message Engine Architecture)
- Related: ADR 0021 (Marshaler Pattern)
