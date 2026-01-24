# Plan File Specification

Plans are structured markdown files that define tasks for Ralph to implement.

## Structure

```markdown
# Plan: [Plan Name]

## Context
[Everything needed to understand this work. Constraints, background, goals.
Write this once, write it well. Tasks reference this implicitly.]

---

## Rules

1. **Pick task:** First task (by number) where status ≠ `complete` and all `Requires` are `complete`
2. **Subtasks are sequential.** Complete 1 before 2.
3. **Task complete when:** All "Done when" + all subtasks checked → set Status: `complete`
4. **Update file after each checkbox.**
5. **New work found?** Add to Discovered section, continue current task.

---

## Tasks

### T1: [Task Title]
> [One-line summary of why this task exists]

**Requires:** —
**Status:** open

**Done when:**
- [ ] [Specific, testable criterion]
- [ ] [Specific, testable criterion]

**Subtasks:**
1. [ ] [First subtask] — [brief what/why if not obvious]
2. [ ] [Second subtask]
3. [ ] [Third subtask]

---

### T2: [Task Title]
> [Why this task exists]

**Requires:** T1
**Status:** blocked

**Done when:**
- [ ] [Criterion]

**Subtasks:**
1. [ ] [Subtask]
2. [ ] [Subtask]

---

## Discovered
<!-- Add with D1, D2, etc. Include "Found in: T1" -->

```

---

## Field Reference

### Status Values
- `open` — Ready to work (no blockers)
- `in_progress` — Currently being worked on
- `blocked` — Waiting on dependencies
- `complete` — All criteria met

### Requires
- `—` means no dependencies
- `T1` means depends on T1 being complete
- `T1, T2` means depends on both

### Done When
- Specific, testable acceptance criteria
- All must be checked for task to be complete

### Subtasks
- Numbered for explicit ordering
- Complete 1 before starting 2
- Brief description, add "— [why]" if not obvious

---

## Task Selection Logic

```
Find first T[n] where:
  - Status ≠ complete
  - Every task in "Requires" has Status = complete

Within that task, find first unchecked subtask.

Return: Task context + subtask to work on.
```

---

## Completion

When a task is complete:
1. All "Done when" checkboxes are checked
2. All subtask checkboxes are checked
3. Change `**Status:** open` → `**Status:** complete`

When ALL tasks are complete, output:
```
<promise>COMPLETE</promise>
```

---

## Example Plan

```markdown
# Plan: JWT Authentication System

## Context
Replace session-based auth with JWT. Enables stateless auth and microservices readiness.

**End state:** Users hit /auth/login → receive JWT + refresh token → use JWT for API calls.

**Constraints:**
- Backward compatible with mobile clients < v2.3
- Token expiry: 15 min access, 7 day refresh
- Must pass security review before deploy

---

## Rules

1. **Pick task:** First task (by number) where status ≠ `complete` and all `Requires` are `complete`
2. **Subtasks are sequential.** Complete 1 before 2.
3. **Task complete when:** All "Done when" + all subtasks checked → set Status: `complete`
4. **Update file after each checkbox.**
5. **New work found?** Add to Discovered section, continue current task.

---

## Tasks

### T1: Design Token Schema
> Need agreement on JWT structure before any code.

**Requires:** —
**Status:** open

**Done when:**
- [ ] JWT claims documented in `/docs/auth/jwt-claims.md`
- [ ] Refresh token rotation strategy documented

**Subtasks:**
1. [ ] Draft JWT claims (sub, iat, exp, roles, permissions)
2. [ ] Design refresh token rotation (one-time use)
3. [ ] Document in `/docs/auth/`

---

### T2: Implement Token Service
> Core JWT generation/validation.

**Requires:** T1
**Status:** blocked

**Done when:**
- [ ] `TokenService` with `generate()` and `validate()` methods
- [ ] Unit tests pass
- [ ] No secrets in code

**Subtasks:**
1. [ ] Implement `generate()` with claims from T1
2. [ ] Implement `validate()` with expiry handling
3. [ ] Write unit tests

---

### T3: Update Login Endpoint
> Modify /auth/login to return JWT.

**Requires:** T2
**Status:** blocked

**Done when:**
- [ ] /auth/login returns `{ accessToken, refreshToken, expiresIn }`
- [ ] API docs updated

**Subtasks:**
1. [ ] Modify LoginController response format
2. [ ] Update API documentation

---

## Discovered

*(None yet)*
```

---

## Plan Location

For queue workflow:
```
.ralph/plans/
├── pending/    # Waiting to be processed
├── current/    # Active plan (0-1 files)
└── completed/  # Finished with logs
```
