# OAuth2 CloudEvents Integration

**Status:** Proposed
**Depends On:** [Message Locals](locals.md) (implemented)
**Design Evolution:** [oauth2-cloudevents.decisions.md](oauth2-cloudevents.decisions.md)

## Overview

OAuth2 integration with the gopipe message/http package. Three independent concerns, two of which require zero framework code:

1. **Authentication** — External HTTP middleware (user-provided). No gopipe API.
2. **Auth context propagation** — Subscriber enricher bridges HTTP identity into messages via `SetLocal`. One config field.
3. **Authorization** — User-written `message.Middleware`. No gopipe API.

## Goals

1. Enable OAuth2 authentication and authorization for CloudEvents HTTP endpoints
2. Bridge HTTP identity into the message pipeline without leaking transport details
3. Ship the minimum API surface — one config field

## Relevant CloudEvents Specifications

### HTTP Protocol Binding — Security Stance

The CloudEvents HTTP protocol binding explicitly delegates security to HTTP:

> "This specification does not introduce any new security features for HTTP, or mandate specific existing features to be used."

Authentication belongs in the HTTP layer, not in the CloudEvents event model.

### Webhook Spec — Authorization Model

The [CloudEvents HTTP Webhook spec](https://github.com/cloudevents/spec/blob/main/cloudevents/http-webhook.md) defines authorization based on OAuth 2.0 Bearer tokens (RFC 6750):

- **Authorization header** (preferred): `Authorization: Bearer <token>`
- HTTPS is mandatory
- Challenge-based schemes MUST NOT be used

### Auth Context Extension (Informational Only)

The [authcontext extension](https://github.com/cloudevents/spec/blob/main/cloudevents/extensions/authcontext.md) defines `authtype`, `authid`, and `authclaims` attributes. Explicitly "purely informational and not intended to secure CloudEvents." Suitable for cross-broker provenance, not for authorization.

## Architecture

### Principle: Three Independent Concerns

| Concern | Mechanism | gopipe API? |
|---------|-----------|-------------|
| **Authentication** | HTTP middleware (user-provided) | No |
| **Propagation** | `SubscriberConfig.Enricher` → `msg.SetLocal()` | One config field |
| **Authorization** | `message.Middleware` (user-written) | No |

### Authentication — HTTP Middleware

External to gopipe. The `Subscriber` implements `http.Handler`, so standard middleware chains work:

```go
mux.Handle("/events", authMiddleware(subscriber))
```

Users bring their own: `go-jwt-middleware`, `coreos/go-oidc`, sidecar (Envoy/Istio), etc.

In the sidecar case, the sidecar validates the JWT and injects claims as headers (`X-Auth-Subject`, `X-Auth-Scopes`). No application-level auth middleware needed.

### Propagation — Subscriber Enricher

One new config field. The enricher runs after message creation, before channel delivery:

```go
type SubscriberConfig struct {
    BufferSize int
    AckTimeout time.Duration
    Validator  func(*http.Request, []*message.RawMessage) error // optional
    Enricher   func(*http.Request, []*message.RawMessage)       // optional
}
```

In `ServeHTTP`, after creating all messages from `ce.FromCloudEvent`:

```go
if s.cfg.Validator != nil {
    if err := s.cfg.Validator(r, msgs); err != nil { /* nack all, return HTTP error */ }
}
if s.cfg.Enricher != nil {
    s.cfg.Enricher(r, msgs)
}
```

Usage (sidecar scenario — claims in headers):

```go
subscriber := http.NewSubscriber(http.SubscriberConfig{
    Enricher: func(r *http.Request, msgs []*message.RawMessage) {
        for _, msg := range msgs {
            msg.SetLocal(principalKey, Principal{
                Subject: r.Header.Get("X-Auth-Subject"),
                Scopes:  strings.Split(r.Header.Get("X-Auth-Scopes"), ","),
            })
        }
    },
})
```

Usage (application middleware scenario — claims in context):

```go
subscriber := http.NewSubscriber(http.SubscriberConfig{
    Enricher: func(r *http.Request, msgs []*message.RawMessage) {
        claims, _ := authn.ClaimsFromContext(r.Context())
        for _, msg := range msgs {
            msg.SetLocal(principalKey, claims)
        }
    },
})
```

**Two propagation channels** are available inside the enricher:
- `msg.SetLocal()` — in-process authorization. Never serialized.
- `msg.Attributes["authtype"]` / `msg.Attributes["authid"]` — cross-broker provenance via CloudEvents authcontext extension. Informational only, MUST NOT be used for authorization.

The enricher is general-purpose — not auth-specific. Same pattern for tracing, tenant extraction, etc.

### Authorization — User-Written Middleware

Users write a `message.Middleware` that reads locals and checks permissions:

```go
func authz(next message.ProcessFunc) message.ProcessFunc {
    return func(ctx context.Context, msg *message.Message) ([]*message.Message, error) {
        p, _ := msg.Local(principalKey).(Principal)
        switch msg.Type() {
        case "CreateOrder":
            if !p.HasScope("orders:write") { return nil, ErrForbidden }
        case "GetOrder":
            if !p.HasScope("orders:read") { return nil, ErrForbidden }
        }
        return next(ctx, msg)
    }
}

router.Use(authz)
```

Composable into any component that accepts middleware (router, publisher, passthrough pipe).

### End-to-End Flow (Sidecar Scenario)

```
Client → Envoy (validates JWT, injects X-Auth-* headers) → Pod
           │
    Subscriber.ServeHTTP
           │
    Enricher: for each msg → msg.SetLocal(principalKey, Principal{...})
           │
    UnmarshalPipe (preserves locals)
           │
    Router middleware: authz reads msg.Local(principalKey), checks by event type
           │
    Handler
```

## Prerequisite Fix

`UnmarshalPipe` (`message/pipes.go:33`) does not propagate locals from `RawMessage` to `Message`:

```go
// Current — locals lost
return []*Message{{
    Data:       instance,
    Attributes: raw.Attributes,
    acking:     raw.acking,
}}, nil

// Fix — preserve locals
return []*Message{{
    Data:       instance,
    Attributes: raw.Attributes,
    acking:     raw.acking,
    locals:     raw.locals,
}}, nil
```

## Tasks

### Task 0: Fix UnmarshalPipe Locals Propagation

**Goal:** Preserve locals across the RawMessage → Message boundary.

**Files to Modify:**
- `message/pipes.go` — add `locals: raw.locals` to struct literal

**Acceptance Criteria:**
- [ ] Locals set on RawMessage survive unmarshal
- [ ] Test: set local on RawMessage, unmarshal, verify local on output Message

### Task 1: Subscriber Enricher

**Goal:** Add enricher callback to HTTP subscriber.

**Files to Modify:**
- `message/http/subscriber.go` — add `Enricher` field to config, call in `ServeHTTP`

**Acceptance Criteria:**
- [ ] Enricher called with full message slice after Validator, before channel delivery
- [ ] Enricher has access to `*http.Request` and `[]*message.RawMessage`
- [ ] Nil enricher is no-op (existing behavior unchanged)
- [ ] Validator called before Enricher; on error, all messages nacked

## Implementation Order

```
Task 0 (UnmarshalPipe fix) → Task 1 (Enricher field)
```

## Security Notes

- Always use HTTPS for CloudEvents HTTP transport (per webhook spec)
- Use SDK-Go >= v2.15.2 to avoid CVE-2024-28110 (credential leakage via shared DefaultClient)
- Prefer JWT validation (local) over introspection (network call) for latency-sensitive paths

## Related Plans

- [webhook-abuse-protection](webhook-abuse-protection.md) — CloudEvents webhook handshake (separate concern)

## Acceptance Criteria

- [ ] All tasks completed
- [ ] Tests pass
- [ ] CHANGELOG updated
