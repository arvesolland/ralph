# Plan File Specification

Plans are markdown files that define tasks for Ralph to implement.

## Required Structure

```markdown
# [Plan Title]

## Overview
Brief description of what this plan accomplishes.

## Tasks

- [ ] Task 1 title
- [ ] Task 2 title
- [ ] Task 3 title
```

## Task Format

Each task MUST:
- Start with `- [ ]` (unchecked checkbox)
- Be a single, atomic unit of work (one commit)
- Be completable in one iteration

### Good Tasks
```markdown
- [ ] Create User model with email and password fields
- [ ] Add POST /api/register endpoint
- [ ] Add input validation for registration
- [ ] Write tests for registration endpoint
```

### Bad Tasks
```markdown
- [ ] Build the authentication system          # Too vague
- [ ] Add login, logout, and password reset    # Multiple things
- [ ] Make it work                             # Not actionable
```

## Optional Sections

### Context
```markdown
## Context
Background information Claude needs to understand the task.
- Why this feature is needed
- Related existing code
- Constraints or requirements
```

### Acceptance Criteria
```markdown
## Acceptance Criteria
- [ ] Users can register with email/password
- [ ] Passwords are hashed before storage
- [ ] Duplicate emails are rejected with 409
- [ ] Tests cover happy path and edge cases
```

### References
```markdown
## References
- Related spec: `specs/features/auth.md`
- Similar implementation: `app/Services/UserService.php`
- API design: `docs/api.md#authentication`
```

## Task Completion

When Claude completes a task, it marks it done:
```markdown
- [x] Create User model with email and password fields
- [ ] Add POST /api/register endpoint
```

When ALL tasks are complete, Claude outputs:
```
<promise>COMPLETE</promise>
```

## Example Plan

```markdown
# User Registration Feature

## Overview
Add user registration functionality to the API.

## Context
- Using Laravel with Sanctum for auth
- Users table already exists but needs password field
- Follow patterns in existing AuthController

## Tasks

- [ ] Add password field to users migration
- [ ] Create RegisterRequest with validation rules
- [ ] Add register method to AuthController
- [ ] Return Sanctum token on successful registration
- [ ] Write feature tests for registration

## Acceptance Criteria
- [ ] POST /api/register accepts email + password
- [ ] Returns 201 with user data and token
- [ ] Returns 422 for validation errors
- [ ] Returns 409 for duplicate email
- [ ] Password is hashed (not stored plain)

## References
- Auth patterns: `app/Http/Controllers/AuthController.php`
- Validation: `app/Http/Requests/LoginRequest.php`
```

## Plan File Location

Plans can be anywhere, but for queue workflow:
```
.ralph/plans/
├── pending/    # Waiting to be processed
├── current/    # Active plan (0-1 files)
└── completed/  # Finished with logs
```
