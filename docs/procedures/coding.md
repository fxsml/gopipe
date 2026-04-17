# Coding Behavior

General coding discipline for gopipe development. Referenced by implement, review, and Go standards procedures.

## Think Before Coding

Before implementing anything:

- State assumptions explicitly — if uncertain, ask
- If multiple interpretations exist, present them; don't pick silently
- If a simpler approach exists, say so and push back when warranted
- If something is unclear, stop and name what's confusing

For non-trivial changes, briefly state:
- Which files will change and why
- Any new types or interfaces needed
- How it will be tested

Ask for confirmation before proceeding if the plan touches shared interfaces or public API.

## Simplicity First

Write the minimum code that solves the problem — nothing speculative.

- No features beyond what was asked
- No abstractions for single-use code
- No "flexibility" or "configurability" that wasn't requested
- No error handling for impossible scenarios

Ask: "Would a senior engineer say this is overcomplicated?" If yes, simplify.

## Surgical Changes

Touch only what you must. Clean up only your own mess.

**When editing existing code:**

- Don't "improve" adjacent code, comments, or formatting
- Don't refactor things that aren't broken
- Match existing style, even if you'd do it differently
- If you notice unrelated dead code, mention it — don't delete it

**When your changes create orphans:**

- Remove imports/variables/functions that YOUR changes made unused
- Don't remove pre-existing dead code unless asked

Every changed line should trace directly to the request.

## Goal-Driven Execution

Define verifiable success criteria before implementing:

- "Add validation" → "Write tests for invalid inputs, then make them pass"
- "Fix the bug" → "Write a test that reproduces it, then make it pass"
- "Refactor X" → "Ensure tests pass before and after"

For multi-step tasks, state a brief plan with a verify step for each:

1. [Step] → verify: [check]
2. [Step] → verify: [check]
3. [Step] → verify: [check]
