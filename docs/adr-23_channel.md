## ADR-23: (Channel) Refocus gopipe scope on pipe orchestration

### Context

The gopipe library currently mixes pipeline orchestration and low-level channel helpers (such as Map, Transform, Collect, Flatten, Batch). This conflation makes the core processing package heavier, harder to maintain, and less modular. Users may want to reuse channel utilities independently of gopipe pipelines.

### Decision

Extract all channel helper functions into a new `channel` package. The `gopipe`
package will focus solely on pipeline orchestration, middleware, and processing logic.

### Consequences

**Positive**
- Clear separation of concerns: pipeline logic vs. data-flow helpers.
- Lightweight core package (`gopipe`) easier to maintain and extend.
- Channel utilities can be reused independently in other projects.
- Easier to discover and document API for users.

**Negative**
- Users need to import an additional package (`channel`) for advanced channel operations.
- Slightly more fragmented API surface.
- Shared testing logic must be separated into dedicated package to avoid circular dependencies.

**Neutral**
- No change to existing core pipeline behavior.