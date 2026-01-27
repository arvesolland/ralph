# Ralph Reverse - Plan Generator

You are generating a Ralph plan to create specs for discovered features in **{{PROJECT_NAME}}**.

{{PROJECT_DESCRIPTION}}

## Your Mission

Read the discovery document and generate a Ralph-format plan with one task per feature. Each task will create a spec file.

---

## FIRST: Read Your Context

Read `scripts/ralph/context.json` to get:
- `mode` - Should be "generate-plan"
- `discoveryFile` - Path to the completed discovery document
- `planFile` - Path where you should create the plan
- `progressFile` - Path for the plan's progress file
- `specsDir` - Where specs should be created

---

## Step 1: Read Discovery Document

Read the discovery file from context.json. Extract:
1. All features marked 🟢 High confidence
2. Their dependencies (Depends On column)
3. Key paths for each feature
4. Any notes or context

---

## Step 2: Generate Plan

Create a plan file at the path from context.json with this structure:

```markdown
# Plan: Reverse Specs for [Project Name]

**Source:** [reverse-discovery.md](./reverse-discovery.md)
**Generated:** YYYY-MM-DD
**Features:** N

## Context

This plan creates Ralph specs for existing features discovered through codebase analysis.

Each task:
1. Analyzes the code for one feature
2. Creates a SPEC.md file following the ralph-spec schema
3. Updates specs/INDEX.md

Refer to the discovery document for feature boundaries and context.

---

## Rules

1. **Pick task:** First task where Status ≠ `complete` and all `Requires` are `complete`
2. **One spec per task:** Each task creates exactly one SPEC.md
3. **Follow the schema:** Use ralph-spec SKILL.md format exactly
4. **Update INDEX:** Add entry to specs/INDEX.md after each spec
5. **Commit after each:** One commit per spec created

---

## Tasks

### T1: Create spec for [Feature Name]
> Document the [feature] including [brief scope from discovery]

**Requires:** — (or T2 if depends on another feature's spec)
**Status:** open
**Scope:** [Key paths from discovery document]

**Done when:**
- [ ] specs/[feature]/SPEC.md created with full schema
- [ ] INDEX.md updated with entry
- [ ] Gotchas section populated

**Subtasks:**
1. [ ] Read all files in scope to understand feature
2. [ ] Identify: summary, goals, key files, data model, dependencies
3. [ ] Extract gotchas from code patterns, comments, error handling
4. [ ] Create specs/[feature]/SPEC.md
5. [ ] Add entry to specs/INDEX.md
6. [ ] Commit: "docs(specs): add [feature] feature spec"

---

### T2: Create spec for [Next Feature]
...

---

## Discovered

<!-- New features found during spec writing go here -->

```

---

## Task Ordering Rules

1. **Features with no dependencies come first**
2. **Dependent features come after their dependencies**
   - If Feature B depends on Feature A, T(B) requires T(A)
3. **Order by complexity** - simpler features first (fewer key paths)
4. **Group related features** when possible

---

## Dependency Mapping

From the discovery document's "Depends On" column:

| Discovery | Plan |
|-----------|------|
| Feature A depends on — | T(A) Requires: — |
| Feature B depends on A | T(B) Requires: T(A) |
| Feature C depends on A, B | T(C) Requires: T(A), T(B) |

This ensures specs are created in the right order (can reference earlier specs).

---

## Scope Field

For each task's Scope field, list the key paths from the discovery document:

```
**Scope:** src/auth/, middleware/auth.ts, models/User.ts
```

This tells the spec writer exactly which files to analyze.

---

## Example Output

```markdown
# Plan: Reverse Specs for MyApp

**Source:** [reverse-discovery.md](./reverse-discovery.md)
**Generated:** 2024-01-27
**Features:** 5

## Context

This plan creates Ralph specs for existing features discovered through codebase analysis.
...

---

## Tasks

### T1: Create spec for Authentication
> Document the auth feature including OAuth and session management

**Requires:** —
**Status:** open
**Scope:** src/auth/, middleware/auth.ts, lib/session.ts

**Done when:**
- [ ] specs/auth/SPEC.md created with full schema
- [ ] INDEX.md updated with entry
- [ ] Gotchas section populated

**Subtasks:**
1. [ ] Read all files in scope to understand feature
2. [ ] Identify: summary, goals, key files, data model, dependencies
3. [ ] Extract gotchas from code patterns, comments, error handling
4. [ ] Create specs/auth/SPEC.md
5. [ ] Add entry to specs/INDEX.md
6. [ ] Commit: "docs(specs): add auth feature spec"

---

### T2: Create spec for User Management
> Document user CRUD, roles, and permissions system

**Requires:** T1
**Status:** blocked
**Scope:** src/users/, models/User.ts, models/Role.ts

...
```

---

## Also Create Progress File

Create an empty progress file at the progress path from context.json:

```markdown
# Reverse Specs Progress

Learnings captured during spec generation.

---
```

This will be populated as specs are written.

---

## Output

1. Read context.json
2. Read the discovery document
3. Create the plan file with all tasks
4. Create empty progress file
5. Output confirmation of what was created
