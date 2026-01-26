# Skill: Ralph Spec to Plan

Generate actionable task plans from feature specifications.

## Overview

This skill transforms a **spec** (what/why) into a **plan** (how/tasks). The goal is atomic, well-defined tasks with specific acceptance criteria that an LLM agent can execute.

## Related Skills

- **ralph-spec** — Feature specifications
- **ralph-plan** — Task execution and lifecycle

## When to Use

- New spec is ready and approved
- Feature needs to move from design to implementation
- Spec exists but has no corresponding plan

## Process

### 1. Read the Spec

Load the spec and extract:
- **Goals** — What success looks like
- **Non-Goals** — Scope boundaries
- **Design Overview** — How it works
- **Key Files** — Where code will live
- **Gotchas** — Constraints to respect
- **Open Questions** — Blockers (must be resolved before planning)

**Stop if:** Open questions are unresolved. Plan cannot be complete with unknowns.

### 2. Identify Work Streams

Break the feature into logical phases:
1. **Foundation** — Core data models, services, infrastructure
2. **Implementation** — Main feature logic
3. **Integration** — Endpoints, UI, connections to existing code
4. **Hardening** — Tests, error handling, edge cases
5. **Polish** — Documentation, cleanup, monitoring

Not every feature needs all phases. Small features might be 2-3 tasks total.

### 3. Generate Tasks

For each work stream, create tasks with:

**Task title:** Action-oriented, starts with verb
- ✓ "Implement token generation service"
- ✓ "Add refresh endpoint"
- ✗ "Token stuff"
- ✗ "Auth"

**Why line:** One sentence explaining purpose
- ✓ "Core service needed before any endpoints can work"
- ✗ "Because we need it"

**Dependencies:** What must complete first
- Explicit `Requires: T1, T2`
- Or `Requires: —` if none

**Acceptance criteria:** Specific, testable conditions
- Each criterion is pass/fail
- Include specific values, behaviors, thresholds
- Reference actual file paths, function names, endpoints

**Subtasks:** Atomic steps to complete the task
- Each completable in one focused session
- Ordered — must be done in sequence
- 3-7 subtasks per task is typical

### 4. Copy Gotchas to Context

**Critical:** Copy relevant gotchas from the spec into the plan's Context section. Workers read the plan, not the spec, during execution. Missing gotchas cause bugs.

```markdown
## Context

[Brief summary...]

### Gotchas (from spec)
- Refresh tokens are one-time-use — reuse indicates theft
- Mobile clients < v2.3 expect cookies alongside JWT
- Key rotation: verify with ANY valid key, sign with PRIMARY only
```

### 5. Validate the Plan

Check:
- [ ] Every spec Goal maps to at least one task's acceptance criteria
- [ ] No spec Non-Goal is accidentally included
- [ ] Dependencies form a valid DAG (no cycles)
- [ ] T1 has no dependencies (something must be first)
- [ ] Gotchas from spec are reflected in Context and relevant acceptance criteria
- [ ] Acceptance criteria are specific (no "works correctly")

### 6. Write the Plan

Output to `plans/pending/{feature-name}.md`

## Task Generation Template

For each task, produce:

```markdown
### T{n}: {Verb} {Object}
> {One-line why this task exists, connecting to spec goals}

**Requires:** {T1, T2 | —}
**Status:** open

**Done when:**
- [ ] {Specific testable criterion with concrete values}
- [ ] {Another specific criterion}
- [ ] {Tests/validation criterion}

**Subtasks:**
1. [ ] {Atomic step}
2. [ ] {Atomic step}
3. [ ] {Atomic step}
```

## Full Plan Template

```markdown
# Plan: [Feature Name]

**Spec:** [/specs/feature/SPEC.md](/specs/feature/SPEC.md)
**Created:** YYYY-MM-DD
**Status:** pending

## Context

[Brief summary — enough to work without reading full spec. Key constraints.]

### Gotchas (from spec)
- [Gotcha 1 copied from spec]
- [Gotcha 2 copied from spec]

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
> [Why]

**Requires:** —
**Status:** open

**Done when:**
- [ ] [Criterion]

**Subtasks:**
1. [ ] [Step]

---

## Discovered

<!-- Tasks found during implementation -->

---

## Completed

*(Note completion date when done)*
```

## Acceptance Criteria Patterns

### For Services/Classes

```markdown
- [ ] `{ClassName}` exists in `{filepath}`
- [ ] `{method}()` returns `{type}` when given `{input}`
- [ ] `{method}()` throws `{ErrorType}` when `{condition}`
- [ ] Unit tests cover `{method}` with >90% coverage
```

### For Endpoints

```markdown
- [ ] `{METHOD} {path}` returns `{status}` with `{response shape}`
- [ ] Request validation rejects `{invalid input}` with 400
- [ ] Authentication required (401 without token)
- [ ] Integration test covers happy path and error cases
```

### For Data Models

```markdown
- [ ] `{Model}` schema defined in `{filepath}`
- [ ] Migration created and tested (up and down)
- [ ] Indexes added for `{fields}` (query performance)
```

### For UI Components

```markdown
- [ ] Component renders `{elements}` given `{props}`
- [ ] `{interaction}` triggers `{behavior}`
- [ ] Loading state displayed while `{async operation}`
- [ ] Error state displayed when `{failure condition}`
```

### For Infrastructure

```markdown
- [ ] `{resource}` provisioned in `{environment}`
- [ ] Configuration in `{config file}` with `{required keys}`
- [ ] Health check endpoint returns 200
- [ ] Monitoring alerts configured for `{conditions}`
```

## Example Transformation

### Input: Spec Summary

```markdown
## Summary
JWT-based auth replacing sessions. 15 min access, 7 day refresh tokens.

## Goals
- Stateless authentication via JWT
- Token refresh without re-login
- Backward compat with mobile < v2.3

## Design Overview
TokenService generates/validates. RefreshService handles rotation.
Login endpoint returns tokens. Middleware validates on requests.

## Key Files
- src/services/token.ts
- src/services/refresh.ts
- src/routes/auth.ts
- src/middleware/auth.ts

## Gotchas
- Refresh tokens one-time-use
- Old mobile clients need session cookie too
```

### Output: Plan

```markdown
# Plan: Auth

**Spec:** [/specs/auth/SPEC.md](/specs/auth/SPEC.md)
**Created:** 2024-01-20
**Status:** pending

## Context

Replace session-based auth with JWT. Backward compat needed for mobile < v2.3.
Tokens: 15 min access, 7 day refresh, one-time-use.

### Gotchas (from spec)
- Refresh tokens are one-time-use — reuse indicates theft, invalidate entire family
- Old mobile clients (< v2.3) need session cookie alongside JWT

---

## Rules

1. **Pick task:** First task where status ≠ `complete` and all `Requires` are `complete`
2. **Subtasks are sequential.** Complete 1 before 2.
3. **Task complete when:** All "Done when" checked → set Status: `complete`
4. **Update file after each checkbox.**
5. **New work found?** Add to Discovered section, continue current task.

---

## Tasks

### T1: Implement TokenService
> Core JWT generation/validation needed before any endpoints work.

**Requires:** —
**Status:** open

**Done when:**
- [ ] `TokenService` class exists in `src/services/token.ts`
- [ ] `generate(userId, roles)` returns signed JWT with 15 min expiry
- [ ] `validate(token)` returns claims or throws `InvalidTokenError`
- [ ] `validate()` accepts tokens signed with any active key (rotation support)
- [ ] Unit tests cover: valid token, expired token, tampered token, rotated key

**Subtasks:**
1. [ ] Create TokenService class skeleton
2. [ ] Implement generate() with standard claims (sub, iat, exp, roles)
3. [ ] Implement validate() with signature and expiry checks
4. [ ] Add multi-key support for rotation
5. [ ] Write unit tests

---

### T2: Implement RefreshService
> Handles token refresh with one-time-use enforcement.

**Requires:** T1
**Status:** blocked

**Done when:**
- [ ] `RefreshService` class exists in `src/services/refresh.ts`
- [ ] `issue(userId)` creates refresh token stored in Redis with 7 day TTL
- [ ] `rotate(refreshToken)` returns new access + refresh tokens
- [ ] `rotate()` invalidates used refresh token (one-time-use)
- [ ] `rotate()` throws `TokenReuseError` if token already used
- [ ] Token family tracking enables full revocation on reuse detection

**Subtasks:**
1. [ ] Set up Redis schema for refresh tokens
2. [ ] Implement issue() with token family tracking
3. [ ] Implement rotate() with one-time-use check
4. [ ] Implement family revocation on reuse
5. [ ] Write integration tests with Redis

---

### T3: Update login endpoint
> Returns JWT tokens. Maintains backward compat for old mobile.

**Requires:** T1, T2
**Status:** blocked

**Done when:**
- [ ] `POST /auth/login` returns `{ accessToken, refreshToken, expiresIn }`
- [ ] Invalid credentials return 401 with error message
- [ ] Old mobile clients (User-Agent < v2.3) also receive session cookie
- [ ] API documentation updated in `/docs/api/auth.md`

**Subtasks:**
1. [ ] Modify LoginController to call TokenService + RefreshService
2. [ ] Add User-Agent detection for old clients
3. [ ] Implement dual-write (JWT + cookie) for backward compat
4. [ ] Update API docs
5. [ ] Write integration tests

---

### T4: Add auth middleware
> Validates JWT on protected routes.

**Requires:** T1
**Status:** blocked

**Done when:**
- [ ] Middleware extracts JWT from Authorization header
- [ ] Valid token populates `req.user` with claims
- [ ] Missing token returns 401 `{ error: "No token provided" }`
- [ ] Invalid/expired token returns 401 `{ error: "Invalid token" }`
- [ ] All routes in `/api/*` except `/api/auth/*` use middleware

**Subtasks:**
1. [ ] Create auth middleware in `src/middleware/auth.ts`
2. [ ] Wire into route configuration
3. [ ] Write tests for valid, missing, expired, tampered tokens

---

## Discovered

<!-- Tasks found during implementation -->

---

## Completed

*(Note completion date when done)*
```

## Validation Checklist

Before finalizing the plan:

- [ ] **Completeness:** Every spec goal has acceptance criteria somewhere
- [ ] **Gotchas copied:** Spec gotchas appear in Context section
- [ ] **Atomicity:** Each subtask is one session of work
- [ ] **Testability:** Every "Done when" is pass/fail checkable
- [ ] **Dependencies:** DAG is valid, T1 has no blockers
- [ ] **Specificity:** No vague criteria ("works", "is good", "handles errors")
- [ ] **File paths:** Actual paths used, not placeholders

## Output

Save to: `plans/pending/{feature-name}.md`

Update spec: Add `**Plan:** [/plans/pending/{feature-name}.md](/plans/pending/{feature-name}.md)`

The plan is now ready. Move to `plans/current/` when starting work.
