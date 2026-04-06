# OAuth2 CloudEvents — Design Evolution: From Auth Framework to One Config Field

**Status:** Decided
**Related Plan:** [oauth2-cloudevents.md](oauth2-cloudevents.md)

## Context

We needed OAuth2 support for the HTTP CloudEvents adapter. The design question: how much framework should gopipe provide for authentication, claims propagation, and authorization?

---

## Phase 1: Three-Layer Auth Framework

The initial design proposed three layers, each with dedicated gopipe API:

1. **Authentication** — external HTTP middleware (no API, fine)
2. **Claims propagation** — `MessageEnricher` named type + `SubscriberConfig.Enricher` field, using `WithValue` (later `SetLocal`) to bridge HTTP context into messages
3. **Authorization** — new `authz` package with `Policy` type, `ByEventType()`, `RequireScope()`, `RequireClaim()`, `RequireAny()`, `Authorize()`

Authorization was router-coupled: registered via `router.Use(authz.ByEventType(...))`.

### Problem: Router Coupling

The proxy scenario exposed the flaw. An HTTP-to-AMQP bridge has no router — it just validates tokens and forwards events. Authentication must work standalone. This led to decoupling authorization from the router.

---

## Phase 2: Standalone Authorization Middleware

Authorization was reframed as a standalone `message.Middleware`, composable into any pipe component:

```go
router.Use(authz.ByEventType(policies))     // router
publisher.Use(authz.ByEventType(policies))   // publisher
pipe.Use(authz.ByEventType(policies))        // passthrough pipe
```

The `authz` package API remained: `ByEventType`, `Policy`, `RequireScope`, `RequireClaim`, `RequireAny`, `Authorize`.

### What Changed

Authorization decoupled from router. But the API surface was still large: 1 new package, 6 exports.

---

## Phase 3: API Surface Audit

Evaluated every proposed export against necessity:

### `Policy` Type

If `Policy` is `func(context.Context, *message.Message) error`, then `Authorize(fn)` is just a type conversion. **Cut.**

### `RequireScope` / `RequireClaim`

These need to extract scopes/claims from the token. But the claims type is user-defined — could be JWT, OIDC, custom struct. gopipe can't generalize without either:
- Coupling to a specific auth library, or
- Defining a `Claims` interface that tries to abstract all auth models

Both pollute the API. **Cut.**

### `RequireAny`

Trivial combinator a user writes in 5 lines. **Cut.**

### `ByEventType`

Saves a switch statement. The user writes this in 10 lines:

```go
func authz(next message.ProcessFunc) message.ProcessFunc {
    return func(ctx context.Context, msg *message.Message) ([]*message.Message, error) {
        p, _ := msg.Local(principalKey).(Principal)
        switch msg.Type() {
        case "CreateOrder":
            if !p.HasScope("orders:write") { return nil, ErrForbidden }
        }
        return next(ctx, msg)
    }
}
```

Clear, explicit, no new types to learn. The user controls the claims type. **Cut.**

### `MessageEnricher` Named Type

`func(*http.Request, *message.RawMessage)` inline in the config is sufficient. Not an interface, not reused elsewhere. **Cut** the named type — keep it as an anonymous function type on the config field.

### Publisher `TokenSource`

Users can set `Authorization` headers via the existing `PublisherConfig.Headers` field. No demand yet. **Deferred.**

### Result

Everything in Layer 3 (authorization) is user code. The only gopipe API is one config field on `SubscriberConfig`.

---

## Phase 4: Headers-as-Locals Alternative

Considered: should the HTTP subscriber automatically set a local for each non-CE HTTP header? This would eliminate the enricher entirely — a regular middleware could read the `Authorization` header from `msg.Local(httpHeadersKey)`.

### Rejected — Four Problems

**1. Lost HTTP error semantics.** If HTTP auth middleware rejects a request, the caller gets `401 Unauthorized`. If a `message.Middleware` rejects a message, it nacks, and the subscriber returns `500 Internal Server Error`. The right HTTP status code is lost once you cross into the message world.

**2. Invalid requests enter the pipeline.** The subscriber accepts the message, puts it on the channel, blocks waiting for ack/nack. The message occupies a buffer slot, a worker, goes through unmarshaling — only to be rejected later. With HTTP-level auth, the request never enters the pipeline.

**3. Transport leakage.** A handler discovers `msg.Local(httpHeadersKey)` and learns the message arrived via HTTP. The message abstraction is supposed to be transport-independent. Headers are an HTTP concept.

**4. Raw header vs validated claims.** The useful thing isn't `Authorization: Bearer <token>` — it's the *validated claims* extracted from it (subject, scopes, expiry). The HTTP auth middleware (or sidecar) validates the token and produces claims. Copying raw headers means re-validating in the message pipeline, duplicating work.

### The Enricher Is the Minimal Bridge

The enricher is opt-in, explicit about what crosses the HTTP→message boundary, and carries validated claims — not raw headers. One config field, zero types.

---

## Decision

**One config field on `SubscriberConfig`. Everything else is user code.**

```go
type SubscriberConfig struct {
    BufferSize int
    AckTimeout time.Duration
    Enricher   func(*http.Request, *message.RawMessage) // optional
}
```

### What gopipe ships

| Component | API |
|-----------|-----|
| Authentication | None — external HTTP middleware or sidecar |
| Propagation | `SubscriberConfig.Enricher` — one config field |
| Authorization | None — user writes a `message.Middleware` |

### What the user owns

- `Principal` type (their domain, their claims model)
- Key type for locals (unexported struct for collision safety)
- Enricher function (3 lines: extract claims, call `SetLocal`)
- Authorization middleware (10 lines: read local, switch on event type)

### Why This Is Enough

The existing primitives cover the full auth flow:
- `SetLocal`/`Local` — carry claims on messages (from locals plan)
- `message.Middleware` — intercept processing for authorization checks
- `http.Handler` — compose auth middleware with subscriber
- `msg.Type()` — dispatch authorization by event type

No new types, no new packages, no new interfaces. The enricher is the only missing piece — the bridge between the HTTP world and the message world that the subscriber intentionally blocks.

### Prerequisite

`UnmarshalPipe` must propagate locals from `RawMessage` to `Message`. Currently locals are lost during unmarshaling — a one-line fix (`locals: raw.locals`).

## API Evolution Path

If patterns emerge across multiple projects, we can later add:
- Named `Principal` interface (if claims models converge)
- `ByEventType` convenience (if the switch pattern becomes boilerplate)
- Publisher `TokenSource` (if outbound auth demand materializes)

But we don't ship them until we see the need. Three lines of user code is not boilerplate.
