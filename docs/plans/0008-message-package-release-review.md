# Plan 0008: Message Package Release Review

**Status:** Review
**Type:** Summary / Release Checklist

## Executive Summary

The message package is **nearly production-ready** with a clean, simple architecture. Core functionality is solid with 91.1% test coverage. Key issues to address: outdated documentation, missing tests for `match` subpackage, and consider middleware support for first release.

---

## Code Quality Assessment

### Strengths

| Aspect | Rating | Notes |
|--------|--------|-------|
| **Simplicity** | ✅ Excellent | Engine reduced to ~230 lines, Start() is 15 lines |
| **Readability** | ✅ Excellent | Clear function names, good comments |
| **Architecture** | ✅ Excellent | Single merger + distributor, convenience wrappers |
| **Test Coverage** | ✅ Good | 91.1% for message package |
| **Error Handling** | ✅ Good | Consistent error handler pattern |
| **Go Idioms** | ✅ Good | Proper use of interfaces, generics, channels |

### Channel/Pipe Package Usage

| Usage | Assessment |
|-------|------------|
| `channel.Filter` | ✅ Correct - used for input matchers |
| `channel.Process` | ✅ Correct - used for marshal/unmarshal (1:0/1 mapping) |
| `pipe.Merger` | ✅ Correct - merges all inputs |
| `pipe.Distributor` | ✅ Correct - routes to outputs |
| `pipe.ProcessPipe` | ✅ Correct - used in Router for 1:N handler dispatch |

---

## Issues to Address Before Release

### Critical (Must Fix)

#### 1. README Outdated
**File:** `message/README.md`

The README shows old two-merger architecture:
```
RawMerger → Unmarshal → TypedMerger → Handler → Distributor
```

But code now uses single merger:
```
RawInput → filter → unmarshal ─┐
                               ├→ Merger → Router → Distributor
TypedInput ────────────────────┘
```

**Action:** Update README to reflect simplified architecture.

#### 2. Match Package Has No Tests
**Coverage:** 0% for `message/match`

Files without tests:
- `match/match.go` - `All()`, `Any()`
- `match/types.go` - `Types()`
- `match/sources.go` - `Sources()`
- `match/like.go` - `Like()`, `LikeAny()`

**Action:** Add tests for match package.

### Important (Should Fix)

#### 3. Name Field Unused in Configs
All configs have `Name string` field that is never used:
- `InputConfig.Name`
- `OutputConfig.Name`
- `HandlerConfig.Name`
- etc.

**Options:**
1. Remove `Name` field (breaking change later if we add metrics)
2. Add basic logging that uses `Name`
3. Document as "reserved for future metrics"

**Recommendation:** Keep field, add slog logging with name.

#### 4. No Graceful Shutdown Documentation
Users need guidance on proper shutdown:
```go
cancel() // cancel context
<-done   // wait for engine
```

**Action:** Add shutdown section to README.

#### 5. Missing CloudEvents Attribute Constants
README mentions constants that don't exist:
```go
const (
    AttrID   = "id"
    AttrType = "type"
    // etc.
)
```

**Options:**
1. Add constants to a `ce.go` file
2. Remove from README

---

## Features for First Release

### Included (Ready)

| Feature | Status | Notes |
|---------|--------|-------|
| Raw I/O (broker integration) | ✅ Ready | AddRawInput/AddRawOutput |
| Typed I/O (internal use) | ✅ Ready | AddInput/AddOutput |
| Dynamic inputs/outputs | ✅ Ready | Add after Start() |
| Loopback | ✅ Ready | AddLoopback |
| Type-based routing | ✅ Ready | Router with handler map |
| Matcher composition | ✅ Ready | match.All, match.Any, match.Types |
| Message acknowledgment | ✅ Ready | Ack/Nack with callbacks |
| JSON marshaling | ✅ Ready | JSONMarshaler |
| Naming strategies | ✅ Ready | KebabNaming, SnakeNaming |

### Consider Adding

#### Middleware Support
**Priority:** Medium
**Effort:** Low-Medium

The Router uses `pipe.ProcessPipe` internally, which supports middleware via `pipe.Config`. Exposing this would allow:
- Logging middleware
- Metrics middleware
- Retry middleware
- Timeout middleware

**Simple implementation:**
```go
type RouterConfig struct {
    // ...existing fields...
    Middleware []pipe.Middleware[*Message, *Message]
}
```

**Recommendation:** Consider for v0.1.0, but not blocking.

#### Metrics/Observability
**Priority:** Low for v0.1.0
**Effort:** Medium

- Message counts per handler
- Latency histograms
- Error rates

**Recommendation:** Add in v0.2.0 after gathering usage feedback.

#### Dead Letter Queue
**Priority:** Low
**Effort:** Medium

Route failed messages to a separate output instead of just calling error handler.

**Recommendation:** Add in v0.2.0.

---

## Simplification Opportunities

### Already Simple

The code is already well-simplified after Plan 0006:
- Engine: 230 lines (was 400+)
- Start(): 15 lines (was 80+)
- AddRawOutput: 1 line convenience wrapper
- AddRawInput: 2 line convenience wrapper

### Potential Further Simplifications

#### 1. Unify Config Types
Current: 6 nearly identical config types.

Could reduce to:
```go
type IOConfig struct {
    Name    string
    Matcher Matcher
}
```

**Assessment:** Low value, configs are cheap and self-documenting.

#### 2. Remove Router Abstraction
Router could be inlined into Engine.

**Assessment:** No - Router is useful standalone and improves testability.

---

## Go Idioms Check

| Idiom | Status | Notes |
|-------|--------|-------|
| Error handling | ✅ | Errors returned, not panics |
| Interface design | ✅ | Small interfaces (Handler, Matcher, Marshaler) |
| Generics | ✅ | Used appropriately (TypedMessage[T]) |
| Context propagation | ✅ | Context passed through pipeline |
| Concurrency | ✅ | Channels, sync.Mutex where needed |
| Package structure | ✅ | match subpackage for matchers |
| Naming | ✅ | Clear, consistent naming |
| Comments | ⚠️ | Good for public API, could improve internal |

---

## Release Checklist

### Before v0.1.0

- [ ] Update README with new architecture diagram
- [ ] Add tests for match package (Types, Sources, All, Any, Like)
- [ ] Add shutdown documentation to README
- [ ] Decide on Name field: remove, use, or document as reserved
- [ ] Add CE attribute constants or remove from README
- [ ] Review and update godoc comments
- [ ] Add CHANGELOG entry

### Optional for v0.1.0

- [ ] Expose middleware support in RouterConfig
- [ ] Add basic slog logging with Name field
- [ ] Add examples/ directory with complete examples

### After v0.1.0

- [ ] Metrics/observability
- [ ] Dead letter queue
- [ ] Additional marshalers (protobuf, avro)
- [ ] Config convention simplification (Plan 0007)

---

## Verdict

**Production Ready:** Yes, with documentation fixes.

The message package is well-designed, simple, and functional. The core implementation is solid. Main gaps are documentation (README outdated) and test coverage for the match subpackage. These can be addressed quickly.

**Recommended Version:** v0.1.0-alpha initially, then v0.1.0 after addressing checklist items.
