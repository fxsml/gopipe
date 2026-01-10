# ADR 0016: Processor Config Struct

**Date:** 2025-12-19
**Status:** Accepted

## Context

Functional options (`WithConcurrency()`, `WithBufferSize()`, etc.) add verbosity and make configuration harder to inspect and test. A config struct provides explicit, type-safe configuration that can be validated, serialized, and passed around.

## Decision

Replace functional options with a config struct for pipe/processor configuration:

```go
// Before: Functional options
pipe := pipe.New(
    pipe.WithConcurrency(4),
    pipe.WithBufferSize(100),
    pipe.WithErrorHandler(handler),
)

// After: Config struct
p := pipe.NewProcessPipe(handler, pipe.Config{
    Concurrency:  4,
    BufferSize:   100,
    ErrorHandler: errHandler,
})
```

Config struct definition:

```go
type Config struct {
    Concurrency    int
    BufferSize     int
    ErrorHandler   func(in any, err error)
    CleanupHandler func(ctx context.Context)
    CleanupTimeout time.Duration
}

var defaultConfig = Config{
    Concurrency: 1,
    ErrorHandler: func(in any, err error) {
        slog.Error("[GOPIPE] Processing failed", slog.Any("input", in), slog.Any("error", err))
    },
}
```

## Consequences

**Breaking Changes:**
- Existing code using functional options must be updated

**Benefits:**
- Configuration is explicit and inspectable
- Easy to validate configuration at construction time
- Config can be serialized (JSON, YAML) for external configuration
- No reflection or interface{} type assertions
- IDE autocomplete shows all options
- Config can be shared and modified

**Drawbacks:**
- Slightly more verbose for simple cases
- Adding new options requires struct field (but is backwards compatible with zero values)

## Links

- Related: ADR 0004, ADR 0015, ADR 0017

## Updates

**2025-12-22:** Updated example to use `NewProcessPipe` constructor. Updated Consequences format to match ADR template.
