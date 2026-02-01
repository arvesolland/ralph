# Progress: Plan Bundles v3

Iteration log - what was done, gotchas, and next steps.

---
### Iteration 1: T1 - Add BundleDir field to Plan struct
**Completed:**
- Added `BundleDir string` field to Plan struct in `internal/plan/plan.go:35`
- Added `IsBundle() bool` method at `internal/plan/plan.go:43-45`
- Updated `Load()` function at `internal/plan/plan.go:51-95` to detect directory vs file, set BundleDir, and load plan.md from inside bundles
- Updated `deriveName()` at `internal/plan/plan.go:105-114` to use directory name for bundles
- Added tests in `internal/plan/plan_test.go`: TestLoad_Bundle, TestDeriveName with bundle cases
- All existing tests pass (backwards compatible)

**Commit:** `06533bf feat(plan): add BundleDir field for directory-based plan bundles`
**Next:** T2 - Progress type and calculation

---
### Iteration 2: T2 - Add Progress type and calculation
**Completed:**
- Added `Progress` struct with Total, Completed, Percent fields at `internal/plan/task.go:162-170`
- Added `CalculateProgress()` function at `internal/plan/task.go:173-185`
- Added `String()` method for "4/10 (40%)" format at `internal/plan/task.go:188-190`
- Added `Bar(width int)` method for visual progress bar at `internal/plan/task.go:194-204`
- Added comprehensive tests: TestCalculateProgress, TestProgress_String, TestProgress_Bar in `internal/plan/task_test.go`

**Commit:** `94e2f76 feat(plan): add Progress type for task completion tracking`
**Next:** T3 - Create bundle scaffolding functions (now unblocked)

---
### Iteration 3: Catch-up - Update plan status
**Completed:**
- Marked T1 and T2 as complete in plan file (all acceptance criteria verified)
- Updated dependent tasks T3, T4, T5, T6, T10 from "blocked" to "open"
- Created this progress file

**Next:** T3 - Create bundle scaffolding functions

---
### Iteration 4: T3 - Create bundle scaffolding functions
**Completed:**
- Verified T3 implementation already exists in `internal/plan/bundle.go`
- `CreateBundle()` at line 104 creates complete bundle directory with all files
- `scaffoldPlan()` at line 152 creates plan.md with template (Context, Rules, Tasks, Discovered sections)
- `scaffoldProgress()` at line 160 creates progress.md with header and format example
- `scaffoldFeedback()` at line 167 creates feedback.md with Pending/Processed sections
- Tests exist in `internal/plan/bundle_test.go` and all pass
- All acceptance criteria verified: duplicate name detection, empty name rejection, proper sanitization

**Commit:** (previous iteration - `dee638f feat(plan): add bundle scaffolding functions`)
**Next:** T4 - Update progress.go for bundle-aware paths (or T5, T10 - all are unblocked)

---
### Iteration 5: T4 - Update progress.go for bundle-aware paths
**Completed:**
- Updated `ProgressPath()` at `internal/plan/progress.go:17-24` to return `{bundleDir}/progress.md` for bundles
- Updated `AppendProgress()` and `AppendProgressWithTime()` to include progress in header: `## Iteration N (YYYY-MM-DD HH:MM) - X/Y (Z%)`
- Added `stripTemplateComments()` helper at `internal/plan/progress.go:43-56` to strip scaffolded template comments on first real entry (iteration 1)
- Added comprehensive tests: TestProgressPath_Bundle, TestReadProgress_Bundle, TestAppendProgress_Bundle, TestStripTemplateComments, TestAppendProgress_StripsTemplateOnFirstIteration
- Updated existing tests to expect new progress format in iteration header
- All tests pass (backwards compatible for flat files)

**Next:** T5 - Update feedback.go for bundle-aware paths

---
### Iteration 6: T5 - Update feedback.go for bundle-aware paths
**Completed:**
- Updated `FeedbackPath()` at `internal/plan/feedback.go:13-23` to check `p.IsBundle()` first and return `{bundleDir}/feedback.md` for bundles
- Added `TestFeedbackPath_Bundle` test with cases for bundles in current directory and with absolute paths
- Added `TestReadFeedback_Bundle` test to verify reading feedback from bundle directory
- Added `TestAppendFeedback_Bundle` test to verify feedback is created in bundle directory (and NOT at flat file location)
- All existing tests still pass (backwards compatible for flat files)
- All acceptance criteria verified

**Next:** T6 - Update queue.go to move directories (or T10 - Enhance status command with progress)

---
### Iteration 7: T6 - Update queue.go to move directories
**Completed:**
- Verified T6 implementation already exists in `internal/plan/queue.go`
- `planDir()` helper at lines 91-96 returns BundleDir for bundles, filepath.Dir(Path) for flat files
- `uniqueCompleteName()` at lines 98-119 generates date suffix with collision counter (name-YYYYMMDD, name-YYYYMMDD-2, etc.)
- `Activate()` at lines 150-197 moves entire bundle directory to current/ and updates Plan.BundleDir/Path
- `Complete()` at lines 203-240 moves bundle to complete/ with unique dated name
- `Reset()` at lines 246-282 moves bundle back to pending/
- `listPlans()` at lines 325-381 handles both bundles (directories) and legacy flat files for backwards compatibility
- Comprehensive tests exist: TestQueue_Activate_Bundle, TestQueue_Complete_Bundle, TestQueue_Complete_Bundle_Collision, TestQueue_Reset_Bundle, TestQueue_FullLifecycle_Bundle, TestQueue_UniqueCompleteName, TestPlanDir
- All 24 queue tests pass

**Note:** listPlans() correctly handles BOTH bundles and flat files for backwards compatibility during migration period, per the Context section "Must support both flat files and bundles during migration period"

**Next:** T7 - Simplify worktree sync for bundles (now unblocked), or T10 - Enhance status command with progress

---
### Iteration 8: T7 - Simplify worktree sync for bundles
**Completed:**
- Updated `SyncToWorktree()` in `internal/worktree/sync.go:20-48` to use `p.Name` for bundles
  - For bundles: files go to `{worktree}/plans/current/{name}/plan.md`, `progress.md`, `feedback.md`
  - For flat files: keeps existing `filepath.Rel()` logic for backwards compatibility
- Updated `SyncFromWorktree()` in `internal/worktree/sync.go:100-135` with same pattern
  - Bundles use `p.Name` for source paths
  - Flat files keep `filepath.Rel()` logic
- Added 3 new tests in `internal/worktree/sync_test.go`:
  - `TestSyncToWorktree_Bundle` - verifies all 3 files sync to bundle structure
  - `TestSyncFromWorktree_Bundle` - verifies plan.md and progress.md sync back
  - `TestSyncFromWorktree_Bundle_NoFeedbackSync` - verifies feedback.md is NOT synced back (human input is one-way)
- All 60 worktree tests pass

**Gotcha:** The "remove old path computation logic" subtask (T7.3) means removing it FOR BUNDLES. Flat files still need filepath.Rel() for backwards compat during migration period.

**Next:** T10 - Enhance status command with progress (T8/T9 are blocked on other tasks)

---
### Iteration 9: T10 - Enhance status command with progress
**Completed:**
- Updated `internal/cli/status.go:37-103` to show progress for current and pending plans
- Current plan now shows:
  - Progress bar using `Progress.Bar(20)`: `[█████████████░░░░░░░] 68%`
  - Task count: `Tasks: 26/38 completed`
  - Uses the plan's actual Branch field instead of hardcoding "feat/" prefix
- Pending plans now show task counts with `progress.String()` format: `name X/Y (Z%)`
- Replaced `queue.Status()` usage for current/pending with direct `queue.Current()` and `queue.Pending()` to access `*Plan` objects with their Tasks
- All 6 status tests pass, full test suite passes
- Verified output with live `ralph status` command

**Next:** T8 - Add migration function (but currently blocked), or plan complete for unblocked tasks
