# Development Procedures

Modular procedure library for gopipe development. Reusable across projects.

## Procedures Index

| File | Topic | Description |
|------|-------|-------------|
| [git.md](git.md) | Git | Workflow, branching, commits |
| [go.md](go.md) | Go | Standards, godoc, testing |
| [documentation.md](documentation.md) | Docs | Structure, templates, ADRs |
| [planning.md](planning.md) | Planning | Plan files and structure |
| [adr.md](adr.md) | ADRs | Architecture Decision Records |
| [release.md](release.md) | Releases | Release and hotfix procedures |
| [feature-release.md](feature-release.md) | Feature Release | Merge feature branches with history cleanup |

## Quick Reference

### Before Every Commit
```bash
make test && make build && make vet
```

### Commit Message Format
```
<type>(<scope>): <description>

Types: feat, fix, docs, style, refactor, test, chore
```

### Branch Naming
```
feature/<name>    # New features
release/vX.Y.Z    # Release prep
hotfix/<name>     # Critical fixes
```

## Usage in Other Projects

Copy `docs/procedures/` to reuse. Update project-specific details in each file.
