---
name: create-adr
description: Create a new Architecture Decision Record. Usage: /create-adr TITLE (e.g. /create-adr add-redis-adapter)
allowed-tools:
  - Bash
  - Read
  - Write
  - Edit
  - Glob
---

# Create ADR

Create an ADR for: `$ARGUMENTS`

Follow the procedure: @../docs/procedures/adr.md

## Steps

1. Read `docs/adr/README.md` to get the next sequential number
2. Create `docs/adr/NNNN-<slugified-title>.md` using the template from the procedure
3. Fill in:
   - `Date:` today's date
   - `Status: Proposed`
   - `Context:` describe the problem or situation
   - `Decision:` placeholder (to be filled)
   - `Consequences:` placeholder (to be filled)
4. Add entry to `docs/adr/README.md` index under **Proposed** status
5. Report the file path created

Use the title from arguments as the ADR title. Slugify it for the filename (lowercase, hyphens).
