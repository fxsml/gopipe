# gopipe Implementation Plans

## Quick Start

1. Get next number from index below
2. Create `NNNN-short-title.md`
3. Add to index

See [Planning Procedures](../procedures/planning.md) for details.

## Index

| Plan | Title | Status | Related ADRs |
|------|-------|--------|--------------|
| [0001](0001-message-engine.md) | Message Engine | Proposed | 0019, 0020, 0021 |
| [0002](0002-marshaler.md) | Marshaler | Proposed | 0021 |

## Dependency Graph

```
0002 (Marshaler)
  |
  v
0001 (Message Engine)
```
