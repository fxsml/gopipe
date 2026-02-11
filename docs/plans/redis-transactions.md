# Plan: Redis Transactions

**Status:** Proposed
**Depends On:** [transaction-handling](transaction-handling.md) (Tasks 0, 1, 2 — shared foundation)
**Related:** [inbox-outbox](inbox-outbox.md) (SQL counterpart)

## Overview

Redis transaction handling using TxPipeline middleware, inbox deduplication via SetNX, and outbox publishing via Redis Streams. Handlers remain TX-unaware. Adapters extract the pipeline from context and queue commands that execute atomically in MULTI/EXEC.

This plan covers the same patterns as the SQL inbox/outbox plan but adapted to Redis's command-oriented transaction model. It depends on the shared foundation from transaction-handling.md (message context field, middleware ordering, Done→Settled rename).

## Goals

1. Handler-scoped Redis TX via TxPipeline middleware (MULTI/EXEC, always short-lived)
2. Idempotent processing via inbox (SetNX with TTL, automatic expiry)
3. Reliable event publishing via outbox stream (XADD within same pipeline)
4. Redis 6.0+ compatible (no XAUTOCLAIM, no Redis Functions)
5. Handlers fully TX-unaware (hex architecture: adapters use pipeline, handlers use ports)
6. Clean adapter API via `Cmdable` interface (unified with or without TX)

## Redis vs SQL Transaction Model

SQL transactions are **ambient** — you BEGIN, execute arbitrary queries that return results, handle errors within the TX, and COMMIT. The TX reference sits in context; adapters extract it and use `ExecContext`/`QueryContext` unchanged.

Redis transactions (MULTI/EXEC) are **command-oriented** — commands are queued in a pipeline, then executed atomically as a batch. Individual command results are only available after EXEC. You cannot branch on a result inside MULTI.

| Concern | SQL | Redis |
|---|---|---|
| TX creation | `db.BeginTx()` → `*sql.Tx` | `client.TxPipeline()` → `redis.Pipeliner` |
| Command execution | Immediate (within TX) | Queued until `Exec()` |
| Error handling | Per-query errors available | Only after `Exec()` |
| Adapter interface | `Executor` (shared by DB and TX) | `redis.Cmdable` (shared by client and pipeline) |
| Atomicity | Full rollback on error | Commands execute as unit; individual failures don't roll back others |
| Inbox approach | INSERT with unique constraint (same TX) | SetNX with TTL (separate, outside TX) |

## Architecture

### Middleware Stack

```
Broker ──▶ Unmarshal ──▶ Router ──▶ Output/Publisher
                          │
              ┌───────────┴────────────────────────┐
              │  Acking middleware                   │  ← outermost
              │  ┌──────────────────────────────┐   │
              │  │  InboxMiddleware              │   │  ← SetNX check (outside TX)
              │  │  ┌────────────────────────┐   │   │
              │  │  │  TxMiddleware          │   │   │  ← TxPipeline / Exec
              │  │  │  ┌──────────────────┐  │   │   │
              │  │  │  │  OutboxMiddleware │  │   │   │  ← XADD to pipeline
              │  │  │  │  ┌────────────┐  │  │   │   │
              │  │  │  │  │  Handler   │  │  │   │   │  ← adapter queues commands
              │  │  │  │  └────────────┘  │  │   │   │
              │  │  │  └──────────────────┘  │   │   │
              │  │  └────────────────────────┘   │   │
              │  └──────────────────────────────┘   │
              └────────────────────────────────────┘
```

### Registration Order

```go
router.Use(
    msgredis.InboxMiddleware(client, msgredis.InboxConfig{Retention: 24 * time.Hour}),
    msgredis.TxMiddleware(client),
    msgredis.OutboxMiddleware(msgredis.OutboxConfig{Stream: "outbox:events"}),
)
```

First registered wraps outermost. Execution order:
1. **Acking** (framework): wraps everything, acks/nacks based on chain result
2. **InboxMiddleware**: SetNX check — duplicate → return nil, nil → acked
3. **TxMiddleware**: creates TxPipeline, puts in context, Exec on success
4. **OutboxMiddleware**: after handler returns, adds XADD to pipeline
5. **Handler**: adapter queues business commands on pipeline

### Component Responsibilities

| Component | Knows TX? | Responsibility |
|---|---|---|
| **TxMiddleware** | Yes — creates | Create TxPipeline, put in context, Exec/Discard |
| **InboxMiddleware** | No | SetNX check, delete on error |
| **OutboxMiddleware** | Yes — uses | Extract pipeline, XADD output events |
| **Handler** | **No** | Call ports (interfaces), pass ctx through |
| **Adapter** | Yes — uses | Extract pipeline via `CmdableFromContext`, queue commands |
| **Acking** | No | Ack/nack based on chain result |

### Why Inbox Is Outside the TX

In SQL, the inbox INSERT runs inside the same TX as business logic — the unique constraint violation is detected atomically. In Redis, MULTI doesn't support branching: all commands are queued, results are only available after EXEC. You cannot check "does inbox key exist?" and conditionally skip the handler inside MULTI.

SetNX is atomic by itself (single command, no race conditions). Placing it outside the TxPipeline keeps the design simple. The tradeoff is a small consistency gap (see below).

## The Inbox Gap

### Normal Flow (no crash)

```
1. InboxMiddleware: SetNX("inbox:{msgID}", "1", 24h) → OK (first time)
2. TxMiddleware: creates pipeline
3. Handler: adapter queues commands on pipeline
4. OutboxMiddleware: adds XADD to pipeline
5. TxMiddleware: pipeline.Exec() → MULTI, HSET, XADD, EXEC → success
6. Acking: msg.Ack() → broker acknowledges
```

All good. Business commands and outbox write execute atomically in EXEC. Broker acks after EXEC.

### Crash Between Inbox and EXEC

```
1. InboxMiddleware: SetNX("inbox:{msgID}", "1", 24h) → OK
2. TxMiddleware: creates pipeline
3. ─── PROCESS CRASHES ───
```

State after crash:
- **Inbox key exists** (SetNX succeeded)
- **Business commands did NOT execute** (EXEC never happened)
- **Outbox entry does NOT exist** (XADD was queued but not executed)
- **Broker message is NOT acked** (acking middleware never ran)

On redelivery (broker redelivers unacked message):
- InboxMiddleware: SetNX("inbox:{msgID}") → **key exists** → duplicate → acked without processing

**The gap:** The message is treated as "already processed" even though business logic didn't run. The message is lost until the inbox key expires.

### TTL-Based Recovery

The inbox key has a TTL (e.g., 24h). After expiry:
- Broker redelivers the message (still unacked, reclaimed via XPENDING + XCLAIM)
- InboxMiddleware: SetNX("inbox:{msgID}") → **key doesn't exist** → processes normally

**Recovery time is bounded by the inbox TTL.** With 24h TTL, the maximum gap is 24h. This is acceptable because:
1. Process crashes between SetNX and EXEC are rare (millisecond window)
2. The broker retains the message in its pending list until acked
3. After TTL expiry, the message is automatically reprocessed

### Why Not Atomic Inbox?

WATCH-based optimistic locking could make inbox + business atomic:

```
WATCH inbox:{msgID}
GET inbox:{msgID}          → if exists, UNWATCH, return duplicate
MULTI
  SET inbox:{msgID} "1" EX ttl
  ...business commands...
  XADD outbox stream ...
EXEC                       → nil if WATCH key changed (retry)
```

This eliminates the gap but adds complexity:
- Two round trips before EXEC (WATCH + GET)
- Retry loop on WATCH conflicts (concurrent consumers claim same message)
- TxPipeline middleware must use `client.Watch()` callback instead of simple `client.TxPipeline()`
- Inbox key must be on same slot as business keys (Redis Cluster constraint)

The simpler SetNX approach is preferred. The gap is bounded by TTL and only affects crash scenarios (not application errors — see below).

### Handler Error Recovery

When the handler returns an error:

```
1. InboxMiddleware: SetNX → OK
2. TxMiddleware: creates pipeline
3. Handler: returns error
4. TxMiddleware: pipeline.Discard() (no EXEC)
5. InboxMiddleware: DEL("inbox:{msgID}") ← removes inbox entry
6. Acking: msg.Nack(err) → broker will redeliver
```

The inbox entry is **deleted on handler error**, ensuring the message is retried. Only successful EXEC (commit) keeps the inbox entry. Application errors do NOT cause the gap — only process crashes do.

## Tasks

### Task 1: Redis Context Helpers

**Goal:** Provide pipeline context helpers and the `Cmdable` interface for adapters. The Redis equivalent of Task 3 in transaction-handling.md.

**Implementation:**
```go
// message/redis/context.go
package redis

import (
    "context"

    goredis "github.com/redis/go-redis/v9"
)

type pipelineKey struct{}

// ContextWithPipeline adds a Redis pipeline to the context.
func ContextWithPipeline(ctx context.Context, pipe goredis.Pipeliner) context.Context {
    return context.WithValue(ctx, pipelineKey{}, pipe)
}

// PipelineFromContext extracts the Redis pipeline from the context.
func PipelineFromContext(ctx context.Context) (goredis.Pipeliner, bool) {
    pipe, ok := ctx.Value(pipelineKey{}).(goredis.Pipeliner)
    return pipe, ok
}

// CmdableFromContext returns the pipeline from context if available,
// otherwise returns the fallback client. Both satisfy redis.Cmdable,
// giving adapters a single API for queuing or executing commands.
func CmdableFromContext(ctx context.Context, fallback goredis.Cmdable) goredis.Cmdable {
    if pipe, ok := PipelineFromContext(ctx); ok {
        return pipe
    }
    return fallback
}
```

**Files to Create:**
- `message/redis/context.go` — pipeline context helpers
- `message/redis/context_test.go`

**Acceptance Criteria:**
- [ ] `ContextWithPipeline` / `PipelineFromContext` round-trip works
- [ ] `CmdableFromContext` returns pipeline when present, fallback when absent
- [ ] Both `redis.UniversalClient` and `redis.Pipeliner` satisfy the returned interface

### Task 2: Redis TxPipeline Middleware

**Goal:** Handler-scoped Redis transaction middleware. Creates TxPipeline before handler, Exec on success, Discard on error.

**Implementation:**
```go
// message/redis/tx_middleware.go
func TxMiddleware(client goredis.UniversalClient) message.Middleware {
    return func(next message.ProcessFunc) message.ProcessFunc {
        return func(ctx context.Context, msg *message.Message) ([]*message.Message, error) {
            pipe := client.TxPipeline()

            msg.WithValue(pipelineKey{}, pipe)

            outputs, err := next(ctx, msg)
            if err != nil {
                pipe.Discard()
                return nil, err
            }

            if _, err := pipe.Exec(ctx); err != nil {
                return nil, fmt.Errorf("redis tx exec: %w", err)
            }

            return outputs, nil
        }
    }
}
```

**Key design:**
- Uses `msg.WithValue` to attach pipeline (same pattern as SQL TxMiddleware for `*sql.Tx`)
- Pipeline auto-propagates into handler context via `messageContext.Value()`
- Adapter extracts via `CmdableFromContext(ctx, r.client)`
- Commands queued during handler execute atomically in EXEC
- Discard on handler error prevents partial execution

**Note on atomicity:** Redis MULTI/EXEC guarantees isolation (no interleaving) but not full rollback. If one command in the pipeline fails at execution (e.g., WRONGTYPE), other commands still execute. In practice, write commands (SET, HSET, XADD) rarely fail unless there's a key type mismatch or OOM — both indicate bugs, not transient errors.

**Files to Create:**
- `message/redis/tx_middleware.go`
- `message/redis/tx_middleware_test.go`

**Acceptance Criteria:**
- [ ] Pipeline created before handler, Exec'd after handler success
- [ ] Pipeline discarded on handler error (no partial execution)
- [ ] Pipeline available to adapters via `CmdableFromContext(ctx, client)` inside handler
- [ ] Handler code has zero Redis imports

### Task 3: Redis Inbox Middleware

**Goal:** Idempotent message processing via SetNX with TTL. Prevents duplicate processing when messages are redelivered.

**Implementation:**
```go
// message/redis/inbox_middleware.go

// InboxConfig configures the inbox middleware.
type InboxConfig struct {
    // Retention is the TTL for inbox keys (default: 24h).
    // After expiry, redelivered messages are processed again.
    Retention time.Duration
    // KeyFunc derives the inbox key from the message.
    // Default: "inbox:{msg.ID()}"
    KeyFunc func(msg *message.Message) string
}

func InboxMiddleware(client goredis.UniversalClient, cfg InboxConfig) message.Middleware {
    if cfg.Retention <= 0 {
        cfg.Retention = 24 * time.Hour
    }
    if cfg.KeyFunc == nil {
        cfg.KeyFunc = func(msg *message.Message) string {
            return "inbox:" + msg.ID()
        }
    }

    return func(next message.ProcessFunc) message.ProcessFunc {
        return func(ctx context.Context, msg *message.Message) ([]*message.Message, error) {
            key := cfg.KeyFunc(msg)

            // SetNX: atomic check-and-set. Returns true if key was set (first time).
            set, err := client.SetNX(ctx, key, "1", cfg.Retention).Result()
            if err != nil {
                return nil, fmt.Errorf("inbox setnx: %w", err)
            }
            if !set {
                // Duplicate: key already exists → skip processing
                return nil, nil
            }

            // Process message
            outputs, err := next(ctx, msg)
            if err != nil {
                // Handler failed: delete inbox entry to allow retry
                client.Del(ctx, key) // best-effort, ignore error
                return nil, err
            }

            return outputs, nil
        }
    }
}
```

**Key design:**
- **SetNX is atomic**: single command, no race conditions between check and set
- **TTL provides auto-cleanup**: no explicit cleanup job needed, inbox entries self-expire
- **Delete on error**: handler failure removes inbox entry, allowing broker redelivery
- **nil, nil for duplicates**: acking middleware acks the message (consistent with SQL inbox)
- **Configurable key function**: allows custom dedup keys (e.g., based on idempotency header)
- **Outside TX**: SetNX executes independently, not part of TxPipeline (see "The Inbox Gap")

**Files to Create:**
- `message/redis/inbox_middleware.go`
- `message/redis/inbox_middleware_test.go`

**Acceptance Criteria:**
- [ ] First-time messages: SetNX succeeds, handler runs
- [ ] Duplicate messages: SetNX fails, returns nil, nil (acked by acking middleware)
- [ ] Handler errors: inbox entry deleted, message redeliverable
- [ ] Inbox keys expire after configured retention
- [ ] Configurable key derivation

### Task 4: Redis Outbox Middleware

**Goal:** Reliable event publishing by writing output events to a Redis Stream within the same TxPipeline as business commands.

**Implementation:**
```go
// message/redis/outbox_middleware.go

// OutboxConfig configures the outbox middleware.
type OutboxConfig struct {
    // Stream is the Redis Stream name for outbox events (required).
    Stream string
    // MarshalFunc serializes a message for the outbox stream entry.
    // Default: JSON marshal of type, source, data, and attributes.
    MarshalFunc func(msg *message.Message) (map[string]any, error)
}

func OutboxMiddleware(cfg OutboxConfig) message.Middleware {
    if cfg.MarshalFunc == nil {
        cfg.MarshalFunc = defaultMarshal
    }

    return func(next message.ProcessFunc) message.ProcessFunc {
        return func(ctx context.Context, msg *message.Message) ([]*message.Message, error) {
            outputs, err := next(ctx, msg)
            if err != nil {
                return nil, err
            }

            // Extract pipeline from context (set by TxMiddleware)
            pipe, ok := PipelineFromContext(msg.Context(ctx))
            if !ok {
                return nil, fmt.Errorf("outbox: no pipeline in context (TxMiddleware required)")
            }

            // Add XADD for each output event to the pipeline
            for _, out := range outputs {
                values, err := cfg.MarshalFunc(out)
                if err != nil {
                    return nil, fmt.Errorf("outbox marshal: %w", err)
                }
                pipe.XAdd(ctx, &goredis.XAddArgs{
                    Stream: cfg.Stream,
                    Values: values,
                })
            }

            // Swallow outputs — they'll be published by OutboxPublisher
            return nil, nil
        }
    }
}
```

**Key design:**
- **Requires TxMiddleware**: errors if no pipeline in context. Outbox without TX is not atomic with business.
- **Swallows outputs**: returns nil instead of output messages. Events are written to the outbox stream, not passed downstream. The OutboxPublisher forwards them to the destination broker.
- **XADD on pipeline**: queued on the same pipeline as business commands. When TxMiddleware calls Exec(), business writes and outbox writes execute atomically in MULTI/EXEC.
- **Configurable marshaling**: default marshals to JSON. Custom marshalers can use msgpack, protobuf, etc.

**Files to Create:**
- `message/redis/outbox_middleware.go`
- `message/redis/outbox_middleware_test.go`

**Acceptance Criteria:**
- [ ] Output events added as XADD to pipeline (same TX as business)
- [ ] Outputs swallowed (not passed to downstream pipeline)
- [ ] Error if no pipeline in context (TxMiddleware required)
- [ ] Events written to configured stream
- [ ] Configurable marshaling

### Task 5: Redis Outbox Publisher

**Goal:** Background publisher that reads events from the outbox stream and forwards them to the destination broker. Uses Redis Streams consumer groups for reliable at-least-once delivery.

**Implementation:**
```go
// message/redis/outbox_publisher.go

// OutboxPublisherConfig configures the outbox publisher.
type OutboxPublisherConfig struct {
    // Client is the Redis client.
    Client goredis.UniversalClient
    // Stream is the outbox stream name (must match OutboxConfig.Stream).
    Stream string
    // Group is the consumer group name (default: "outbox-publisher").
    Group string
    // Consumer is this instance's consumer name (default: hostname).
    Consumer string
    // BatchSize is the max messages to read per XREADGROUP (default: 10).
    BatchSize int64
    // BlockTimeout is the XREADGROUP block duration (default: 1s).
    BlockTimeout time.Duration
    // ClaimInterval is how often to check for idle messages (default: 30s).
    ClaimInterval time.Duration
    // MaxIdleTime is the idle threshold for claiming messages (default: 60s).
    MaxIdleTime time.Duration
    // UnmarshalFunc deserializes outbox stream entries into a message and destination.
    UnmarshalFunc func(values map[string]any) (*message.RawMessage, string, error)
}

// OutboxPublisher reads from the outbox Redis Stream and emits messages
// for forwarding to the destination broker.
type OutboxPublisher struct {
    cfg OutboxPublisherConfig
}

// Source returns a channel of messages read from the outbox stream.
// Each message's acking maps to XACK on the outbox stream.
// Runs until ctx is cancelled.
func (p *OutboxPublisher) Source(ctx context.Context) (<-chan *message.RawMessage, error) {
    // 1. Create consumer group (XGROUP CREATE ... MKSTREAM)
    // 2. Start read loop (XREADGROUP with ">")
    // 3. Start claim loop (XPENDING + XCLAIM for idle messages)
    // 4. Unmarshal entries, emit as *RawMessage
    // 5. On msg.Ack() → XACK on outbox stream
    // 6. On msg.Nack() → no XACK, message stays in PEL for reclaim
}
```

**Idle message claiming (Redis 6.0 compatible):**
```go
// XAUTOCLAIM requires Redis 6.2+. For 6.0 compatibility, use XPENDING + XCLAIM.
func (p *OutboxPublisher) claimIdle(ctx context.Context) ([]goredis.XMessage, error) {
    pending, err := p.cfg.Client.XPendingExt(ctx, &goredis.XPendingExtArgs{
        Stream: p.cfg.Stream,
        Group:  p.cfg.Group,
        Idle:   p.cfg.MaxIdleTime,
        Start:  "-",
        End:    "+",
        Count:  p.cfg.BatchSize,
    }).Result()
    if err != nil {
        return nil, err
    }

    var ids []string
    for _, pe := range pending {
        ids = append(ids, pe.ID)
    }
    if len(ids) == 0 {
        return nil, nil
    }

    return p.cfg.Client.XClaim(ctx, &goredis.XClaimArgs{
        Stream:   p.cfg.Stream,
        Group:    p.cfg.Group,
        Consumer: p.cfg.Consumer,
        MinIdle:  p.cfg.MaxIdleTime,
        Messages: ids,
    }).Result()
}
```

**Key design:**
- **Consumer groups for reliability**: XREADGROUP tracks which messages have been delivered. Unacked messages remain in the Pending Entries List (PEL) for reclaim.
- **XPENDING + XCLAIM for Redis 6.0**: XAUTOCLAIM (6.2+) is not available on Azure Redis 6.0. The two-step approach achieves the same result.
- **Ack maps to XACK**: when the downstream broker confirms delivery, the outbox entry is acknowledged and removed from the PEL.
- **Nack leaves in PEL**: failed deliveries are retried via the claim loop when they exceed MaxIdleTime.

**Files to Create:**
- `message/redis/outbox_publisher.go`
- `message/redis/outbox_publisher_test.go`

**Acceptance Criteria:**
- [ ] Reads from outbox stream via XREADGROUP consumer group
- [ ] Emits RawMessage with acking bound to XACK
- [ ] Idle messages reclaimed via XPENDING + XCLAIM (Redis 6.0 compatible)
- [ ] Consumer group auto-created on start
- [ ] Configurable batch size, block timeout, claim intervals

## Implementation Order

```
transaction-handling.md Tasks 0, 1, 2 (shared foundation)
  ↓
Task 1: Redis Context Helpers
  ↓
Task 2: TxPipeline Middleware     Task 3: Inbox Middleware
            ↓                               ↓
            └───────────────────────────────┘
                          ↓
              Task 4: Outbox Middleware
                          ↓
              Task 5: Outbox Publisher
```

Tasks 2 and 3 are independent (inbox doesn't use the pipeline). Task 4 depends on both (outbox uses pipeline, often paired with inbox). Task 5 depends on Task 4 (reads what outbox writes).

## End-to-End Trace

### Setup

```go
client := goredis.NewClient(&goredis.Options{Addr: "localhost:6379"})

router := message.NewRouter(message.PipeConfig{
    AckStrategy: message.AckOnSuccess,
})
router.Use(
    msgredis.InboxMiddleware(client, msgredis.InboxConfig{Retention: 24 * time.Hour}),
    msgredis.TxMiddleware(client),
    msgredis.OutboxMiddleware(msgredis.OutboxConfig{Stream: "outbox:events"}),
)
router.AddHandler("place-order", nil, NewPlaceOrderHandler(orderRepo))
```

### Execution Flow

```
1.  Broker delivers message (msg.ID = "abc-123")
2.  Router receives message
3.  Acking middleware (outermost) calls next:
4.    InboxMiddleware: SetNX("inbox:abc-123", "1", 24h) → true (first time)
5.      TxMiddleware: pipe = client.TxPipeline(), msg.WithValue(pipelineKey, pipe)
6.        OutboxMiddleware calls next:
7.          Handler: repo.Save(ctx, order) → adapter queues HSET on pipeline
8.          Handler returns: []OrderPlaced{{OrderID: "xyz"}}
9.        OutboxMiddleware: pipe.XAdd("outbox:events", {type: "order.placed", ...})
10.       OutboxMiddleware returns: nil, nil (outputs swallowed)
11.     TxMiddleware: pipe.Exec(ctx) → MULTI, HSET, XADD, EXEC → success
12.   InboxMiddleware returns: nil, nil
13. Acking middleware: msg.Ack() → broker ack (AFTER Exec)
```

### Duplicate Handling

```
1.  Broker redelivers message (msg.ID = "abc-123", same as before)
2.  Router receives message
3.  Acking middleware calls next:
4.    InboxMiddleware: SetNX("inbox:abc-123") → false (key exists)
5.    InboxMiddleware returns: nil, nil (duplicate)
6.  Acking middleware: msg.Ack() → broker ack (skipped processing)
```

### Handler (application layer — TX-unaware)

```go
func NewPlaceOrderHandler(repo OrderRepository) message.Handler {
    return message.NewCommandHandler(
        func(ctx context.Context, cmd PlaceOrder) ([]OrderPlaced, error) {
            order := NewOrder(cmd)
            if err := repo.Save(ctx, order); err != nil {
                return nil, err
            }
            return []OrderPlaced{{OrderID: order.ID}}, nil
        },
        message.CommandHandlerConfig{Source: "/orders"},
    )
}
```

### Adapter (infrastructure layer — TX-aware)

```go
type RedisOrderRepo struct{ client goredis.UniversalClient }

func (r *RedisOrderRepo) Save(ctx context.Context, order Order) error {
    cmd := msgredis.CmdableFromContext(ctx, r.client)
    return cmd.HSet(ctx, "order:"+order.ID, map[string]any{
        "product_id": order.ProductID,
        "quantity":   order.Quantity,
        "status":     "placed",
    }).Err()
}
```

When `TxMiddleware` is active, `CmdableFromContext` returns the pipeline — `HSet` queues the command (returns nil). When no middleware is active, it returns the client — `HSet` executes immediately and returns any error.

## Watermill Comparison

Watermill's Redis driver (`watermill-redisstream`) uses Redis Streams for pub/sub:

| Concern | Watermill | gopipe (this plan) |
|---|---|---|
| Redis primitive | Streams (XADD/XREADGROUP) | Streams + TxPipeline |
| Delivery guarantee | At-least-once | At-least-once + inbox dedup |
| Transaction support | None (uses SQL Forwarder) | Native via TxPipeline |
| Outbox pattern | SQL-based Forwarder component | Redis Stream within same TxPipeline |
| Nack handling | In-memory re-send loop | Broker redelivery |
| Idle message recovery | XPENDING + XCLAIM | XPENDING + XCLAIM |
| Marshaling | msgpack metadata + raw payload | Configurable (default: JSON) |

Key insight: Watermill keeps Redis transactions **outside** the Redis driver entirely. Their outbox pattern uses a SQL Forwarder — write to SQL outbox table within SQL TX, then a daemon forwards to Redis Streams. gopipe's approach is more native: write to Redis outbox stream within the same TxPipeline as business commands, no SQL involved.

## Open Questions

1. **go-redis dependency**: The `message/redis` package adds `github.com/redis/go-redis/v9` as a dependency. Should this be a separate Go module to keep the core dependency-free?
2. **Redis Cluster**: TxPipeline (MULTI/EXEC) requires all keys to be on the same slot. Should the plan address hash tags (`{tag}`) for cluster compatibility, or defer to documentation?
3. **Outbox stream trimming**: The outbox stream grows with processed entries. Should OutboxPublisher trim after XACK (XDEL)? Or use MAXLEN on XADD for approximate trimming?
4. **Redis Streams broker**: This plan covers Redis as a transaction/outbox store. A separate plan could cover Redis Streams as a general-purpose message broker (subscriber + publisher) for gopipe.
5. **Outbox entry format**: Should the default marshal include the destination topic in the stream entry, or should there be one outbox stream per destination?

## Acceptance Criteria

- [ ] All tasks completed
- [ ] Tests pass (`make test`)
- [ ] Build passes (`make build && make vet`)
- [ ] Handler code has zero Redis imports
- [ ] Pipeline executes atomically (business + outbox in same MULTI/EXEC)
- [ ] Inbox dedup prevents duplicate processing
- [ ] Inbox entries auto-expire (TTL)
- [ ] Inbox entries deleted on handler error (retry allowed)
- [ ] Outbox publisher reclaims idle messages (Redis 6.0 compatible)
- [ ] Commit-before-ack ordering maintained
