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

Types: `feat`, `fix`, `docs`, `style`, `refactor`, `test`, `chore`, `ci`

The type reflects the **content** of the change, not who made it.

| Content | Type | Examples |
|---------|------|---------|
| New user-facing feature | `feat` | new API, new skill/hook infrastructure |
| Bug fix | `fix` | correct wrong behavior |
| User-facing docs | `docs` | CHANGELOG, ADRs, godoc, README, architecture docs |
| Internal tooling & config | `chore` | `.claude/` skills, hooks, CLAUDE.md, AGENTS.md, Makefile |
| CI/CD pipeline changes | `ci` | GitHub Actions workflows, CI configuration |
| Code restructure, no behavior change | `refactor` | — |
| Tests only | `test` | — |

Examples:
```
feat(message): add CloudEvents validation
fix(router): handle nil handler gracefully
docs: update architecture roadmap
docs(procedures): tighten release procedure go.mod rules
chore: add Claude Code skills, hooks, and CLAUDE.md integration
ci: update Go version requirement to 1.24
```

## Multi-Module Tagging

Tag in dependency order — Go proxy resolves from published tags:

1. `channel/vX.Y.Z` — no internal dependencies
2. `pipe/vX.Y.Z` — depends on channel
3. `message/vX.Y.Z` — depends on channel and pipe
4. `examples/vX.Y.Z` — optional

Push all at once: `git push origin channel/vX.Y.Z pipe/vX.Y.Z message/vX.Y.Z`

**go.mod refs:** If a dependency module has **any changes** (`.go`, `.md`, anything), update all downstream go.mod files to the new version before tagging those modules. Tag each module, push, update the next module's go.mod, then tag that module. Only skip updates if `git diff vOLD..HEAD -- <module>/` produces empty output.

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
- Skipping go.mod updates when a dependency had any changes — consumers won't get fixes or features via MVS
- Attempting to push develop directly — the deny list blocks it; instead prepare the merge locally and ask the user to run `! git push origin develop`
- Forgetting to sync develop after release (`git merge main` → develop)
- Force pushing without lease (`--force` instead of `--force-with-lease`)

## Reference Procedures

- @../docs/procedures/git.md — branch naming, commits, version management
- @../docs/procedures/feature-release.md — full merge workflow with approval gates
- @../docs/procedures/release.md — tagging, hotfixes, multi-module release
