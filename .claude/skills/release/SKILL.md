---
name: release
description: Release develop to main with multi-module tags and GitHub release. Usage: /release VERSION (e.g. /release v0.13.0)
disable-model-invocation: true
allowed-tools:
  - Bash
  - Read
  - Edit
---

# Release

Execute Phases 4–6 of the Feature Branch Release Procedure for version: `$ARGUMENTS`

Read the full procedure before starting: @../docs/procedures/release.md

## Steps

**Phase 4 — Merge to main:**
1. Check for cross-module API changes: `git diff vOLD..HEAD -- channel/ pipe/`
2. Update CHANGELOG: move [Unreleased] to new version section with date
3. Create release PR: `gh pr create --base main --head develop --title "release: VERSION"`
4. Show PR checks
5. **PAUSE: ask for approval before merge**
6. `gh pr merge --merge`

**Phase 5 — Tag and release:**
1. `git checkout main && git pull origin main`
2. Determine if go.mod refs need updating (cross-module API changes?)
3. **PAUSE: ask for approval before creating tags**
4. Create tags in dependency order: `channel/VERSION`, `pipe/VERSION`, `message/VERSION`
5. **PAUSE: ask for approval before pushing tags**
6. Push module tags + bare version tag
7. `gh release create VERSION --title "VERSION" --notes "..."`

**Phase 6 — Post-release:**
1. `git checkout develop && git merge main && git push origin develop`
2. Verify module publication: `go list -m github.com/fxsml/gopipe/channel@VERSION`
3. Report release URL

## Rules

- Never skip an approval gate
- Tag in dependency order: channel → pipe → message
- Check cross-module API changes before deciding whether to update go.mod refs
- Merge back to develop after release
