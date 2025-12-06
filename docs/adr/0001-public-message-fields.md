# ADR 0001: Public Message Fields

## Status
Accepted

## Context
Message[T] had private `payload` and `properties` fields with accessor methods. This created unnecessary indirection and verbosity for simple field access.

## Decision
Make `Payload` and `Properties` public fields:
- `Payload T` - direct access to message data
- `Properties map[string]any` - simple map for properties

Remove accessor methods `Payload()` and `Properties()`.

## Consequences

**Breaking Changes:**
- `msg.Payload()` → `msg.Payload`
- `msg.Properties()` → `msg.Properties`

**Benefits:**
- Direct field access
- Less verbose
- Simpler API
- Standard Go idiom

**Drawbacks:**
- No encapsulation (acceptable for data structures)
