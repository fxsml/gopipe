# OAuth2 CloudEvents Integration

**Status:** Proposed
**Depends On:** Transaction Handling (WithValue context wrapper)

## Overview

Research and design for OAuth2 integration with the gopipe message/http package. Three independent concerns:

1. **Authentication** — HTTP middleware validates tokens (gatekeeper). Works standalone, does not require a router or handler.
2. **Auth context propagation** — Enricher bridges HTTP identity into messages. Two channels: `WithValue` for in-process claims (security-sensitive, does not cross broker boundaries), and CloudEvents `authcontext` attributes for cross-broker provenance (informational only).
3. **Authorization** — Router-level middleware checks claims against required permissions. Optional, only relevant when a router is present.

## Goals

1. Authenticate incoming CloudEvents HTTP requests using OAuth2 Bearer tokens
2. Propagate auth context: `WithValue` for in-process authorization, `authcontext` attributes for cross-broker provenance
3. Authorize message processing at the router level using token claims (optional)
4. Support topologies without routers (e.g., HTTP-to-AMQP proxy) where authentication alone is sufficient
5. Align with CloudEvents spec conventions and the existing gopipe architecture

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

### Principle: Three Independent Concerns

| Concern | Layer | Mechanism | Question Answered | Required? |
|---------|-------|-----------|-------------------|-----------|
| **Authentication** | HTTP middleware | Token validation (JWT / introspection) | "Who is this?" | Yes (gatekeeper) |
| **Auth context** | Subscriber enricher | `WithValue` (in-process) + `authcontext` attributes (cross-broker) | "Who triggered this?" | Optional |
| **Authorization** | Router middleware | Claims check by event type | "Are they allowed to do this?" | Optional (requires router) |

Authentication is complete on its own. A simple HTTP-to-AMQP proxy that forwards events to an internal broker needs only Layer 1 — no router, no handler, just the gatekeeper. Auth context and authorization build on top when needed.

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

### Layer 2: Auth Context Propagation — Subscriber Enricher

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

**Two propagation channels — different purposes:**

The enricher can set both:

1. **`WithValue` — in-process authorization claims.** Security-sensitive. Used by Layer 3 authorization middleware. Does not cross broker boundaries (lost on serialization). This is the right channel for OAuth2 scopes, roles, and any data used for access control decisions.

2. **`authcontext` attributes — cross-broker provenance.** Informational only. Survives serialization across broker boundaries (HTTP → AMQP → Kafka). Tells downstream handlers who *triggered* the event. Uses the CloudEvents [authcontext extension](https://github.com/cloudevents/spec/blob/main/cloudevents/extensions/authcontext.md) (`authtype`, `authid`, `authclaims`). Per spec: MUST NOT contain actual credentials.

```go
subscriber := http.NewSubscriber(http.SubscriberConfig{
    Enricher: func(r *http.Request, msg *message.RawMessage) {
        claims, _ := authn.ClaimsFromContext(r.Context())

        // In-process: for authorization middleware (Layer 3)
        msg.WithValue(claimsKey, claims)

        // Cross-broker: informational provenance for downstream handlers
        msg.Attributes["authtype"] = claims.Type()   // e.g., "user", "service_account"
        msg.Attributes["authid"]   = claims.Subject() // e.g., hashed user ID
    },
})
```

**Why both channels:**
- `WithValue` is secure (in-process only) but ephemeral (lost on serialization)
- `authcontext` attributes survive broker boundaries but are informational only (not for access control)
- A handler on an internal AMQP broker can see *who triggered* the original HTTP request via `authid`, without the original OAuth2 token being forwarded

**The proxy scenario:** An HTTP-to-AMQP bridge doesn't need a router. Layer 1 (auth middleware) rejects unauthorized requests. The enricher sets `authcontext` attributes on forwarded messages so downstream consumers on the internal broker know who triggered the event. Authentication is complete — no authorization layer needed.

```
External HTTP → AuthN middleware → Subscriber → enricher → AMQP broker
                  (gatekeeper)                    (sets authcontext)
                                                        │
                                          Internal consumers read
                                          authtype/authid attributes
```

**Why an enricher, not automatic propagation:**
- The current design explicitly avoids propagating HTTP context (it's a deliberate boundary)
- An enricher is opt-in and explicit about what crosses the HTTP→message boundary
- Different deployments need different data extracted (claims, tenant ID, trace context)
- It composes cleanly with the WithValue pattern from the transaction branch

### Layer 3: Authorization — Router-Level Middleware by Event Type

The router is the natural place for authorization. Each handler processes a specific event type (via `Handler.EventType()`), making event type the equivalent of an HTTP URL path. Authorization middleware registered on the router inspects the message's `type` attribute and checks claims accordingly.

Authorization middleware is a `message.Middleware` that checks claims stored via `WithValue`. Thanks to the `messageContext.Value()` auto-propagation (from the transaction branch), claims set via `msg.WithValue(key, val)` are visible to any code calling `ctx.Value(key)` inside the handler.

**Design: Event type as authorization "path"**

The router already dispatches by event type. Authorization maps the same key to required permissions. This centralizes security policy in one place (analogous to HTTP router-level auth config) and keeps handlers free of auth concerns.

**Granularity boundary:** Router-level authorization answers "is this caller allowed to trigger processing of *this event type*?" Business rules that depend on event data (e.g., "can this user create orders above $10k?") belong in the handler. This mirrors HTTP: route middleware checks roles/scopes, the handler checks business logic.

**Middleware factories:**

```go
// ByEventType maps event types to authorization policies.
// Messages with types not in the map are rejected (closed by default).
func ByEventType(policies map[string]Policy) message.Middleware

// RequireScope creates a Policy that checks for an OAuth2 scope in the claims.
func RequireScope(scope string) Policy

// RequireClaim creates a Policy that checks for a specific claim value.
func RequireClaim(key string, expected any) Policy

// RequireAny accepts if any of the provided policies pass.
func RequireAny(policies ...Policy) Policy

// Authorize creates a Policy from a custom authorization function.
func Authorize(fn func(ctx context.Context, msg *message.Message) error) Policy
```

**Usage — single authorization middleware on the router:**

```go
router.Use(authz.ByEventType(map[string]authz.Policy{
    "CreateOrder": authz.RequireScope("orders:write"),
    "GetOrder":    authz.RequireScope("orders:read"),
    "CancelOrder": authz.RequireAny(
        authz.RequireScope("orders:admin"),
        authz.RequireClaim("role", "manager"),
    ),
}))
```

**Usage — custom authorization function:**

```go
router.Use(authz.ByEventType(map[string]authz.Policy{
    "CreateOrder": authz.Authorize(func(ctx context.Context, msg *message.Message) error {
        claims := authz.ClaimsFromContext(ctx)
        if !claims.HasScope("orders:write") {
            return authz.ErrForbidden
        }
        return nil
    }),
}))
```

**Different routers, different policies:** Because authorization is registered per-router via `router.Use()`, different routers can enforce different policies. An "admin" router and a "public" router can coexist with distinct authorization requirements for the same event types — no API changes needed.

**Why router-level, not per-handler:**
- Centralizes security policy — all authorization rules visible in one place
- Event type is already the dispatch key, matching it for auth is natural
- Composable across routers (admin vs public)
- No changes to `AddHandler` or `Handler` interface
- Consistent with how `Router.Use()` already works for other cross-cutting concerns

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

- **Do not use message `Attributes` for authorization decisions.** Authorization uses `WithValue` (in-process, not serialized). The `authcontext` attributes are strictly informational provenance — they tell downstream handlers who triggered the event, but MUST NOT be used as an authorization mechanism.
- **Do not embed credentials in CloudEvents.** Per the authcontext extension spec: `authclaims` "MUST NOT contain actual credentials for impersonation." Use hashed identifiers, not tokens.
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

## Decisions

1. **Authentication is standalone.** HTTP auth middleware works without a router or handler. An HTTP-to-AMQP proxy only needs Layer 1.
2. **Two propagation channels.** `WithValue` for in-process authorization (security-sensitive, ephemeral). `authcontext` attributes for cross-broker provenance (informational, survives serialization). Never use attributes for authorization decisions.
3. **Router-level authorization by event type.** Authorization middleware is registered on the router via `router.Use()`, mapping event types to policies. No changes to `AddHandler` or `Handler` interface needed. Different routers can enforce different policies. Optional — only relevant when a router is present.
4. **General-purpose enricher.** The `MessageEnricher` is not OAuth-specific. It bridges any HTTP→message context: claims, tenant ID, trace context, etc.
5. **Publisher auth is HTTP-specific.** The outbound counterpart to the inbound enricher. Optional `TokenSource` on the Publisher injects `Authorization: Bearer <token>` into outbound HTTP requests. Lives in `message/http`, not relevant for other transports (Kafka, NATS, etc. have their own auth mechanisms). Aligns with the Subscriptions spec's ACCESSTOKEN credential type.

## Related Plans

- [webhook-abuse-protection](webhook-abuse-protection.md) — CloudEvents webhook handshake (separate concern)

## Open Questions

1. **Closed vs open default for ByEventType:** Should event types not in the policy map be rejected (closed by default) or allowed (open by default)? Closed is safer but requires listing all types.
