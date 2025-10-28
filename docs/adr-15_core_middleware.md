## ADR-15: Core Middleware via Functional Options

See #15

### Context

Core cross-cutting concerns (ADR-9, ADR-8) need a predictable, minimal way to attach to processors without manual middleware ordering. Concerns: recover, metadata, metrics (includes logging), context shaping. Cancel handling remains part of the innermost processor implementation and is not middleware.

### Decision

Expose core concerns through dedicated options:

```go
WithRecover()            // panic recovery (outermost)
WithMetadata(provider)   // context + error metadata enrichment
WithMetrics(collector)   // per-item metrics; supports multiple collectors
WithLoggerConfig(cfg)    // logger configuration integrated into metrics
WithTimeout(d)           // per-item context timeout
WithoutContextPropagation() // disable parent context propagation
WithCancel(fn)           // replace cancel func (innermost override)
WithMiddleware(mw)       // custom middleware (advanced)
```

Ordering enforced internally (`config.applyMiddleware`):
1. Cancel override
2. Context (timeout / propagation)
3. Metrics (includes logger)
4. User custom middleware (as given)
5. Metadata providers
6. Recover (outermost)

### Consequences

**Positive**
- reduced misordering risk
- concise activation
- consistent chain
- simplified middleware usage
- provides logger per default


**Negative**
- less manual control over fine-grained ordering of core concerns.

**Neutral**
- custom behavior still possible via `WithMiddleware`.
