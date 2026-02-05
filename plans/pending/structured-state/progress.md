# Progress: structured-state

<!-- This file tracks what was done in each iteration, gotchas discovered, and what to do next. -->
<!-- Format: ## Iteration N (YYYY-MM-DD HH:MM) - X/Y (Z%) -->

---
### Iteration 1: T1 — Define state types and YAML serialization
**Completed:** Created `internal/state/` package with `types.go` containing all foundation types:
- `PlanStatus` enum (draft, ready, active, blocked, complete) with `IsValid()` method
- `TaskStatus` enum (todo, claimed, doing, blocked, done, skipped) with `IsValid()` method
- `PlanState` struct with ID, Title, Status, CreatedAt, Tasks, Feedback
- `TaskState` struct with ID, Title, Description, Status, Requires, Criteria, Notes, Artifacts, StartedAt, DoneAt
- `Criterion` struct with Text, Done, DoneAt
- `Artifacts` struct with Commits, FilesTouched
- `Feedback` struct with ID, Scope, Author, Message, Resolved, ResolvedAt, CreatedAt
- All fields have both `yaml` and `json` struct tags
- Created `types_test.go` with 7 tests: IsValid for both enums, YAML round-trip, JSON round-trip, JSON output verification, YAML output verification, empty optional fields round-trip

**Gotcha:** YAML `omitempty` is needed on the `Feedback` slice field in `PlanState` — without it, a nil slice serializes as `feedback: []` and deserializes as `[]Feedback{}`, breaking `reflect.DeepEqual` round-trip checks. Fixed by adding `omitempty` to both YAML and JSON tags.
**Next:** T2 (state load/save with atomic writes) or T3 (validation rules) — both depend only on T1 which is now complete.


---
### Iteration 2: T2 — Implement state load/save with atomic writes
**Completed:** Created `internal/state/store.go` with three functions:
- `StatePath(bundleDir)` — returns path to `state.yaml` within a bundle directory
- `LoadState(bundleDir)` — reads and parses `state.yaml`, returns `(nil, nil)` for missing file (backward compat), clear error for invalid YAML
- `SaveState(state, bundleDir)` — marshals to YAML, writes atomically via temp+rename pattern (reused from `runner/context.go`), creates parent dirs if needed

Created `internal/state/store_test.go` with 7 tests:
- `TestStatePath` — path construction
- `TestSaveAndLoadState` — round-trip save then load, deep equality check
- `TestLoadStateMissingFile` — returns `(nil, nil)` for backward compat
- `TestLoadStateInvalidYAML` — returns parse error
- `TestSaveStateCreatesDirectory` — creates nested dirs automatically
- `TestSaveStateAtomicWrite` — overwrites atomically, no leftover temp files
- `TestLoadStateUnreadableDir` — non-existent directory returns `(nil, nil)`

All 14 state tests pass. Full test suite passes.

**Next:** T3 (validation rules) — depends only on T1. T4 depends on T1+T3. T5 and T6 depend on T2+T3.

## Iteration 2 (2026-02-06) - 14/88 (16%)
Claude execution completed.


## Iteration 2 (2026-02-06 08:01) - 14/88 (15%)
Claude execution completed in 2m1.450936833s.


---
### Iteration 3: T3 — Implement state validation rules
**Completed:** Created `internal/state/validate.go` with five validation functions:
- `ValidatePlanTransition(from, to PlanStatus) error` — validates plan status transitions using an allow-list map (draft→ready→active→complete, active↔blocked)
- `ValidateTaskTransition(from, to TaskStatus) error` — validates task status transitions (todo→claimed→doing→done, doing↔blocked, any→skipped; done/skipped are terminal)
- `ValidateDependencies(tasks []TaskState) error` — checks for missing dependency IDs and uses DFS with coloring to detect cycles (direct, indirect, self-referencing)
- `ValidateCompletion(task *TaskState) error` — criteria gate: all criteria must be done before task can complete; tasks without criteria always pass
- `ValidateFeedbackScope(scope string, tasks []TaskState) error` — validates scope is "plan" or "task:Tn" where Tn exists

Created `internal/state/validate_test.go` with 16 test functions covering:
- Valid and invalid plan transitions (5 valid, 9 invalid)
- Invalid plan statuses (bogus source/target)
- Valid and invalid task transitions (9 valid, 13 invalid)
- Invalid task statuses
- Dependency validation: valid deps, no deps, missing ID, direct cycle, indirect cycle, self-cycle
- Completion: all criteria met, criterion not met (1-indexed), no criteria
- Feedback scope: plan, valid task, unknown task, invalid formats (empty, "global", "task:", "PLAN", "task")

All 30 state tests pass. Full test suite passes.

**Next:** T4 (task selection algorithm) — depends on T1 + T3 (both complete). T5 and T6 depend on T2 + T3 (both complete).


## Iteration 3 (2026-02-06 08:03) - 21/88 (23%)
Claude execution completed in 2m23.111033791s.


---
### Iteration 4: T4 — Implement task selection algorithm
**Completed:** Created `internal/state/selection.go` with:
- `TaskPick` struct with TaskID and Reason fields (JSON-tagged for context output)
- `Selection` struct with SuggestedNext (*TaskPick), Available ([]TaskPick), Blocked ([]TaskPick)
- `ComputeSelection(state *PlanState) *Selection` — classifies tasks into available/blocked sets:
  - Done/skipped/doing/claimed tasks are excluded from both sets
  - Explicitly blocked tasks go to blocked list
  - Todo tasks with unmet deps go to blocked list (with reason listing unmet dep IDs)
  - Todo tasks with all deps met go to available list
  - SuggestedNext = first available by numeric ID order
- `parseTaskNum()` — extracts numeric suffix from "T12" → 12 for stable ordering
- `sortByTaskID()` — sorts TaskPick slices by numeric task ID
- Handles edge cases: nil state, empty tasks, skipped deps count as resolved

Created `internal/state/selection_test.go` with 13 test functions:
- Happy path, all done, all blocked (circular deps), dep chains, parallel tasks
- Skipped deps count, explicitly blocked status, in-progress tasks excluded
- Nil state, empty tasks, numeric sorting (T2 < T10), blocked reason strings
- parseTaskNum unit tests (valid IDs, invalid IDs, edge cases)

All 43 state package tests pass. Full test suite passes.

**Next:** T5 (task mutation operations) or T6 (feedback operations) — both depend on T2 + T3 (complete). T7 depends on T4 + T5 + T6.


## Iteration 4 (2026-02-06 08:06) - 29/88 (32%)
Claude execution completed in 2m22.418192958s.


---
### Iteration 5: T5 — Implement task mutation operations
**Completed:** Created `internal/state/mutations.go` with all mutation functions:
- `nextTaskID(state)` — scans existing task IDs, returns next `T{n}`
- `findTask(state, taskID)` — returns pointer to task by ID or error
- `AddTask(state, title, requires, criteria)` — creates task with auto-ID, validates dep IDs exist, builds criteria from strings
- `ClaimTask(state, taskID)` — validates todo→claimed transition + deps met, sets status to doing with StartedAt
- `CheckCriterion(state, taskID, index)` / `UncheckCriterion()` — 1-indexed, bounds-checked, sets/clears DoneAt
- `CompleteTask(state, taskID, commits, filesTouched)` — validates criteria gate + status transition, sets done + DoneAt + artifacts
- `SkipTask(state, taskID, reason)` — validates transition, appends reason to notes
- `SetPlanStatus(state, status, reason)` — validates plan status transition
- `parseRequires()` / `parseCriteria()` — string parsing helpers for CLI use
- `nextFeedbackNum()` — scans feedback IDs, returns next F{n} number

Created `internal/state/mutations_test.go` with 30 tests covering all operations + error cases.

**Gotcha:** `ClaimTask` needs to validate `todo → claimed` (not `todo → doing` directly) since the task transition table doesn't allow skipping the `claimed` state. The function transitions through `claimed` internally and immediately sets `doing` since there are no leases in v1.
**Next:** T6 (feedback operations) — depends on T2 + T3 (both complete). T7 depends on T4 + T5 + T6.


## Iteration 5 (2026-02-06 08:08) - 38/88 (43%)
Claude execution completed in 2m52.412655583s.


---
### Iteration 6: T6 — Implement feedback operations
**Completed:** Created `internal/state/feedback.go` with four exported functions:
- `AddFeedback(state, scope, author, message)` — validates scope via `ValidateFeedbackScope`, validates non-empty message, auto-assigns F{n} ID using existing `nextFeedbackNum()` from mutations.go, sets CreatedAt timestamp
- `ResolveFeedback(state, feedbackID)` — finds feedback by ID, checks not already resolved, sets Resolved=true + ResolvedAt timestamp
- `UnresolvedFeedback(state)` — filters and returns all feedback where Resolved=false
- `FeedbackForTask(state, taskID)` — filters by scope="task:{taskID}"

Created `internal/state/feedback_test.go` with 15 tests covering:
- AddFeedback: plan scope, task scope, auto-increment IDs, invalid scope, empty message, empty scope
- ResolveFeedback: happy path, not found, already resolved
- UnresolvedFeedback: mixed resolved/unresolved, empty, all resolved
- FeedbackForTask: multiple matches, no matches, no feedback at all

All 88 state package tests pass (15 new + 73 existing). Full suite passes.

**Gotcha:** `nextFeedbackNum()` was already implemented in mutations.go (iteration 5) — reused directly, no duplication needed.
**Next:** T7 (context payload + `ralph context` command) — depends on T4 + T5 + T6 (all complete). T8 (task CLI) depends on T5 + T6 (both complete). T9 depends on T2 (complete).


## Iteration 6 (2026-02-06 08:11) - 44/88 (50%)
Claude execution completed in 1m59.549823917s.


---
### Iteration 7: T7 — Build context payload (subtasks 1-5, 9)
**Completed:** Created `internal/state/context.go` with:
- `ContextPayload` struct (top-level with Plan, Tasks, Feedback, Selection, Summary fields)
- `PayloadPlan`, `PayloadTasks`, `PayloadFeedback`, `PayloadSelection`, `PayloadSummary` nested structs
- `BuildContext(state *PlanState) *ContextPayload` — assembles full payload from plan state
  - Tasks sorted by numeric ID (reuses `parseTaskNum` from selection.go)
  - Feedback sorted by `created_at` timestamp
  - Computes selection via existing `ComputeSelection()`
  - Handles nil state (returns zero-value payload with empty arrays, no nulls in JSON)
- `computeSummary()` — calculates total, by_status counts, done_ratio
- `nonNilPicks()` — ensures nil slices serialize as `[]` not `null` in JSON

Created `internal/state/context_test.go` with 9 tests:
- `TestBuildContext_NilState` — nil input returns empty payload, no null arrays in JSON
- `TestBuildContext_FullState` — full plan with tasks, feedback, selection, summary
- `TestBuildContext_DeterministicOrdering` — T1<T2<T10 sorting, feedback by created_at
- `TestBuildContext_AllTasksDone` — no suggested_next, done_ratio=1.0
- `TestBuildContext_EmptyTasks` — empty task list edge case
- `TestBuildContext_JSONOutput` — JSON round-trip verification
- `TestBuildContext_NoNullArraysInJSON` — explicit null array checks
- `TestComputeSummary` — by_status counts and done_ratio
- `TestComputeSummary_Empty` — nil tasks edge case

All 97 state tests pass. Full test suite passes.

**Gotcha:** `BuildContext` signature takes only `*PlanState` (not `*runner.Context` as the plan suggested) — runner context info (iteration, branch) can be added later when wiring T11. The payload types are self-contained in the state package.
**Next:** T7 subtask 6 — Create `internal/cli/context.go` with cobra command (and subtasks 7-8, 10).


## Iteration 7 (2026-02-06 08:14) - 47/88 (53%)
Claude execution completed in 3m35.847832458s.


---
### Iteration 8: T7 — Build context payload CLI command (subtasks 6-8, 10)
**Completed:** Created `internal/cli/context.go` with:
- `contextCmd` cobra command: `ralph context <plan-path>` with `--json` flag
- `runContext()` — loads state.yaml from bundle, builds context payload, outputs JSON or human-readable
- `resolveBundleDir()` — resolves plan path argument to bundle directory (handles directory, plan.md file, or flat file with clear error)
- `outputContextJSON()` — pretty-printed JSON to stdout via `json.Encoder`
- `outputContextHuman()` — human-readable summary with plan info, progress, selection, blocked/available tasks, unresolved feedback, and task details with criteria counts
- Command registered via `init()` with `rootCmd.AddCommand(contextCmd)`

Created `internal/cli/context_test.go` with 8 tests:
- `TestRunContext_JSON` — JSON output with full payload verification (plan ID, title, tasks, summary, selection)
- `TestRunContext_Human` — human-readable output with key section checks
- `TestRunContext_NoStateYaml` — clear error for missing state.yaml
- `TestRunContext_NonexistentPath` — error for nonexistent path
- `TestRunContext_PlanFilePath` — passing plan.md file path resolves to bundle
- `TestResolveBundleDir_Directory` — directory path returns absolute path
- `TestResolveBundleDir_PlanFile` — plan.md resolves to parent bundle dir
- `TestResolveBundleDir_FlatFile` — flat file returns "not a bundle" error

All tests pass (105 state tests + 8 new CLI context tests). Full test suite passes.

T7 is now **complete** — all subtasks checked, all "Done when" criteria verified.

**Gotcha:** `BuildContext` signature takes `*PlanState` only (not `*runner.Context` as plan suggested) — runner context info can be added when wiring T11. Also, `plan.Load()` treats `plan.md` as a flat file when you pass the file path directly (not the directory), so `resolveBundleDir` handles this by checking `filepath.Base == "plan.md"` and using the parent directory.
**Next:** T8 (task CLI subcommands) — depends on T5+T6 (both complete). T9 (worktree sync) depends on T2 (complete). T10 (scaffold state.yaml from plan.md) depends on T2+T5 (both complete). All three are unblocked.


## Iteration 8 (2026-02-06 08:18) - 52/88 (59%)
Claude execution completed in 3m19.210389208s.


---
### Iteration 9: T8 — Add `ralph task` and `ralph feedback` CLI subcommands (all subtasks)
**Completed:** Created two new CLI files with full command trees:

**`internal/cli/task.go`** — 7 commands:
- `taskCmd` parent: `ralph task`
- `taskAddCmd`: `ralph task add <plan> --title "..." [--requires T1,T2] [--criteria "a;b;c"]` — calls `state.AddTask()`, saves atomically
- `taskClaimCmd`: `ralph task claim <plan> <task-id>` — calls `state.ClaimTask()`, saves atomically
- `taskCompleteCmd`: `ralph task complete <plan> <task-id> [--commits a,b]` — calls `state.CompleteTask()`, saves atomically
- `taskSkipCmd`: `ralph task skip <plan> <task-id> --reason "..."` — calls `state.SkipTask()`, saves atomically
- `taskCriterionCheckCmd`: `ralph task criterion check <plan> <task-id> <index>` — 1-indexed, calls `state.CheckCriterion()`
- `taskCriterionUncheckCmd`: `ralph task criterion uncheck <plan> <task-id> <index>` — calls `state.UncheckCriterion()`
- `loadAndMutate()` helper for DRY load→mutate→save→output pattern
- `parseCommaSep()`, `parseSemicolonSep()` for flag parsing
- All commands support `--json` via persistent flag on `taskCmd`

**`internal/cli/feedback.go`** — 3 commands:
- `feedbackCmd` parent: `ralph feedback`
- `feedbackAddCmd`: `ralph feedback add <plan> --scope plan --message "..." [--author human]` — calls `state.AddFeedback()`
- `feedbackResolveCmd`: `ralph feedback resolve <plan> <feedback-id>` — calls `state.ResolveFeedback()`
- All commands support `--json` via persistent flag on `feedbackCmd`

**`internal/cli/task_test.go`** — 15 tests covering:
- taskAdd (human + JSON output), taskClaim (human + JSON), taskComplete with commits, taskSkip with reason
- criterionCheck (human + JSON), criterionUncheck, invalid index error
- NoStateYaml error, DepsNotMet error, parseCommaSep, parseSemicolonSep

**`internal/cli/feedback_test.go`** — 8 tests covering:
- feedbackAdd (human + JSON + invalid scope), feedbackResolve (human + JSON + not found)
- NoStateYaml error

All 21 new tests pass. Full test suite (all packages) passes.

T8 is now **complete** — all 11 subtasks checked, all "Done when" criteria verified.

**Gotcha:** Reused `resolveBundleDir()` from context.go for all commands — consistent bundle resolution across the CLI. The `loadAndMutate()` pattern keeps all task commands DRY (load state → call mutation → save state → output result).
**Next:** T9 (worktree sync for state.yaml) — depends on T2 (complete). T10 (scaffold state.yaml from plan.md) — depends on T2+T5 (both complete). T12 (prompt templates) — depends on T7+T8 (both complete now). All three are unblocked.


## Iteration 9 (2026-02-06 08:22) - 63/88 (71%)
Claude execution completed in 4m10.583795167s.

