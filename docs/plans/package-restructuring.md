# Package Restructuring Plan

**Date:** 2025-12-16
**Status:** Proposed
**Priority:** PREREQUISITE - Part of Layer 0 Foundation
**Related:** layer-0-foundation-cleanup.md

## Executive Summary

Restructure gopipe to focus on messaging and pipelining. Move generic processor/pipe primitives to a subpackage, promote message-specific components (Router, Publisher, Subscriber) to the main package, and consolidate middleware as message-specific.

## Goals

1. **Focus main package on messaging** - gopipe becomes the message pipeline framework
2. **Minimize generics exposure** - Only pipe handlers and router handlers use generics
3. **Clear package hierarchy** - Logical separation of concerns
4. **Simplified imports** - Common operations from `gopipe` package directly

## Current Structure

```
github.com/fxsml/gopipe/
├── *.go                    # Generic processors, pipes, options, middleware types
├── message/
│   ├── message.go          # Message, TypedMessage[T], Attributes, Acking
│   ├── handler.go          # Handler interface, Matcher, NewHandler
│   ├── pipe.go             # message.Pipe (extends gopipe.Pipe)
│   ├── router.go           # Router, RouterConfig
│   ├── publisher.go        # Publisher, PublisherConfig
│   ├── subscriber.go       # Subscriber, SubscriberConfig
│   ├── sender.go           # Sender interface
│   ├── receiver.go         # Receiver interface
│   ├── broker/             # ChannelBroker, HTTP broker, IO broker
│   ├── cloudevents/        # CloudEvents support
│   ├── cqrs/               # Command/Event handlers
│   └── multiplex/          # Multiplexing support
├── middleware/
│   ├── message.go          # NewMessageMiddleware
│   └── correlation.go      # MessageCorrelation
└── channel/
    └── *.go                # Channel utilities (Merge, Broadcast, etc.)
```

### Current Import Patterns

```go
import (
    "github.com/fxsml/gopipe"
    "github.com/fxsml/gopipe/message"
    "github.com/fxsml/gopipe/message/broker"
    "github.com/fxsml/gopipe/middleware"
)

// Must qualify everything
pipe := gopipe.NewProcessPipe(handler, opts...)
msg := message.New(data)
router := message.NewRouter(config)
broker := broker.NewChannelBroker(config)
mw := middleware.MessageCorrelation()
```

## Proposed Structure

```
github.com/fxsml/gopipe/
├── *.go                    # Message-focused: Router, Handler, Publisher, Subscriber
├── message/
│   └── message.go          # Message entity ONLY: Message, TypedMessage[T], Attributes, Acking
├── pipe/
│   ├── pipe.go             # Pipe[In, Out] interface
│   ├── processor.go        # Processor[In, Out], ProcessFunc, CancelFunc/DropFunc
│   ├── process.go          # NewProcessPipe
│   ├── transform.go        # NewTransformPipe
│   ├── filter.go           # NewFilterPipe
│   ├── batch.go            # NewBatchPipe
│   ├── sink.go             # NewSinkPipe
│   ├── config.go           # ProcessorConfig (non-generic)
│   └── option.go           # Deprecated options (backward compat)
├── middleware/
│   ├── middleware.go       # Core types, Chain()
│   ├── recover.go          # Recover middleware
│   ├── retry.go            # Retry middleware
│   ├── timeout.go          # Timeout middleware
│   ├── logging.go          # Logging middleware (slog)
│   ├── metrics.go          # Metrics middleware
│   ├── correlation.go      # Correlation ID propagation
│   └── drop.go             # Drop handler middleware
├── broker/
│   ├── channel.go          # ChannelBroker (in-process)
│   ├── http.go             # HTTP broker
│   └── io.go               # IO-based broker
├── cloudevents/
│   └── event.go            # CloudEvents support
├── cqrs/
│   ├── command.go          # NewCommandHandler
│   ├── event.go            # NewEventHandler
│   └── marshaler.go        # Marshaler interfaces
└── channel/
    └── *.go                # Channel utilities (unchanged)
```

## New Import Patterns

```go
import (
    "github.com/fxsml/gopipe"
    "github.com/fxsml/gopipe/message"
    "github.com/fxsml/gopipe/pipe"
    "github.com/fxsml/gopipe/middleware"
    "github.com/fxsml/gopipe/broker"
)

// Message operations from main package
router := gopipe.NewRouter(config)
handler := gopipe.NewHandler(process, match)
pub := gopipe.NewPublisher(sender, config)
sub := gopipe.NewSubscriber(receiver, config)

// Message entity from message package
msg := message.New(data)

// Generic pipes from pipe package (rare - most use message pipes)
proc := pipe.NewProcessPipe(handler, drop, config)

// Middleware chain
chain := middleware.Chain(
    middleware.Recover(cfg),
    middleware.Logging(cfg),
)

// Brokers from broker package
b := broker.NewChannel(config)
```

## Component Migration

### To Main Package (`gopipe/`)

| From | To | Notes |
|------|-----|-------|
| `message.Router` | `gopipe.Router` | Main routing component |
| `message.RouterConfig` | `gopipe.RouterConfig` | |
| `message.Handler` | `gopipe.Handler` | Message handler interface |
| `message.NewHandler` | `gopipe.NewHandler` | |
| `message.Pipe` | `gopipe.Pipe` | Message pipe interface |
| `message.NewPipe` | `gopipe.NewPipe` | |
| `message.Publisher` | `gopipe.Publisher` | |
| `message.PublisherConfig` | `gopipe.PublisherConfig` | |
| `message.Subscriber` | `gopipe.Subscriber` | |
| `message.SubscriberConfig` | `gopipe.SubscriberConfig` | |
| `message.Sender` | `gopipe.Sender` | Interface |
| `message.Receiver` | `gopipe.Receiver` | Interface |
| `message.Generator` | `gopipe.Generator` | Message generator |
| `message.Matcher` | `gopipe.Matcher` | |
| `message.Match*` | `gopipe.Match*` | All match functions |
| `message.MiddlewareFunc` | `gopipe.MiddlewareFunc` | Message-specific middleware |

### To `message/` Package (Keep)

| Component | Notes |
|-----------|-------|
| `Message` | Core message entity |
| `TypedMessage[T]` | Generic typed message |
| `Attributes` | CloudEvents-compatible attributes |
| `Acking` | Acknowledgment coordination |
| `Properties` | Thread-safe properties |
| `New()` | Message factory |
| `NewWithAcking()` | Message with acking factory |
| `Copy()` | Message copy utility |
| `Attr*` constants | Attribute key constants |
| `Prop*` constants | Property key constants |

### To `pipe/` Package (New)

| From | To | Notes |
|------|-----|-------|
| `gopipe.Pipe[In, Out]` | `pipe.Pipe[In, Out]` | Generic interface |
| `gopipe.Processor[In, Out]` | `pipe.Processor[In, Out]` | Generic interface |
| `gopipe.ProcessFunc[In, Out]` | `pipe.ProcessFunc[In, Out]` | |
| `gopipe.CancelFunc[In]` | `pipe.DropFunc[In]` | Renamed |
| `gopipe.NewProcessor` | `pipe.NewProcessor` | |
| `gopipe.StartProcessor` | `pipe.StartProcessor` | |
| `gopipe.NewProcessPipe` | `pipe.NewProcessPipe` | |
| `gopipe.NewTransformPipe` | `pipe.NewTransformPipe` | |
| `gopipe.NewFilterPipe` | `pipe.NewFilterPipe` | |
| `gopipe.NewBatchPipe` | `pipe.NewBatchPipe` | |
| `gopipe.NewSinkPipe` | `pipe.NewSinkPipe` | |
| `gopipe.ApplyPipe` | `pipe.ApplyPipe` | |
| `gopipe.MiddlewareFunc[In, Out]` | `pipe.MiddlewareFunc[In, Out]` | Generic middleware |
| `gopipe.Option[In, Out]` | `pipe.Option[In, Out]` | Deprecated |
| `gopipe.FanIn[T]` | `pipe.FanIn[T]` | |
| `gopipe.Generator[Out]` | `pipe.Generator[Out]` | Generic generator |
| Config structs | `pipe.ProcessorConfig` | Non-generic |

### To `middleware/` Package

| From | To | Notes |
|------|-----|-------|
| `gopipe.With*` options | `middleware.*` | As middleware |
| `middleware.MessageCorrelation` | `middleware.Correlation` | Simplified name |
| `middleware.NewMessageMiddleware` | `middleware.NewHandler` | |
| New: Logging, Metrics, Retry, Recover, Timeout, Drop | | |

### To `broker/` Package

| From | To | Notes |
|------|-----|-------|
| `message/broker.ChannelBroker` | `broker.Channel` | Simplified name |
| `message/broker.ChannelBrokerConfig` | `broker.ChannelConfig` | |
| HTTP broker | `broker.HTTP` | |
| IO broker | `broker.IO` | |

### To `cloudevents/` Package

| From | To | Notes |
|------|-----|-------|
| `message/cloudevents.*` | `cloudevents.*` | Unchanged |

### To `cqrs/` Package

| From | To | Notes |
|------|-----|-------|
| `message/cqrs.*` | `cqrs.*` | Unchanged |

## Type Aliases for Backward Compatibility

### In Main Package

```go
// gopipe/compat.go

// Deprecated: Use pipe.Processor instead.
type Processor[In, Out any] = pipe.Processor[In, Out]

// Deprecated: Use pipe.ProcessFunc instead.
type ProcessFunc[In, Out any] = pipe.ProcessFunc[In, Out]

// Deprecated: Use middleware.Retry instead.
func WithRetryConfig[In, Out any](cfg RetryConfig) Option[In, Out] {
    // Delegate to middleware
}
```

### In Message Package

```go
// message/compat.go

// Deprecated: Use gopipe.Router instead.
type Router = gopipe.Router

// Deprecated: Use gopipe.NewRouter instead.
var NewRouter = gopipe.NewRouter
```

## Generic Type Reduction

### Before (Current)

```go
// Everything is generic
pipe := gopipe.NewProcessPipe[Order, ShippingCmd](
    handler,
    gopipe.WithConcurrency[Order, ShippingCmd](4),
    gopipe.WithTimeout[Order, ShippingCmd](5*time.Second),
    gopipe.WithRetryConfig[Order, ShippingCmd](cfg),
)
```

### After (Proposed)

```go
// Message-based (no generics in common case)
router := gopipe.NewRouter(gopipe.RouterConfig{
    Concurrency: 4,
})
router.Register(orderHandler)
router.Register(paymentHandler)

// Generic only when needed (pipe package)
proc := pipe.NewProcessPipe(
    handler,
    nil,  // drop
    pipe.ProcessorConfig{Concurrency: 4},
    middleware.Timeout(middleware.TimeoutConfig{Duration: 5*time.Second}),
)
```

## Implementation Order

```
1. Create pipe/ package
   ├── Move generic types
   ├── Add ProcessorConfig
   └── Add deprecated wrappers in gopipe/

2. Restructure middleware/
   ├── Move implementations from gopipe/
   ├── Add Chain(), Apply()
   └── Add message-specific middleware

3. Promote message components to gopipe/
   ├── Router, Handler, Matcher
   ├── Publisher, Subscriber
   ├── Sender, Receiver interfaces
   └── Add deprecated wrappers in message/

4. Move broker/ to top level
   ├── Rename to simpler names
   └── Add deprecated wrappers

5. Move cloudevents/, cqrs/ to top level

6. Clean up message/ package
   ├── Keep only Message entity
   └── Remove promoted components

7. Update all imports
   ├── Internal code
   └── Examples and tests
```

## Breaking Changes

1. **Import paths change** - `message.Router` → `gopipe.Router`
2. **Generic pipes move** - `gopipe.NewProcessPipe` → `pipe.NewProcessPipe`
3. **Options deprecated** - `WithTimeout` → `middleware.Timeout`
4. **Broker paths change** - `message/broker` → `broker`

## Mitigation

1. **Deprecation period** - Keep old paths with deprecation warnings
2. **Type aliases** - Provide aliases for all moved types
3. **go fix tool** - Consider providing automated migration
4. **Version bump** - This is a v0.x → v0.y change (breaking allowed)

## Benefits

1. **Clearer focus** - Main package is message pipeline framework
2. **Simpler imports** - Common operations from `gopipe` directly
3. **Reduced generics** - Only `pipe/` and handler functions need generics
4. **Logical hierarchy** - Subpackages clearly scoped
5. **Better discoverability** - Router, Handler, Publisher at top level

## Risks

1. **Large refactoring** - Many files affected
2. **Import churn** - Users must update imports
3. **Circular dependencies** - Must be careful with package dependencies

## Dependency Graph (Proposed)

```
                    ┌──────────────┐
                    │   gopipe     │ ◄── Main package (messaging)
                    │  (Router,    │
                    │  Publisher,  │
                    │  Subscriber) │
                    └──────┬───────┘
                           │
         ┌─────────────────┼─────────────────┐
         │                 │                 │
         ▼                 ▼                 ▼
┌─────────────┐    ┌─────────────┐    ┌─────────────┐
│   message   │    │    pipe     │    │  middleware │
│  (Message,  │    │ (Processor, │    │  (Retry,    │
│  Attributes)│    │  Pipe[I,O]) │    │  Logging)   │
└─────────────┘    └──────┬──────┘    └─────────────┘
                          │
                          ▼
                   ┌─────────────┐
                   │   channel   │
                   │  (Merge,    │
                   │  Broadcast) │
                   └─────────────┘

┌─────────────┐    ┌─────────────┐    ┌─────────────┐
│   broker    │    │ cloudevents │    │    cqrs     │
│  (Channel,  │    │  (Event)    │    │ (Command,   │
│   HTTP)     │    │             │    │  Event)     │
└─────────────┘    └─────────────┘    └─────────────┘
       │                  │                  │
       └──────────────────┴──────────────────┘
                          │
                          ▼
                    ┌──────────────┐
                    │   gopipe     │ (implements interfaces)
                    └──────────────┘
```

## Acceptance Criteria

- [ ] `pipe/` package contains all generic processor/pipe types
- [ ] Main `gopipe/` package exports Router, Handler, Publisher, Subscriber
- [ ] `message/` package contains only Message entity and related types
- [ ] `middleware/` package contains all middleware implementations
- [ ] `broker/`, `cloudevents/`, `cqrs/` are top-level packages
- [ ] Deprecated wrappers provide backward compatibility
- [ ] All tests pass
- [ ] Examples updated to new import paths

## Related Documents

- [Layer 0: Foundation Cleanup](layer-0-foundation-cleanup.md)
- [Middleware Refactoring](middleware-refactoring/middleware-refactoring.md)
- [Cancel/Drop Path Refactoring](cancel-path-refactoring/cancel-path-refactoring.md)
