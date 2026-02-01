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
**Status:** open

**Done when:**
- [ ] `ProgressPath()` returns `{bundleDir}/progress.md` for bundles
- [ ] `ProgressPath()` returns legacy path for flat files (backwards compat)
- [ ] `AppendProgress()` includes progress percentage in iteration header

**Subtasks:**
1. [ ] Update `ProgressPath()` to check `p.BundleDir` first
2. [ ] Update `AppendProgress()` to calculate and include progress in header format: `## Iteration N (YYYY-MM-DD HH:MM) - X/Y (Z%)`
3. [ ] Add helper to strip template comments on first real entry
4. [ ] Update tests in `internal/plan/progress_test.go`

---

### T5: Update feedback.go for bundle-aware paths
> FeedbackPath must work with both bundles and flat files

**Requires:** T1
**Status:** open

**Done when:**
- [ ] `FeedbackPath()` returns `{bundleDir}/feedback.md` for bundles
- [ ] `FeedbackPath()` returns legacy path for flat files (backwards compat)

**Subtasks:**
1. [ ] Update `FeedbackPath()` to check `p.BundleDir` first
2. [ ] Update tests in `internal/plan/feedback_test.go`

---

### T6: Update queue.go to move directories
> Queue operations must move entire bundle directories atomically

**Requires:** T1
**Status:** open

**Done when:**
- [ ] `Activate()` moves entire bundle directory to current/
- [ ] `Complete()` moves bundle to complete/ with date suffix (name-YYYYMMDD)
- [ ] `Reset()` moves bundle back to pending/
- [ ] `listPlans()` scans for directories only, skips files
- [ ] Collision handling: name-YYYYMMDD-2, name-YYYYMMDD-3, etc.

**Subtasks:**
1. [ ] Add `planDir(p *Plan) string` helper (returns BundleDir or Dir(Path))
2. [ ] Add `uniqueCompleteName(name string) string` for date suffix with collision counter
3. [ ] Update `Activate()` to move directory and update Plan.BundleDir/Path
4. [ ] Update `Complete()` to move directory with unique name
5. [ ] Update `Reset()` to move directory back to pending
6. [ ] Update `listPlans()` to only process directories (skip files)
7. [ ] Update tests in `internal/plan/queue_test.go`

---

### T7: Simplify worktree sync for bundles
> Remove complex filepath.Rel() gymnastics - use bundle name as key

**Requires:** T6
**Status:** blocked

**Done when:**
- [ ] `SyncToWorktree()` copies bundle files to `{worktree}/plans/current/{name}/`
- [ ] `SyncFromWorktree()` copies plan.md and progress.md back (NOT feedback.md)
- [ ] No more `filepath.Rel()` with fallback logic

**Subtasks:**
1. [ ] Update `SyncToWorktree()` to use `p.Name` for destination path
2. [ ] Update `SyncFromWorktree()` to use `p.Name` for source path
3. [ ] Remove old path computation logic
4. [ ] Update tests in `internal/worktree/sync_test.go`

---

### T8: Add migration function
> Convert existing flat files to bundles

**Requires:** T3
**Status:** blocked

**Done when:**
- [ ] `MigrateToBundles(plansDir string) error` converts all flat files
- [ ] Migration moves plan.md into bundle directory
- [ ] Migration moves associated .progress.md and .feedback.md files
- [ ] Migration skips existing bundles (directories)
- [ ] Migration creates scaffolded files if associated files missing

**Subtasks:**
1. [ ] Add `MigrateToBundles()` function to `internal/plan/bundle.go`
2. [ ] Iterate pending/, current/, complete/ directories
3. [ ] For each .md file (not .progress.md or .feedback.md), create bundle
4. [ ] Move associated files into bundle, renaming to progress.md/feedback.md
5. [ ] Add tests for migration scenarios

---

### T9: Add CLI plan commands
> User-facing commands for creating plans and migrating

**Requires:** T3, T8
**Status:** blocked

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
**Status:** open

**Done when:**
- [ ] Current plan shows progress bar: `[████████░░░░░░░░░░░░] 40%`
- [ ] Current plan shows task count: `Tasks: 4/10 completed`
- [ ] Pending plans show task counts: `add-auth 0/5 (0%)`

**Subtasks:**
1. [ ] Update `internal/cli/status.go` to calculate progress for current plan
2. [ ] Add progress bar display using `Progress.Bar()`
3. [ ] Add task counts for pending plans
4. [ ] Test output formatting

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
