---
name: implement
description: >-
  Implement an issue from a spec file, URL, or inline description. Follows
  TDD workflow: read spec, explore code, plan, test first, implement, commit.
  Use when the user says "implement", "work on issue", provides an issue URL,
  or asks to build/fix something from a ticket, spec, or backlog item.
argument-hint: "[path|url|description]"
---

# Implement Issue

Standardized workflow for implementing issues — bug fixes, features, or refactors.

## Input

`$ARGUMENTS` is one of:
- **File path** (e.g., `docs/plans/feature-name.md`) → read the file
- **URL** (starts with `http`) → fetch and parse the issue content
- **Inline description** → use as-is

## Workflow

### 1. Understand the Issue

- Read the issue spec from `$ARGUMENTS`
- Identify: what type of change? (`fix`, `feat`, `refactor`, `test`, `docs`, `chore`)
- Identify: which package(s) are affected? (`channel`, `pipe`, `message`)
- Check AGENTS.md for relevant architecture decisions and common mistakes

### 2. Explore Before Coding

- Read the source files listed in or implied by the issue
- Read existing tests for those files
- Understand current behavior before changing anything
- If the issue references exemplar files, read those too
- Identify the correct package (`channel/`, `pipe/`, `message/`)

### 3. Plan

Briefly state:
- Which files will change and why
- Any new types or interfaces needed
- How it will be tested

Ask for confirmation if the plan is non-trivial or touches shared interfaces or public API.

### 4. Create Branch (if needed)

- Check current branch — if already on a task branch, skip
- Otherwise, branch from `develop` (never from `main`):
  - Bug fix: `fix/<short-description>`
  - Feature: `feature/<short-description>`
  - Refactor: `refactor/<short-description>`

### 5. Tests First

- If a failing test already exists, read and understand it
- If not, write a test that captures the expected behavior BEFORE implementing
- Use table-driven tests with `t.Parallel()` where safe
- Test both success and error paths
- Verify the test fails for the right reason before implementing

### 6. Implement

- Make the minimal change to satisfy the acceptance criteria
- Follow patterns from AGENTS.md and the relevant package conventions
- Do NOT modify files outside the scope defined in the issue
- Do NOT refactor or "improve" unrelated code

### 7. Commit

- Use conventional commit format: `<type>(<scope>): <description>`
- Scopes: `channel`, `pipe`, `message`, `examples`, or sub-paths like `message/http`
- If the issue has a GitHub issue number, add `Closes #NNN` in the commit footer
- One commit per issue — keep it atomic

### 8. Summary

Report:
- What changed
- What was intentionally left out of scope
- Any follow-up issues worth creating

## Hard Rules

- Never implement more than what the issue describes
- Every change must have a test that would have caught the bug or validates the feature
- Do not add dependencies without discussing first — see dependencies.md for the checklist
- If the issue touches public API or inter-package contracts, flag it before proceeding

## Reference Procedures

- @../docs/procedures/coding.md — behavioral rules: simplicity, scope discipline, TDD
- @../docs/procedures/go.md — Go standards, godoc, testing patterns
- @../docs/procedures/git.md — branch naming, commit format, approval gates
- @../docs/procedures/dependencies.md — external dependency and package boundary rules
