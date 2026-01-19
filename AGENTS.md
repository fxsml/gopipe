# AGENTS.md

Guidelines for AI coding agents working on gopipe.

## Quick Commands

```bash
make test   # Run all tests
make build  # Build all packages
make vet    # Run linters
```

## Key Rules

1. **Never merge to main** — Use PRs through develop
2. **Conventional commits** — `feat:`, `fix:`, `docs:`, etc.
3. **Document before push** — Features, ADRs, CHANGELOG
4. **Test before push** — `make test && make build && make vet`

## Claude Code Integration

### Slash Commands

Use slash commands for common workflows:

| Command | Description |
|---------|-------------|
| `/release-feature BRANCH` | Merge feature branch to develop |
| `/release VERSION` | Release develop to main with tags |
| `/hotfix NAME` | Create and release hotfix |
| `/create-feature NAME` | Create feature branch |
| `/verify` | Run pre-push checks |
| `/docs-lint` | Check documentation quality |
| `/create-adr TITLE` | Create new ADR |
| `/create-plan TITLE` | Create implementation plan |
| `/changelog add TYPE DESC` | Add CHANGELOG entry |
| `/review-pr [NUMBER]` | Review PR against standards |

Commands are in `.claude/commands/` and follow procedures from `docs/procedures/`. They require approval for destructive operations.

### Skills (Auto-Applied)

Domain expertise is automatically applied via skills in `.claude/skills/`:

| Skill | Expertise Area |
|-------|----------------|
| `managing-git-workflow` | Git flow, branch naming, multi-module tagging, approval gates |
| `developing-go-code` | Go standards, testing, common mistakes to avoid |
| `building-message-pipelines` | Message package architecture, Engine, Router, Handler patterns |

Skills are loaded automatically when relevant to the conversation context.

## Package Overview

| Package | Purpose | Key Types |
|---------|---------|-----------|
| `channel/` | Stateless channel operations | Filter, Transform, Merge, Broadcast |
| `pipe/` | Stateful components with lifecycle | ProcessPipe, Merger, Distributor |
| `message/` | CloudEvents message handling | Engine, Router, Handler |

## Project Structure

| Location | Content |
|----------|---------|
| [docs/procedures/](docs/procedures/) | Modular procedures (git, go, docs, planning) |
| [docs/plans/](docs/plans/) | Implementation plans |
| [docs/adr/](docs/adr/) | Architecture decisions |
| [docs/patterns/](docs/patterns/) | Design patterns and examples |

### Procedures Reference

| Topic | Procedure |
|-------|-----------|
| Git workflow, commits, releases | [git.md](docs/procedures/git.md) |
| Go standards, godoc, testing | [go.md](docs/procedures/go.md) |
| Documentation, ADRs, templates | [documentation.md](docs/procedures/documentation.md) |
| Plans, prompts, hierarchy | [planning.md](docs/procedures/planning.md) |

## Architecture Decisions

### Message Package: Single Merger Architecture

```
RawInputs → Unmarshal ─┐
                       ├→ Merger → Router → Distributor
TypedInputs ───────────┘                          │
                                       ┌──────────┴──────────┐
                                 TypedOutput            RawOutput
```

**Why:** Single merger is simpler. Each raw input has its own unmarshal pipe that feeds typed messages into the shared merger.

Loopback is not built into Engine—use `plugin.Loopback` which connects TypedOutput back to TypedInput via the existing Add* APIs.

### API Conventions

| Context | Pattern | Example |
|---------|---------|---------|
| Constructors | Config struct | `NewEngine(EngineConfig{})` |
| Methods | Direct parameters | `AddHandler("name", matcher, h)` |
| Optional filtering | `nil` = match all | `AddOutput("out", nil)` |

### Matcher Interface

```go
type Matcher interface {
    Match(attrs Attributes) bool  // Uses Attributes, not *Message
}
```

**Why:** All matchers only access attributes. Using `*Message` would require wrapper allocation for raw message matching.

### Handler is Self-Describing

```go
type Handler interface {
    EventType() string   // CE type for routing
    NewInput() any       // Creates instance for unmarshaling
    Handle(ctx, msg) ([]*Message, error)
}
```

**Why:** No central registry needed. Handler knows its type and can create instances.

## Common Mistakes

### ❌ Using channel.Process for filtering

```go
// WRONG - Process is for 1:N mapping
channel.Process(in, func(msg) []*Message {
    if match { return []*Message{msg} }
    return nil
})

// CORRECT - Filter with side effect
channel.Filter(in, func(msg) bool {
    if matcher.Match(msg) { return true }
    errorHandler(msg, ErrRejected)
    return false
})
```

### ❌ Creating components in Start()

```go
// WRONG - creates forwarding complexity
func (e *Engine) Start() {
    e.distributor = NewDistributor()  // Too late
}

// CORRECT - create upfront, Add* calls component directly
func NewEngine() *Engine {
    return &Engine{
        distributor: NewDistributor(),  // Ready for AddOutput()
    }
}
```

**Why:** `Distributor.AddOutput()` and `Merger.AddInput()` work before `Distribute()`/`Merge()` is called.

### ❌ N goroutines for loopback

```go
// WRONG - N loopbacks = N goroutines forwarding to same channel
for _, lb := range loopbacks {
    ch := distributor.AddOutput(lb.matcher)
    go forward(ch, loopbackIn)  // Wasteful
}

// CORRECT - combine matchers, single output
combined := match.Any(matchers...)
loopbackCh := distributor.AddOutput(combined)
typedMerger.AddInput(loopbackCh)  // Direct, no forwarding
```

### ❌ Handler.Name() method

Handler should NOT own its name. Name is a wiring concern handled by Engine:

```go
// WRONG
type Handler interface { Name() string }

// CORRECT - name is parameter to AddHandler
engine.AddHandler("process-orders", matcher, handler)
```

### ❌ Copy() sharing Attributes map

```go
// WRONG - modifications affect original
func Copy(msg, data) *Message {
    return &Message{Attributes: msg.Attributes}  // Shared reference
}

// CORRECT - clone for independence
func Copy(msg, data) *Message {
    return &Message{Attributes: maps.Clone(msg.Attributes)}
}
```

## Rejected Alternatives

### Combined Marshaler with Registry

```go
// REJECTED
type Marshaler interface {
    Register(goType reflect.Type)
    TypeName(goType) string
    Unmarshal(data, ceType) (any, error)
}
```

**Why:** Single responsibility. Split into:
- `Marshaler` — pure serialization
- `Handler.NewInput()` — provides instances for unmarshaling

### PipeHandler Interface

```go
// REJECTED
type PipeHandler interface {
    EventType() string
    Pipe(ctx, in <-chan) (<-chan, error)
}
```

**Why:** Over-engineering. `EventType()` returning `"*"` for multi-type handlers is a hack. Router as component (not interface) is cleaner.

### Named Outputs with RouteOutput

```go
// REJECTED
engine.AddOutput("shipments", ch)
engine.RouteOutput("handler", "shipments")
```

**Why:** Pattern matching on CE type is more declarative. AddOutput returns channel directly.

### Engine Owns I/O Lifecycle

```go
// REJECTED
engine.AddSubscriber("orders", subscriber)
```

**Why:** Doesn't handle leader election, dynamic scaling. External concern.

## Naming Decisions

| Chosen | Rejected | Reason |
|--------|----------|--------|
| `EventTypeNaming` | `NamingStrategy` | More precise about what it names |
| `InputRegistry` | `TypeRegistry` | Matches `Handler.NewInput()` method |
| `Use()` | `ApplyMiddleware()` | Standard Go pattern (gin, echo, etc.) |
| `KebabNaming` | `DotNaming` | Describes output format: `order.created` |

## File Organization

### message/ package structure

```
message/
├── doc.go          # Package docs with Design Notes
├── engine.go       # Engine orchestrator
├── router.go       # Handler routing with middleware
├── handler.go      # Handler interface, NewHandler, NewCommandHandler
├── message.go      # Message types, Copy, Acking
├── marshaler.go    # Marshaler interface, JSONMarshaler
├── naming.go       # EventTypeNaming, KebabNaming, SnakeNaming
├── registry.go     # InputRegistry, FactoryMap
├── matcher.go      # Matcher interface
├── errors.go       # Error types
├── match/          # Matcher implementations
├── middleware/     # CorrelationID, etc.
└── plugin/         # Loopback, ProcessLoopback, BatchLoopback
```

## Deprecation Procedure

When deprecating code:

1. Add godoc deprecation notice:
   ```go
   // Deprecated: Use NewFunction instead.
   func OldFunction() {}
   ```

2. Update relevant plan documentation
3. Add migration guide in feature docs
4. Update CHANGELOG under `[Unreleased]`

## Historical Context

For full rejected alternatives and design evolution, see `docs/plans/archive/`.
