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
