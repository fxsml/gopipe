# ADR 0020: Message Engine Architecture

**Date:** 2025-12-22
**Status:** Proposed

## Context

The current message package requires manual wiring of components. Handlers receive `[]byte` and must unmarshal themselves. There's no internal message loop for handler-to-handler routing.

## Decision

Introduce a Message Engine that orchestrates message flow:

```
Subscribers → Unmarshal → FanIn → Router → Handler ─┐
                           ↑                        ↓
                           │                     FanOut ← Generators
                           │                     /    \
                           └── internal (typed) ┘      └→ Marshal → Publishers
```

1. Marshals only at external boundaries (`[]byte` ↔ Go types)
2. FanIn merges subscriber channels (after unmarshal)
3. Routes by Go type using `reflect.Type`
4. FanOut routes outputs: internal (typed) or external (marshaled)
5. Generators feed directly into FanOut (already typed)
6. Internal loopback via `gopipe://` skips marshal/unmarshal
7. Auto-generates type registry on `Start()` from handlers
8. Sources can be added/removed at runtime with individual contexts

## Requirements

1. **Dynamic Sources**: Add/remove subscribers at runtime
   - Use case: Leader election - only leader subscribes, unsubscribe on leadership loss

2. **Generators**: Internal message sources (Ticker, Cron, Once)
   - Interface: `Generate(ctx) (<-chan *Message, error)` (no topic)

3. **Individual Context Control**: Each source has independent lifecycle
   - Start/stop sources without stopping the engine
   - Each source respects its own context for cancellation

4. **Named Subscribers**: Register by name, subscribe to topics dynamically
   - `AddSubscriber(name, sub)` then `Subscribe(ctx, name, topics...)`
   - Generator: `Generate(ctx) (<-chan *Message, error)`

## Consequences

**Benefits:**
- Single component wires subscriber → router → publisher
- Handlers receive typed data, not `[]byte`
- Internal routing without external broker

**Drawbacks:**
- Runtime type dispatch
- More complex than manual wiring for simple cases

## Links

- Related: ADR 0018 (Interface Naming Conventions)
- Related: ADR 0019 (Remove Sender/Receiver)
- Related: ADR 0021 (Marshaler Pattern)
- Plan: [0001-message-engine.md](../plans/0001-message-engine.md)
