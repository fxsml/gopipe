# ADR 0007: Public Message Fields

**Date:** 2025-11-01
**Status:** Implemented

## Context
Message[T] had private `data` and `attributes` fields with accessor methods. This created unnecessary indirection and verbosity for simple field access.

## Decision
Make `Data` and `Attributes` public fields:
- `Data T` - direct access to message data
- `Attributes map[string]any` - simple map for attributes

Remove accessor methods `Data()` and `Attributes()`.

## Consequences

**Breaking Changes:**
- `msg.Data()` → `msg.Data`
- `msg.Attributes()` → `msg.Attributes`

**Benefits:**
- Direct field access
- Less verbose
- Simpler API
- Standard Go idiom

**Drawbacks:**
- No encapsulation (acceptable for data structures)

## Links

- Related: ADR 0008 (Remove Attributes Thread-Safety)
- Related: ADR 0010 (Dual Message Types)

## Updates

**2025-12-22:** Added Links section. Updated Consequences format to match ADR template.
