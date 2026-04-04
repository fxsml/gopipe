# ADR 0026: Acking Middleware Outermost

**Date:** 2026-04-04
**Status:** Proposed

## Context

The router composes user middleware around an acking middleware that ack/nacks the broker message based on processing outcome. Current order is `User → Acking → Handler` — user middleware wraps acking. This was fine when middleware had no side effects that needed to outlive the handler's return.

With TxMiddleware (handler-scoped transactions) and forthcoming inbox/outbox middleware, user middleware performs work AFTER the handler returns — specifically `tx.Commit()`. Under the current ordering, acking fires inside user middleware, so the broker ack happens BEFORE commit:

```
TxMiddleware (outer):
  BEGIN TX
  Acking (inner):
    handler()
    msg.Ack()         ← broker ack fires here
  tx.Commit()         ← if this fails, data is lost AND the broker already acked
```

This is unsafe. At-least-once delivery requires that commit succeed before the broker is told processing succeeded.

## Decision

Reverse middleware order: the acking middleware wraps user middleware.

```
Acking (outer):
  TxMiddleware (inner):
    BEGIN TX
    handler()
    tx.Commit()       ← commit first
  msg.Ack()           ← then broker ack — safe
```

Implementation in `router.Pipe()`:

```go
// Apply user middleware: first registered wraps outermost
fn := r.process
for i := len(r.middleware) - 1; i >= 0; i-- {
    fn = r.middleware[i](fn)
}

// Apply acking as outermost middleware (wraps everything)
fn = r.ackingMiddleware()(fn)
```

This matches Watermill's model where the subscriber/router controls acking around the entire processing chain.

**Semantics preserved:**
- `AckOnSuccess`: handler error → nack, handler success → ack (unchanged)
- `AckManual`: handler manages ack/nack (unchanged)
- `AckForward`: input acked when all outputs acked (unchanged)

**Semantics fixed:**
- Any side effect in user middleware that runs after `next()` returns is now guaranteed to complete before the broker is acked

## Consequences

**Breaking Changes:**
- User middleware that assumed it runs OUTSIDE acking scope will now run INSIDE it. If middleware called `msg.Ack()`/`msg.Nack()` explicitly, behavior may differ (should migrate to `AckManual` or rely on return-value semantics).

**Benefits:**
- Commit-before-ack safety for transactional middleware (TxMiddleware, inbox, outbox)
- User middleware can reliably perform post-handler work (flush logs, close spans, commit transactions)
- Aligns with Watermill's well-tested model
- Required foundation for the transaction-handling plan and inbox/outbox patterns

**Drawbacks:**
- Behavioral change that users of existing middleware must be aware of
- Middleware that manually ack/nacks now composes differently with the acking strategy

## Links

- Plan: [transaction-handling](../plans/transaction-handling.md) (Task 1)
- Related: ADR 0006 (Message Acknowledgment), ADR 0017 (Middleware for ProcessFunc), ADR 0025 (Message Context for In-Process Values)
- Enables: [inbox-outbox plan](../plans/inbox-outbox.md)
