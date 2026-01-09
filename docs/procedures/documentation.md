# Documentation Procedures

Procedures for maintaining gopipe documentation.

## Quick Start

```bash
# Run documentation lint (Claude-assisted)
make docs-lint
```

## Documentation Lint Procedure

Run this procedure to check documentation quality and consistency.

### Step 1: Check Broken Internal Links

Verify all internal markdown links resolve to existing files:

```bash
# Find all markdown links and check they exist
grep -roh '\[.*\]([^http][^)]*\.md)' docs/ | \
  sed 's/.*(\(.*\))/\1/' | sort -u
```

**Check for:**
- Links to non-existent files
- Links to moved/renamed files
- Incorrect relative paths

### Step 2: Verify ADR Index

Compare ADR files with README index:

```bash
# List ADR files
ls docs/adr/*.md | grep -v README

# Compare with README entries
grep -E '^\| \[' docs/adr/README.md
```

**Check for:**
- ADRs missing from index
- Index entries for deleted ADRs
- Status mismatches (file says Implemented, index says Accepted)

### Step 3: Verify Procedures Index

Compare procedure files with README index:

```bash
# List procedure files
ls docs/procedures/*.md | grep -v README

# Compare with README entries
grep -E '^\| \[' docs/procedures/README.md
```

**Check for:**
- Procedures missing from index
- Index entries for deleted procedures

### Step 4: Check CHANGELOG

Verify CHANGELOG structure:

```bash
head -50 CHANGELOG.md
```

**Check for:**
- `[Unreleased]` section exists at top
- Version sections have dates
- No duplicate version entries

### Step 5: Check Plans Index

Compare plan files with README index:

```bash
# List active plans
ls docs/plans/*.md | grep -v README

# List archived plans
ls docs/plans/archive/*.md

# Compare with README
cat docs/plans/README.md
```

**Check for:**
- Active plans missing from "Current" table
- Archived plans missing from "Archive" table
- Status mismatches

### Step 6: Report

Output a summary of issues found:

```
Documentation Lint Report
=========================
[ ] Broken links: X found
[ ] ADR index: X missing, X stale
[ ] Procedures index: X missing, X stale
[ ] CHANGELOG: OK / Issues found
[ ] Plans index: X missing, X stale
```

## Directory Structure

```
docs/
├── adr/                 # Architecture Decision Records
├── patterns/            # Design patterns and examples
├── plans/               # Implementation plans (active + archive/)
├── procedures/          # Development procedures (this)
└── README.md            # Documentation overview
```

## Before Pushing Code

- [ ] CHANGELOG.md updated under `[Unreleased]`
- [ ] ADR created for architectural decisions (see [adr.md](adr.md))
- [ ] Godoc for public APIs
- [ ] README.md for core component changes

## Plan Documentation

Active plans live in `docs/plans/` with descriptive names. Completed plans are archived in `docs/plans/archive/`.

See [planning.md](planning.md) for plan templates and procedures.

## Maintenance

Weekly:
- [ ] Run `make docs-lint` and fix issues
- [ ] Review open plans for status updates

Before release:
- [ ] Move `[Unreleased]` to version section in CHANGELOG
- [ ] Update ADR statuses (Accepted → Implemented)
