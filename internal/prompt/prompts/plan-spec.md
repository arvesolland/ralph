# Plan File Specification

Plans are structured markdown files that define tasks for Ralph to implement.

## Task Management

Task state is managed by ATM (Autonomous Task Manager) via the `atm-cli` command. The plan markdown defines *what* to build (tasks, criteria, context), while ATM tracks runtime state (task status, progress).

Agents manage task status via `atm-cli` commands:
```bash
atm-cli task list              # View all tasks
atm-cli task start <id>        # Start a task
atm-cli task done <id>         # Complete a task
atm-cli task skip <id>         # Skip a task
atm-cli task add --title "..." # Add discovered work
```

## Structure

```markdown
# Plan: [Plan Name]

## Context
[Everything needed to understand this work. Constraints, background, goals.
Write this once, write it well. Tasks reference this implicitly.]

---

## Rules

1. **Pick task:** First task (by number) where status is not complete and all dependencies are satisfied
2. **Tasks are sequential.** Complete one before the next.
3. **Task complete when:** All acceptance criteria met
4. **New work found?** Add via `atm-cli task add`, continue current task.

---

## Tasks

### T1: [Task Title]
> [One-line summary of why this task exists]

**Requires:** —
**Status:** open

**Done when:**
- [ ] [Specific, testable criterion]
- [ ] [Specific, testable criterion]

---

### T2: [Task Title]
> [Why this task exists]

**Requires:** T1
**Status:** blocked

**Done when:**
- [ ] [Criterion]

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
- All must be verified for task to be complete

---

## Task Selection Logic

```
Find first T[n] where:
  - Status is not complete
  - Every task in "Requires" has Status = complete

Return: Task context + work to do.
```

---

## Completion

When a task is complete:
1. All "Done when" criteria are verified
2. Mark task done via `atm-cli task done <id>`

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

**End state:** Users hit /auth/login -> receive JWT + refresh token -> use JWT for API calls.

**Constraints:**
- Backward compatible with mobile clients < v2.3
- Token expiry: 15 min access, 7 day refresh
- Must pass security review before deploy

---

## Rules

1. **Pick task:** First task (by number) where status is not complete and all dependencies satisfied
2. **Tasks are sequential.** Complete one before the next.
3. **Task complete when:** All "Done when" criteria met
4. **New work found?** Add via `atm-cli task add`, continue current task.

---

## Tasks

### T1: Design Token Schema
> Need agreement on JWT structure before any code.

**Requires:** —
**Status:** open

**Done when:**
- [ ] JWT claims documented in `/docs/auth/jwt-claims.md`
- [ ] Refresh token rotation strategy documented

---

### T2: Implement Token Service
> Core JWT generation/validation.

**Requires:** T1
**Status:** blocked

**Done when:**
- [ ] `TokenService` with `generate()` and `validate()` methods
- [ ] Unit tests pass
- [ ] No secrets in code

---

### T3: Update Login Endpoint
> Modify /auth/login to return JWT.

**Requires:** T2
**Status:** blocked

**Done when:**
- [ ] /auth/login returns `{ accessToken, refreshToken, expiresIn }`
- [ ] API docs updated

---

## Discovered

*(None yet)*
```
