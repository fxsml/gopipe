---
name: developing-go-code
description: |
  Provides Go development standards and best practices for the gopipe codebase.
  Apply when writing or reviewing Go code: testing patterns, godoc, error handling,
  API conventions, and common anti-patterns to avoid.
user-invocable: false
---

# Developing Go Code

## Build Commands

```bash
make test   # Run all tests
make build  # Build all packages
make vet    # Run linters
make check  # All of the above
```

## API Conventions

| Context | Pattern | Example |
|---------|---------|---------|
| Constructors | Config struct | `NewEngine(EngineConfig{})` |
| Methods | Direct parameters | `AddHandler("name", matcher, h)` |
| Optional filtering | `nil` = match all | `AddOutput("out", nil)` |

## Godoc Standards

First sentence starts with the function name and describes what it does. Include a usage example for non-trivial functions:

```go
// GroupBy aggregates items from the input channel by key, emitting batches
// when size or time limits are reached.
//
// Example:
//
//	groups := channel.GroupBy(orders, func(o Order) string {
//	    return o.CustomerID
//	}, channel.GroupByConfig{MaxSize: 10})
func GroupBy[K comparable, V any](...) <-chan Group[K, V]
```

## Testing

- Table-driven tests preferred
- Use `t.Parallel()` where safe
- Test both success and error paths
- Mock external dependencies
- Run with `-race` flag: `go test -race ./...`

## Error Handling

- Return errors, don't panic (except in `Must*` functions)
- Wrap with context: `fmt.Errorf("operation: %w", err)`
- Check errors immediately after call

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
    e.distributor = NewDistributor()
}

// CORRECT - create upfront so Add* works before Start
func NewEngine() *Engine {
    return &Engine{distributor: NewDistributor()}
}
```

### Handler.Name() method

Handler should NOT own its name — name is a wiring concern:

```go
// WRONG
type Handler interface { Name() string }

// CORRECT - name is parameter to AddHandler
engine.AddHandler("process-orders", matcher, handler)
```

### Copy() sharing Attributes map

```go
// WRONG - modifications affect original
return &Message{Attributes: msg.Attributes}

// CORRECT - clone for independence
return &Message{Attributes: maps.Clone(msg.Attributes)}
```

## Reference Procedures

- @../docs/procedures/go.md — full Go standards, deprecation, error handling
- @../AGENTS.md — architecture decisions, common mistakes, naming decisions
