# Plan: Structured Task State (Agent Control Plane v1)

**Created:** 2026-02-06
**Status:** pending

## Context

Replace Ralph's markdown-checkbox-based task management with a structured YAML state machine (`state.yaml`) inside plan bundles. This gives agents deterministic task selection, criteria-gated completion, scoped feedback, and a rich JSON context payload every iteration — without rebuilding existing infrastructure.

### What exists today

- Plans are markdown files with `[ ]`/`[x]` checkboxes parsed by `internal/plan/task.go`
- Completion verified by counting unchecked boxes + Opus LLM verification (`internal/runner/verify.go`)
- Context passed to agent is thin: `context.json` has paths, branch, iteration count only
- Feedback is a flat `feedback.md` with unstructured markdown sections
- Agent figures out "what's next" by reading the plan markdown
- Queue management (`pending/current/complete/`) is solid and stays as-is

### What changes

- `state.yaml` added to plan bundles as machine-owned runtime state
- New `internal/state/` package for types, load/save, task selection
- `ralph context <plan> --json` outputs everything the agent needs in one structured payload
- Task lifecycle: agent claims tasks, checks criteria, completes when all criteria met
- Feedback becomes structured with scope (plan-level or task-scoped)
- Verification becomes criteria-gated (cheaper + more reliable than LLM verification)
- Prompt system updated to inject structured context

### What stays the same

- `plans/pending|current|complete/` queue (unchanged)
- `plan.md` as human-readable spec (unchanged, but agent stops updating checkboxes in it)
- `progress.md` for iteration log (unchanged)
- Worktree isolation (unchanged)
- Worker loop, Slack notifications, git operations (unchanged)
- `context.json` still exists (for iteration count, paths) but supplemented by structured context

### Key design decisions

- `state.yaml` lives inside plan bundles alongside `plan.md` — no new `.rlc/` directory
- Agent gets full plan state every iteration (no hidden state)
- `rl context` returns `suggested_next` + `available_tasks` — agent chooses within guardrails
- Task IDs are `T1`, `T2`, etc. — stable, monotonic
- Feedback IDs are `F1`, `F2`, etc.
- Criteria are 1-indexed for human readability
- No leases/locks for v1 (existing three-layer concurrency lock is sufficient)
- No events.jsonl for v1 (git commit log serves as audit trail)
- LLM verification kept as optional safety net, but criteria gate is primary

### Gotchas

- `state.yaml` must be synced to/from worktrees like other bundle files
- Atomic writes (temp + rename) already used for `context.json` — reuse pattern
- Plan bundles that lack `state.yaml` should work as before (backward compat)
- The prompt system uses `{{PLACEHOLDER}}` substitution — new `{{CONTEXT_JSON}}` placeholder needed
- Task selection must handle edge cases: all tasks done, circular deps, no eligible tasks

---

## Rules

1. **Pick task:** First task where status ≠ `complete` and all `Requires` are `complete`
2. **Subtasks are sequential.** Complete 1 before 2.
3. **Task complete when:** All "Done when" + all subtasks checked → set Status: `complete`
4. **Update file after each checkbox.**
5. **New work found?** Add to Discovered section, continue current task.

---

## Tasks

### T1: Define state types and YAML serialization
> Foundation types that everything else builds on — must be right before building operations

**Requires:** —
**Status:** complete

**Done when:**
- [x] `PlanState` struct with ID, Title, Status, Tasks, Feedback fields
- [x] `TaskState` struct with ID, Title, Status, Requires, Criteria, Notes, Artifacts fields
- [x] `Criterion` struct with Text, Done, DoneAt fields
- [x] `Feedback` struct with ID, Scope, Author, Message, Resolved, CreatedAt fields
- [x] `PlanStatus` and `TaskStatus` enums with valid values
- [x] YAML round-trip tests pass (marshal → unmarshal → compare)
- [x] JSON marshaling works for context output
- [x] `go test ./internal/state/...` passes

**Subtasks:**
1. [x] Create `internal/state/` package directory
2. [x] Create `internal/state/types.go` with all struct definitions
   - `PlanStatus`: draft, ready, active, blocked, complete
   - `TaskStatus`: todo, claimed, doing, blocked, done, skipped
   - `PlanState`: id, title, status, created_at, tasks, feedback
   - `TaskState`: id, title, description, status, requires, criteria, notes, artifacts (commits, files_touched), started_at, done_at
   - `Criterion`: text, done, done_at
   - `Feedback`: id, scope (plan or task:Tn), author, message, resolved, resolved_at, created_at
3. [x] Add YAML struct tags to all fields
4. [x] Add JSON struct tags to all fields (for context output)
5. [x] Create `internal/state/types_test.go` with round-trip tests
6. [x] Add `PlanStatus.IsValid()` and `TaskStatus.IsValid()` validation methods

---

### T2: Implement state load/save with atomic writes
> Safe persistence of state.yaml — foundation for all mutations

**Requires:** T1
**Status:** complete

**Done when:**
- [x] `LoadState(bundleDir string) (*PlanState, error)` reads and parses state.yaml
- [x] `SaveState(state *PlanState, bundleDir string) error` writes atomically (temp + rename)
- [x] Missing state.yaml returns nil state (not an error) for backward compat
- [x] Invalid YAML returns clear parse error
- [x] Concurrent writes don't corrupt file (temp + rename pattern)
- [x] `go test ./internal/state/...` passes

**Subtasks:**
1. [x] Create `internal/state/store.go`
2. [x] Implement `LoadState()` — read `{bundleDir}/state.yaml`, unmarshal, validate
3. [x] Implement `SaveState()` — marshal, write to `.tmp`, rename (reuse pattern from `context.go`)
4. [x] Implement `StatePath(bundleDir string) string` helper
5. [x] Handle missing file case: return `nil, nil` (plan has no structured state yet)
6. [x] Create `internal/state/store_test.go` — test load, save, missing file, invalid yaml, atomic write

---

### T3: Implement state validation rules
> Enforce valid transitions and data integrity — prevents bad state from agent mistakes

**Requires:** T1
**Status:** complete

**Done when:**
- [x] Plan status transitions validated (draft→ready→active→complete, active↔blocked)
- [x] Task status transitions validated (todo→claimed→doing→done, doing↔blocked, any→skipped)
- [x] Dependency validation: no missing task IDs, no cycles
- [x] Criteria gate: task cannot become `done` unless all criteria are checked
- [x] Feedback scope validation: must be "plan" or "task:Tn" where Tn exists
- [x] Errors are descriptive strings (not codes — keep it simple for v1)
- [x] `go test ./internal/state/...` passes

**Subtasks:**
1. [x] Create `internal/state/validate.go`
2. [x] Implement `ValidatePlanTransition(from, to PlanStatus) error`
3. [x] Implement `ValidateTaskTransition(from, to TaskStatus) error`
4. [x] Implement `ValidateDependencies(tasks []TaskState) error` — check for missing IDs and cycles
5. [x] Implement `ValidateCompletion(task *TaskState) error` — check all criteria done
6. [x] Implement `ValidateFeedbackScope(scope string, tasks []TaskState) error`
7. [x] Create `internal/state/validate_test.go` — test all valid and invalid transitions, cycle detection, criteria gate

---

### T4: Implement task selection algorithm
> Deterministic eligibility computation — the brain of the control plane

**Requires:** T1, T3
**Status:** complete

**Done when:**
- [x] `Selection` struct with SuggestedNext, Available, Blocked fields
- [x] `ComputeSelection(state *PlanState) *Selection` returns correct task sets
- [x] Available: status=todo, all requires are done, not blocked
- [x] Blocked: status=blocked OR has unmet dependencies (with reasons)
- [x] SuggestedNext: first available by numeric ID order
- [x] Each pick includes a reason string explaining why it's available/blocked
- [x] Returns nil suggested_next when no tasks eligible (all done or all blocked)
- [x] `go test ./internal/state/...` passes

**Subtasks:**
1. [x] Create `internal/state/selection.go`
2. [x] Define `Selection` struct with `SuggestedNext *TaskPick`, `Available []TaskPick`, `Blocked []TaskPick`
3. [x] Define `TaskPick` struct with `TaskID string`, `Reason string`
4. [x] Implement `ComputeSelection()` — iterate tasks, classify into available/blocked
5. [x] Implement numeric ID sorting (parse `T12` → 12 for stable ordering)
6. [x] Create `internal/state/selection_test.go` — test happy path, all done, all blocked, dep chains, parallel tasks

---

### T5: Implement task mutation operations
> The API agents use to update state — claim, check criteria, complete

**Requires:** T2, T3
**Status:** blocked

**Done when:**
- [ ] `AddTask(state, title, requires, criteria) (*TaskState, error)` — auto-assigns next T{n} ID
- [ ] `ClaimTask(state, taskID) error` — sets status to doing (no leases in v1)
- [ ] `CheckCriterion(state, taskID, index) error` — 1-indexed, sets done=true + done_at
- [ ] `UncheckCriterion(state, taskID, index) error` — sets done=false, clears done_at
- [ ] `CompleteTask(state, taskID, commits, filesTouched) error` — validates criteria gate, sets done
- [ ] `SkipTask(state, taskID, reason) error` — sets skipped with note
- [ ] `SetPlanStatus(state, status, reason) error` — validates transition
- [ ] All mutations validate before applying
- [ ] `go test ./internal/state/...` passes

**Subtasks:**
1. [ ] Create `internal/state/mutations.go`
2. [ ] Implement `nextTaskID(state)` — scans existing IDs, returns next T{n}
3. [ ] Implement `findTask(state, taskID)` — returns pointer to task or error
4. [ ] Implement `AddTask()` with auto-ID and criteria parsing
5. [ ] Implement `ClaimTask()` — validate status=todo, deps met, then set doing
6. [ ] Implement `CheckCriterion()` / `UncheckCriterion()` — 1-indexed, bounds check
7. [ ] Implement `CompleteTask()` — validate all criteria checked, set done + done_at + artifacts
8. [ ] Implement `SkipTask()` — set skipped + add reason to notes
9. [ ] Implement `SetPlanStatus()` — validate transition
10. [ ] Create `internal/state/mutations_test.go` — test each operation including error cases

---

### T6: Implement feedback operations
> Structured, scoped feedback replaces flat feedback.md for machine state

**Requires:** T2, T3
**Status:** blocked

**Done when:**
- [ ] `AddFeedback(state, scope, author, message) (*Feedback, error)` — auto-assigns F{n} ID
- [ ] `ResolveFeedback(state, feedbackID) error` — sets resolved=true + resolved_at
- [ ] `UnresolvedFeedback(state) []Feedback` — returns all unresolved
- [ ] `FeedbackForTask(state, taskID) []Feedback` — returns scoped feedback
- [ ] Scope validation enforced on add
- [ ] `go test ./internal/state/...` passes

**Subtasks:**
1. [ ] Create `internal/state/feedback.go`
2. [ ] Implement `nextFeedbackID(state)` — scans existing IDs, returns next F{n}
3. [ ] Implement `AddFeedback()` — validate scope, create with timestamp
4. [ ] Implement `ResolveFeedback()` — find by ID, set resolved
5. [ ] Implement `UnresolvedFeedback()` — filter where resolved=false
6. [ ] Implement `FeedbackForTask()` — filter by scope="task:Tn"
7. [ ] Create `internal/state/feedback_test.go`

---

### T7: Build context payload and `ralph context` command
> The golden command — everything the agent needs in one JSON payload

**Requires:** T4, T5, T6
**Status:** blocked

**Done when:**
- [ ] `ContextPayload` struct defined with plan, tasks, feedback, selection, summary fields
- [ ] `BuildContext(state *PlanState, ctx *runner.Context) *ContextPayload` assembles full payload
- [ ] `ralph context <plan-path> --json` outputs valid JSON to stdout
- [ ] `ralph context <plan-path>` outputs human-readable summary to stdout
- [ ] Payload includes: plan metadata, all tasks with criteria, unresolved feedback, selection (suggested + available + blocked), summary stats
- [ ] Output is deterministic: tasks sorted by ID, feedback sorted by created_at
- [ ] Plans without state.yaml get a useful error or empty payload
- [ ] `go test ./internal/state/...` and `go test ./internal/cli/...` pass

**Subtasks:**
1. [ ] Create `internal/state/context.go` with `ContextPayload` struct
2. [ ] Define nested structs: `PayloadPlan`, `PayloadTasks` (with summary), `PayloadFeedback`, `PayloadSelection`
3. [ ] Implement `BuildContext()` — assembles payload from state + runner context
4. [ ] Implement `Summary` computation: total, by_status counts, done_ratio
5. [ ] Ensure deterministic ordering in all slices
6. [ ] Create `internal/cli/context.go` with cobra command
7. [ ] Implement `--json` flag for machine output and default human-readable output
8. [ ] Register command in `init()`
9. [ ] Create `internal/state/context_test.go` — test payload assembly, ordering, nil state handling
10. [ ] Create `internal/cli/context_test.go` — test CLI output

---

### T8: Add `ralph task` CLI subcommands
> Agent-facing commands for task lifecycle — claim, criterion check, complete

**Requires:** T5, T6
**Status:** blocked

**Done when:**
- [ ] `ralph task add <plan> --title "..." [--requires T1,T2] [--criteria "a;b;c"]` works
- [ ] `ralph task claim <plan> <task-id>` works
- [ ] `ralph task complete <plan> <task-id> [--commits a,b]` works
- [ ] `ralph task criterion check <plan> <task-id> <index>` works
- [ ] `ralph task criterion uncheck <plan> <task-id> <index>` works
- [ ] `ralph task skip <plan> <task-id> --reason "..."` works
- [ ] `ralph feedback add <plan> --scope task:T2 --message "..." [--author human]` works
- [ ] `ralph feedback resolve <plan> <feedback-id>` works
- [ ] All commands save state atomically after mutation
- [ ] All commands support `--json` output flag
- [ ] `go test ./internal/cli/...` passes

**Subtasks:**
1. [ ] Create `internal/cli/task.go` with `taskCmd` parent command
2. [ ] Add `taskAddCmd` — parse flags, call `AddTask()`, save
3. [ ] Add `taskClaimCmd` — parse args, call `ClaimTask()`, save
4. [ ] Add `taskCompleteCmd` — parse args/flags, call `CompleteTask()`, save
5. [ ] Add `taskSkipCmd` — parse args/flags, call `SkipTask()`, save
6. [ ] Add `taskCriterionCmd` with `check`/`uncheck` subcommands
7. [ ] Create `internal/cli/feedback.go` with `feedbackCmd` parent
8. [ ] Add `feedbackAddCmd` — parse flags, call `AddFeedback()`, save
9. [ ] Add `feedbackResolveCmd` — parse args, call `ResolveFeedback()`, save
10. [ ] Add `--json` output support to all commands
11. [ ] Create `internal/cli/task_test.go` and `internal/cli/feedback_test.go`

---

### T9: Wire state.yaml into worktree sync
> State must travel with the plan bundle between main worktree and execution worktree

**Requires:** T2
**Status:** blocked

**Done when:**
- [ ] `SyncToWorktree()` copies `state.yaml` from main → worktree (alongside plan.md, progress.md, feedback.md)
- [ ] `SyncFromWorktree()` copies `state.yaml` from worktree → main (alongside plan.md, progress.md)
- [ ] Plans without `state.yaml` sync normally (backward compat)
- [ ] `go test ./internal/worktree/...` passes

**Subtasks:**
1. [ ] Update `internal/worktree/sync.go` `SyncToWorktree()` — add state.yaml to file list
2. [ ] Update `internal/worktree/sync.go` `SyncFromWorktree()` — add state.yaml to file list
3. [ ] Update tests in `internal/worktree/sync_test.go` — verify state.yaml included in sync

---

### T10: Scaffold state.yaml from plan.md on bundle creation
> When a plan bundle is created or first run, generate initial state.yaml from the markdown spec

**Requires:** T2, T5
**Status:** blocked

**Done when:**
- [ ] `InitStateFromPlan(plan *plan.Plan) (*PlanState, error)` extracts tasks from plan.md and creates initial state
- [ ] Task titles extracted from `### T1: Title` headings in plan.md
- [ ] Dependencies extracted from `**Requires:** T1, T2` fields
- [ ] Criteria extracted from `**Done when:**` checkbox lists
- [ ] `ralph plan create <name>` scaffolds state.yaml alongside plan.md
- [ ] Running `ralph run` or `ralph worker` on a plan without state.yaml auto-generates it
- [ ] `go test ./internal/state/...` passes

**Subtasks:**
1. [ ] Create `internal/state/init.go`
2. [ ] Implement `InitStateFromPlan()` — parse plan.md headings, requires, criteria into PlanState
3. [ ] Parse `### T{n}: Title` headings into TaskState entries
4. [ ] Parse `**Requires:** T1, T2` into requires arrays
5. [ ] Parse `**Done when:** - [ ] criterion text` into Criteria slices
6. [ ] Set initial plan status to `active`, task statuses to `todo` (or `blocked` if deps)
7. [ ] Update `internal/plan/bundle.go` `CreateBundle()` — also scaffold empty state.yaml
8. [ ] Add auto-init logic: if state.yaml missing when runner starts, generate from plan.md
9. [ ] Create `internal/state/init_test.go` with test fixture plan.md files

---

### T11: Wire runner loop to use structured context
> The integration point — runner uses state.yaml instead of markdown parsing for task management

**Requires:** T7, T9, T10
**Status:** blocked

**Done when:**
- [ ] Runner loads state.yaml at start of each iteration
- [ ] Prompt includes structured context JSON (via `{{CONTEXT_JSON}}` placeholder)
- [ ] After agent iteration, runner detects which task was worked on from state.yaml changes
- [ ] Completion check uses criteria gate: plan complete when all tasks done in state.yaml
- [ ] LLM verification skipped when state.yaml exists and all tasks/criteria are done (cheaper)
- [ ] LLM verification kept as fallback for plans without state.yaml (backward compat)
- [ ] Progress.md still updated with iteration summary
- [ ] `go test ./internal/runner/...` passes

**Subtasks:**
1. [ ] Update `internal/runner/loop.go` `runIteration()` — load state.yaml, build context payload
2. [ ] Update `internal/runner/loop.go` `buildPrompt()` — add `CONTEXT_JSON` override from context payload
3. [ ] Update `internal/prompt/builder.go` — support `{{CONTEXT_JSON}}` placeholder
4. [ ] Update completion detection in `Run()` — if state.yaml exists, check all tasks done instead of checkbox counting
5. [ ] Update verification path — skip LLM verify if criteria-gated completion passes
6. [ ] Keep existing verify.go as fallback for plans without state.yaml
7. [ ] After iteration: reload state.yaml (agent may have updated it via CLI commands)
8. [ ] Update `internal/runner/loop_test.go` — test criteria-gated completion path

---

### T12: Update prompt templates for structured state protocol
> Tell the agent how to use the new structured state system

**Requires:** T7, T8
**Status:** blocked

**Done when:**
- [ ] `prompt.md` updated with instructions for reading context JSON
- [ ] Agent told to use `ralph task claim/complete/criterion` commands
- [ ] Agent told to read `suggested_next` + `available_tasks` for task selection
- [ ] Agent told to check criteria as it verifies each acceptance criterion
- [ ] `{{CONTEXT_JSON}}` placeholder documented in prompt
- [ ] Old markdown-checkbox-based instructions kept as fallback section (for plans without state.yaml)

**Subtasks:**
1. [ ] Update `internal/prompt/prompts/prompt.md` — add structured state protocol section
2. [ ] Add section explaining context JSON format with example
3. [ ] Add section explaining task lifecycle: claim → work → criterion check → complete
4. [ ] Add section explaining feedback reading + resolution
5. [ ] Keep existing markdown-based instructions under "Legacy plans" heading
6. [ ] Update `internal/prompt/prompts/plan-spec.md` — note that state.yaml is now the source of truth for task status

---

## Discovered

<!-- Tasks found during implementation -->

