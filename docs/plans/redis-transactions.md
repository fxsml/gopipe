# Plan: Redis Integration

**Status:** Proposed
**Depends On:** [transaction-handling](transaction-handling.md) (Tasks 0, 1, 2 — shared foundation)
**Related:** [inbox-outbox](inbox-outbox.md) (SQL counterpart)

## Overview

Complete Redis integration for gopipe: standalone Redis Streams pub/sub (subscriber + publisher) and transaction middleware (TxPipeline, outbox). The standalone broker provides a persistent buffer channel — a lighter alternative to Azure Service Bus or similar managed brokers. The middleware layer adds transactional guarantees on top.

All components live in `message/redis/` and use `go-redis/v9` with `UniversalClient` (standalone, sentinel, or cluster).

## Goals

1. **Standalone Redis Streams pub/sub**: subscriber (XREADGROUP) and publisher (XADD) as drop-in replacements for any broker
2. Handler-scoped Redis TX via TxPipeline middleware (MULTI/EXEC, always short-lived)
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

## Architecture

### Middleware Stack

```
Broker ──▶ Unmarshal ──▶ Router ──▶ Output/Publisher
                          │
              ┌───────────┴────────────────────────┐
              │  Acking middleware                   │  ← outermost
              │  ┌──────────────────────────────┐   │
              │  │  TxMiddleware                │   │  ← TxPipeline / Exec
              │  │  ┌────────────────────────┐  │   │
              │  │  │  OutboxMiddleware      │  │   │  ← XADD to pipeline
              │  │  │  ┌──────────────────┐  │  │   │
              │  │  │  │  Handler         │  │  │   │  ← adapter queues commands
              │  │  │  └──────────────────┘  │  │   │
              │  │  └────────────────────────┘  │   │
              │  └──────────────────────────────┘   │
              └────────────────────────────────────┘
```

### Registration Order

```go
router.Use(
    msgredis.TxMiddleware(client),
    msgredis.OutboxMiddleware(msgredis.OutboxConfig{Stream: "outbox:events"}),
)
```

First registered wraps outermost. Execution order:
1. **Acking** (framework): wraps everything, acks/nacks based on chain result
2. **TxMiddleware**: creates TxPipeline, puts in context, Exec on success
3. **OutboxMiddleware**: after handler returns, adds XADD to pipeline
4. **Handler**: adapter queues business commands on pipeline

### Component Responsibilities

| Component | Knows TX? | Responsibility |
|---|---|---|
| **TxMiddleware** | Yes — creates | Create TxPipeline, put in context, Exec/Discard |
| **OutboxMiddleware** | Yes — uses | Extract pipeline, XADD output events |
| **Handler** | **No** | Call ports (interfaces), pass ctx through |
| **Adapter** | Yes — uses | Extract pipeline via `CmdableFromContext`, queue commands |
| **Acking** | No | Ack/nack based on chain result |

### Idempotency

This plan does **not** include an inbox middleware for Redis. Unlike SQL (where INSERT with unique constraint runs atomically in the same TX), Redis MULTI/EXEC cannot branch on results — you cannot check "does key exist?" and conditionally skip the handler inside MULTI.

A SetNX-based inbox outside the TX would have a consistency gap: if the process crashes between SetNX and EXEC, the message appears "processed" but business logic didn't run. Recovery requires waiting for TTL expiry.

**Recommendation:** Design handlers to be idempotent. Redis operations are naturally idempotent (HSET, SET, ZADD with same values). The broker provides at-least-once delivery with retry. Idempotent handlers + broker retry = safe duplicate handling without framework complexity.

See [Future Considerations](#future-considerations) for inbox design options if needed.

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

### Task 3: Redis Outbox Middleware

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

---

## Standalone Broker

The standalone Redis Streams pub/sub provides a persistent buffer channel with at-least-once delivery. Uses consumer groups for competing consumers, XPENDING + XCLAIM for dead consumer recovery. Independent of the middleware — usable as a drop-in broker for any gopipe pipeline.

### Task 4: Redis Streams Subscriber

**Goal:** Subscribe to a Redis Stream via consumer groups. Follows the same pattern as the CloudEvents subscriber — backed by `GeneratePipe`, returns `<-chan *RawMessage`.

**Implementation:**
```go
// message/redis/subscriber.go

type SubscriberConfig struct {
    // Stream is the Redis Stream to consume from.
    Stream string
    // Group is the consumer group name.
    Group string
    // Consumer is this instance's name within the group (default: hostname).
    Consumer string
    // OldestID is the starting ID for a new consumer group (default: "0" = all messages).
    // Set to "$" to only receive new messages.
    OldestID string
    // BatchSize is the XREADGROUP COUNT parameter (default: 10).
    BatchSize int64
    // BlockTimeout is the XREADGROUP BLOCK duration (default: 1s).
    BlockTimeout time.Duration
    // ClaimInterval is how often to check for idle pending messages (default: 30s).
    ClaimInterval time.Duration
    // MaxIdleTime is the idle threshold before claiming a message (default: 60s).
    MaxIdleTime time.Duration
    // Unmarshal converts a stream entry to a RawMessage.
    // Default: JSON unmarshal with CloudEvents attribute mapping.
    Unmarshal func(id string, values map[string]any) (*message.RawMessage, error)
    // Pipe config for the underlying GeneratePipe (concurrency, etc.).
    PipeConfig pipe.Config
}

type Subscriber struct {
    client goredis.UniversalClient
    cfg    SubscriberConfig
    gen    *pipe.GeneratePipe[*message.RawMessage]
}

func NewSubscriber(client goredis.UniversalClient, cfg SubscriberConfig) *Subscriber

func (s *Subscriber) Subscribe(ctx context.Context) (<-chan *message.RawMessage, error) {
    // 1. Create consumer group (XGROUP CREATE stream group oldestID MKSTREAM)
    //    Ignore BUSYGROUP error (group already exists)
    // 2. Return s.gen.Generate(ctx) — backed by receive()
    // 3. Start background goroutine for claimIdle loop
}
```

**Core read loop (inside GeneratePipe):**
```go
func (s *Subscriber) receive(ctx context.Context) ([]*message.RawMessage, error) {
    streams, err := s.client.XReadGroup(ctx, &goredis.XReadGroupArgs{
        Group:    s.cfg.Group,
        Consumer: s.cfg.Consumer,
        Streams:  []string{s.cfg.Stream, ">"},
        Count:    s.cfg.BatchSize,
        Block:    s.cfg.BlockTimeout,
    }).Result()
    if errors.Is(err, goredis.Nil) {
        return nil, nil // no messages, GeneratePipe loops
    }
    if err != nil {
        return nil, err
    }

    var msgs []*message.RawMessage
    for _, xMsg := range streams[0].Messages {
        // Bridge acking: msg.Ack() → XACK, msg.Nack() → stays in PEL
        acking := message.NewAcking(
            func() { s.client.XAck(ctx, s.cfg.Stream, s.cfg.Group, xMsg.ID) },
            func(err error) { /* no XACK — message stays in PEL for reclaim */ },
        )

        raw, err := s.cfg.Unmarshal(xMsg.ID, xMsg.Values)
        if err != nil {
            return nil, fmt.Errorf("unmarshal stream entry %s: %w", xMsg.ID, err)
        }
        raw.SetAcking(acking)
        msgs = append(msgs, raw)
    }
    return msgs, nil
}
```

**Idle message claiming (Redis 6.0 compatible):**
```go
// XAUTOCLAIM requires Redis 6.2+. For 6.0 compatibility, use XPENDING + XCLAIM.
func (s *Subscriber) claimIdle(ctx context.Context) {
    pending, _ := s.client.XPendingExt(ctx, &goredis.XPendingExtArgs{
        Stream: s.cfg.Stream,
        Group:  s.cfg.Group,
        Idle:   s.cfg.MaxIdleTime,
        Start:  "-", End: "+",
        Count:  s.cfg.BatchSize,
    }).Result()

    var ids []string
    for _, pe := range pending {
        ids = append(ids, pe.ID)
    }
    if len(ids) == 0 {
        return
    }

    // XClaim with MinIdle prevents double-claiming by concurrent subscribers
    s.client.XClaim(ctx, &goredis.XClaimArgs{
        Stream:   s.cfg.Stream,
        Group:    s.cfg.Group,
        Consumer: s.cfg.Consumer,
        MinIdle:  s.cfg.MaxIdleTime,
        Messages: ids,
    })
    // Claimed messages appear in next XREADGROUP with ID "0" (pending re-read)
}
```

**Key design:**
- **GeneratePipe**: `receive()` returns a batch per call; GeneratePipe handles concurrency, lifecycle, and channel emission — same pattern as CloudEvents subscriber
- **Consumer groups**: XREADGROUP with `">"` gets new messages. Pending messages (claimed or unacked) are re-read with ID `"0"` on restart.
- **XPENDING + XCLAIM**: background loop reclaims messages from dead consumers. `MinIdle` prevents race between concurrent subscribers.
- **Ack = XACK**: removes from PEL. **Nack = no-op**: message stays in PEL for reclaim by another consumer.
- **Configurable unmarshal**: default assumes JSON CloudEvents fields. Custom unmarshal for outbox entries, protobuf, etc.

**Files to Create:**
- `message/redis/subscriber.go`
- `message/redis/subscriber_test.go`

**Acceptance Criteria:**
- [ ] Returns `<-chan *RawMessage` from `Subscribe()`
- [ ] Uses `GeneratePipe` for lifecycle and concurrency
- [ ] Consumer group auto-created on first subscribe
- [ ] msg.Ack() → XACK, msg.Nack() → stays in PEL
- [ ] Idle messages reclaimed via XPENDING + XCLAIM (Redis 6.0)
- [ ] Configurable batch size, block timeout, claim intervals, unmarshal

### Task 5: Redis Streams Publisher

**Goal:** Publish messages to a Redis Stream via XADD. Follows the same pattern as the CloudEvents publisher — backed by `SinkPipe`, consumes `<-chan *RawMessage`.

**Implementation:**
```go
// message/redis/publisher.go

type PublisherConfig struct {
    // Stream is the destination Redis Stream.
    Stream string
    // MaxLen caps the stream length (0 = no limit).
    MaxLen int64
    // Approx uses MAXLEN ~ for O(1) trimming instead of exact (default: true).
    Approx bool
    // Marshal converts a RawMessage to stream entry field-value pairs.
    // Default: JSON marshal with CloudEvents attribute mapping.
    Marshal func(msg *message.RawMessage) (map[string]any, error)
    // Pipe config for the underlying SinkPipe (concurrency, etc.).
    PipeConfig pipe.Config
}

type Publisher struct {
    client goredis.UniversalClient
    cfg    PublisherConfig
    sink   *pipe.ProcessPipe[*message.RawMessage, struct{}]
}

func NewPublisher(client goredis.UniversalClient, cfg PublisherConfig) *Publisher

func (p *Publisher) Publish(ctx context.Context, ch <-chan *message.RawMessage) (<-chan struct{}, error) {
    // Returns p.sink.Pipe(ctx, ch)
}

func (p *Publisher) send(ctx context.Context, raw *message.RawMessage) error {
    values, err := p.cfg.Marshal(raw)
    if err != nil {
        raw.Nack(err)
        return fmt.Errorf("marshal: %w", err)
    }

    _, err = p.client.XAdd(ctx, &goredis.XAddArgs{
        Stream: p.cfg.Stream,
        Values: values,
        MaxLen: p.cfg.MaxLen,
        Approx: p.cfg.Approx,
    }).Result()
    if err != nil {
        raw.Nack(err)
        return err
    }

    raw.Ack()
    return nil
}
```

**Key design:**
- **SinkPipe**: `send()` processes one message at a time; SinkPipe handles concurrency and lifecycle — same pattern as CloudEvents publisher
- **MAXLEN with Approx**: `XADD stream MAXLEN ~ N` trims the stream approximately, preventing unbounded growth with O(1) cost
- **Ack on XADD success**: signals to upstream that the message was durably written
- **Configurable marshal**: default assumes JSON CloudEvents fields. Custom marshal for outbox forwarding, protobuf, etc.

**Files to Create:**
- `message/redis/publisher.go`
- `message/redis/publisher_test.go`

**Acceptance Criteria:**
- [ ] Consumes `<-chan *RawMessage` from `Publish()`
- [ ] Uses `SinkPipe` for lifecycle and concurrency
- [ ] XADD with configurable MAXLEN trimming
- [ ] msg.Ack() on success, msg.Nack(err) on failure
- [ ] Configurable marshaling

### Task 6: Outbox Forwarding

**Goal:** Forward events from the outbox stream to the destination broker. Composes the Subscriber (Task 5) on the outbox stream with a Publisher (Task 6 or any broker publisher) to the destination.

**Implementation:**

The outbox forwarder is not a new component — it's a composition of Subscriber + Engine + Publisher. A helper creates a pre-configured Subscriber with outbox-specific unmarshal:

```go
// message/redis/outbox_forward.go

// OutboxForwarderConfig configures the outbox forwarder.
type OutboxForwarderConfig struct {
    // Client is the Redis client.
    Client goredis.UniversalClient
    // Stream is the outbox stream (must match OutboxConfig.Stream).
    Stream string
    // Group is the consumer group name (default: "outbox-forwarder").
    Group string
    // Consumer is this instance's name (default: hostname).
    Consumer string
    // SubscriberConfig overrides for the underlying subscriber.
    SubscriberConfig
}

// NewOutboxSubscriber creates a Subscriber pre-configured for the outbox stream.
// The unmarshal function decodes outbox entries written by OutboxMiddleware.
func NewOutboxSubscriber(cfg OutboxForwarderConfig) *Subscriber {
    return NewSubscriber(cfg.Client, SubscriberConfig{
        Stream:    cfg.Stream,
        Group:     cfg.Group,
        Consumer:  cfg.Consumer,
        Unmarshal: outboxUnmarshal, // decodes outbox entry format
        // ... inherit other settings from cfg.SubscriberConfig
    })
}
```

**Usage — wire through Engine:**
```go
// Outbox subscriber (reads from outbox stream)
outboxSub := msgredis.NewOutboxSubscriber(msgredis.OutboxForwarderConfig{
    Client: client,
    Stream: "outbox:events",
})
outboxCh, _ := outboxSub.Subscribe(ctx)

// Connect to engine as input
engine.AddRawInput("outbox", nil, outboxCh)

// Output goes to destination publisher (Redis Streams, HTTP, anything)
destCh, _ := engine.AddRawOutput("destination", nil)
destPub := msgredis.NewPublisher(client, msgredis.PublisherConfig{
    Stream: "order-events",
    MaxLen: 10000,
})
destPub.Publish(ctx, destCh)
```

**Key design:**
- **No custom component**: outbox forwarding composes existing primitives (Subscriber + Engine + Publisher)
- **Outbox-specific unmarshal**: `outboxUnmarshal` decodes the entry format produced by OutboxMiddleware
- **Destination-agnostic**: the outbox subscriber outputs `RawMessage` — wire it to any publisher (Redis, HTTP, CloudEvents, etc.)
- **Ack = XACK on outbox stream**: when the destination publisher confirms delivery

**Files to Create:**
- `message/redis/outbox_forward.go`
- `message/redis/outbox_forward_test.go`

**Acceptance Criteria:**
- [ ] `NewOutboxSubscriber` returns a pre-configured Subscriber
- [ ] Outbox unmarshal decodes entries from OutboxMiddleware
- [ ] Composes with any publisher via Engine wiring
- [ ] Ack on destination publish → XACK on outbox stream

## Implementation Order

```
transaction-handling.md Tasks 0, 1, 2 (shared foundation)
  ↓
Task 1: Redis Context Helpers
  ↓                                              (independent track)
Task 2: TxMiddleware              Task 4: Subscriber    Task 5: Publisher
  ↓                                  ↓                     ↓
Task 3: Outbox Middleware            └─────────────────────┘
  ↓                                          ↓
  └──────────────────────────────Task 6: Outbox Forwarding
```

Two independent tracks: **middleware** (Tasks 1-3) and **broker** (Tasks 4-5). Task 6 ties them together.

## End-to-End Trace

### Setup — Redis Streams as Broker

```go
client := goredis.NewClient(&goredis.Options{Addr: "localhost:6379"})

// --- Subscriber (input) ---
sub := msgredis.NewSubscriber(client, msgredis.SubscriberConfig{
    Stream: "commands",
    Group:  "order-service",
})

// --- Engine with middleware ---
engine := message.NewEngine(message.EngineConfig{})
engine.Router().Use(
    msgredis.TxMiddleware(client),
    msgredis.OutboxMiddleware(msgredis.OutboxConfig{Stream: "outbox:events"}),
)
engine.AddHandler("place-order", nil, NewPlaceOrderHandler(orderRepo))

// Wire subscriber → engine
inputCh, _ := sub.Subscribe(ctx)
engine.AddRawInput("commands", nil, inputCh)
engineDone, _ := engine.Start(ctx)

// --- Outbox forwarding (separate pipeline) ---
outboxSub := msgredis.NewOutboxSubscriber(msgredis.OutboxForwarderConfig{
    Client: client,
    Stream: "outbox:events",
})
outboxCh, _ := outboxSub.Subscribe(ctx)
destPub := msgredis.NewPublisher(client, msgredis.PublisherConfig{
    Stream: "order-events",
    MaxLen: 10000,
})
destPub.Publish(ctx, outboxCh)
```

### Execution Flow

```
1.  Broker delivers message (msg.ID = "abc-123")
2.  Router receives message
3.  Acking middleware (outermost) calls next:
4.    TxMiddleware: pipe = client.TxPipeline(), msg.WithValue(pipelineKey, pipe)
5.      OutboxMiddleware calls next:
6.        Handler: repo.Save(ctx, order) → adapter queues HSET on pipeline
7.        Handler returns: []OrderPlaced{{OrderID: "xyz"}}
8.      OutboxMiddleware: pipe.XAdd("outbox:events", {type: "order.placed", ...})
9.      OutboxMiddleware returns: nil, nil (outputs swallowed)
10.   TxMiddleware: pipe.Exec(ctx) → MULTI, HSET, XADD, EXEC → success
11. Acking middleware: msg.Ack() → broker ack (AFTER Exec)
```

### Duplicate Handling

Handled at the application level via idempotent operations. If the broker redelivers a message:

```go
// Idempotent: HSET overwrites with same values — no side effects
cmd.HSet(ctx, "order:"+order.ID, map[string]any{
    "product_id": order.ProductID,
    "quantity":   order.Quantity,
    "status":     "placed",
})
```

The handler runs again, produces the same result, outbox may get a duplicate entry (forwarder handles dedup or downstream is idempotent).

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
| Delivery guarantee | At-least-once | At-least-once |
| Transaction support | None (uses SQL Forwarder) | Native via TxPipeline |
| Outbox pattern | SQL-based Forwarder component | Redis Stream within same TxPipeline |
| Idempotency | Application-level | Application-level (idempotent handlers) |
| Nack handling | In-memory re-send loop | Broker redelivery |
| Idle message recovery | XPENDING + XCLAIM | XPENDING + XCLAIM |
| Marshaling | msgpack metadata + raw payload | Configurable (default: JSON) |

Key insight: Watermill keeps Redis transactions **outside** the Redis driver entirely. Their outbox pattern uses a SQL Forwarder — write to SQL outbox table within SQL TX, then a daemon forwards to Redis Streams. gopipe's approach is more native: write to Redis outbox stream within the same TxPipeline as business commands, no SQL involved.

## Open Questions

1. **go-redis dependency**: The `message/redis` package adds `github.com/redis/go-redis/v9` as a dependency. Should this be a separate Go module to keep the core dependency-free?
2. **Redis Cluster**: TxPipeline (MULTI/EXEC) requires all keys to be on the same slot. Should the plan address hash tags (`{tag}`) for cluster compatibility, or defer to documentation?
3. **Outbox stream trimming**: The outbox stream grows with processed entries. Outbox XADD could use MAXLEN for approximate trimming, or the forwarder could XDEL after XACK.
4. **Outbox entry format**: Should the default marshal include the destination topic in the stream entry, or should there be one outbox stream per destination?
5. **Default marshal format**: JSON is simple but verbose. msgpack (like Watermill) is more compact. Should the default be JSON with msgpack as an option, or vice versa?

## Acceptance Criteria

- [ ] All tasks completed
- [ ] Tests pass (`make test`)
- [ ] Build passes (`make build && make vet`)
- [ ] **Broker**: Subscriber returns `<-chan *RawMessage` via GeneratePipe
- [ ] **Broker**: Publisher consumes `<-chan *RawMessage` via SinkPipe
- [ ] **Broker**: Consumer group created, XACK on ack, PEL reclaim on idle
- [ ] **Middleware**: Handler code has zero Redis imports
- [ ] **Middleware**: Pipeline executes atomically (business + outbox in same MULTI/EXEC)
- [ ] **Forwarding**: Outbox subscriber composes with any publisher
- [ ] **All**: Redis 6.0 compatible (no XAUTOCLAIM, no Redis Functions)
- [ ] **All**: Commit-before-ack ordering maintained

## Future Considerations

### Redis Inbox Middleware

An inbox middleware for Redis was considered but deferred. The analysis:

**The Problem:** Redis MULTI/EXEC cannot branch on results. You cannot check "does key exist?" and conditionally skip the handler inside MULTI. An inbox must run outside the transaction.

**SetNX Approach:** Use `SetNX(inbox:{msgID}, "1", TTL)` before the TxPipeline. If key exists, skip processing.

**The Gap:** If the process crashes between SetNX and EXEC:
- Inbox key exists (SetNX succeeded)
- Business logic did NOT run (EXEC never happened)
- On redelivery, SetNX fails → message skipped → **lost until TTL expires**

**WATCH-based Atomic Approach:** Use WATCH + MULTI/EXEC to make inbox atomic with business:
```
WATCH inbox:{msgID}
GET inbox:{msgID}
if exists: UNWATCH, return duplicate
MULTI
  SET inbox:{msgID} "1" EX ttl
  ...business commands...
EXEC
```

This eliminates the gap but adds:
- Retry loop on WATCH conflicts
- Redis Cluster constraint: inbox key must be on same slot as business keys (requires hash tag coordination)
- TxMiddleware must use `client.Watch()` callback instead of simple `TxPipeline()`

**Decision:** Neither approach provides sufficient value:
- SetNX has a consistency gap (bounded by TTL, but still a gap)
- WATCH adds complexity and cluster constraints
- Redis operations are naturally idempotent — handlers can be designed to handle duplicates
- The broker provides at-least-once delivery with retry

**Recommendation:** Use idempotent handlers. If framework-enforced idempotency is required, use SQL (where inbox INSERT runs atomically in the same TX).
