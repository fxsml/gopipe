# ADR 0023: Engine Simplification

**Date:** 2025-01-28
**Status:** Implemented

## Context

The message engine accumulated complexity: messageTracker, multi-pool routing, auto-acking, and loopback plugins. Loopbacks were particularly problematic—under high volume, buffer exhaustion causes deadlocks due to circular dependencies (merger → router → distributor → loopback → merger).

## Decision

Remove complex features and simplify shutdown:

1. **Remove messageTracker** — shutdown is timeout-based only
2. **Remove multi-pool support** — single `RouterPool` config
3. **Remove auto-acking** — handler responsibility (AutoAck middleware available)
4. **Remove loopback plugins** — use external message queues instead
5. **Consistent ShutdownTimeout** — `<= 0` forces immediate, `> 0` waits then forces

```go
// Handler acks explicitly
func (h *MyHandler) Handle(ctx context.Context, msg *Message) ([]*Message, error) {
    result, err := process(msg)
    if err != nil {
        msg.Nack(err)
        return nil, err
    }
    msg.Ack()
    return result, nil
}
```

## Consequences

**Breaking Changes:**
- Removed: `AddPoolWithConfig`, `AddHandlerToPool`, `AddLoopbackInput`, `AddLoopbackOutput`, `AdjustInFlight`
- Removed: `plugin.Loopback`, `plugin.ProcessLoopback`, `plugin.BatchLoopback`, `plugin.GroupLoopback`

**Benefits:**
- ~320 lines less code
- No deadlock risk from loopback resource exhaustion
- Simpler mental model: merger → router → distributor

**Drawbacks:**
- No in-process loopbacks (use external queue)
- Handlers must ack explicitly (or use middleware)

## Links

- Related: [ADR 0020](0020-message-engine-architecture.md)
- Related: [ADR 0022](0022-message-package-redesign.md)
- Plan: [0012](../plans/archive/0012-engine-simplification.md)
