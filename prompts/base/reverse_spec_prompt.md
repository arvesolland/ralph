# Ralph Reverse - Single Feature Spec Generator

You are creating a Ralph spec for an existing feature in **{{PROJECT_NAME}}**.

{{PROJECT_DESCRIPTION}}

{{TECH_STACK}}

## Your Mission

Analyze the code for a single feature and create a comprehensive SPEC.md file.

## Project Principles

{{PRINCIPLES}}

## Code Patterns

{{PATTERNS}}

---

## FIRST: Read Your Context

Read `scripts/ralph/context.json` to get:
- `mode` - Should be "single-feature"
- `featureName` - Name of the feature to document
- `featurePath` - Optional path hint (may be empty)
- `specDir` - Directory to create (e.g., specs/auth/)
- `specFile` - Full path for SPEC.md
- `projectRoot` - Project root path

---

## Step 1: Locate the Feature

If `featurePath` is provided, start there. Otherwise, search for the feature:

```bash
# Try common locations
ls src/[featureName]/
ls lib/[featureName]/
ls app/[featureName]/

# Search for mentions
grep -r "[featureName]" --include="*.ts" --include="*.js" -l
```

---

## Step 2: Analyze the Feature

Read all relevant files and extract:

### Summary
- What does this feature do?
- What problem does it solve?
- Who uses it (users, other features, external systems)?

### Goals
- What are the success criteria?
- What key outcomes does it achieve?

### Non-Goals (inferred)
- What does this feature explicitly NOT do?
- What's out of scope?

### Design Overview
- How does it work conceptually?
- What's the architecture?
- Key flows (happy path, error handling)

### Key Files
- Entry points (3-7 most important files)
- Don't list every file, just the ones a new dev should read first

### Data Model
- What data structures does it use?
- How is data stored/retrieved?
- Key types/interfaces

### External Dependencies
- External services, APIs
- Third-party libraries specific to this feature
- Infrastructure requirements

### Gotchas
This is the most important section! Look for:
- Error handling patterns (what can go wrong?)
- Edge cases in the code
- TODO/FIXME/HACK comments
- Non-obvious constraints
- Performance considerations
- Security considerations
- Things that surprised you

### Dependencies on Other Features
- What other features does this depend on?
- What features depend on this?

---

## Step 3: Create the Spec

Create the directory and SPEC.md file:

```markdown
# Feature: [Name]

**ID:** F[N]
**Status:** complete
**Requires:** — (or F[other] if depends on another feature)

## Summary

[2-3 sentences. What is this feature? What problem does it solve?]

## Goals

- [What success looks like]
- [Key outcome]

## Non-Goals

- [Explicit scope boundary]
- [What this will NOT do]

## Design

### Overview

[How it works conceptually. Architecture decisions. Key flows in prose.]

### Key Files

| File | Purpose |
|------|---------|
| `src/path/to/entry.ts` | Main entry point |
| `src/path/to/core.ts` | Core logic |
| `src/path/to/types.ts` | Type definitions |

### Data Model

[Conceptual description of data structures and storage. Not code.]

### External Dependencies

[Services, APIs, infrastructure this feature requires]

## Gotchas

> Hard-won learnings. Non-obvious constraints. Things that will bite future devs.

- **[Gotcha Title]:** Explanation of the gotcha
- **[Another Gotcha]:** What developers need to know

## Open Questions

- [ ] Any unresolved questions discovered during analysis

## References

- [Link to relevant docs, if any]

---

## Changelog

- YYYY-MM-DD: Spec created via ralph-reverse
```

---

## Step 4: Update INDEX.md

Add an entry to `specs/INDEX.md`:

```markdown
| F[N] | [Feature Name] | complete | [[feature]](feature/SPEC.md) | — |
```

If INDEX.md doesn't exist, create it:

```markdown
# Spec Index

| ID | Feature | Status | Path | Requires |
|----|---------|--------|------|----------|
| F1 | [Feature] | complete | [[feature]](feature/SPEC.md) | — |
```

---

## Rules

### DO:
1. **Read the actual code** - Don't guess, analyze
2. **Focus on Gotchas** - Most valuable part of a spec
3. **Link to key files** - 3-7 entry points, not every file
4. **Mark status as complete** - Feature already exists
5. **Be concise** - Specs are maps, not territory

### DON'T:
1. **Don't include code blocks** - They rot quickly
2. **Don't list every file** - Just key entry points
3. **Don't duplicate code comments** - Summarize instead
4. **Don't make up information** - Only document what you find

---

## Gotcha Mining

Look for gotchas in these places:

| Source | What to Extract |
|--------|-----------------|
| `// TODO`, `// FIXME`, `// HACK` | Known issues |
| Error handling blocks | What can go wrong |
| Configuration/env vars | Setup requirements |
| Test files | Edge cases being tested |
| Complex conditionals | Non-obvious logic |
| Comments explaining "why" | Design decisions |
| Retry/fallback logic | Reliability concerns |

---

## Output

1. Read context.json for feature info
2. Locate and analyze all relevant code
3. Create specs/[feature]/SPEC.md
4. Update specs/INDEX.md
5. Confirm what was created
