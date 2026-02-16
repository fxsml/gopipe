# OAuth2 CloudEvents Integration

**Status:** Proposed
**Depends On:** Transaction Handling (WithValue context wrapper)

## Overview

Research and design for OAuth2 integration with the gopipe message/http package, separating authentication (HTTP middleware) from authorization (router handler middleware using claims via the WithValue context propagation mechanism).

## Goals

1. Authenticate incoming CloudEvents HTTP requests using OAuth2 Bearer tokens
2. Authorize message processing at the handler level using token claims
3. Propagate auth context through the pipeline without using message attributes
4. Align with CloudEvents spec conventions and the existing gopipe architecture

## Relevant CloudEvents Specifications

### HTTP Protocol Binding — Security Stance

The CloudEvents HTTP protocol binding explicitly delegates security to HTTP:

> "This specification does not introduce any new security features for HTTP, or mandate specific existing features to be used."

This confirms that authentication belongs in the HTTP layer, not in the CloudEvents event model.

### Webhook Spec — Authorization Model

The [CloudEvents HTTP Webhook spec](https://github.com/cloudevents/spec/blob/main/cloudevents/http-webhook.md) defines an authorization model based on OAuth 2.0 Bearer tokens (RFC 6750):

- **Authorization header** (preferred): `Authorization: Bearer <token>`
- **URI query parameter** (discouraged): `?access_token=<token>`
- HTTPS is mandatory
- Challenge-based schemes (e.g., HTTP Digest) MUST NOT be used

This is the closest the CloudEvents ecosystem gets to prescribing OAuth2 — and it applies specifically to the HTTP transport layer, reinforcing the separation of concerns.

### Auth Context Extension (Informational Only)

The [authcontext extension](https://github.com/cloudevents/spec/blob/main/cloudevents/extensions/authcontext.md) defines `authtype`, `authid`, and `authclaims` attributes. However, it is explicitly "purely informational and is not intended to secure CloudEvents." These attributes describe who *triggered* the event, not who is *sending* it. They are not suitable for transport-level authentication/authorization.

### Subscriptions Spec — Sink Credentials

The [Subscriptions spec](https://github.com/cloudevents/spec/blob/main/subscriptions/spec.md) defines `ACCESSTOKEN` and `REFRESHTOKEN` credential types for authenticating event *delivery*. This is relevant for the Publisher (outbound), not the Subscriber (inbound).

### What the Spec Does NOT Provide

- No dedicated OAuth2 specification
- No standard for HMAC/signature-based authentication (open discussion in [Issue #703](https://github.com/cloudevents/spec/issues/703))
- No authorization model beyond the webhook abuse-protection handshake

## Architecture

### Principle: Separation of Authentication and Authorization

| Concern | Layer | Mechanism | Question Answered |
|---------|-------|-----------|-------------------|
| **Authentication** | HTTP middleware | Token validation (JWT / introspection) | "Who is this?" |
| **Claims propagation** | Subscriber enricher | `msg.WithValue(claimsKey, claims)` | "Pass identity downstream" |
| **Authorization** | Router handler middleware | Claims check against required permissions | "Are they allowed to do this?" |

### Layer 1: Authentication — HTTP Middleware

Standard `net/http` middleware wrapping the `Subscriber`'s `http.Handler`. This is pure HTTP — no gopipe awareness needed.

```
HTTP request → AuthN middleware → Subscriber.ServeHTTP → channel
                   │
                   ├─ Extract Bearer token from Authorization header
                   ├─ Validate (JWT signature / introspection endpoint)
                   ├─ 401 on invalid/missing token
                   └─ Store claims in http.Request context
```

The `Subscriber` already implements `http.Handler`, so any standard Go HTTP middleware chain works:

```go
mux := http.NewServeMux()
mux.Handle("/events", authMiddleware(subscriber))
```

**Implementation options:**
- **JWT validation** (local, no network call): verify signature, check `exp`/`aud`/`iss`
- **Token introspection** (RFC 7662): query authorization server, cache responses
- **JWKS rotation**: periodically fetch signing keys from `/.well-known/jwks.json`

This layer is intentionally outside gopipe. Users bring their own auth middleware — `go-jwt-middleware`, `coreos/go-oidc`, hand-written, etc.

### Layer 2: Claims Propagation — Subscriber Enricher

The HTTP subscriber currently does not propagate HTTP request context to messages (by design — see `subscriber.go:43`). We need a bridge from `*http.Request` context to message context.

**Proposed mechanism:** An enricher callback on the Subscriber, invoked after message creation but before channel delivery.

```go
// MessageEnricher extracts values from the HTTP request and attaches them
// to the message via WithValue. Called once per message in ServeHTTP.
type MessageEnricher func(r *http.Request, msg *message.RawMessage)
```

```go
type SubscriberConfig struct {
    BufferSize int
    AckTimeout time.Duration
    Enricher   MessageEnricher // optional
}
```

In `ServeHTTP`, after `ce.FromCloudEvent`:

```go
msg, err := ce.FromCloudEvent(&events[i], shared)
if err != nil { ... }

if s.cfg.Enricher != nil {
    s.cfg.Enricher(r, msg)
}

s.ch <- msg
```

**Usage:**

```go
subscriber := http.NewSubscriber(http.SubscriberConfig{
    Enricher: func(r *http.Request, msg *message.RawMessage) {
        claims, _ := authn.ClaimsFromContext(r.Context())
        msg.WithValue(claimsKey, claims)
    },
})
```

**Why an enricher, not automatic propagation:**
- The current design explicitly avoids propagating HTTP context (it's a deliberate boundary)
- An enricher is opt-in and explicit about what crosses the HTTP→message boundary
- Different deployments need different data extracted (claims, tenant ID, trace context)
- It composes cleanly with the WithValue pattern from the transaction branch

### Layer 3: Authorization — Router Handler Middleware

This is the "routes" equivalent. In HTTP, routes have per-endpoint authorization. In gopipe, the handler is the natural equivalent — each handler processes a specific event type (like a URL endpoint processes specific paths).

Authorization middleware is a `message.Middleware` that checks claims stored via `WithValue`. Thanks to the `messageContext.Value()` auto-propagation (from the transaction branch), claims set via `msg.WithValue(key, val)` are visible to any code calling `ctx.Value(key)` inside the handler.

**Middleware factories:**

```go
// RequireClaim rejects messages where the claim at key doesn't match the expected value.
func RequireClaim(key string, expected any) message.Middleware

// RequireScope rejects messages that lack the given OAuth2 scope.
func RequireScope(scope string) message.Middleware

// RequireAny accepts if any of the provided checks pass.
func RequireAny(checks ...message.Middleware) message.Middleware

// Authorize accepts a custom authorization function.
func Authorize(fn func(ctx context.Context, msg *message.Message) error) message.Middleware
```

**Usage with per-handler middleware:**

Currently, `Router.Use()` applies middleware globally. For handler-level authorization (the "routes" analogy), we have two options:

**Option A — Compose into the handler itself:**

Wrap the handler's processing function with authorization logic before registration. No router API changes needed.

```go
handler := message.NewCommandHandler[PlaceOrder, OrderPlaced](
    placeOrderFn,
    message.CommandHandlerConfig{Source: "/orders"},
)
// Wrap with authorization
handler = authz.WithMiddleware(handler, authz.RequireScope("orders:write"))

engine.AddHandler("place-order", nil, handler)
```

Where `authz.WithMiddleware` returns a new `Handler` that runs the middleware before delegating to the inner handler.

**Option B — Per-handler middleware on AddHandler:**

Extend the router API with optional per-handler middleware.

```go
engine.AddHandler("place-order", nil, handler,
    message.WithMiddleware(authz.RequireScope("orders:write")),
)
```

Option A is simpler and requires no changes to the Router. Option B is more ergonomic but changes the `AddHandler` signature. Both are compatible with the existing architecture.

### How Claims Flow End-to-End

```
1. HTTP request with Authorization: Bearer <token>
       │
2. AuthN middleware validates token, stores claims in r.Context()
       │
3. Subscriber.ServeHTTP creates RawMessage from CloudEvent
       │
4. Enricher bridges: msg.WithValue(claimsKey, claims)
       │
5. Engine: unmarshal → Router → handler
       │
6. Router applies middleware chain:
   AuthZ middleware → ctx.Value(claimsKey) → check scopes/claims →
       │                                         │
       │                                    reject (return error)
       │
7. Handler receives ctx with claims accessible via ctx.Value()
```

The key insight: steps 6 and 7 work because of the `messageContext.Value()` chain from the transaction branch. When the router calls `msg.Context(ctx)`, the resulting context checks the message's WithValue store before delegating to the parent. No bridging middleware needed between message and context.

### What We Do NOT Do

- **Do not use message `Attributes` for auth context.** Attributes serialize across broker boundaries. OAuth claims are request-scoped and in-process-only. The WithValue context wrapper is the correct propagation mechanism.
- **Do not embed credentials in CloudEvents.** Per the authcontext extension spec: `authclaims` "MUST NOT contain actual credentials for impersonation."
- **Do not build an OAuth2 server.** gopipe validates tokens, it doesn't issue them.
- **Do not couple to a specific OAuth2 library.** The enricher and middleware patterns accept any claims type.

## Comparison with Other Systems

| System | AuthN | AuthZ | Claims Propagation |
|--------|-------|-------|--------------------|
| **Knative Eventing** | OIDC tokens on addressable resources | `EventPolicy` CRD (from/to/filters) | Kubernetes service accounts |
| **AsyncAPI** | Declarative security schemes (all 4 OAuth2 flows) | Per-operation security bindings | Spec-level, not runtime |
| **CloudEvents SDK-Go** | `WithMiddleware` on HTTP client | None built-in | `context.WithValue` via middleware |
| **gopipe (proposed)** | HTTP middleware (user-provided) | Handler-level middleware (claims check) | `msg.WithValue` → `messageContext.Value` |

The gopipe approach is closest to the CloudEvents SDK-Go pattern but adds structured authorization at the handler level and uses the WithValue mechanism instead of relying on HTTP context propagation (which gopipe intentionally does not do).

## Security Notes

- Always use HTTPS for CloudEvents HTTP transport (per webhook spec)
- Use SDK-Go >= v2.15.2 to avoid CVE-2024-28110 (credential leakage via shared DefaultClient)
- Cache token introspection responses, but never beyond the token's `exp` time
- Prefer JWT validation (local) over introspection (network call) for latency-sensitive paths

## Implementation Order

```
Transaction branch (WithValue) ──→ Enricher on Subscriber ──→ AuthZ middleware
        (prerequisite)                  (Layer 2)                (Layer 3)
```

Layer 1 (HTTP AuthN middleware) is external — users bring their own. Gopipe provides documentation and examples but no built-in OAuth2 library dependency.

## Open Questions

1. **Option A vs B for per-handler middleware:** Should authorization be composed into handlers (no API change) or added as a router feature (new `AddHandler` option)?
2. **Enricher generality:** Should the enricher be OAuth-specific or a general-purpose HTTP→message bridge? (Recommendation: general-purpose — it's useful for tracing, tenant extraction, etc.)
3. **Publisher auth:** For outbound requests, should the Publisher support injecting Bearer tokens from a token source (e.g., `golang.org/x/oauth2.TokenSource`)? This aligns with the Subscriptions spec's ACCESSTOKEN credential type.
4. **Webhook abuse protection:** Should we implement the CloudEvents webhook handshake (OPTIONS with `WebHook-Request-Origin`)? This is orthogonal to OAuth2 but related to HTTP security.
