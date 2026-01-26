# Skill: Ralph Specs

Work with feature specifications in a codebase.

## Overview

Specs describe **what** a feature is and **why** it exists. They are durable documents that outlive implementation details. Specs are not code — they link to code.

Plans are separate — they live in `plans/` and track task execution. See the **ralph-plan** skill for that schema.

## Related Skills

- **ralph-plan** — Task execution and lifecycle
- **ralph-spec-to-plan** — Generate plans from specs

## Structure

```
specs/
├── INDEX.md          # Lookup table for all features
├── feature-a/
│   ├── SPEC.md       # Feature spec
│   └── sub-feature/
│       └── SPEC.md   # Sub-feature spec
└── feature-b/
    └── SPEC.md

plans/
├── pending/          # Plans not yet started
│   └── feature-a.md
├── current/          # Plans in active development
│   └── feature-b.md
└── complete/         # Finished plans (archive)
    └── feature-c.md
```

**Conventions:**
- One folder per feature in `specs/`
- `SPEC.md` is always the spec file
- Sub-features are nested folders
- Plans live separately in `plans/{pending,current,complete}/`
- Plan filename matches feature name: `auth.md`, `search.md`

## INDEX.md Schema

```markdown
# Spec Index

| ID | Feature | Status | Path | Requires |
|----|---------|--------|------|----------|
| F1 | Auth | in_progress | [auth](auth/SPEC.md) | — |
| F1.1 | ↳ OAuth | planned | [auth/oauth](auth/oauth/SPEC.md) | F1 |
| F1.2 | ↳ MFA | planned | [auth/mfa](auth/mfa/SPEC.md) | F1 |
| F2 | Payments | blocked | [payments](payments/SPEC.md) | F1 |
| F3 | Search | complete | [search](search/SPEC.md) | — |

## By Status

**In Progress:** F1
**Blocked:** F2
**Planned:** F1.1, F1.2
**Complete:** F3
```

**Status values:** `planned` | `in_progress` | `blocked` | `complete`

## SPEC.md Schema

```markdown
# Feature: [Name]

**ID:** F1
**Status:** planned | in_progress | blocked | complete
**Requires:** — *(or F2, F3 if dependencies)*

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
| `src/path/to/file.ts` | Brief purpose |

### Data Model
[Conceptual description of data structures and storage. Not code.]

### External Dependencies
[Services, APIs, infrastructure this feature requires]

## Gotchas
[Hard-won learnings. Non-obvious constraints. Things that will bite future devs.]

- [Gotcha 1]
- [Gotcha 2]

## Sub-Features

| ID | Sub-Feature | Status | Path |
|----|-------------|--------|------|
| F1.1 | Name | status | [path/](path/SPEC.md) |

## Plan

**Plan:** [/plans/current/feature-name.md](/plans/current/feature-name.md)

## Open Questions
- [ ] Question 1 → *Pending*
- [x] Question 2 → **Decision:** Answer

## References
- [Link to relevant docs]

---

## Changelog
- YYYY-MM-DD: Change description
```

## Working With Specs

### Finding a Feature

1. Open `specs/INDEX.md`
2. Find by name, ID, or status
3. Follow link to `SPEC.md`

### Creating a New Feature

1. Create folder: `specs/feature-name/`
2. Create `SPEC.md` using schema above
3. Add entry to `INDEX.md`
4. If feature has significant tasks, generate plan using **ralph-spec-to-plan** skill

### Creating a Sub-Feature

1. Create nested folder: `specs/parent/sub-feature/`
2. Create `SPEC.md` with `Requires: F[parent]`
3. Add to parent's Sub-Features table
4. Add to `INDEX.md` with hierarchical ID (F1.1)

### Updating a Spec

When implementation changes:
- Update **Key Files** table if files added/moved/removed
- Add to **Gotchas** when you learn something non-obvious
- Update **Status** in both `SPEC.md` and `INDEX.md`
- Add **Changelog** entry

Do NOT update:
- Summary, Goals, Non-Goals (these are stable unless feature pivots)
- Design Overview (unless architecture actually changes)

### Completing a Feature

1. Set `Status: complete` in `SPEC.md`
2. Update `INDEX.md` status
3. Ensure **Gotchas** captures key learnings
4. Ensure **Key Files** is accurate
5. Add changelog entry

## Principles

1. **Specs are maps, not territory.** Point to code, don't replicate it.

2. **Gotchas are gold.** Capture hard-won learnings immediately.

3. **Key Files = entry points.** Link to 3-7 important files, not every file.

4. **Status lives in two places.** INDEX.md (for scanning) and SPEC.md (for detail). Keep in sync.

5. **Specs are durable.** Written once, updated rarely. If you're updating often, it's implementation detail — put it elsewhere.

6. **No code in specs.** Unless it's a gotcha or a critical interface that rarely changes.

## Commands

```bash
# Find all in-progress features
grep "in_progress" specs/INDEX.md

# Find feature by ID
grep "^| F2 " specs/INDEX.md

# List all specs
find specs -name "SPEC.md"

# Check for specs without INDEX entry
for f in $(find specs -name "SPEC.md"); do
  id=$(grep "^\*\*ID:\*\*" "$f" | sed 's/.*\*\* //')
  grep -q "$id" specs/INDEX.md || echo "Missing from INDEX: $f"
done
```

## Integration With Plans

Specs and plans are separate:

```
specs/auth/SPEC.md           # What and why (durable)
plans/current/auth.md        # How and status (volatile)
```

Spec links to plan: `**Plan:** [/plans/current/auth.md](/plans/current/auth.md)`

Plan lifecycle:
1. Created in `plans/pending/` from spec (see **ralph-spec-to-plan** skill)
2. Moved to `plans/current/` when work begins
3. Moved to `plans/complete/` when all tasks done

## Anti-Patterns

- **Code blocks in specs** — Rots quickly. Link to files instead.

- **Detailed API schemas** — Put in API docs or code. Spec just describes conceptually.

- **Updating spec for every code change** — Specs are stable. Only update for architectural changes.

- **Orphaned specs** — Always keep INDEX.md in sync.

- **Specs without Gotchas** — If a feature is complete and has no gotchas, you forgot to write them down.
