# ADR 0016: Channel Package Separation

**Date:** 2024-10-01
**Status:** Implemented

## Context

The gopipe library mixed pipeline orchestration with low-level channel helpers (Map, Transform, Collect, Flatten, Batch). This made the core package heavier and less modular.

## Decision

Extract all channel helper functions into a new `channel` package. The `gopipe` package focuses solely on pipeline orchestration, middleware, and processing logic.

## Consequences

**Positive:**
- Clear separation of concerns: pipeline logic vs data-flow helpers
- Lightweight core package easier to maintain
- Channel utilities reusable independently

**Negative:**
- Users need additional import for advanced channel operations
- Slightly more fragmented API surface

## Links

- Package: `github.com/fxsml/gopipe/channel`
