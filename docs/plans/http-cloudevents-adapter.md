# HTTP CloudEvents Adapter

**Status:** Proposed

## Overview

HTTP pub/sub adapter for CloudEvents using standard library `net/http`. Provides topic-based subscription and batch publishing with pipe integration.

## Goals

1. Topic-based HTTP subscription with per-request goroutines (no concurrency limit)
2. Batch publishing using `BatchPipe` for high-throughput scenarios
3. Acking bridged to HTTP response codes
4. Minimal wrapper — reuse `message.ParseRaw`, `message.RawMessage`, `message.Acking`

## Architecture

```
SUBSCRIBER (HTTP Server)              PUBLISHER (HTTP Client)
────────────────────────              ───────────────────────

net/http (goroutine/request)          input chan
         │                                  │
         ▼                                  ▼
   ServeHTTP()                     ┌──────────────────┐
   ParseRaw()                      │ Publish()        │──► HTTP POST (single)
         │                         │ PublishStream()  │──► SinkPipe → HTTP POST
         ▼                         │ PublishBatch()   │──► BatchPipe → HTTP POST (batch)
   buffered channel                └──────────────────┘
         │
         ▼
   downstream consumer
```

## Design Decisions

### Subscriber: Direct HTTP Handler (not GeneratePipe)

**Problem:** `GeneratePipe` with fixed `Concurrency` limits parallel HTTP requests.

**Solution:** Use `net/http` directly — each request runs in its own goroutine (unlimited). Parse immediately, send to buffered channel, wait for ack.

```go
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
    msg, _ := message.ParseRaw(r.Body)

    done := make(chan error, 1)
    msg.acking = message.NewAcking(
        func() { done <- nil },
        func(e error) { done <- e },
    )

    s.ch <- msg  // buffered, backpressure here

    err := <-done  // wait for downstream ack/nack
    if err != nil {
        http.Error(w, err.Error(), 500)
    }
}
```

**Trade-off:** No middleware on receive path (middleware requires pipe). See Future: Autoscale.

### Publisher: Pipe-Based

Uses existing pipe infrastructure:

| Method | Pipe | Description |
|--------|------|-------------|
| `Publish()` | None | Direct HTTP call |
| `PublishStream()` | `SinkPipe` | Concurrent single sends |
| `PublishBatch()` | `BatchPipe` | Collect → batch HTTP POST |

Middleware supported via `Use()`.

### Acking Strategy

| Direction | Ack | Nack |
|-----------|-----|------|
| Subscribe (inbound) | HTTP 200 | HTTP 500 |
| Publish (outbound) | HTTP 2xx received | HTTP 4xx/5xx or error |
| Batch publish | `SharedAcking` — all succeed or batch fails |

### No cehttp Dependency

Use `message.ParseRaw()` / `RawMessage.MarshalJSON()` directly. Avoids CloudEvents SDK HTTP protocol complexity while remaining spec-compliant for structured JSON mode.

## API

### Subscriber

```go
type SubscriberConfig struct {
    Addr        string        // ":8080"
    Path        string        // "/events" (topic appended)
    BufferSize  int           // channel buffer per topic
    ReadTimeout time.Duration
}

type Subscriber struct { ... }

func NewSubscriber(cfg SubscriberConfig) *Subscriber
func (s *Subscriber) Subscribe(ctx context.Context, topic string) (<-chan *message.RawMessage, error)
func (s *Subscriber) ServeHTTP(w http.ResponseWriter, r *http.Request)
func (s *Subscriber) Start(ctx context.Context) error
func (s *Subscriber) Handler(topic string) http.Handler
```

### Publisher

```go
type PublisherConfig struct {
    Client      *http.Client
    TargetURL   string        // "http://host/events"
    Concurrency int           // for stream/batch
    Headers     http.Header
}

type BatchConfig struct {
    MaxSize     int
    MaxDuration time.Duration
    Concurrency int
}

type Publisher struct { ... }

func NewPublisher(cfg PublisherConfig) *Publisher
func (p *Publisher) Publish(ctx context.Context, topic string, msg *message.RawMessage) error
func (p *Publisher) PublishStream(ctx, topic, in <-chan) (<-chan struct{}, error)
func (p *Publisher) PublishBatch(ctx, topic, in <-chan, cfg BatchConfig) (<-chan struct{}, error)
func (p *Publisher) Use(mw ...middleware.Middleware) error
```

## Content Types

| Format | Content-Type |
|--------|--------------|
| Single | `application/cloudevents+json` |
| Batch | `application/cloudevents-batch+json` |

## File Structure

```
message/cloudevents/http/
├── doc.go
├── subscriber.go
├── subscriber_test.go
├── publisher.go
├── publisher_test.go
├── batch.go
├── batch_test.go
└── http_test.go        # E2E tests
```

## Test Plan

### Unit Tests

**subscriber_test.go**
- `TestSubscriber_Subscribe` — returns channel
- `TestSubscriber_Subscribe_MultipleTopic` — concurrent topics
- `TestSubscriber_ContextCancellation` — closes channel
- `TestSubscriber_ServeHTTP_Parse` — single and batch
- `TestSubscriber_ServeHTTP_InvalidJSON` — 400 response
- `TestSubscriber_Acking` — HTTP 200/500 based on ack/nack

**publisher_test.go**
- `TestPublisher_Publish` — single send
- `TestPublisher_Publish_Ack` — ack on 2xx
- `TestPublisher_Publish_Nack` — nack on error
- `TestPublisher_PublishStream` — concurrent sends
- `TestPublisher_PublishBatch_Size` — batches by MaxSize
- `TestPublisher_PublishBatch_Duration` — flushes by MaxDuration
- `TestPublisher_PublishBatch_SharedAcking` — batch acking

**batch_test.go**
- `TestParseBatch` — JSON array parsing
- `TestMarshalBatch` — JSON array serialization

### E2E Tests

**http_test.go**
- `TestHTTP_E2E_SingleEvent` — publish → subscribe → ack roundtrip
- `TestHTTP_E2E_BatchEvent` — batch roundtrip
- `TestHTTP_E2E_MultiTopic` — concurrent topics
- `TestHTTP_E2E_Backpressure` — slow consumer handling

## Example

```go
// Subscriber
sub := http.NewSubscriber(http.SubscriberConfig{Addr: ":8080", Path: "/events"})
orders, _ := sub.Subscribe(ctx, "orders")
go sub.Start(ctx)

// Publisher with batching
pub := http.NewPublisher(http.PublisherConfig{TargetURL: "http://localhost:8080/events"})
done, _ := pub.PublishBatch(ctx, "orders", inputCh, http.BatchConfig{MaxSize: 100})
```

## Future: Autoscale Worker Pool Integration

Current limitation: Subscriber uses direct HTTP handler (no pipe), so no middleware support on receive path.

With planned **autoscale worker pool** feature (see outline below), subscriber could use `GeneratePipe` with unlimited/autoscale workers:

```go
// Future API
pipe.Config{
    MinWorkers: 1,
    MaxWorkers: 0,  // 0 = unlimited
    Autoscale:  true,
}
```

This would enable:
1. Middleware on subscriber (retry, metrics, logging)
2. Consistent pipe-based architecture for both directions
3. Backpressure-aware scaling

See: [Autoscale Worker Pool Outline](#autoscale-worker-pool-outline)

---

## Autoscale Worker Pool Outline

**Context:** How autoscale would enable unlimited/dynamic workers for HTTP adapter.

### Current State

```go
// pipe/processing.go
type Config struct {
    Concurrency int  // Fixed worker count
    BufferSize  int
    // ...
}
```

Fixed concurrency limits throughput for bursty workloads (HTTP requests).

### Proposed Enhancement

```go
type Config struct {
    // Existing
    Concurrency int  // Renamed: MinWorkers (or keep for backward compat)

    // New
    MaxWorkers      int           // 0 = unlimited
    ScaleUpThresh   float64       // Channel fill ratio to scale up (e.g., 0.8)
    ScaleDownThresh float64       // Channel fill ratio to scale down (e.g., 0.2)
    ScaleInterval   time.Duration // How often to check (e.g., 100ms)
}
```

### Worker Pool Behavior

```
                    ┌─────────────────────────────────┐
                    │        Autoscale Pool           │
                    │                                 │
input ──►  buffer   │   worker 1  ◄──┐               │
           [####__] │   worker 2     │ scale up/down │ ──► output
           80% full │   worker N  ◄──┘               │
                    │                                 │
                    └─────────────────────────────────┘

Scale up:   buffer > 80% full → spawn worker (up to MaxWorkers)
Scale down: buffer < 20% full → stop worker (down to MinWorkers)
Unlimited:  MaxWorkers=0 → spawn on demand, no cap
```

### Impact on HTTP Adapter Components

| Component | Current | With Autoscale |
|-----------|---------|----------------|
| **Subscriber (GeneratePipe)** | Fixed N workers calling Receive() | Unlimited workers, scale with request rate |
| **PublishStream (SinkPipe)** | Fixed N concurrent sends | Scale with backlog |
| **PublishBatch (BatchPipe)** | Fixed N batch processors | Scale with batch backlog |

### Subscriber with Autoscale

```go
// Current: Limited to Concurrency workers
source := cloudevents.NewSubscriber(receiver, SubscriberConfig{
    Concurrency: 10,  // Only 10 concurrent HTTP requests
})

// Future: Unlimited, autoscaling
source := cloudevents.NewSubscriber(receiver, SubscriberConfig{
    MinWorkers: 1,
    MaxWorkers: 0,     // Unlimited
    Autoscale:  true,
})
```

Now HTTP subscriber can:
1. Handle unlimited concurrent requests (like direct ServeHTTP)
2. Use middleware (retry, metrics, circuit breaker)
3. Apply backpressure via buffer size

### Implementation Considerations

1. **Worker lifecycle:** Idle workers should exit after timeout
2. **Metrics:** Expose current/min/max worker counts
3. **Graceful shutdown:** Wait for in-flight work
4. **Per-pool autoscale:** Named pools can have different scaling policies

### Relation to Named Pools (Plan 0011)

Named pools + autoscale = per-handler scaling:

```go
router.AddPoolWithConfig("http-ingest", PoolConfig{
    MinWorkers: 1,
    MaxWorkers: 0,  // Unlimited for HTTP
    Autoscale:  true,
})

router.AddPoolWithConfig("erp-sync", PoolConfig{
    MinWorkers: 1,
    MaxWorkers: 5,  // Rate-limited external API
    Autoscale:  true,
})
```

### Files to Modify

| File | Changes |
|------|---------|
| `pipe/processing.go` | Add MinWorkers, MaxWorkers, autoscale logic |
| `pipe/config.go` | New config fields |
| `pipe/autoscale.go` | Scaling algorithm (new file) |
| `message/cloudevents/subscriber.go` | Use autoscale config |
| `message/router.go` | Pass autoscale config to pools |

---

## Acceptance Criteria

- [ ] Subscriber handles unlimited concurrent HTTP requests
- [ ] Publisher supports single, stream, and batch modes
- [ ] Acking correctly bridges to HTTP responses
- [ ] Batch format uses `application/cloudevents-batch+json`
- [ ] All unit tests pass
- [ ] E2E test demonstrates full roundtrip
- [ ] Example in `examples/` directory
