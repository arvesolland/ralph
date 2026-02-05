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

