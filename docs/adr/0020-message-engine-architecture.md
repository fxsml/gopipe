# ADR 0020: Message Engine Architecture

**Date:** 2025-12-22
**Status:** Implemented (v0.11.0) — see [ADR 0022](0022-message-package-redesign.md)

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
3. Routes by CE type to handlers (via Marshaler's type registry)
4. FanOut routes outputs via Matcher patterns
5. Loopback via `AddLoopback()` skips marshal/unmarshal
6. Type registry in Marshaler with auto-registration via NamingStrategy
7. Sources can be added/removed at runtime with individual contexts

## MVP Requirements

1. **Engine accepts channels** - External code manages subscription lifecycle
2. **Pattern-based output routing** - Matcher interface with SQL LIKE syntax
3. **Auto-routing via NamingStrategy** - Handler EventType → CE type
4. **Loopback** - Internal re-processing without marshal/unmarshal

## Future Requirements (Optional)

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
