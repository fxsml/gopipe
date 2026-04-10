# gopipe

Go library for composable data pipelines with CloudEvents support.

@../AGENTS.md

## Procedures

Load these on demand when the relevant task arises:

| Task | Procedure |
|------|-----------|
| Git workflow, branch naming, commits | @../docs/procedures/git.md |
| Go standards, godoc, testing | @../docs/procedures/go.md |
| Documentation, CHANGELOG, doc linting | @../docs/procedures/documentation.md |
| Planning and plan documents | @../docs/procedures/planning.md |
| ADR creation and lifecycle | @../docs/procedures/adr.md |
| Feature branch merge (history cleanup) | @../docs/procedures/feature-release.md |
| Release and multi-module tagging | @../docs/procedures/release.md |

## Claude Code Skills

### Auto-Applied (context-loaded)

| Skill | When Used |
|-------|-----------|
| `managing-git-workflow` | Any git, branch, or release operation |
| `developing-go-code` | Writing or reviewing Go code |
| `building-message-pipelines` | Working with the message package |

### Slash Commands (user-invoked)

| Command | Purpose |
|---------|---------|
| `/release-feature BRANCH` | Merge feature branch to develop (Phases 1–3) |
| `/release VERSION` | Release develop to main with tags (Phases 4–6) |
| `/hotfix NAME` | Create and release a hotfix |
| `/create-feature NAME` | Create feature branch from develop |
| `/verify` | Run `make test && make build && make vet` |
| `/docs-lint` | Check documentation quality |
| `/create-adr TITLE` | Create new Architecture Decision Record |
| `/create-plan TITLE` | Create implementation plan |
| `/changelog TYPE DESC` | Add entry to CHANGELOG |
| `/review-pr [NUMBER]` | Review PR against project standards |
