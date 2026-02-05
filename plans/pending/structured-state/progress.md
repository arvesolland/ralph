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

