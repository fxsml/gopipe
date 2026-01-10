# Config Convention Analysis

**Status:** Analysis
**Related:** Plan 0006

## Current State

### Engine Method Configs

| Method | Config | Used Fields | Unused Fields |
|--------|--------|-------------|---------------|
| `AddInput(ch, cfg)` | `InputConfig` | `Matcher` | `Name` |
| `AddRawInput(ch, cfg)` | `RawInputConfig` | `Matcher` | `Name` |
| `AddOutput(cfg)` | `OutputConfig` | `Matcher` | `Name` |
| `AddRawOutput(cfg)` | `RawOutputConfig` | `Matcher` | `Name` |
| `AddLoopback(cfg)` | `LoopbackConfig` | `Matcher` | `Name` |
| `AddHandler(h, cfg)` | `HandlerConfig` | `Matcher` | `Name` |

**Observation:** All configs have identical structure (`Name` + `Matcher`), but `Name` is never used.

### Constructor Configs

| Constructor | Config | Notes |
|-------------|--------|-------|
| `NewEngine(cfg)` | `EngineConfig` | All fields used |
| `NewRouter(cfg)` | `RouterConfig` | All fields used |
| `NewCommandHandler(fn, cfg)` | `CommandHandlerConfig` | Different pattern - constructor config |

**Convention:** Constructor configs are different from method configs. Constructor configs configure the component, method configs configure individual operations.

---

## Option 1: Keep Configs (Current)

**Signature:**
```go
AddOutput(cfg OutputConfig) <-chan *Message
AddInput(ch <-chan *Message, cfg InputConfig) error
AddHandler(h Handler, cfg HandlerConfig) error
```

**Pros:**
- Extensible for future fields
- Consistent struct-based API
- Named fields (self-documenting)

**Cons:**
- Verbose: `AddOutput(OutputConfig{Matcher: m})` vs `AddOutput(m)`
- Empty configs common: `AddInput(ch, InputConfig{})`
- 6 config types that are essentially identical

---

## Option 2: Matcher as Parameter

**Signature:**
```go
AddOutput(matcher Matcher) <-chan *Message  // nil = catch-all
AddInput(ch <-chan *Message, matcher Matcher) error
AddHandler(h Handler, matcher Matcher) error
```

**Pros:**
- Concise: `AddOutput(nil)` or `AddOutput(match.Types("foo"))`
- No empty struct boilerplate
- Matcher is the only used field anyway

**Cons:**
- Adding future fields requires signature change
- `Name` would require separate method or functional options
- Less self-documenting

---

## Option 3: Functional Options

**Signature:**
```go
AddOutput(opts ...OutputOption) <-chan *Message
AddInput(ch <-chan *Message, opts ...InputOption) error

// Usage:
engine.AddOutput(WithMatcher(m), WithName("events"))
engine.AddInput(ch)  // no options = defaults
```

**Pros:**
- Clean defaults (no empty struct)
- Extensible without signature changes
- Self-documenting option names

**Cons:**
- More types to define (Option funcs)
- Slightly more complex implementation
- Go convention varies on this

---

## Option 4: Hybrid - Matcher + Functional Options

**Signature:**
```go
AddOutput(matcher Matcher, opts ...OutputOption) <-chan *Message
AddInput(ch <-chan *Message, matcher Matcher, opts ...InputOption) error

// Usage:
engine.AddOutput(nil)  // catch-all, no options
engine.AddOutput(match.Types("foo"), WithName("events"))
```

**Pros:**
- Most common use case (just matcher) is concise
- Extensible for future options
- Clear what's required vs optional

**Cons:**
- `nil` for catch-all less clear than `OutputConfig{}`
- Two concepts: parameter + options

---

## Future Features That Would Need Config

| Feature | Needed For | Config Impact |
|---------|------------|---------------|
| `Name` for metrics/tracing | All | Already in config (unused) |
| Per-input/output buffer size | Performance tuning | New field |
| Priority hints | Ordering control | New field |
| Error handler override | Per-input/output errors | New field |
| Retry policy | Resilience | New field (or separate method) |
| Backpressure strategy | Flow control | New field |

**Analysis:** Future features favor keeping configs, but most are advanced use cases.

---

## Recommendation

### Short-term: Simplify with Matcher Parameter

For the common case (which is 99% of usage):

```go
// Before
engine.AddOutput(OutputConfig{Matcher: match.Types("event")})
engine.AddInput(ch, InputConfig{})

// After
engine.AddOutput(match.Types("event"))
engine.AddInput(ch, nil)
```

### Long-term: Add Functional Options When Needed

When `Name` or other features are actually implemented:

```go
engine.AddOutput(match.Types("event"), WithName("events"))
```

### Alternative: Keep Configs, Add Convenience Methods

```go
// Keep existing
AddOutput(cfg OutputConfig) <-chan *Message

// Add convenience
AddCatchAllOutput() <-chan *Message  // = AddOutput(OutputConfig{})
AddMatchingOutput(m Matcher) <-chan *Message  // = AddOutput(OutputConfig{Matcher: m})
```

---

## Questions to Resolve

1. **Is `Name` going to be implemented?** If yes, configs make sense. If no, remove it.
2. **How common is the empty config case?** If very common, simplify.
3. **What's the Go convention in similar libraries?** Research needed.
4. **Do we want symmetry with pipe package?** Pipe uses configs in constructors.

---

## Decision Needed

Before implementing, decide:

- [ ] Keep current config pattern (extensible, verbose)
- [ ] Matcher as parameter (concise, less extensible)
- [ ] Functional options (clean, more complex)
- [ ] Hybrid approach
- [ ] Convenience methods alongside configs
