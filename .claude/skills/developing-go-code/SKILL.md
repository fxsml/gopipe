---
name: developing-go-code
description: |
  Provides Go development standards and best practices for the gopipe codebase.
  This skill helps with testing, building, code patterns, and avoiding common mistakes.
  Use when writing or reviewing Go code in the gopipe repository.
---

# Developing Go Code

This skill provides guidance for Go development in the gopipe repository.

## Key Knowledge

### Build Commands

```bash
make test   # Run all tests
make build  # Build all packages
make vet    # Run linters
make check  # Run all above
```

### Package Overview

| Package | Purpose | Key Types |
|---------|---------|-----------|
| `channel/` | Stateless channel operations | Filter, Transform, Merge, Broadcast |
| `pipe/` | Stateful components with lifecycle | ProcessPipe, Merger, Distributor |
| `message/` | CloudEvents message handling | Engine, Router, Handler |

### API Conventions

| Context | Pattern | Example |
|---------|---------|---------|
| Constructors | Config struct | `NewEngine(EngineConfig{})` |
| Methods | Direct parameters | `AddHandler("name", matcher, h)` |
| Optional filtering | `nil` = match all | `AddOutput("out", nil)` |

### Matcher Interface

```go
type Matcher interface {
    Match(attrs Attributes) bool  // Uses Attributes, not *Message
}
```

**Why:** All matchers only access attributes. Using `*Message` would require wrapper allocation for raw message matching.

### Handler Interface

```go
type Handler interface {
    EventType() string   // CE type for routing
    NewInput() any       // Creates instance for unmarshaling
    Handle(ctx, msg) ([]*Message, error)
}
```

**Why:** Handler is self-describing. No central registry needed.

## Common Mistakes to Avoid

### Using channel.Process for filtering

```go
// WRONG - Process is for 1:N mapping
channel.Process(in, func(msg) []*Message {
    if match { return []*Message{msg} }
    return nil
})

// CORRECT - Filter with side effect
channel.Filter(in, func(msg) bool {
    if matcher.Match(msg) { return true }
    errorHandler(msg, ErrRejected)
    return false
})
```

### Creating components in Start()

```go
// WRONG - creates forwarding complexity
func (e *Engine) Start() {
    e.distributor = NewDistributor()  // Too late
}

// CORRECT - create upfront, Add* calls component directly
func NewEngine() *Engine {
    return &Engine{
        distributor: NewDistributor(),  // Ready for AddOutput()
    }
}
```

### N goroutines for loopback

```go
// WRONG - N loopbacks = N goroutines forwarding to same channel
for _, lb := range loopbacks {
    ch := distributor.AddOutput(lb.matcher)
    go forward(ch, loopbackIn)  // Wasteful
}

// CORRECT - combine matchers, single output
combined := match.Any(matchers...)
loopbackCh := distributor.AddOutput(combined)
typedMerger.AddInput(loopbackCh)  // Direct, no forwarding
```

### Handler.Name() method

Handler should NOT own its name. Name is a wiring concern:

```go
// WRONG
type Handler interface { Name() string }

// CORRECT - name is parameter to AddHandler
engine.AddHandler("process-orders", matcher, handler)
```

### Copy() sharing Attributes map

```go
// WRONG - modifications affect original
func Copy(msg, data) *Message {
    return &Message{Attributes: msg.Attributes}  // Shared reference
}

// CORRECT - clone for independence
func Copy(msg, data) *Message {
    return &Message{Attributes: maps.Clone(msg.Attributes)}
}
```

## Testing Guidelines

1. Use table-driven tests for multiple cases
2. Test both success and error paths
3. Use `t.Parallel()` for independent tests
4. Mock external dependencies
5. Test concurrent access with `-race` flag

## Reference Documents

- [docs/procedures/go.md](docs/procedures/go.md) - Testing, building, vetting
- [AGENTS.md](AGENTS.md) - Common mistakes, API conventions
