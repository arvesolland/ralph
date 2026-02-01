# Plan: Plan Bundles v3

**Spec:** N/A (internal refactor)
**Created:** 2026-02-01
**Status:** pending

## Context

Refactor plan management from flat files to directory bundles. Each plan becomes a self-contained directory with all associated files (plan.md, progress.md, feedback.md). Directory location determines state (pending/current/complete).

Key design decisions:
- No metadata files - directory location IS the state
- context.json stays in worktree - execution-specific, deleted on reset
- Scaffold all files upfront with explanatory headers
- Progress calculated from task checkboxes using existing CountComplete()/CountTotal()
- Backwards compatible via BundleDir field (empty = legacy flat file)

### Gotchas
- Must support both flat files and bundles during migration period
- Directory rename is atomic on POSIX but need counter for same-day completions
- Feedback.md only syncs TO worktree, not back (human input is one-way)
- Existing tests use flat file structure - need to update test fixtures

---

## Rules

1. **Pick task:** First task where status ≠ `complete` and all `Requires` are `complete`
2. **Subtasks are sequential.** Complete 1 before 2.
3. **Task complete when:** All "Done when" checked → set Status: `complete`
4. **Update file after each checkbox.**
5. **New work found?** Add to Discovered section, continue current task.

---

## Tasks

### T1: Add BundleDir field to Plan struct
> Foundation for all bundle operations - Plan must know if it's a bundle or flat file

**Requires:** —
**Status:** complete

**Done when:**
- [x] `Plan` struct has `BundleDir string` field
- [x] `IsBundle() bool` method returns true when BundleDir is set
- [x] `Load()` detects directory vs file and sets fields accordingly
- [x] Existing tests still pass (backwards compatible)

**Subtasks:**
1. [x] Add `BundleDir string` field to Plan struct in `internal/plan/plan.go`
2. [x] Add `IsBundle() bool` method to Plan
3. [x] Update `Load()` to check if path is directory, set BundleDir and load plan.md from inside
4. [x] Update `deriveName()` to use directory name for bundles
5. [x] Run `go test ./internal/plan/...` to verify no regressions

---

### T2: Add Progress type and calculation
> Enable progress tracking (e.g., "4/10 (40%)") needed for status display and scaffolding

**Requires:** —
**Status:** complete

**Done when:**
- [x] `Progress` struct exists with Total, Completed, Percent fields
- [x] `CalculateProgress(tasks []Task) Progress` works correctly
- [x] `Progress.String()` returns "4/10 (40%)" format
- [x] `Progress.Bar(width int)` returns visual bar like "[████░░░░░░]"

**Subtasks:**
1. [x] Add Progress struct to `internal/plan/task.go`
2. [x] Add `CalculateProgress()` function using existing `CountTotal()`/`CountComplete()`
3. [x] Add `String()` method for text format
4. [x] Add `Bar(width int)` method for visual progress bar
5. [x] Add tests in `internal/plan/task_test.go`

---

### T3: Create bundle scaffolding functions
> Core feature - create plan bundles with all files and proper headers

**Requires:** T1
**Status:** complete

**Done when:**
- [x] `CreateBundle(plansDir, name string) (*Plan, error)` creates complete bundle
- [x] `plan.md` has template with overview, tasks sections, and instructions
- [x] `progress.md` has header explaining format with example entry
- [x] `feedback.md` has Pending/Processed sections with instructions
- [x] Bundle creation fails if plan already exists

**Subtasks:**
1. [x] Create `internal/plan/bundle.go` file
2. [x] Implement `scaffoldPlan(bundleDir, name string) error` with template
3. [x] Implement `scaffoldProgress(bundleDir, name string) error` with header
4. [x] Implement `scaffoldFeedback(bundleDir, name string) error` with sections
5. [x] Implement `CreateBundle()` that calls all scaffold functions
6. [x] Add tests in `internal/plan/bundle_test.go`

---

### T4: Update progress.go for bundle-aware paths
> ProgressPath must work with both bundles and flat files

**Requires:** T1
**Status:** complete

**Done when:**
- [x] `ProgressPath()` returns `{bundleDir}/progress.md` for bundles
- [x] `ProgressPath()` returns legacy path for flat files (backwards compat)
- [x] `AppendProgress()` includes progress percentage in iteration header

**Subtasks:**
1. [x] Update `ProgressPath()` to check `p.BundleDir` first
2. [x] Update `AppendProgress()` to calculate and include progress in header format: `## Iteration N (YYYY-MM-DD HH:MM) - X/Y (Z%)`
3. [x] Add helper to strip template comments on first real entry
4. [x] Update tests in `internal/plan/progress_test.go`

---

### T5: Update feedback.go for bundle-aware paths
> FeedbackPath must work with both bundles and flat files

**Requires:** T1
**Status:** complete

**Done when:**
- [x] `FeedbackPath()` returns `{bundleDir}/feedback.md` for bundles
- [x] `FeedbackPath()` returns legacy path for flat files (backwards compat)

**Subtasks:**
1. [x] Update `FeedbackPath()` to check `p.BundleDir` first
2. [x] Update tests in `internal/plan/feedback_test.go`

---

### T6: Update queue.go to move directories
> Queue operations must move entire bundle directories atomically

**Requires:** T1
**Status:** complete

**Done when:**
- [x] `Activate()` moves entire bundle directory to current/
- [x] `Complete()` moves bundle to complete/ with date suffix (name-YYYYMMDD)
- [x] `Reset()` moves bundle back to pending/
- [x] `listPlans()` scans for directories only, skips files
- [x] Collision handling: name-YYYYMMDD-2, name-YYYYMMDD-3, etc.

**Subtasks:**
1. [x] Add `planDir(p *Plan) string` helper (returns BundleDir or Dir(Path))
2. [x] Add `uniqueCompleteName(name string) string` for date suffix with collision counter
3. [x] Update `Activate()` to move directory and update Plan.BundleDir/Path
4. [x] Update `Complete()` to move directory with unique name
5. [x] Update `Reset()` to move directory back to pending
6. [x] Update `listPlans()` to only process directories (skip files)
7. [x] Update tests in `internal/plan/queue_test.go`

---

### T7: Simplify worktree sync for bundles
> Remove complex filepath.Rel() gymnastics - use bundle name as key

**Requires:** T6
**Status:** complete

**Done when:**
- [x] `SyncToWorktree()` copies bundle files to `{worktree}/plans/current/{name}/`
- [x] `SyncFromWorktree()` copies plan.md and progress.md back (NOT feedback.md)
- [x] No more `filepath.Rel()` with fallback logic

**Subtasks:**
1. [x] Update `SyncToWorktree()` to use `p.Name` for destination path
2. [x] Update `SyncFromWorktree()` to use `p.Name` for source path
3. [x] Remove old path computation logic
4. [x] Update tests in `internal/worktree/sync_test.go`

---

### T8: Add migration function
> Convert existing flat files to bundles

**Requires:** T3
**Status:** complete

**Done when:**
- [x] `MigrateToBundles(plansDir string) error` converts all flat files
- [x] Migration moves plan.md into bundle directory
- [x] Migration moves associated .progress.md and .feedback.md files
- [x] Migration skips existing bundles (directories)
- [x] Migration creates scaffolded files if associated files missing

**Subtasks:**
1. [x] Add `MigrateToBundles()` function to `internal/plan/bundle.go`
2. [x] Iterate pending/, current/, complete/ directories
3. [x] For each .md file (not .progress.md or .feedback.md), create bundle
4. [x] Move associated files into bundle, renaming to progress.md/feedback.md
5. [x] Add tests for migration scenarios

---

### T9: Add CLI plan commands
> User-facing commands for creating plans and migrating

**Requires:** T3, T8
**Status:** open

**Done when:**
- [ ] `ralph plan create <name>` creates scaffolded bundle in pending/
- [ ] `ralph plan migrate` converts all flat files to bundles
- [ ] Help text explains the commands

**Subtasks:**
1. [ ] Create `internal/cli/plan.go`
2. [ ] Add `planCmd` as parent command
3. [ ] Add `planCreateCmd` that calls `plan.CreateBundle()`
4. [ ] Add `planMigrateCmd` that calls `plan.MigrateToBundles()`
5. [ ] Register commands in init()
6. [ ] Update help text

---

### T10: Enhance status command with progress
> Show progress bars and task counts in ralph status

**Requires:** T2
**Status:** complete

**Done when:**
- [x] Current plan shows progress bar: `[████████░░░░░░░░░░░░] 40%`
- [x] Current plan shows task count: `Tasks: 4/10 completed`
- [x] Pending plans show task counts: `add-auth 0/5 (0%)`

**Subtasks:**
1. [x] Update `internal/cli/status.go` to calculate progress for current plan
2. [x] Add progress bar display using `Progress.Bar()`
3. [x] Add task counts for pending plans
4. [x] Test output formatting

---

### T11: Update init command
> Point users to new plan create command

**Requires:** T9
**Status:** blocked

**Done when:**
- [ ] "Next steps" in `ralph init` mentions `ralph plan create <name>`

**Subtasks:**
1. [ ] Update next steps text in `internal/cli/init.go`

---

## Discovered

<!-- Tasks found during implementation -->

---

## Completed

<!-- Completion dates will be added here -->
