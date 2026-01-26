# Skill: Ralph Plans

Work with task plans for features and projects.

## Overview

Plans track **how** work gets done — tasks, status, and progress. Plans are volatile (updated frequently) while specs are durable.

Plans live in a `plans/` folder at the repo root with lifecycle subfolders.

## Related Skills

- **ralph-spec** — Feature specifications
- **ralph-spec-to-plan** — Generate plans from specs

## Structure

```
plans/
├── pending/          # Plans ready to start
│   ├── oauth.md
│   └── mfa.md
├── current/          # Plans in active development (max 1-2 recommended)
│   └── auth.md
└── complete/         # Finished plans (archive)
    └── search.md
```

**Lifecycle:**
1. `pending/` — Plan created from spec, waiting to start
2. `current/` — Actively being worked
3. `complete/` — All tasks done, archived for reference

**Naming:** Plan filename matches feature name: `auth.md`, `search.md`

## Plan Schema

```markdown
# Plan: [Feature Name]

**Spec:** [/specs/feature/SPEC.md](/specs/feature/SPEC.md)
**Created:** YYYY-MM-DD
**Status:** pending | current | complete

## Context

[Brief summary — enough to work without reading full spec. Key constraints.]

### Gotchas (from spec)
- [Critical gotcha 1 copied from spec]
- [Critical gotcha 2 copied from spec]

---

## Rules

1. **Pick task:** First task where status ≠ `complete` and all `Requires` are `complete`
2. **Subtasks are sequential.** Complete 1 before 2.
3. **Task complete when:** All "Done when" checked → set Status: `complete`
4. **Update file after each checkbox.**
5. **New work found?** Add to Discovered section, continue current task.

---

## Tasks

### T1: [Task Title]
> [One-line why this task exists]

**Requires:** —
**Status:** open

**Done when:**
- [ ] [Specific, testable acceptance criterion]
- [ ] [Specific, testable acceptance criterion]

**Subtasks:**
1. [ ] [Atomic subtask]
2. [ ] [Atomic subtask]

---

### T2: [Task Title]
> [One-line why]

**Requires:** T1
**Status:** blocked

**Done when:**
- [ ] [Criterion]

---

## Discovered

<!-- Tasks found during implementation -->

---

## Completed

- YYYY-MM-DD: Plan completed, moved to complete/
```

## Task Quality

### Good "Done When" Criteria

Acceptance criteria must be **specific**, **testable**, and **atomic**.

**Good:**
- [ ] `TokenService.generate()` returns valid JWT
- [ ] Unit tests pass with >90% coverage
- [ ] `/auth/login` returns `{ accessToken, refreshToken, expiresIn }`
- [ ] Old mobile clients (User-Agent check) still receive session cookie
- [ ] Security team approved (link in Notes)

**Bad:**
- [ ] Auth works *(vague)*
- [ ] Tests written *(how many? what coverage?)*
- [ ] API updated *(which endpoints? what changes?)*
- [ ] Reviewed *(by whom? what approval?)*

### Good Subtasks

Subtasks should be **atomic** — completable in one focused session.

**Good:**
1. [ ] Create `TokenService` class skeleton
2. [ ] Implement `generate()` method
3. [ ] Implement `validate()` method
4. [ ] Add signing key rotation support
5. [ ] Write unit tests

**Bad:**
1. [ ] Implement TokenService *(too big — break it down)*
2. [ ] Do the auth stuff *(vague)*
3. [ ] Tests *(which tests?)*

## Working With Plans

### Finding Next Task

```
1. Open plan in plans/current/
2. Find first T[n] where:
   - Status ≠ complete
   - All tasks in Requires have Status = complete
3. Within that task, find first unchecked subtask
4. That's your next work item
```

### Starting a Plan

1. Move from `plans/pending/` to `plans/current/`
2. Update `**Status:** current` in the plan
3. Update spec's Plan link if needed
4. Begin with T1

### Completing a Task

1. Verify ALL "Done when" boxes checked
2. Verify ALL subtask boxes checked
3. Set `**Status:** complete`
4. Commit the updated plan
5. Move to next task (any task with `Requires: T[this]` becomes unblocked)

### Completing a Plan

1. Verify ALL tasks have `**Status:** complete`
2. Add completion date to `## Completed` section
3. Move file from `plans/current/` to `plans/complete/`
4. Update spec status to `complete`
5. Update `specs/INDEX.md`

### Discovering New Work

When you find work not in the plan:

1. **Don't interrupt current task**
2. Add to `## Discovered` section:

```markdown
## Discovered

### D1: [What you found]
> Found during: T2

**Requires:** —
**Status:** open

**Done when:**
- [ ] [Criterion]
```

3. Continue current task
4. Discovered work gets prioritized later (may become new task or new plan)

## Plan Lifecycle Commands

```bash
# See what's in progress
ls plans/current/

# See what's ready to start
ls plans/pending/

# Start a plan
mv plans/pending/auth.md plans/current/

# Complete a plan
mv plans/current/auth.md plans/complete/

# Find all incomplete tasks across current plans
grep -l "Status: open\|Status: in_progress\|Status: blocked" plans/current/*
```

## Integration With Ralph Scripts

Ralph scripts process plans from the `plans/` folder:

```bash
# Run ralph on a plan
./scripts/ralph/ralph.sh plans/current/auth.md

# Add to queue
./scripts/ralph/ralph-worker.sh --add plans/pending/oauth.md

# Process queue
./scripts/ralph/ralph-worker.sh --loop
```

## Anti-Patterns

- **Multiple plans in current/** — Focus. Limit to 1-2 active plans.

- **Skipping subtasks** — Always complete in order. They're ordered for a reason.

- **Forgetting to update** — Stale plans cause confusion. Update after every checkbox.

- **Vague "Done when"** — Must be specific and testable. Not "works" but "returns X when given Y".

- **Giant subtasks** — If a subtask takes more than a session, break it down further.

- **Plans without specs** — Always create spec first. Plan references spec for context.

- **Discovered work ignored** — Capture it. Don't let it slip through the cracks.

- **Missing Gotchas in Context** — Copy critical gotchas from spec to plan Context so workers don't miss them.
