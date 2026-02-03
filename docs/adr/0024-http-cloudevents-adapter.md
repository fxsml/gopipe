# ADR 0024: HTTP CloudEvents Adapter Design

**Date:** 2025-01-30
**Status:** Implemented

## Context

We need a specialized HTTP CloudEvents adapter optimized for HTTP-only use cases.
The existing `message/cloudevents` package is a generic wrapper for ANY CloudEvents
SDK protocol (NATS, Kafka, HTTP, AMQP, etc.), which is valuable for multi-protocol
support but not optimized for HTTP-specific scenarios.

During design exploration, we considered building an HTTP adapter with internal
topic management, server lifecycle methods, and multi-topic subscription API.
However, this would duplicate functionality already available in the standard
library (`http.ServeMux`) and add unnecessary complexity.

## Decision

Simplify subscriber to single-responsibility: convert HTTP request to channel message.

**Remove:**
- Internal topic map management
- Server lifecycle (`Start()`, `Addr()`)
- Multi-topic subscription (`Subscribe(ctx, topic)`)

**Keep:**
- `ServeHTTP` implementing `http.Handler`
- `C()` returning the message channel
- Batch parsing support

**Delegate to user:**
- Topic routing via `http.ServeMux`
- Server lifecycle via `http.ListenAndServe`

```go
// Before (complex)
sub := NewSubscriber(cfg)
ch1, _ := sub.Subscribe(ctx, "orders")
ch2, _ := sub.Subscribe(ctx, "payments")
sub.Start(ctx)

// After (simple)
orders := NewSubscriber(cfg)
payments := NewSubscriber(cfg)

mux := http.NewServeMux()
mux.Handle("/events/orders", orders)
mux.Handle("/events/payments", payments)
http.ListenAndServe(":8080", mux)
```

**Minimal implementation:**
```go
func (s *Subscriber) ServeHTTP(w http.ResponseWriter, r *http.Request) {
    msg, err := message.ParseRaw(r.Body)  // reuse existing
    if err != nil {
        http.Error(w, err.Error(), 400)
        return
    }

    done := make(chan error, 1)
    msg = message.NewRaw(msg.Data, msg.Attributes, message.NewAcking(
        func() { done <- nil },
        func(e error) { done <- e },
    ))

    s.ch <- msg
    if err := <-done; err != nil {
        http.Error(w, err.Error(), 500)
    }
}
```

## Consequences

**Benefits:**
- ~150 lines less code
- Reuses `message.ParseRaw`, `message.NewAcking` (no reimplementation)
- Composes with standard library patterns
- Single responsibility: HTTP → channel → ack → response

**Drawbacks:**
- User manages server lifecycle (standard Go pattern)
- Slightly more setup code for multi-topic

## Links

- Plan: [0013-http-cloudevents-adapter](../plans/archive/0013-http-cloudevents-adapter.md)
