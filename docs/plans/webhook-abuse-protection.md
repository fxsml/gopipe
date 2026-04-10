# Webhook Abuse Protection

**Status:** Proposed

## Overview

Implement the CloudEvents [HTTP Webhook spec](https://github.com/cloudevents/spec/blob/main/cloudevents/http-webhook.md) abuse protection handshake in the HTTP Subscriber. This prevents event senders from being weaponized to flood arbitrary endpoints.

## Problem

Without abuse protection, an attacker can register a webhook subscription pointing to a victim's URL. The event sender then unwittingly delivers events to the victim, participating in a denial-of-service attack. The sender has no way to verify that the target actually wants to receive events.

```
Attacker registers subscription:  target = https://victim.example.com/api
Sender starts delivering events → victim is flooded
```

## Spec Summary

The [CloudEvents HTTP Webhook spec (Section 4)](https://github.com/cloudevents/spec/blob/main/cloudevents/http-webhook.md) defines a validation handshake:

### Handshake Flow

```
Sender                                    Receiver (Subscriber)
  │                                              │
  │── OPTIONS /events ──────────────────────────→│
  │   WebHook-Request-Origin: sender.example.com │
  │   WebHook-Request-Rate: 120                  │
  │                                              │
  │←─ 200 OK ───────────────────────────────────│
  │   WebHook-Allowed-Origin: sender.example.com │
  │   WebHook-Allowed-Rate: 120                  │
  │                                              │
  │   (handshake passed — sender starts          │
  │    delivering events via POST)               │
```

### Request Headers (Sender → Receiver)

| Header | Required | Description |
|--------|----------|-------------|
| `WebHook-Request-Origin` | Yes | DNS name identifying the sender |
| `WebHook-Request-Callback` | No | URL for async permission grant (receiver can call back later) |
| `WebHook-Request-Rate` | No | Desired delivery rate in requests per minute |

### Response Headers (Receiver → Sender)

| Header | Required | Description |
|--------|----------|-------------|
| `WebHook-Allowed-Origin` | Yes | Echo the origin or `*` to accept all |
| `WebHook-Allowed-Rate` | No | Granted rate, or `*` for unlimited |

### Key Constraints

- The handshake uses HTTP `OPTIONS` method
- MUST respond with `200 OK` to accept (not 204)
- The handshake "does not aim to establish an authentication or authorization context" — it only protects the sender from being weaponized
- HTTPS is mandatory for event delivery (but the OPTIONS handshake itself may happen over HTTP during setup)

## Design

### Subscriber Extension

Add optional abuse protection to the HTTP Subscriber via configuration:

```go
type SubscriberConfig struct {
    BufferSize      int
    AckTimeout      time.Duration
    Enricher        MessageEnricher         // optional (from OAuth2 plan)
    AllowedOrigins  []string                // optional: accepted sender origins
    AllowAllOrigins bool                    // optional: accept any origin (WebHook-Allowed-Origin: *)
}
```

### ServeHTTP Changes

When `AllowedOrigins` or `AllowAllOrigins` is configured, `ServeHTTP` handles OPTIONS requests:

```go
func (s *Subscriber) ServeHTTP(w http.ResponseWriter, r *http.Request) {
    // ... existing subscribed check ...

    if r.Method == http.MethodOptions {
        s.handleWebhookValidation(w, r)
        return
    }

    // ... existing POST handling ...
}

func (s *Subscriber) handleWebhookValidation(w http.ResponseWriter, r *http.Request) {
    origin := r.Header.Get("WebHook-Request-Origin")
    if origin == "" {
        // Not a webhook validation request — plain OPTIONS
        w.WriteHeader(http.StatusOK)
        return
    }

    if !s.isOriginAllowed(origin) {
        w.WriteHeader(http.StatusForbidden)
        return
    }

    if s.cfg.AllowAllOrigins {
        w.Header().Set("WebHook-Allowed-Origin", "*")
    } else {
        w.Header().Set("WebHook-Allowed-Origin", origin)
    }

    if rate := r.Header.Get("WebHook-Request-Rate"); rate != "" {
        w.Header().Set("WebHook-Allowed-Rate", rate) // echo requested rate
    }

    w.WriteHeader(http.StatusOK)
}
```

### Callback Support (Optional, Deferred)

The spec also defines `WebHook-Request-Callback` for asynchronous approval — the receiver can call back to a URL later to grant permission. This is more complex (requires outbound HTTP calls, state management) and can be deferred to a later iteration. The synchronous handshake covers the primary use case.

## Tasks

### Task 1: OPTIONS Handler

**Goal:** Handle webhook validation handshake in ServeHTTP

**Files to Create/Modify:**
- `message/http/subscriber.go` — add OPTIONS handling and origin validation

**Acceptance Criteria:**
- [ ] OPTIONS with `WebHook-Request-Origin` returns 200 with `WebHook-Allowed-Origin`
- [ ] Unconfigured subscriber ignores OPTIONS (existing behavior unchanged)
- [ ] Unknown origins return 403 when `AllowedOrigins` is configured
- [ ] `AllowAllOrigins` responds with `*`
- [ ] `WebHook-Request-Rate` is echoed in `WebHook-Allowed-Rate`

### Task 2: Tests

**Goal:** Comprehensive test coverage for the handshake

**Files to Create/Modify:**
- `message/http/subscriber_test.go` — add webhook validation tests

**Acceptance Criteria:**
- [ ] Test: allowed origin returns 200 with correct headers
- [ ] Test: disallowed origin returns 403
- [ ] Test: wildcard origin acceptance
- [ ] Test: rate negotiation
- [ ] Test: non-webhook OPTIONS (no `WebHook-Request-Origin` header)
- [ ] Test: unconfigured subscriber still rejects OPTIONS with MethodNotAllowed (existing behavior)

## Implementation Order

```
Task 1 (OPTIONS handler) → Task 2 (tests)
```

This plan is independent of the OAuth2 integration and can be implemented first.

## Acceptance Criteria

- [ ] All tasks completed
- [ ] Tests pass
- [ ] CHANGELOG updated
