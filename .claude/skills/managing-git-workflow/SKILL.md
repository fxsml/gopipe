---
name: managing-git-workflow
description: |
  Provides expertise in git flow procedures for the gopipe multi-module Go repository.
  This skill helps with branch naming, commit conventions, release procedures, and
  multi-module tagging. Use when working with git operations, releases, or branch management.
---

# Managing Git Workflow

This skill provides guidance for git operations in the gopipe repository, which uses git flow with multi-module Go tagging.

## Key Knowledge

### Branch Naming

- `feature/` - New features branched from develop
- `release/` - Release preparation branched from develop
- `hotfix/` - Emergency fixes branched from main
- `claude/` - Claude-generated branches (auto-named)

### Commit Conventions

Use conventional commits format:

| Type | Description | Example |
|------|-------------|---------|
| `feat` | New feature | `feat(message): add Engine type` |
| `fix` | Bug fix | `fix(message): correct Copy() to clone Attributes` |
| `refactor` | Code restructuring | `refactor(message): simplify Add* methods` |
| `docs` | Documentation | `docs(message): update architecture diagrams` |
| `test` | Tests | `test(message): add Engine integration tests` |
| `chore` | Maintenance | `chore: update dependencies` |

### Multi-Module Tagging

gopipe has three Go modules that must be tagged in dependency order:

1. `channel/` - No dependencies on other modules
2. `pipe/` - Depends on channel
3. `message/` - Depends on pipe and channel

Tag format: `module/vX.Y.Z` (e.g., `channel/v0.13.0`, `pipe/v0.13.0`, `message/v0.13.0`)

Always tag in this order to ensure Go proxy resolves dependencies correctly.

### Approval Gates

**CRITICAL:** Always pause and ask for explicit approval before:
- Interactive rebase or history rewrite
- Force push to any branch
- Merge to develop or main
- Creating or pushing tags
- Creating GitHub releases

### Key Rules

1. **Never merge directly to main** - Always use PRs through develop
2. **Document before push** - Update CHANGELOG, docs for features
3. **Test before push** - `make test && make build && make vet`
4. **Rebase feature branches** - Keep history clean before merge

## Common Mistakes to Avoid

- **Merging to main without PR**: Always go feature → develop → main via PRs
- **Tagging out of order**: channel must be tagged before pipe, pipe before message
- **Force pushing without approval**: Always show what will change and get approval
- **Forgetting to sync develop**: After release, merge main back to develop

## Workflows

### Feature Development
1. Create branch from develop: `feature/name`
2. Make changes with conventional commits
3. Use `/project:release-feature` to merge to develop

### Release
1. Ensure develop has all features
2. Use `/project:release VERSION` to release to main with tags

### Hotfix
1. Create branch from main: `hotfix/name`
2. Fix and test
3. Use `/project:hotfix` to release

## Reference Documents

- [docs/procedures/git.md](docs/procedures/git.md) - Branch naming, commit conventions
- [docs/procedures/feature-release.md](docs/procedures/feature-release.md) - Full release workflow
- [docs/procedures/release.md](docs/procedures/release.md) - Tagging, versioning
