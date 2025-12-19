# Claude Code Procedures for gopipe

**Procedures Library:** See [docs/procedures/](docs/procedures/) for modular, reusable procedures.

## Quick Reference

| Topic | Procedure |
|-------|-----------|
| Git workflow, commits, releases | [git.md](docs/procedures/git.md) |
| Go standards, godoc, testing | [go.md](docs/procedures/go.md) |
| Documentation, ADRs, templates | [documentation.md](docs/procedures/documentation.md) |
| Plans, prompts, hierarchy | [planning.md](docs/procedures/planning.md) |
| Roadmap restructuring, repo separation | [restructuring.md](docs/procedures/restructuring.md) |

## Essential Commands

```bash
make test   # Run tests
make build  # Build
make vet    # Lint
```

## Key Rules

1. **Never merge to main** - Use PRs through develop
2. **Conventional commits** - `feat:`, `fix:`, `docs:`, etc.
3. **Document before push** - Features, ADRs, CHANGELOG
4. **Test before push** - `make test && make build && make vet`

## Project Documentation

| Location | Content |
|----------|---------|
| [docs/roadmap/](docs/roadmap/) | Hierarchical roadmap with examples |
| [docs/manual/](docs/manual/) | User manual |
| [docs/plans/](docs/plans/) | Legacy plans (see roadmap/) |
| [docs/adr/](docs/adr/) | Architecture decisions |
| [docs/features/](docs/features/) | Feature documentation |

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

## Plan Prompt Convention

**Every plan file must have a corresponding `.prompt.md` file.**

```
docs/plans/layer-1-message.md         # Full plan
docs/plans/layer-1-message.prompt.md  # Optimized prompt
```

The prompt file contains only what's needed to execute the plan. See [planning.md](docs/procedures/planning.md) for details.

---

**Version:** 4.0
**Last Updated:** 2025-12-17
