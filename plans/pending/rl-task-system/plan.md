# Plan: rl Task Management System

**Spec:** Minimal Linear clone for git-native task/plan management
**Created:** 2026-02-04
**Status:** pending

## Context

Build `rl` - a minimal, git-native task management system that replaces external tools like Linear for Ralph's workflow. All data lives in the repo, syncs via git push/pull, and is designed for both human and AI agent use.

### Design Principles

1. **Git-native**: All state in text files, distributed via git
2. **Agent-first**: JSON output, structured data, idempotent operations
3. **Single source of truth**: No sync conflicts between systems
4. **Human-friendly**: YAML/Markdown readable and editable by hand
5. **Minimal**: Only what Ralph needs, nothing more

### Architecture

```
ralph/
├── cmd/
│   ├── ralph/main.go       # existing
│   └── rl/main.go          # new binary (thin wrapper)
├── internal/
│   ├── rl/                 # rl core (zero ralph dependencies)
│   │   ├── plan/           # plan CRUD, state machine
│   │   ├── task/           # task CRUD, dependencies
│   │   ├── event/          # timeline, event log
│   │   ├── discussion/     # comments, feedback
│   │   ├── store/          # file I/O, atomic writes
│   │   └── cmd/            # cobra CLI commands
│   └── ...                 # existing ralph packages
```

### Data Model

```
.rl/
├── config.yaml             # workspace settings
└── plans/
    └── auth-flow/
        ├── plan.yaml       # metadata + tasks (structured)
        ├── spec.md         # human-written spec (optional)
        └── events.jsonl    # append-only timeline
```

**Example plan.yaml:**
```yaml
id: auth-flow
title: OAuth2 Authentication Flow
status: active
branch: feat/auth-flow
created_at: 2026-02-04T08:00:00Z
assignee: claude

tasks:
  - id: T1
    title: Add OAuth2 client library
    description: |
      Install and configure the OAuth2 client library.
      Must support PKCE flow for security.
    status: done
    requires: []
    criteria:
      - text: Library installed and importable
        done: true
        done_at: 2026-02-04T10:00:00Z
      - text: PKCE flow configured
        done: true
        done_at: 2026-02-04T10:15:00Z
      - text: Unit tests pass
        done: true
        done_at: 2026-02-04T10:30:00Z
    done_at: 2026-02-04T10:30:00Z
    commits: [abc123, def456]

  - id: T2
    title: Create auth middleware
    description: Middleware to validate JWT tokens on protected routes.
    status: in_progress
    requires: [T1]
    criteria:
      - text: Validates JWT signature
        done: true
        done_at: 2026-02-04T11:00:00Z
      - text: Handles token refresh
        done: false
      - text: Returns proper 401 errors
        done: false
      - text: Supports session-based auth fallback
        skipped: true
        skip_reason: "Decided to use JWT-only, no session fallback needed"
        skipped_at: 2026-02-04T11:30:00Z
      - text: Integration tests pass
        done: false

  - id: T3
    title: Add login/logout routes
    status: pending
    requires: [T2]
    criteria:
      - text: POST /login initiates OAuth flow
        done: false
      - text: GET /callback handles redirect
        done: false
      - text: POST /logout clears session
        done: false

discussion:
  - id: c1
    author: human
    message: Use PKCE flow instead of implicit grant
    created_at: 2026-02-04T09:00:00Z
    resolved: true
    resolved_at: 2026-02-04T10:00:00Z
```

### Gotchas

- `internal/rl/` must have zero imports from `internal/cli/`, `internal/runner/`, etc. to enable future extraction
- Keep backwards compatibility: Ralph's existing plan bundles should still work
- Events.jsonl must be append-only for conflict-free git merges
- YAML anchors/aliases can cause issues - keep plan.yaml simple

---

## Rules

1. **Pick task:** First task where status is not `complete` and all `Requires` are `complete`
2. **Subtasks are sequential.** Complete 1 before 2.
3. **Task complete when:** All "Done when" checked and verified, then set Status: `complete`
4. **Update file after each checkbox.**
5. **New work found?** Add to Discovered section, continue current task.

---

## Tasks

### T1: Create rl package structure
> Set up the internal/rl/ package hierarchy with proper boundaries

**Requires:** —
**Status:** pending

**Done when:**
- [ ] `internal/rl/` directory exists with subpackages
- [ ] Each subpackage has a doc.go with package documentation
- [ ] No imports from other internal packages (except shared utilities)
- [ ] `go build ./internal/rl/...` succeeds

**Subtasks:**
1. [ ] Create directory structure: `internal/rl/{plan,task,event,discussion,store,cmd}/`
2. [ ] Add doc.go to each package explaining its purpose
3. [ ] Create `internal/rl/rl.go` with package-level documentation
4. [ ] Verify build succeeds with `go build ./internal/rl/...`

---

### T2: Implement store package (file I/O)
> Low-level file operations: atomic writes, YAML/JSON parsing, locking

**Requires:** T1
**Status:** pending

**Done when:**
- [ ] `AtomicWrite(path, content)` writes via temp file + rename
- [ ] `ReadYAML(path, v)` and `WriteYAML(path, v)` work correctly
- [ ] `AppendJSONL(path, v)` appends single JSON line
- [ ] `ReadJSONL(path) []json.RawMessage` reads all lines
- [ ] File locking prevents concurrent writes (advisory)

**Subtasks:**
1. [ ] Create `internal/rl/store/atomic.go` with atomic write function
2. [ ] Create `internal/rl/store/yaml.go` with YAML read/write helpers
3. [ ] Create `internal/rl/store/jsonl.go` with JSONL append/read helpers
4. [ ] Create `internal/rl/store/lock.go` with file locking (flock)
5. [ ] Add comprehensive tests in `internal/rl/store/store_test.go`

---

### T3: Define core data types
> Plan, Task, Event, Comment structs with YAML/JSON tags

**Requires:** T1
**Status:** pending

**Done when:**
- [ ] `Plan` struct with metadata, tasks slice, state, spec
- [ ] `Task` struct with full fields (see below)
- [ ] `Criterion` struct for acceptance criteria with done flag
- [ ] `Event` struct with timestamp, type, and type-specific data
- [ ] `Comment` struct with id, author, message, resolved flag
- [ ] All types marshal/unmarshal correctly to YAML/JSON

Task struct fields:
```go
type Task struct {
    ID          string      `yaml:"id"`           // T1, T2, etc.
    Title       string      `yaml:"title"`        // Short description
    Description string      `yaml:"description"`  // Detailed spec (markdown)
    Status      TaskStatus  `yaml:"status"`       // pending, in_progress, done, skipped
    Requires    []string    `yaml:"requires"`     // Task IDs this depends on
    Criteria    []Criterion `yaml:"criteria"`     // Acceptance criteria (must all pass)
    Assignee    string      `yaml:"assignee"`     // Optional owner
    CreatedAt   time.Time   `yaml:"created_at"`
    DoneAt      *time.Time  `yaml:"done_at"`      // When completed
    Commits     []string    `yaml:"commits"`      // Linked commit SHAs
}

type Criterion struct {
    Text      string     `yaml:"text"`       // "All tests pass"
    Done      bool       `yaml:"done"`       // Checkbox state
    DoneAt    *time.Time `yaml:"done_at"`    // When checked
    Skipped   bool       `yaml:"skipped"`    // If criterion is N/A or unfulfillable
    SkipReason string    `yaml:"skip_reason"` // Why it was skipped
    SkippedAt *time.Time `yaml:"skipped_at"` // When skipped
}
```

**Subtasks:**
1. [ ] Create `internal/rl/plan/types.go` with Plan and PlanState structs
2. [ ] Create `internal/rl/task/types.go` with Task, TaskStatus, Criterion types
3. [ ] Create `internal/rl/event/types.go` with Event and EventType enum
4. [ ] Create `internal/rl/discussion/types.go` with Comment struct
5. [ ] Add tests verifying YAML/JSON round-trip for all types

---

### T4: Implement plan package (CRUD)
> Create, read, update, delete plans with state machine

**Requires:** T2, T3
**Status:** pending

**Done when:**
- [ ] `Create(dir, name, spec)` creates plan bundle with all files
- [ ] `Load(dir)` loads plan from directory
- [ ] `Save(plan)` persists plan to disk atomically
- [ ] `List(rootDir)` returns all plans with their status
- [ ] `Delete(plan)` removes plan directory
- [ ] Status transitions validated: draft→ready→active→blocked→complete

**Subtasks:**
1. [ ] Create `internal/rl/plan/plan.go` with Plan methods
2. [ ] Implement `Create()` with bundle scaffolding (plan.yaml, events.jsonl)
3. [ ] Implement `Load()` with YAML parsing and validation
4. [ ] Implement `Save()` using atomic write
5. [ ] Implement `List()` scanning .rl/plans/ directory
6. [ ] Implement `Delete()` with confirmation requirement
7. [ ] Add state machine validation in `SetStatus()`
8. [ ] Add tests in `internal/rl/plan/plan_test.go`

---

### T5: Implement task package (CRUD + dependencies + criteria)
> Task operations within plans, including dependency and acceptance criteria management

**Requires:** T3, T4
**Status:** pending

**Done when:**
- [ ] `Add(plan, title, opts)` adds task to plan with optional criteria
- [ ] `Get(plan, taskID)` returns task by ID
- [ ] `Update(plan, taskID, updates)` modifies task fields
- [ ] `Done(plan, taskID)` marks task complete with timestamp
- [ ] `Link(plan, taskID, requires)` sets dependencies
- [ ] `Next(plan)` returns next actionable task (respects dependencies)
- [ ] `AddCriterion(plan, taskID, text)` adds acceptance criterion
- [ ] `CheckCriterion(plan, taskID, index)` marks criterion done
- [ ] `UncheckCriterion(plan, taskID, index)` marks criterion not done
- [ ] `SkipCriterion(plan, taskID, index, reason)` marks criterion as skipped/N/A
- [ ] `EditCriterion(plan, taskID, index, newText)` updates criterion text
- [ ] `DeleteCriterion(plan, taskID, index)` removes criterion (with confirmation)
- [ ] `Progress(task)` returns criteria completion (e.g., "3/5", excludes skipped)
- [ ] Task can be marked Done when all criteria are checked OR skipped
- [ ] Circular dependency detection works

**Subtasks:**
1. [ ] Create `internal/rl/task/task.go` with task operations
2. [ ] Implement `Add()` with auto-generated task ID and initial criteria
3. [ ] Implement `Get()` with task lookup by ID
4. [ ] Implement `Update()` for title, description, status changes
5. [ ] Implement `Done()` with criteria validation (all must be checked)
6. [ ] Implement `Link()` for dependency management
7. [ ] Implement `Next()` using topological sort of dependencies
8. [ ] Implement `AddCriterion()`, `CheckCriterion()`, `UncheckCriterion()`
9. [ ] Implement `SkipCriterion()` with reason tracking
10. [ ] Implement `EditCriterion()` and `DeleteCriterion()`
11. [ ] Implement `Progress()` returning completion stats (excludes skipped)
12. [ ] Update `Done()` to accept skipped criteria as fulfilled
13. [ ] Add cycle detection in dependency validation
14. [ ] Add tests in `internal/rl/task/task_test.go`

---

### T6: Implement event package (timeline)
> Append-only event log for full history

**Requires:** T2, T3
**Status:** pending

**Done when:**
- [ ] `Append(planDir, event)` adds event to events.jsonl
- [ ] `List(planDir)` returns all events in order
- [ ] `ListByType(planDir, eventType)` filters by type
- [ ] `Since(planDir, timestamp)` returns events after time
- [ ] Events include: plan.created, plan.activated, task.done, blocked, unblocked, comment.added, iteration.start, iteration.end

**Subtasks:**
1. [ ] Create `internal/rl/event/event.go` with event operations
2. [ ] Define EventType constants for all event types
3. [ ] Implement `Append()` using store.AppendJSONL
4. [ ] Implement `List()` reading all events
5. [ ] Implement `ListByType()` with filtering
6. [ ] Implement `Since()` with timestamp comparison
7. [ ] Add helper constructors: `NewTaskDoneEvent()`, `NewBlockedEvent()`, etc.
8. [ ] Add tests in `internal/rl/event/event_test.go`

---

### T7: Implement discussion package (comments)
> Comments and feedback with threading

**Requires:** T3, T6
**Status:** pending

**Done when:**
- [ ] `Add(plan, message, opts)` adds comment (plan or task level)
- [ ] `List(plan)` returns all comments
- [ ] `ListUnresolved(plan)` returns pending comments
- [ ] `Resolve(plan, commentID)` marks comment processed
- [ ] `Reply(plan, commentID, message)` adds threaded reply
- [ ] Comments stored in plan.yaml under discussion field

**Subtasks:**
1. [ ] Create `internal/rl/discussion/discussion.go`
2. [ ] Implement `Add()` with auto-generated comment ID
3. [ ] Implement `List()` and `ListUnresolved()` with filtering
4. [ ] Implement `Resolve()` setting resolved flag + timestamp
5. [ ] Implement `Reply()` for threaded comments
6. [ ] Emit events on comment add/resolve
7. [ ] Add tests in `internal/rl/discussion/discussion_test.go`

---

### T8: Implement CLI foundation
> Cobra command structure with JSON output support

**Requires:** T1
**Status:** pending

**Done when:**
- [ ] `cmd/rl/main.go` initializes cobra root command
- [ ] `--output=json` flag works globally for machine-readable output
- [ ] `--dir` flag specifies workspace root (defaults to git root)
- [ ] Help text is clear and follows conventions
- [ ] Version command shows build info

**Subtasks:**
1. [ ] Create `internal/rl/cmd/root.go` with root command
2. [ ] Add `--output` persistent flag (text, json)
3. [ ] Add `--dir` persistent flag with git root detection
4. [ ] Create `internal/rl/cmd/output.go` for formatted output helpers
5. [ ] Create `cmd/rl/main.go` that calls cmd.Execute()
6. [ ] Add version command with build info injection
7. [ ] Update Makefile to build rl binary
8. [ ] Update .goreleaser.yaml to include rl binary

---

### T9: Implement plan CLI commands
> rl plan create/list/show/start/complete commands

**Requires:** T4, T8
**Status:** pending

**Done when:**
- [ ] `rl plan create <name>` creates plan in .rl/plans/
- [ ] `rl plan list` shows all plans with status
- [ ] `rl plan show <name>` displays plan details
- [ ] `rl plan start <name>` activates plan (draft→active)
- [ ] `rl plan complete <name>` marks plan done
- [ ] `rl plan delete <name>` removes plan (with confirmation)
- [ ] All commands support --output=json

**Subtasks:**
1. [ ] Create `internal/rl/cmd/plan.go` with plan subcommand
2. [ ] Implement `plan create` calling plan.Create()
3. [ ] Implement `plan list` with table/json output
4. [ ] Implement `plan show` with detailed view
5. [ ] Implement `plan start` with status transition
6. [ ] Implement `plan complete` with status transition
7. [ ] Implement `plan delete` with --force flag
8. [ ] Add tests for CLI commands

---

### T10: Implement task CLI commands
> rl task add/list/done/start/criteria commands

**Requires:** T5, T8
**Status:** pending

**Done when:**
- [ ] `rl task add <title>` adds task to current plan
- [ ] `rl task add <title> --plan=<name>` adds to specific plan
- [ ] `rl task add <title> --criteria="Tests pass,Docs updated"` adds with criteria
- [ ] `rl task list` shows tasks (filterable by status, plan)
- [ ] `rl task show <id>` displays task with criteria status
- [ ] `rl task done <id>` marks task complete (fails if criteria unchecked)
- [ ] `rl task start <id>` marks task in_progress
- [ ] `rl task link <id> --requires=<ids>` sets dependencies
- [ ] `rl task next` shows next actionable task
- [ ] `rl task criteria add <id> <text>` adds acceptance criterion
- [ ] `rl task criteria check <id> <index>` marks criterion done
- [ ] `rl task criteria uncheck <id> <index>` marks criterion not done
- [ ] `rl task criteria skip <id> <index> --reason="..."` marks as N/A with reason
- [ ] `rl task criteria edit <id> <index> <new-text>` updates criterion text
- [ ] `rl task criteria delete <id> <index>` removes criterion (requires --force)
- [ ] `rl task criteria list <id>` shows criteria with status (done/skipped/pending)
- [ ] All commands support --output=json

**Subtasks:**
1. [ ] Create `internal/rl/cmd/task.go` with task subcommand
2. [ ] Implement `task add` with --plan, --criteria flags
3. [ ] Implement `task list` with --status, --plan filters
4. [ ] Implement `task show` with criteria display
5. [ ] Implement `task done` with criteria validation
6. [ ] Implement `task start` for status change
7. [ ] Implement `task link` for dependencies
8. [ ] Implement `task next` using task.Next()
9. [ ] Create `internal/rl/cmd/criteria.go` as task subcommand
10. [ ] Implement `task criteria add/check/uncheck/list`
11. [ ] Implement `task criteria skip` with --reason flag
12. [ ] Implement `task criteria edit` for text updates
13. [ ] Implement `task criteria delete` with --force flag
14. [ ] Add tests for CLI commands

---

### T11: Implement feedback CLI commands
> rl comment/block/unblock commands

**Requires:** T7, T8
**Status:** pending

**Done when:**
- [ ] `rl comment <message>` adds comment to current plan
- [ ] `rl comment <message> --task=<id>` adds to specific task
- [ ] `rl comment list` shows comments (--unresolved filter)
- [ ] `rl comment resolve <id>` marks comment processed
- [ ] `rl block <reason>` marks current plan as blocked
- [ ] `rl unblock <resolution>` clears blocked status
- [ ] All commands support --output=json

**Subtasks:**
1. [ ] Create `internal/rl/cmd/comment.go` with comment subcommand
2. [ ] Implement `comment` (add) with --task flag
3. [ ] Implement `comment list` with --unresolved filter
4. [ ] Implement `comment resolve` calling discussion.Resolve()
5. [ ] Create `internal/rl/cmd/block.go` with block/unblock commands
6. [ ] Implement `block` setting plan status + adding event
7. [ ] Implement `unblock` clearing status + adding event
8. [ ] Add tests for CLI commands

---

### T12: Implement status and dashboard commands
> Quick overview commands for current state

**Requires:** T9, T10
**Status:** pending

**Done when:**
- [ ] `rl` (no args) shows dashboard: current plan, progress, blockers
- [ ] `rl status` same as above
- [ ] `rl timeline` shows recent events across all plans
- [ ] `rl board` shows kanban-style view by status
- [ ] Dashboard shows progress bar and task counts

**Subtasks:**
1. [ ] Create `internal/rl/cmd/status.go` with status command
2. [ ] Implement dashboard view with current plan summary
3. [ ] Add progress bar rendering (reuse from ralph or create shared)
4. [ ] Implement `timeline` command reading events.jsonl
5. [ ] Implement `board` command grouping tasks by status
6. [ ] Make root command show dashboard when no args
7. [ ] Add tests for output formatting

---

### T13: Implement context command for agents
> Machine-readable context dump for AI agent prompts

**Requires:** T9, T10, T11
**Status:** pending

**Done when:**
- [ ] `rl context` outputs JSON with current plan, tasks, feedback
- [ ] Context includes: plan metadata, all tasks with status, unresolved comments, blockers
- [ ] Output is stable (sorted keys, consistent ordering)
- [ ] Can be piped directly into agent prompt

**Subtasks:**
1. [ ] Create `internal/rl/cmd/context.go`
2. [ ] Define Context struct with all relevant fields
3. [ ] Implement context gathering from current plan
4. [ ] Ensure JSON output is stable/deterministic
5. [ ] Add tests verifying context structure

---

### T14: Implement batch command for agents
> Execute multiple commands atomically

**Requires:** T9, T10, T11
**Status:** pending

**Done when:**
- [ ] `rl batch` reads commands from stdin
- [ ] Commands executed in order, stopping on first error
- [ ] `--continue-on-error` flag continues despite errors
- [ ] Output shows result of each command
- [ ] Useful for agent multi-step operations

**Subtasks:**
1. [ ] Create `internal/rl/cmd/batch.go`
2. [ ] Implement command parsing from stdin
3. [ ] Execute commands sequentially
4. [ ] Add --continue-on-error flag
5. [ ] Output results in structured format
6. [ ] Add tests for batch execution

---

### T15: Integrate rl with Ralph runner
> Ralph uses rl for task management instead of checkbox parsing

**Requires:** T13, T14
**Status:** pending

**Done when:**
- [ ] Runner calls `rl context --json` for agent context
- [ ] Runner calls `rl task done <id>` when task completed
- [ ] Runner calls `rl block` when blocker detected
- [ ] Runner calls `rl comment` for feedback
- [ ] Existing ralph plan bundles can be imported to rl format

**Subtasks:**
1. [ ] Create `internal/runner/rl.go` with rl integration helpers
2. [ ] Update `internal/runner/loop.go` to use rl for context
3. [ ] Update task completion detection to call rl
4. [ ] Update blocker handling to call rl block
5. [ ] Add migration function: ralph plan bundle → rl plan
6. [ ] Add integration tests

---

### T16: Add configuration support
> .rl/config.yaml for workspace settings

**Requires:** T4
**Status:** pending

**Done when:**
- [ ] `.rl/config.yaml` defines workspace settings
- [ ] Custom statuses can be defined
- [ ] Default plan template configurable
- [ ] Notification settings (for future webhook support)

**Subtasks:**
1. [ ] Create `internal/rl/config/config.go`
2. [ ] Define Config struct with all settings
3. [ ] Implement `Load()` with defaults
4. [ ] Support custom task statuses
5. [ ] Support custom plan templates
6. [ ] Add `rl init` command to create config
7. [ ] Add tests for config loading

---

### T17: Add git integration helpers
> Branch creation, commit linking, PR support

**Requires:** T4, T6
**Status:** pending

**Done when:**
- [ ] `rl plan start` creates git branch `feat/<plan-name>`
- [ ] Commits with `[T1]` in message auto-link to task
- [ ] `rl commits <plan>` shows commits for plan
- [ ] `rl pr <plan>` creates PR via gh CLI

**Subtasks:**
1. [ ] Create `internal/rl/git/git.go` with git helpers
2. [ ] Implement branch creation on plan start
3. [ ] Implement commit message parsing for task links
4. [ ] Implement `commits` command showing plan commits
5. [ ] Implement `pr` command wrapping gh CLI
6. [ ] Add events for git operations
7. [ ] Add tests (mocking git commands)

---

### T18: Add search and filter capabilities
> Query tasks and plans efficiently

**Requires:** T4, T5
**Status:** pending

**Done when:**
- [ ] `rl search <query>` searches across plans and tasks
- [ ] `rl task list --filter="status=pending AND plan=auth"` works
- [ ] Filter syntax is simple and documented
- [ ] Search is fast (no external dependencies)

**Subtasks:**
1. [ ] Create `internal/rl/query/query.go` with query parsing
2. [ ] Implement simple filter syntax parser
3. [ ] Add search command
4. [ ] Add --filter flag to list commands
5. [ ] Document filter syntax in help text
6. [ ] Add tests for query parsing and filtering

---

### T19: Documentation and examples
> README, help text, example workflows

**Requires:** T9, T10, T11, T12
**Status:** pending

**Done when:**
- [ ] README.md in internal/rl/ explains the system
- [ ] All commands have clear --help text
- [ ] Example workflow documented (create plan → work → complete)
- [ ] Agent integration documented
- [ ] Migration from Linear documented

**Subtasks:**
1. [ ] Write internal/rl/README.md with overview
2. [ ] Review and improve all command help text
3. [ ] Add examples/ directory with sample plans
4. [ ] Document agent workflow (context → batch → done)
5. [ ] Document data format (plan.yaml, events.jsonl)

---

### T20: Testing and polish
> Comprehensive tests, edge cases, error messages

**Requires:** T1-T19
**Status:** pending

**Done when:**
- [ ] Unit test coverage > 80% for internal/rl/
- [ ] Integration tests for CLI commands
- [ ] Error messages are clear and actionable
- [ ] Edge cases handled (empty plans, missing files, etc.)
- [ ] Performance acceptable for repos with 100+ plans

**Subtasks:**
1. [ ] Add missing unit tests across all packages
2. [ ] Create integration test suite for CLI
3. [ ] Review and improve error messages
4. [ ] Test with large number of plans (performance)
5. [ ] Test concurrent access scenarios
6. [ ] Fix any issues found during testing

---

## Discovered

<!-- Tasks found during implementation -->

---

## Completed

<!-- Completion dates will be added here -->
