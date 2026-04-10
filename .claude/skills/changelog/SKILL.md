---
name: changelog
description: Add an entry to the CHANGELOG under [Unreleased]. Usage: /changelog TYPE DESC (e.g. /changelog Added support for Redis adapter)
allowed-tools:
  - Read
  - Edit
---

# Changelog

Add a CHANGELOG entry for: `$ARGUMENTS`

## Steps

1. Read `CHANGELOG.md` to find the `[Unreleased]` section
2. Parse arguments: first word is the subsection type (`Added`, `Changed`, `Fixed`, `Removed`, `Deprecated`), rest is the description
3. Add a bullet point under the correct subsection within `[Unreleased]`
   - Create the subsection header if it doesn't exist yet
   - Keep alphabetical order of subsections: Added → Changed → Deprecated → Fixed → Removed
4. Report the line added

## CHANGELOG Format

```markdown
## [Unreleased]

### Added

- New feature description

### Fixed

- Bug fix description
```

If `$ARGUMENTS` does not start with a recognized subsection type, ask the user to clarify.
