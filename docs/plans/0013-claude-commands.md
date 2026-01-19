# Plan 0013: Claude Code Commands and Skills Integration

**Status:** Implemented
**Created:** 2026-01-19
**Author:** Claude

## Overview

Implement Claude Code slash commands and skills in the gopipe repository to automate git flow procedures and provide domain expertise.

## Goals

1. Automate git flow procedures with explicit slash commands
2. Provide domain expertise via auto-applied skills
3. Ensure Claude always follows defined procedures
4. Keep all configuration in the repository (not local)

## Non-Goals

- Creating a separate CLAUDE.md file (AGENTS.md already serves this purpose)
- Moving procedures from `docs/procedures/` to `.claude/`

## Implementation

### Directory Structure

```
.claude/
├── settings.local.json          # Updated with git/gh permissions
├── settings.json                 # Project-wide settings (committed)
├── commands/                     # Slash commands
│   ├── release-feature.md
│   ├── release.md
│   ├── hotfix.md
│   ├── create-feature.md
│   ├── docs-lint.md
│   ├── create-adr.md
│   ├── create-plan.md
│   ├── verify.md
│   ├── changelog.md
│   └── review-pr.md
└── skills/
    ├── managing-git-workflow/SKILL.md
    ├── developing-go-code/SKILL.md
    └── building-message-pipelines/SKILL.md
```

### Commands Implemented

| Command | Description | Procedure Reference |
|---------|-------------|---------------------|
| `/release-feature BRANCH` | Merge feature to develop | feature-release.md Phases 1-3 |
| `/release VERSION` | Release to main with tags | feature-release.md Phases 4-6 |
| `/hotfix NAME` | Create and release hotfix | release.md |
| `/create-feature NAME` | Create feature branch | git.md |
| `/verify` | Run pre-push checks | go.md |
| `/docs-lint` | Check documentation | documentation.md |
| `/create-adr TITLE` | Create new ADR | adr.md |
| `/create-plan TITLE` | Create implementation plan | planning.md |
| `/changelog add TYPE DESC` | Add CHANGELOG entry | documentation.md |
| `/review-pr [NUMBER]` | Review PR | Multiple |

Note: Commands are invoked without prefix (e.g., `/verify`). Claude Code shows "(project)" in `/help` to indicate they're project-scoped.

### Skills Implemented

| Skill | Expertise Area |
|-------|----------------|
| `managing-git-workflow` | Git flow, branch naming, multi-module tagging |
| `developing-go-code` | Go standards, testing, common mistakes |
| `building-message-pipelines` | Message package architecture |

### Key Design Decisions

1. **Gerund naming for skills**: `managing-git-workflow` instead of `git-expert` (follows Claude Code conventions)

2. **YAML frontmatter for skills**: Each SKILL.md includes name and description in frontmatter for discovery

3. **Approval gates**: All commands that perform destructive operations require explicit approval

4. **Procedures stay in docs/**: Kept in `docs/procedures/` for visibility to all users, not just Claude

5. **No CLAUDE.md**: AGENTS.md already serves as the AI guidelines file

## Files Changed

### Created
- `.claude/settings.json`
- `.claude/commands/*.md` (10 files)
- `.claude/skills/*/SKILL.md` (3 files)
- `docs/plans/0013-claude-commands.md`

### Modified
- `.claude/settings.local.json` - Added git, gh, make permissions
- `AGENTS.md` - Added Claude Code Integration section

## Testing

Commands can be tested by invoking them:
- `/project:verify` - Should run make test/build/vet
- `/project:docs-lint` - Should check documentation quality

## Related

- [docs/procedures/](../procedures/) - Source procedures for commands
- [AGENTS.md](../../AGENTS.md) - AI agent guidelines
