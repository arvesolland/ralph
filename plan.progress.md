# Progress: Plan Bundles v3

Iteration log - what was done, gotchas, and next steps.

---
### Iteration 1: T1 - Add BundleDir field to Plan struct
**Completed:**
- Added `BundleDir string` field to Plan struct (`internal/plan/plan.go:30`)
- Added `IsBundle() bool` method that returns `true` when BundleDir is set (`internal/plan/plan.go:40-43`)
- Updated `Load()` to detect if path is a directory (bundle) or file (flat), sets BundleDir and loads plan.md from inside bundles (`internal/plan/plan.go:52-79`)
- Updated `deriveName()` signature to accept `isBundle bool` parameter - bundles use directory name, flat files strip .md extension (`internal/plan/plan.go:81-93`)
- Created test fixture `internal/plan/testdata/test-bundle/plan.md`
- Added tests for `IsBundle()` and `Load()` with bundle directory (`internal/plan/plan_test.go:37-79`)
- Updated `TestDeriveName` to include bundle test cases

**Gotcha:** The `deriveName()` function signature changed to accept an `isBundle` parameter. This is a breaking change for any external callers, but the function is unexported so it's fine.

**Next:** T2 - Add Progress type and calculation (no dependencies)

---
### Iteration 2: T2 - Add Progress type and calculation
**Completed:**
- Added `Progress` struct with `Total`, `Completed`, and `Percent` fields (`internal/plan/task.go:163-170`)
- Added `CalculateProgress(tasks []Task) Progress` function that uses existing `CountTotal()` and `CountComplete()` (`internal/plan/task.go:172-183`)
- Added `String()` method that returns format like "4/10 (40%)" (`internal/plan/task.go:185-188`)
- Added `Bar(width int)` method that returns visual bar like "[████░░░░░░]" (`internal/plan/task.go:190-200`)
- Added comprehensive tests in `internal/plan/task_test.go`: `TestCalculateProgress`, `TestProgress_String`, `TestProgress_Bar`

**Gotcha:** The Bar() method calculates filled blocks based on `Completed/Total * width`, not `Percent/100 * width`, to ensure accurate representation even for edge cases.

**Next:** T3 - Create bundle scaffolding functions (requires T1, which is complete)

---
### Iteration 3: T3 - Create bundle scaffolding functions
**Completed:**
- Created `internal/plan/bundle.go` with all scaffolding functions
- `CreateBundle(plansDir, name string) (*Plan, error)` creates bundle in `pending/` with sanitized name
- `scaffoldPlan()` creates plan.md with template including: title, status, context, rules, tasks section, discovered, completed sections
- `scaffoldProgress()` creates progress.md with header and format instructions/example
- `scaffoldFeedback()` creates feedback.md with Pending and Processed sections plus instructions
- `sanitizeBundleName()` converts plan names to valid directory names (lowercase, hyphens, no special chars)
- Created `internal/plan/bundle_test.go` with comprehensive tests:
  - `TestCreateBundle`: creates bundle, fails on duplicate, fails on empty/invalid names, sanitizes names
  - `TestScaffoldPlan`, `TestScaffoldProgress`, `TestScaffoldFeedback`: verify file contents
  - `TestSanitizeBundleName`: edge cases for name sanitization
- All 96 tests in plan package pass

**Gotcha:** None - straightforward implementation following existing patterns.

**Next:** T4 - Update progress.go for bundle-aware paths OR T5/T6 (all require T1 which is complete)

---
### Iteration 4: T4, T5, T6 - Bundle-aware paths and queue operations
**Completed:**
- Confirmed T4 (progress.go bundle-aware) was already complete from commit a908cc8
- Confirmed T5 (feedback.go bundle-aware) was already complete from commit fae213d
- Updated plan.md to mark T4 and T5 as complete (plan file was out of sync)
- Implemented T6: Updated `internal/plan/queue.go` to move directories for bundles:
  - Added `planDir(p *Plan) string` helper that returns BundleDir for bundles, Dir(Path) for flat files
  - Added `uniqueCompleteName(name string) string` for date suffix with collision counter (name-YYYYMMDD, name-YYYYMMDD-2, etc.)
  - Updated `Activate()` to move entire bundle directory and update Plan.BundleDir/Path
  - Updated `Complete()` to move bundle with unique dated name
  - Updated `Reset()` to move bundle back to pending
  - Updated `listPlans()` to scan for both bundle directories (containing plan.md) AND legacy flat .md files
- Added comprehensive tests in `internal/plan/queue_test.go`:
  - `TestQueue_Pending_WithBundles` - mix of bundles and flat files
  - `TestQueue_Activate_Bundle` - moving bundle from pending to current
  - `TestQueue_Complete_Bundle` - moving bundle to complete with date suffix
  - `TestQueue_Complete_Bundle_Collision` - collision handling (-2, -3, etc.)
  - `TestQueue_Reset_Bundle` - moving bundle back to pending
  - `TestQueue_FullLifecycle_Bundle` - full activate/reset/complete cycle
  - `TestQueue_UniqueCompleteName` - unit test for date suffix generation
  - `TestPlanDir` - unit test for planDir helper

**Gotcha:** The `listPlans()` function needs to support BOTH bundles and flat files for backwards compatibility during migration. It scans directories for plan.md and also processes standalone .md files.

**Next:** T7 - Simplify worktree sync for bundles (requires T6, which is now complete)

---
### Iteration 5: T7, T8, T9, T10, T11 - Final tasks completion
**Completed:**
- Verified T7 (worktree sync bundle-aware) was already implemented in commit 434f410
  - `SyncToWorktree()` and `SyncFromWorktree()` use `p.Name` for bundle paths
  - Tests exist: `TestSyncToWorktree_Bundle`, `TestSyncFromWorktree_Bundle`, `TestSyncFromWorktree_Bundle_NoFeedbackSync`
- Verified T8 (MigrateToBundles) was already implemented in commit d3a771b
  - Function `MigrateToBundles()` exists in `internal/plan/bundle.go:179`
- Verified T10 (status command progress) was already implemented in commit d79c51b
  - `internal/cli/status.go` uses `CalculateProgress()` and `Progress.Bar()`
- Updated plan.md to mark T7, T8, T10 as complete (plan file was out of sync with code)
- Implemented T9: Created `internal/cli/plan.go` with:
  - `planCmd` parent command for bundle management
  - `planCreateCmd` - calls `plan.CreateBundle()` to scaffold new bundles
  - `planMigrateCmd` - calls `plan.MigrateToBundles()` for legacy migration
  - Comprehensive help text explaining bundle structure and usage
- Implemented T11: Updated `internal/cli/init.go` line 153
  - Changed "Create a plan in plans/pending/" to "Create a plan: ralph plan create <name>"
- All tests pass (including new CLI tests for plan commands)

**Gotcha:** Plan file was significantly out of sync with actual implementation. T7, T8, and T10 were already completed in previous iterations but the plan file still showed them as blocked.

**Next:** Plan complete - all 11 tasks are done
