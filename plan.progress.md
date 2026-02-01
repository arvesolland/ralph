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
