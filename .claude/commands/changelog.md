# Command: changelog

Add or update CHANGELOG entries.

## Procedure Reference

This command follows the [Keep a Changelog](https://keepachangelog.com/) format as specified in the CHANGELOG.md header.

## Usage

Add an entry:
```
/changelog add TYPE "description"
```

Prepare for release:
```
/changelog release VERSION
```

## Required Arguments

For `add`:
- TYPE: One of `feat`, `fix`, `refactor`, `docs`, `test`, `chore`, `breaking`
- Description: What changed (in quotes)

For `release`:
- VERSION: Version number (e.g., `v0.14.0`)

## Execution

### Mode: Add Entry

1. Parse type and map to CHANGELOG section:
   | Type | Section |
   |------|---------|
   | `feat` | Added |
   | `fix` | Fixed |
   | `refactor` | Changed |
   | `docs` | Changed |
   | `test` | Changed |
   | `chore` | Changed |
   | `breaking` | Breaking Changes |

2. Read current CHANGELOG.md

3. Find `[Unreleased]` section

4. Add entry under appropriate subsection:
   ```markdown
   ### Added

   - **[scope]:** Description
   ```

5. If subsection doesn't exist, create it in order:
   - Breaking Changes
   - Added
   - Changed
   - Deprecated
   - Removed
   - Fixed
   - Security

6. Write updated CHANGELOG.md

7. Optionally commit:
   ```bash
   git add CHANGELOG.md
   git commit -m "docs: update CHANGELOG"
   ```

### Mode: Prepare Release

1. Read current CHANGELOG.md

2. Find `[Unreleased]` section content

3. If empty, abort: "No unreleased changes to release"

4. Create new version section:
   ```markdown
   ## [VERSION] - YYYY-MM-DD

   [content from Unreleased]
   ```

5. Clear `[Unreleased]` section (keep header, remove content)

6. Write updated CHANGELOG.md

7. Do NOT commit (the release command handles this)

## Examples

```bash
# Add a feature
/changelog add feat "message: add batch processing support"

# Add a fix
/changelog add fix "pipe: correct race condition in Merger"

# Add breaking change
/changelog add breaking "message: rename ParseRawMessage to ParseRaw"

# Prepare release
/changelog release v0.14.0
```

## Completion Summary

For `add`:
- Entry added to `[Unreleased]` section
- Section: [Added/Fixed/Changed/etc.]

For `release`:
- Moved unreleased content to `[VERSION]` section
- Ready for release command

## Error Handling

- If CHANGELOG.md doesn't exist, create with standard header
- If `[Unreleased]` section missing, create it
- If entry already exists (duplicate), warn but don't add
- If version already exists, abort with error
