# ADR 0008: Remove Attributes Thread-Safety

**Date:** 2025-11-01
**Status:** Implemented

## Context
Attributes was a struct with `sync.RWMutex` and methods `Get/Set/Delete/Range`. This added complexity and overhead for most use cases where messages are not shared across goroutines.

## Decision
Change Attributes from `*Attributes` struct to plain `map[string]any`.

Remove all methods: `Get`, `Set`, `Delete`, `Range`, accessor methods.

Add simple pass-through functions with `Props` suffix:
- `IDProps(m map[string]any) (string, bool)`
- `CorrelationIDProps(m map[string]any) (string, bool)`
- `CreatedAtProps(m map[string]any) (time.Time, bool)`
- `SubjectProps(m map[string]any) (string, bool)`
- `ContentTypeProps(m map[string]any) (string, bool)`

## Consequences

**Breaking Changes:**
- `msg.Attributes.Get("key")` → `msg.Attributes["key"]`
- `msg.Attributes.Set("key", val)` → `msg.Attributes["key"] = val`
- `msg.Attributes.Delete("key")` → `delete(msg.Attributes, "key")`
- `msg.Attributes.Range(fn)` → `for k, v := range msg.Attributes { ... }`
- `msg.ID()` → `IDProps(msg.Attributes)`

**Benefits:**
- Zero overhead map access
- No lock contention
- Standard Go idioms
- Simpler implementation

**Drawbacks:**
- No thread-safety (users must synchronize if sharing messages)
- More verbose for typed property access

## Links

- Related: ADR 0007 (Public Message Fields)
- Related: ADR 0009 (Remove Noisy Attributes)

## Updates

**2025-11-15:** API changed from `*Props` suffix functions to methods on `Attributes` type (e.g., `msg.Attributes.ID()`).

**2025-12-22:** Updated Consequences format to match ADR template.
