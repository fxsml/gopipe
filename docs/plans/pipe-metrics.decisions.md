# pipe-metrics: Design Evolution

**Status:** Resolved
**Related Plan:** [pipe-metrics.md](pipe-metrics.md)

## Context

Design questions that arose during implementation of the pipe-layer metrics feature
(branch `claude/gopipe-metrics-exploration-oQo27`). Recorded here so git history can
be rewritten without losing the rationale.

---

## Decision 1: `message/otel` as a separate Go module

**Decision:** `message/otel` is a separate Go module (`github.com/fxsml/gopipe/message/otel`)
rather than a subpackage of `message`.

**Why:** OTel pulls in a large dependency tree. Users of the base `message` package should
not be forced to vendor it. The separate module makes OTel strictly opt-in, consistent with
how the broader Go ecosystem handles this (e.g. `go-redis/extra/redisotel`).

**Why not a subpackage:** A subpackage shares the parent module's `go.mod`, which would add
OTel to every consumer of `message`. The module boundary is the only way to keep the
dependency opt-in.

---

## Decision 2: `BatchSizeFunc` / `Middleware` removed from `message/otel`

**Evolution:**

1. Initial implementation added `BatchSizeFunc func(*message.Message) int` to `Config`
   and recorded `gopipe.message.batch.size` as a histogram inside a `Middleware` type.
   `DefaultBatchSize` tried `Sizer` interface → `reflect.Slice` → fallback `1`.

2. PR review identified this as too domain-specific. A separate `Middleware` type in
   `message/otel` was also considered: it recorded `processed`, `process.duration`,
   and `inflight`, adding `cloudevents.event.type` via a `sync.Map` cache.

3. **Final decision:** remove `Middleware` from `message/otel` entirely. All instrumentation
   lives in `PipeMetrics` (assigned to `pipe.Config.Metrics`). `PipeMetrics.RecordProcessing`
   records processing duration and output count with the same label set as wait metrics,
   including the `cloudevents.type` dimension via the default `message.LabelFunc`. This
   keeps a single instrumentation surface rather than two (pipe-level + middleware-level)
   that users must configure independently.

**Consequence:** Duration metrics are recorded at the pipe layer (includes the full
`ProcessFunc` call chain including any middleware). Handler-only duration is not separately
observable — acceptable trade-off for a simpler API.

---

## Decision 3: `context.Background()` in Merger and Distributor goroutines

**Decision:** Merger and Distributor metrics calls use `context.Background()`, not the
`ctx` passed to `Merge()`/`Distribute()`.

**Why:** These goroutines are infrastructure — they forward messages between channels with
no per-request scope. The `ctx` they receive controls shutdown, not tracing. Passing it
to OTel instruments would cancel metric recording on pipeline shutdown, which is wrong.
Storing `ctx` in the struct to thread it into `startInput` was considered and rejected:
storing a context in a struct is a Go anti-pattern.

**Consequence for exemplars:** If OTel exemplar support (linking histogram samples to
trace spans) is ever needed for merger/distributor, `AddInput(ctx, ch)` and an internal
`route(ctx, msg)` signature change would be required. Deferred — no concrete use case.

---

## Decision 4: `WaitOpReceive` / `WaitOpSend` as exported constants

**Decision:** Export string constants `WaitOpReceive = "receive"` and `WaitOpSend = "send"`
on `pipe.Metrics` rather than using bare strings at call sites.

**Why:** User-implemented `Metrics` backends switch on `WaitInfo.Operation`. Without
constants, typos in user code produce silent no-ops. The constants also serve as
documentation: they enumerate all valid values.

---

## Decision 5: Pull-based buffer depth via `Stats()` and observable gauges

**Decision:** Buffer depth and capacity are exposed via `Stats()` pull snapshots and OTel
observable gauges (`PipeMetrics.ObserveStats`), not recorded on every send via a push
interface method.

**Supersedes:** The earlier design had a `RecordQueue` method on `pipe.Metrics` called
after every successful send. This was removed.

**Why pull:** Recording on every send adds a metrics call to the hot path even when the
collection interval is, say, 15 seconds. OTel's observable gauge mechanism is designed
exactly for this: a callback polled at collection time. Buffer occupancy is a point-in-time
gauge; emitting it per-send is equivalent to recording a gauge 10,000 times per second and
keeping only the last value.

**Why a single `RegisterCallback`:** OTel requires all instruments observed in one callback
to be registered together. Using a single callback for `depth`, `capacity`, and `inflight`
ensures they are observed atomically (same `Stats()` call) and is the idiomatic OTel
multi-instrument pattern.

**`Inflight` in `Stats`:** Active worker count is tracked via an `atomic.Int64` incremented
on handler entry and decremented on return. This is O(1) and lock-free. It gives operators
visibility into whether workers are saturated vs. idle waiting for input — a useful
complement to buffer depth.

---

## Decision 6: `LabelFunc` for dynamic per-message labels

**Decision:** All config types (`pipe.Config`, `pipe.MergerConfig`, `pipe.DistributorConfig`,
and their message-layer counterparts) carry a `LabelFunc func(val any) map[string]string`
field alongside the static `Labels map[string]string`.

**Why:** Static labels cover pipeline-wide dimensions (`pipeline`, `stage`). The CE type
dimension (`cloudevents.type`) varies per message and cannot be captured at config time.
`LabelFunc` is called once per received value; the returned map is merged with `Labels`
before being passed to `RecordProcessing` and `RecordWait(WaitOpSend)`.

**Default in message package:** All message-layer configs default `LabelFunc` to
`messageLabelFunc`, which extracts `cloudevents.type` from each `*Message`. This means
duration, output count, and total are dimensioned by CE type without any explicit user
configuration when `Metrics` is set.

**Receive-wait uses static labels only:** `LabelFunc` is applied after the value is received
from the input channel; the receive-wait duration is recorded before the value is available.
This is consistent and documented in the `Config.LabelFunc` godoc.

---

## Decision 7: `gopipe.process.total` with `error.type=""` on success

**Decision:** Replace a separate `process.errors` counter with a single `process.total`
counter that carries `error.type=""` on success and the Go error type string on failure.

**Why:** Prometheus users commonly want to compute both total throughput and error rate
from a single counter using label matchers (`{error_type!=""}` for errors). A separate
errors counter requires `ignoring(error_type)` in rate queries, which is error-prone.
The unified `process.total` counter with an `error.type` dimension is the OTel semantic
convention for this pattern.

---

## Decision 9: Per-call allocation in labelsToAttrs — no caching in PipeMetrics

**Decision:** `labelsToAttrs` is called on every `RecordProcessing` and `RecordWait`
invocation and allocates a fresh `[]attribute.KeyValue` each time. No `sync.Map` cache
is added to `PipeMetrics`.

**Why no caching:**
A cache keyed on the merged label set (e.g. `{"pipeline":"orders","cloudevents.type":"process.order"}`)
would be safe only when label cardinality is bounded. A `LabelFunc` that emits per-message
dimensions (order IDs, trace IDs, user IDs) would cause unbounded `sync.Map` growth — a
memory leak. Because `PipeMetrics` cannot inspect the `LabelFunc` to determine cardinality,
adding a cache would be a correctness risk for users who supply high-cardinality label functions.

**What was fixed instead:**
`RecordProcessing` previously allocated a second slice (`make+copy`) for `processTotal`'s
`error.type` attribute on every call, including the success path where `errType=""`.
This was replaced with a single allocation covering both base attrs and the `error.type`
slot. Benchmark results (noop meter, Intel Core Ultra 5 135U):

| Benchmark | Before | After |
|-----------|--------|-------|
| `RecordProcessing_NoLabels` | 8 allocs, 272 B | 8 allocs, 272 B |
| `RecordProcessing_StaticLabels` | 11 allocs, 1040 B | 10 allocs, 912 B |
| `RecordProcessing_MergedLabels` | 11 allocs, 1424 B | 10 allocs, 1232 B |
| `RecordProcessing_Error` | 11 allocs, 1048 B | 10 allocs, 920 B |
| `RecordWait_StaticLabels` | 5 allocs, 416 B | 5 allocs, 416 B (unchanged) |
| `RecordProcessing_HighCardinality` | 14 allocs, 1764 B | 13 allocs, 1572 B |

**Guidance for users who need zero-allocation hot paths:**
Implement `pipe.Metrics` directly with pre-built `[]attribute.KeyValue` slices captured
at construction time. This is the only approach that is safe for all cardinalities.

---

## Decision 8: Message age deferred

**`gopipe.message.age`** — time from CE `time` attribute to handler start — was identified
as a strong future addition to `PipeMetrics.RecordProcessing`. It is a universal SLO
signal (not domain-specific), derived entirely from a standard CloudEvents attribute, and
requires no user configuration.

Deferred as over-engineering for the initial release. Implement when there is a concrete
operational need.
