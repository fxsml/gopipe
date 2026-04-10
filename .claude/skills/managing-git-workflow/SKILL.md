---
name: managing-git-workflow
description: |
  Provides expertise in git flow procedures for the gopipe multi-module Go repository.
  Apply when working with git operations, branch management, releases, or multi-module
  tagging. Covers branch naming, commit conventions, approval gates, and tag ordering.
user-invocable: false
---

# Managing Git Workflow

## Branch Naming

| Branch | Purpose | Base |
|--------|---------|------|
| `feature/*` | New features | develop |
| `release/*` | Release preparation | develop |
| `hotfix/*` | Critical fixes | main |
| `claude/*` | Claude-generated branches | develop |

## Commit Conventions

Conventional commits required: `<type>(<scope>): <description>`

Types: `feat`, `fix`, `docs`, `style`, `refactor`, `test`, `chore`

Examples:
```
feat(message): add CloudEvents validation
fix(router): handle nil handler gracefully
docs: update architecture roadmap
```

## Multi-Module Tagging

Tag in dependency order — Go proxy resolves from published tags:

1. `channel/vX.Y.Z` — no internal dependencies
2. `pipe/vX.Y.Z` — depends on channel
3. `message/vX.Y.Z` — depends on channel and pipe
4. `examples/vX.Y.Z` — optional

Push all at once: `git push origin channel/vX.Y.Z pipe/vX.Y.Z message/vX.Y.Z`

## Approval Gates

**Always pause and ask for explicit approval before:**
- Interactive rebase or history rewrite
- Force push to any branch
- Merge to develop or main
- Creating or pushing tags
- Creating GitHub releases

## Key Rules

- Never merge directly to main — always PRs through develop
- Never force push to main or develop
- Run `make test && make build && make vet` before push
- Document before push: CHANGELOG, ADRs, godoc

## Common Mistakes

- Tagging out of order (channel must precede pipe, pipe must precede message)
- Forgetting to sync develop after release (`git merge main` → develop)
- Force pushing without lease (`--force` instead of `--force-with-lease`)

## Reference Procedures

- @../docs/procedures/git.md — branch naming, commits, version management
- @../docs/procedures/feature-release.md — full merge workflow with approval gates
- @../docs/procedures/release.md — tagging, hotfixes, multi-module release
