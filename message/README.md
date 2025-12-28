# message

CloudEvents message handling with type-based routing.

## Overview

The `message` package provides:

- **Message** - CloudEvents-aligned message with typed data and attributes
- **Engine** - Orchestrates message flow between inputs, handlers, and outputs
- **Handler** - Type-safe command/event handlers with automatic marshaling

## Engine Architecture

```
Input → unmarshal → Merger → handler → Distributor → marshal → Output
        (pipe)      (merge)   (pipe)    (route)       (pipe)
```

The Engine uses `pipe.Merger` for input merging and `pipe.Distributor` for output routing:

- **Merger** combines multiple input channels into a single stream
- **Distributor** routes messages to outputs using first-match-wins semantics
- Loopback messages feed back into the Merger for re-processing

## Usage

```go
engine := message.NewEngine(message.EngineConfig{
    Marshaler: message.NewJSONMarshaler(),
})

// Register handlers
handler := message.NewCommandHandler(
    func(ctx context.Context, cmd OrderCommand) ([]OrderEvent, error) {
        return []OrderEvent{{ID: cmd.ID, Status: "created"}}, nil
    },
    message.CommandHandlerConfig{
        Source: "/orders",
        Naming: message.KebabNaming,
    },
)
engine.AddHandler(handler, message.HandlerConfig{Name: "orders"})

// Add inputs and outputs
input := make(chan *message.RawMessage, 100)
engine.AddInput(input, message.InputConfig{Name: "orders-in"})
output := engine.AddOutput(message.OutputConfig{Name: "orders-out"})

// Start engine
ctx, cancel := context.WithCancel(context.Background())
defer cancel()
done, _ := engine.Start(ctx)

// Send/receive messages
input <- &message.RawMessage{
    Data:       []byte(`{"id": "123"}`),
    Attributes: message.Attributes{"type": "order.command"},
}

out := <-output
// out.Data contains marshaled OrderEvent
```

## Dynamic Input/Output

Inputs and outputs can be added after Start():

```go
engine.Start(ctx)

// Add new input dynamically
newInput := make(chan *message.RawMessage, 100)
engine.AddInput(newInput, message.InputConfig{Name: "new-input"})

// Add new output dynamically
newOutput := engine.AddOutput(message.OutputConfig{
    Matcher: &myMatcher{},
})
```

## Loopback

Loopback allows messages to be re-processed without marshaling:

```go
engine.AddLoopback(message.LoopbackConfig{
    Matcher: &typeMatcher{pattern: "intermediate.event"},
})
```

Messages matching the loopback criteria are fed back to the handler pipeline, skipping marshal/unmarshal for efficiency.

## Message Types

### RawMessage

Raw bytes with CloudEvents attributes:

```go
type RawMessage = TypedMessage[[]byte]
```

### Message

Typed message with unmarshaled data:

```go
msg := &message.Message{
    Data:       myStruct,
    Attributes: message.Attributes{
        "type":   "order.created",
        "source": "/orders",
    },
}
```

### Attributes

CloudEvents-aligned attribute keys:

```go
const (
    AttrID              = "id"
    AttrType            = "type"
    AttrSource          = "source"
    AttrSubject         = "subject"
    AttrTime            = "time"
    AttrDataContentType = "datacontenttype"
    AttrDataSchema      = "dataschema"
    AttrSpecVersion     = "specversion"
)
```

## Handlers

### CommandHandler

Processes commands and returns events:

```go
handler := message.NewCommandHandler(
    func(ctx context.Context, cmd CreateOrder) ([]OrderCreated, error) {
        return []OrderCreated{{OrderID: cmd.ID}}, nil
    },
    message.CommandHandlerConfig{
        Source: "/orders",
        Naming: message.KebabNaming,
    },
)
```

### Handler Interface

```go
type Handler interface {
    EventType() string
    NewInput() any
    Handle(ctx context.Context, msg *Message) ([]*Message, error)
}
```

## TODO: Architecture Rethink

### Current Issues

1. **Context stored in Engine struct** - Storing `ctx context.Context` in the Engine struct is bad Go idiom. Context should flow through function calls, not be stored in structs.

2. **Per-input unmarshal pipes** - Each input creates its own unmarshal pipe. This duplicates logic and complicates AddInput.

3. **Unnecessary channel.Cancel wrapper** - Inputs are wrapped with `channel.Cancel` but pipes already handle context cancellation internally.

4. **Complex AddInput/AddOutput after Start** - Dynamic add requires checking `started` state and either storing for later or immediately wiring to merger/distributor.

### Proposed Simplification

Current architecture:
```
Input₁ → Cancel → Filter → Unmarshal₁ ─┐
Input₂ → Cancel → Filter → Unmarshal₂ ─┼→ Merger[*Message] → Handler → Distributor → Marshal → Output
Input₃ → Cancel → Filter → Unmarshal₃ ─┘
```

Simplified architecture:
```
Input₁ → Filter₁ ─┐
Input₂ → Filter₂ ─┼→ Merger[*RawMessage] → Unmarshal → Handler → Distributor → Marshal → Output
Input₃ → Filter₃ ─┘
```

Benefits:
- One unmarshal pipe instead of per-input
- Remove channel.Cancel (pipes handle ctx)
- Merger operates on `*RawMessage` (simpler type flow)
- AddInput just adds to merger directly

### Questions to Resolve

1. Can we keep dynamic AddInput/AddOutput API without storing ctx in struct?
   - Option: Pass ctx to AddInput/AddOutput when called after Start
   - Option: Use internal context derived from Start's ctx

2. Should unmarshal happen before or after merge?
   - Before: Current approach, per-input pipes
   - After: Simpler, one pipe, but all inputs must use same marshaler

3. Are channel helpers needed or do pipe components suffice?
   - `channel.Process` - still useful for filtering with side effects
   - `channel.Cancel` - redundant if using pipes

### Next Steps

- [ ] Evaluate if single unmarshal pipe (after merge) works for all use cases
- [ ] Remove ctx from Engine struct, pass through function calls
- [ ] Simplify AddInput to just add to merger
- [ ] Consider if channel.Cancel adds value over pipe cancellation
