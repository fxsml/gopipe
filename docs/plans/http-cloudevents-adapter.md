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

## Future: Autoscale Worker Pool

Current limitation: Subscriber uses direct HTTP handler (no pipe), so no middleware support on receive path.

With [Autoscale Worker Pool](autoscale-worker-pool.md), subscriber could use `GeneratePipe` with unlimited workers, enabling middleware support.

## Acceptance Criteria

- [ ] Subscriber handles unlimited concurrent HTTP requests
- [ ] Publisher supports single, stream, and batch modes
- [ ] Acking correctly bridges to HTTP responses
- [ ] Batch format uses `application/cloudevents-batch+json`
- [ ] All unit tests pass
- [ ] E2E test demonstrates full roundtrip
- [ ] Example in `examples/` directory
