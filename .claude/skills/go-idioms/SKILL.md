---
name: go-idioms
description: |
  Idiomatic Go principles from spf13/go-skills (github.com/spf13/go-skills),
  vendored for gopipe. Apply alongside developing-go-code when writing or
  reviewing Go code: package organization, interface design, concurrency,
  testing, and stdlib modernization (Go 1.21-1.25).
user-invocable: false
---

# Go Idioms (spf13/go-skills)

Source: https://github.com/spf13/go-skills (`go/SKILL.md`), MIT licensed.
Vendored as a project skill because the upstream repo has no versioned
release; treat this file as a snapshot, not a live dependency.

This skill complements `developing-go-code` (gopipe-specific conventions)
with broader idiomatic-Go guidance. Where the two disagree, gopipe's own
conventions in `developing-go-code` and `AGENTS.md` win — see "Deliberate
Deviations" below.

## Core Principles

- **Clear over clever** — if a function needs three reads to follow its
  control flow, rewrite it.
- **Useful zero values** — design types whose zero value works, like
  `sync.Mutex`.
- **Return early** — handle edge cases up front; keep the happy path
  unindented.

## Package Organization

- **Start flat.** Only split into packages for real domain boundaries or
  to decouple independent concerns — not preemptively.
- **`internal/` for subsystems, not applications.** Reserve it for complex
  internal-only subsystems in a library; don't add it reflexively.
- **Domain packages, not layers.** Name packages after what they own
  (`jobs/`, `billing/`), never architectural layers (`service/`,
  `repository/`, `controller/`) — layer packages cause circular imports
  and interface proliferation.

## Interface Design

- **Discover, don't design.** Write concrete types first; extract an
  interface only once multiple types need to be interchangeable.
- **Define where used.** Interfaces belong in the consuming package, not
  alongside their implementations.
- **Accept interfaces, return structs.** Parameters take the minimal
  interface needed; constructors return concrete types so callers get
  direct field access.

## Concurrency

- **Share by communicating.** Prefer channels over mutexes where it fits.
- **Bounded concurrency via `errgroup.SetLimit`** instead of hand-rolled
  worker pools.
- **Every `go func()` needs a clear exit condition** — typically
  `context.Context` cancellation or a channel close.
- Modern stdlib: `sync/atomic` typed values, `context.WithoutCancel`,
  `sync.WaitGroup.Go` (1.25).

## Testing

- **Table-driven tests** with `t.Run()` per case.
- **`t.Helper()`** in test helpers so failures point at the real test.
- **Fakes over mocks** — small hand-written types satisfying an implicit
  interface.
- **`golang.org/x/tools`-style golden files** in `testdata/` for complex
  fixtures.
- **`cmp.Diff`** (`google/go-cmp`) over `reflect.DeepEqual` for readable
  diffs.
- **`testing/synctest`** (1.25) for deterministic concurrent/timeout
  tests — never `time.Sleep` to coordinate goroutines in a test.
- `t.Context()`, `b.Loop()`, `t.Chdir()` for newer test/benchmark needs.

## Generics

Use generics to eliminate duplicated algorithms across types — not for
polymorphism or "I don't know the type yet" (`any` as a constraint is a
smell). Prefer concrete implementations; generify after 3+ repeats.
`comparable` for map keys, `cmp.Ordered` for comparisons.

## Standard Library Essentials (1.21+)

- `slices`: `Contains`, `Index`, `SortFunc`, `Compact`, `Clone`, `Concat`.
- `maps`: `Keys`, `Values`, `Clone`, `DeleteFunc`, `Equal`.
- `cmp`: `Compare`, `Or` (replaces ternary chains); built-in `min`/`max`.
- `errors.Join` to combine multiple errors without a third-party package.
- Iterators (1.23): return `iter.Seq[T]` instead of allocating a slice.
- `for i := range 10` (1.22) instead of a C-style counting loop.
- `any` instead of `interface{}` (1.18).

## Error Handling

- Errors are values — check them explicitly, don't treat them as
  exceptions.
- Wrap with `fmt.Errorf("action: %w", err)` when the caller may need to
  unwrap or match the cause.
- Never both log and return the same error — pick one boundary.

## HTTP

`net/http.ServeMux` (1.22) handles method + path-parameter routing
natively (`mux.HandleFunc("GET /users/{id}", h)`, `r.PathValue("id")`).
Reach for chi/gorilla only for named route generation or regex
constraints. Production servers always set `ReadHeaderTimeout`,
`ReadTimeout`, `WriteTimeout`, `IdleTimeout`, and shut down gracefully via
`signal.NotifyContext` + a fresh (non-signal) context to drain in-flight
requests.

## Deliberate Deviations (gopipe-specific)

These are intentional gopipe choices that differ from upstream go-skills
defaults — don't "fix" them without an ADR:

| go-skills default | gopipe convention | Where documented |
|---|---|---|
| Functional options (`...Option`) | `Config` struct per constructor (`NewEngine(EngineConfig{})`) | `AGENTS.md` API Conventions |
| — | Handler is self-describing (`EventType()`, `NewInput()`) instead of a central registry | `AGENTS.md` |
| — | Matcher takes `Attributes`, not `*Message`, to avoid wrapper allocation | `AGENTS.md` |

## Reference

- `developing-go-code` — gopipe-specific API conventions and anti-patterns
  (this skill supplements it, doesn't replace it)
- `docs/procedures/go.md` — godoc standards, pre-push checklist
- Upstream: https://github.com/spf13/go-skills
