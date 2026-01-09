# Contributing to gopipe

Thank you for your interest in contributing to gopipe! This document provides guidelines for human contributors.

## Quick Start

```bash
# Clone the repository
git clone https://github.com/fxsml/gopipe.git
cd gopipe

# Run tests
go test ./...

# Run specific package tests
go test ./channel -v
go test ./message -v
```

## Project Structure

```
gopipe/
├── channel/          # Stateless channel operations (Merge, Filter, Transform, etc.)
├── pipe/             # Stateful components with lifecycle (ProcessPipe, Merger, Distributor)
│   └── middleware/   # Pipe middleware (Retry, Logger, Metrics, etc.)
├── message/          # CloudEvents message handling
│   ├── match/        # Matchers (Types, Sources, All, Any)
│   ├── middleware/   # Message middleware (CorrelationID)
│   └── plugin/       # Plugins (Loopback, ProcessLoopback, BatchLoopback)
├── examples/         # Working examples (01-05)
└── docs/             # Documentation
    ├── adr/          # Architecture Decision Records
    ├── plans/        # Implementation plans
    └── procedures/   # Development procedures
```

## Core Components

### Channel Package
Stateless channel operations: `Merge`, `Filter`, `Transform`, `Broadcast`, `GroupBy`, etc.

### Pipe Package
Stateful components with lifecycle: `ProcessPipe`, `Merger`, `Distributor`, `Generator`.

### Message Package
CloudEvents message handling: `Engine`, `Router`, `Handler`, with type-based routing.

See [README.md](README.md) for quick start examples and package overview.

## Development Workflow

### 1. Create a Feature Branch

```bash
git checkout -b feature/your-feature-name main
```

### 2. Make Changes

- Write code following Go best practices
- Add tests for new functionality
- Update documentation

### 3. Run Tests

```bash
# All tests
go test ./...

# With coverage
go test ./... -cover

# Specific package
go test ./message -v
```

### 4. Update Documentation

**Required before committing:**

1. **CHANGELOG.md**
   - Add entry under `[Unreleased]`
   - Use sections: Added, Changed, Deprecated, Removed, Fixed, Security

2. **Architecture Decision Records** (for architectural changes)
   - Create `docs/adr/NNNN-decision-name.md`
   - Include: Date, Status, Context, Decision, Consequences

3. **Godoc** (for new public APIs)
   - Add package doc.go with overview
   - Document all exported types and functions

4. **README.md** (for significant changes)
   - Update examples if API changed
   - Add examples
   - Ensure examples are tested and up-to-date

### 5. Documentation Standards

**Public API godoc:**
- Precise and concise
- Focus on what the API does, not implementation details
- Include brief usage example in godoc when helpful

**Example:**
```go
// GroupBy aggregates items from the input channel by key, emitting batches
// when size or time limits are reached.
//
// Example:
//
//	groups := channel.GroupBy(orders, func(o Order) string {
//	    return o.CustomerID
//	}, channel.GroupByConfig{MaxSize: 10})
func GroupBy[K comparable, V any](
    in <-chan V,
    keyFunc func(V) K,
    config GroupByConfig,
) <-chan Group[K, V]
```

### 6. Commit

```bash
git add .
git commit -m "feat(package): add your feature

Detailed description of what was added/changed.
"
```

Follow [Conventional Commits](https://www.conventionalcommits.org/):
- `feat:` - New feature
- `fix:` - Bug fix
- `docs:` - Documentation only
- `refactor:` - Code refactoring
- `test:` - Adding tests
- `chore:` - Maintenance

### 7. Push and Create Pull Request

```bash
git push origin feature/your-feature-name
```

Create PR on GitHub with:
- Clear description
- Link to feature documentation
- List of changes
- Test results

## Testing Guidelines

### Test Requirements

- All new code must have tests
- Maintain or improve code coverage
- Test edge cases and error conditions
- Use table-driven tests for multiple scenarios

### Test Organization

```go
func TestFeature(t *testing.T) {
    tests := []struct {
        name     string
        input    interface{}
        expected interface{}
    }{
        {"basic case", input1, expected1},
        {"edge case", input2, expected2},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // test implementation
        })
    }
}
```

### Example Tests

Examples in documentation must be tested. Ensure all examples compile and run:

```bash
# Test all examples build
go build ./examples/...

# Run specific example
go run ./examples/broker/main.go
```

## Code Style

- Follow `go fmt` formatting
- Use `golint` and `go vet`
- Keep functions focused and small
- Use meaningful variable names
- Add comments for exported functions

## Pre-Commit Checklist

Before committing, ensure:

- [ ] All tests pass (`go test ./...`)
- [ ] Code is formatted (`go fmt ./...`)
- [ ] Documentation updated:
  - [ ] Feature docs created/updated
  - [ ] CHANGELOG.md updated
  - [ ] ADRs created if needed
  - [ ] README.md updated for core changes
- [ ] Public API has precise godoc
- [ ] Godoc references feature docs (no example code in godoc)
- [ ] Examples are tested and up-to-date
- [ ] No breaking changes without migration guide

## Architecture Decision Records (ADRs)

For significant architectural decisions:

1. Create ADR in `docs/adr/NNNN-name.md`
2. Follow format: Date, Status, Context, Decision, Consequences
3. Link to related features and code
4. Update `docs/adr/README.md` to categorize by status

Status values:
- **Proposed**: Under consideration
- **Accepted**: Agreed upon but not implemented
- **Implemented**: Fully implemented
- **Superseded**: Replaced by later decision

## Getting Help

- Review [docs/adr/](docs/adr/) for architectural decisions
- Check [docs/procedures/](docs/procedures/) for development procedures
- See [AGENTS.md](AGENTS.md) for AI agent guidance
- Open an issue for questions or bugs

## AI Assistant Workflow

For AI assistants working on this codebase, see [AGENTS.md](AGENTS.md) for:
- Quick commands
- Architecture decisions
- Common mistakes to avoid
- Project structure

Key differences for AI:
- Humans: Focus on this CONTRIBUTING.md
- AI Assistants: Follow AGENTS.md guidance

## Release Process

This project uses [git flow](https://www.atlassian.com/git/tutorials/comparing-workflows/gitflow-workflow) and [semantic versioning](https://semver.org/).

### Finishing a Release

After a release PR is approved and CI passes:

```bash
# 1. Merge the release PR on GitHub (use "Create a merge commit")
#    URL: https://github.com/fxsml/gopipe/pull/XX

# 2. Update local main branch
git checkout main
git pull origin main

# 3. Tag the release
git tag v0.X.Y
git push origin v0.X.Y

# 4. Merge release back to develop
git checkout develop
git merge main
git push origin develop

# 5. (Optional) Delete the release branch
git branch -d release/v0.X.Y
git push origin --delete release/v0.X.Y
```

### Creating the GitHub Release

After tagging:

```bash
# 1. Create the release on GitHub
gh release create v0.X.Y --title "v0.X.Y" --notes "Release notes here..."

# 2. Close related issues
gh issue close <issue-number> --comment "Released in v0.X.Y: https://github.com/fxsml/gopipe/releases/tag/v0.X.Y"
```

**Release notes template:**

```markdown
## What's New in v0.X.Y

### Features
- Feature description (#issue)

### Bug Fixes
- Fix description (#issue)

### Breaking Changes
- Description of breaking change

### Full Changelog
https://github.com/fxsml/gopipe/compare/vPREV...vX.Y.Z
```

### Version Numbering

- **Major** (v1.0.0): Breaking changes after v1.0
- **Minor** (v0.X.0): New features, breaking changes before v1.0
- **Patch** (v0.0.X): Bug fixes only

### Checking Version

```bash
# Install git-semver (one time)
make install-tools

# Show current version
make version
```

### Creating a Hotfix

For critical fixes to production:

```bash
# 1. Create hotfix branch from main
git checkout main
git checkout -b hotfix/v0.X.Y

# 2. Make fixes, commit, push
git commit -m "fix: critical bug description"
git push -u origin hotfix/v0.X.Y

# 3. Create PR to main, get review, merge

# 4. Tag and merge back to develop (same as release)
git checkout main && git pull
git tag v0.X.Y
git push origin v0.X.Y
git checkout develop && git merge main && git push
```

## Code of Conduct

- Be respectful and constructive
- Focus on what's best for the project
- Welcome newcomers
- Give and receive feedback gracefully

## License

By contributing, you agree that your contributions will be licensed under the same license as the project.
