---
name: docs-lint
description: Check documentation quality and consistency across the project.
allowed-tools:
  - Bash
  - Read
  - Glob
  - Grep
---

# Docs Lint

Run the documentation lint procedure: @../docs/procedures/documentation.md

Execute each step and report issues found:

1. **Broken internal links** — scan all markdown links in `docs/`, verify targets exist
2. **ADR index** — compare `docs/adr/*.md` files with `docs/adr/README.md` entries; check status consistency
3. **Procedures index** — compare `docs/procedures/*.md` with `docs/procedures/README.md`
4. **CHANGELOG** — verify `[Unreleased]` section exists, version sections have dates, no duplicates
5. **Plans index** — compare `docs/plans/*.md` (active) and `docs/plans/archive/*.md` with `docs/plans/README.md`

Output a summary report:

```
Documentation Lint Report
=========================
[ ] Broken links: X found
[ ] ADR index: X missing, X stale
[ ] Procedures index: X missing, X stale
[ ] CHANGELOG: OK / Issues
[ ] Plans index: X missing, X stale
```

List specific issues with file paths so they can be fixed.
