# ADR 0011: CQRS Implementation

**Date:** 2025-12-08
**Status:** Implemented

## Context

CQRS (Command Query Responsibility Segregation) separates read and write operations in event-driven architectures. gopipe needed type-safe command handlers that receive typed commands and emit typed events.

## Decision

Implement CQRS support directly in the `message` package:

```go
// Command handler: Command -> Events
func NewCommandHandler[C, E any](
    fn func(ctx context.Context, cmd C) ([]E, error),
    cfg CommandHandlerConfig,
) Handler

// CommandHandlerConfig configures a command handler.
type CommandHandlerConfig struct {
    Source string          // required, CE source attribute
    Naming EventTypeNaming // derives CE types for input and output
}

// Generic handler for event processing (side effects)
func NewHandler[T any](
    fn func(ctx context.Context, msg *Message) ([]*Message, error),
    naming EventTypeNaming,
) Handler
```

Design principles: Type-safe, simple, convention-based type derivation.

## Consequences

**Benefits:**
- Type-safe CQRS with generics
- Command handlers receive typed values directly
- Automatic CloudEvents attribute generation
- EventTypeNaming derives CE types from Go types

**Drawbacks:**
- Runtime type assertions in generic handlers

## Links

- Related: ADR 0020 (Message Engine Architecture)
- Package: `github.com/fxsml/gopipe/message`

## Updates

**2025-12-11:** Package moved from `cqrs` to `message/cqrs`.

**2025-12-22:** Fixed Links references. Updated Consequences format to match ADR template.

**2026-01-09:** Simplified. CQRS functionality integrated directly into `message` package. Removed separate `message/cqrs` package.
