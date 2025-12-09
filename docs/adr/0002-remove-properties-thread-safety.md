# ADR 0002: Remove Properties Thread-Safety

**Date:** 2024-11-01
**Status:** Implemented

## Context
Properties was a struct with `sync.RWMutex` and methods `Get/Set/Delete/Range`. This added complexity and overhead for most use cases where messages are not shared across goroutines.

## Decision
Change Properties from `*Properties` struct to plain `map[string]any`.

Remove all methods: `Get`, `Set`, `Delete`, `Range`, accessor methods.

Add simple pass-through functions with `Props` suffix:
- `IDProps(m map[string]any) (string, bool)`
- `CorrelationIDProps(m map[string]any) (string, bool)`
- `CreatedAtProps(m map[string]any) (time.Time, bool)`
- `SubjectProps(m map[string]any) (string, bool)`
- `ContentTypeProps(m map[string]any) (string, bool)`

## Consequences

**Breaking Changes:**
- `msg.Properties.Get("key")` → `msg.Properties["key"]`
- `msg.Properties.Set("key", val)` → `msg.Properties["key"] = val`
- `msg.Properties.Delete("key")` → `delete(msg.Properties, "key")`
- `msg.Properties.Range(fn)` → `for k, v := range msg.Properties { ... }`
- `msg.ID()` → `IDProps(msg.Properties)`

**Benefits:**
- Zero overhead map access
- No lock contention
- Standard Go idioms
- Simpler implementation

**Drawbacks:**
- No thread-safety (users must synchronize if sharing messages)
- More verbose for typed property access
