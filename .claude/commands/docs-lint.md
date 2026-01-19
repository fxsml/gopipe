# Command: docs-lint

Run documentation quality and consistency checks.

## Procedure Reference

This command implements the Documentation Lint Procedure from `docs/procedures/documentation.md`.

## Usage

```
/docs-lint
```

No arguments required.

## Execution

### Step 1: Check Broken Internal Links

Find all markdown links in `docs/` and verify they resolve to existing files:

1. Extract all relative links from markdown files
2. Verify each linked file exists
3. Report any broken links

### Step 2: Verify ADR Index

Compare `docs/adr/` files with `docs/adr/README.md` entries:

1. List all ADR files: `ls docs/adr/*.md | grep -v README`
2. Extract entries from README.md
3. Report:
   - Missing from index (files exist but not in README)
   - Stale entries (in README but file doesn't exist)

### Step 3: Verify Procedures Index

Compare `docs/procedures/` files with `docs/procedures/README.md` entries:

1. List all procedure files
2. Extract entries from README.md
3. Report missing or stale entries

### Step 4: Check CHANGELOG Structure

Verify CHANGELOG.md format:

1. `[Unreleased]` section exists at top
2. Version sections have dates in format `[X.Y.Z] - YYYY-MM-DD`
3. No duplicate version entries
4. Sections use correct headers (Added, Changed, Fixed, etc.)

### Step 5: Check Plans Index

Compare `docs/plans/` files with `docs/plans/README.md` entries:

1. List all plan files
2. Extract entries from README.md
3. Report missing or stale entries

### Step 6: Check Examples

Verify examples are runnable:

1. List example directories in `examples/`
2. Check each has a `main.go`
3. Optionally run `go build` on each

## Output Report

```
Documentation Lint Report
=========================

Broken Links: X found
  - docs/foo.md:42 -> missing-file.md

ADR Index:
  - Missing: 3 files not in README
  - Stale: 0 entries

Procedures Index:
  - Missing: 0
  - Stale: 0

CHANGELOG:
  - Status: OK / X issues found

Plans Index:
  - Missing: 1 file not in README
  - Stale: 0 entries

Examples:
  - All buildable: YES / NO

Overall: PASS / FAIL (X issues)
```

## Auto-Fix Option

After reporting, offer to fix automatically:

"Found X fixable issues. Fix them now? (yes/no)"

Fixable issues:
- Add missing entries to indexes
- Remove stale entries from indexes

Non-fixable (require manual intervention):
- Broken links (need to determine correct target)
- CHANGELOG format issues
