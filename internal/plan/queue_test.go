package plan

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// createTestQueue sets up a temporary queue directory structure.
func createTestQueue(t *testing.T) (string, func()) {
	t.Helper()

	tmpDir, err := os.MkdirTemp("", "queue-test-*")
	if err != nil {
		t.Fatalf("creating temp dir: %v", err)
	}

	// Create queue subdirectories
	for _, sub := range []string{"pending", "current", "complete"} {
		if err := os.MkdirAll(filepath.Join(tmpDir, sub), 0755); err != nil {
			os.RemoveAll(tmpDir)
			t.Fatalf("creating %s dir: %v", sub, err)
		}
	}

	cleanup := func() {
		os.RemoveAll(tmpDir)
	}

	return tmpDir, cleanup
}

// createTestPlanFile creates a plan file with the given name in the specified directory.
func createTestPlanFile(t *testing.T, dir, name string) string {
	t.Helper()

	content := `# Plan: ` + name + `

**Status:** pending

## Tasks

- [ ] Task 1
- [ ] Task 2
`
	path := filepath.Join(dir, name+".md")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("creating test plan %s: %v", name, err)
	}
	return path
}

func TestNewQueue(t *testing.T) {
	q := NewQueue("/some/path")
	if q.BaseDir != "/some/path" {
		t.Errorf("expected BaseDir /some/path, got %s", q.BaseDir)
	}
}

func TestQueue_Pending(t *testing.T) {
	tmpDir, cleanup := createTestQueue(t)
	defer cleanup()

	q := NewQueue(tmpDir)

	// Empty queue
	plans, err := q.Pending()
	if err != nil {
		t.Fatalf("listing pending: %v", err)
	}
	if len(plans) != 0 {
		t.Errorf("expected 0 plans, got %d", len(plans))
	}

	// Add some plans
	createTestPlanFile(t, q.pendingDir(), "plan-b")
	createTestPlanFile(t, q.pendingDir(), "plan-a")
	createTestPlanFile(t, q.pendingDir(), "plan-c")

	plans, err = q.Pending()
	if err != nil {
		t.Fatalf("listing pending: %v", err)
	}
	if len(plans) != 3 {
		t.Errorf("expected 3 plans, got %d", len(plans))
	}

	// Verify sorted by name
	expectedOrder := []string{"plan-a", "plan-b", "plan-c"}
	for i, p := range plans {
		if p.Name != expectedOrder[i] {
			t.Errorf("plan %d: expected %s, got %s", i, expectedOrder[i], p.Name)
		}
	}
}

func TestQueue_Pending_SkipsNonMdFiles(t *testing.T) {
	tmpDir, cleanup := createTestQueue(t)
	defer cleanup()

	q := NewQueue(tmpDir)

	// Create a .md file and some non-.md files
	createTestPlanFile(t, q.pendingDir(), "real-plan")
	os.WriteFile(filepath.Join(q.pendingDir(), "readme.txt"), []byte("not a plan"), 0644)
	os.WriteFile(filepath.Join(q.pendingDir(), "notes"), []byte("also not a plan"), 0644)

	plans, err := q.Pending()
	if err != nil {
		t.Fatalf("listing pending: %v", err)
	}
	if len(plans) != 1 {
		t.Errorf("expected 1 plan, got %d", len(plans))
	}
	if plans[0].Name != "real-plan" {
		t.Errorf("expected real-plan, got %s", plans[0].Name)
	}
}

func TestQueue_Pending_SkipsProgressAndFeedback(t *testing.T) {
	tmpDir, cleanup := createTestQueue(t)
	defer cleanup()

	q := NewQueue(tmpDir)

	// Create plan and associated files
	createTestPlanFile(t, q.pendingDir(), "my-plan")
	os.WriteFile(filepath.Join(q.pendingDir(), "my-plan.progress.md"), []byte("progress"), 0644)
	os.WriteFile(filepath.Join(q.pendingDir(), "my-plan.feedback.md"), []byte("feedback"), 0644)

	plans, err := q.Pending()
	if err != nil {
		t.Fatalf("listing pending: %v", err)
	}
	if len(plans) != 1 {
		t.Errorf("expected 1 plan, got %d", len(plans))
	}
}

func TestQueue_Current_Empty(t *testing.T) {
	tmpDir, cleanup := createTestQueue(t)
	defer cleanup()

	q := NewQueue(tmpDir)

	current, err := q.Current()
	if err != nil {
		t.Fatalf("getting current: %v", err)
	}
	if current != nil {
		t.Errorf("expected nil current, got %v", current)
	}
}

func TestQueue_Current_WithPlan(t *testing.T) {
	tmpDir, cleanup := createTestQueue(t)
	defer cleanup()

	q := NewQueue(tmpDir)

	createTestPlanFile(t, q.currentDir(), "active-plan")

	current, err := q.Current()
	if err != nil {
		t.Fatalf("getting current: %v", err)
	}
	if current == nil {
		t.Fatal("expected current plan, got nil")
	}
	if current.Name != "active-plan" {
		t.Errorf("expected active-plan, got %s", current.Name)
	}
}

func TestQueue_Activate(t *testing.T) {
	tmpDir, cleanup := createTestQueue(t)
	defer cleanup()

	q := NewQueue(tmpDir)

	// Create a pending plan
	planPath := createTestPlanFile(t, q.pendingDir(), "to-activate")
	plan, err := Load(planPath)
	if err != nil {
		t.Fatalf("loading plan: %v", err)
	}

	// Activate it
	if err := q.Activate(plan); err != nil {
		t.Fatalf("activating plan: %v", err)
	}

	// Verify it moved
	if _, err := os.Stat(planPath); !os.IsNotExist(err) {
		t.Error("plan file still exists in pending")
	}

	expectedNewPath := filepath.Join(q.currentDir(), "to-activate.md")
	if _, err := os.Stat(expectedNewPath); err != nil {
		t.Errorf("plan file not in current: %v", err)
	}

	// Plan's path should be updated
	if plan.Path != expectedNewPath {
		t.Errorf("plan path not updated: expected %s, got %s", expectedNewPath, plan.Path)
	}
}

func TestQueue_Activate_QueueFull(t *testing.T) {
	tmpDir, cleanup := createTestQueue(t)
	defer cleanup()

	q := NewQueue(tmpDir)

	// Create a current plan
	createTestPlanFile(t, q.currentDir(), "already-active")

	// Create a pending plan
	planPath := createTestPlanFile(t, q.pendingDir(), "waiting")
	plan, err := Load(planPath)
	if err != nil {
		t.Fatalf("loading plan: %v", err)
	}

	// Try to activate - should fail
	err = q.Activate(plan)
	if err != ErrQueueFull {
		t.Errorf("expected ErrQueueFull, got %v", err)
	}
}

func TestQueue_Activate_NotInPending(t *testing.T) {
	tmpDir, cleanup := createTestQueue(t)
	defer cleanup()

	q := NewQueue(tmpDir)

	// Create a plan in complete/ (not pending/)
	planPath := createTestPlanFile(t, q.completeDir(), "already-done")
	plan, err := Load(planPath)
	if err != nil {
		t.Fatalf("loading plan: %v", err)
	}

	// Try to activate - should fail
	err = q.Activate(plan)
	if err != ErrPlanNotInPending {
		t.Errorf("expected ErrPlanNotInPending, got %v", err)
	}
}

func TestQueue_Complete(t *testing.T) {
	tmpDir, cleanup := createTestQueue(t)
	defer cleanup()

	q := NewQueue(tmpDir)

	// Create a current plan
	planPath := createTestPlanFile(t, q.currentDir(), "finishing")
	plan, err := Load(planPath)
	if err != nil {
		t.Fatalf("loading plan: %v", err)
	}

	// Complete it
	if err := q.Complete(plan); err != nil {
		t.Fatalf("completing plan: %v", err)
	}

	// Verify it moved
	if _, err := os.Stat(planPath); !os.IsNotExist(err) {
		t.Error("plan file still exists in current")
	}

	expectedNewPath := filepath.Join(q.completeDir(), "finishing.md")
	if _, err := os.Stat(expectedNewPath); err != nil {
		t.Errorf("plan file not in complete: %v", err)
	}

	// Plan's path should be updated
	if plan.Path != expectedNewPath {
		t.Errorf("plan path not updated: expected %s, got %s", expectedNewPath, plan.Path)
	}
}

func TestQueue_Complete_NotInCurrent(t *testing.T) {
	tmpDir, cleanup := createTestQueue(t)
	defer cleanup()

	q := NewQueue(tmpDir)

	// Create a plan in pending/ (not current/)
	planPath := createTestPlanFile(t, q.pendingDir(), "still-pending")
	plan, err := Load(planPath)
	if err != nil {
		t.Fatalf("loading plan: %v", err)
	}

	// Try to complete - should fail
	err = q.Complete(plan)
	if err != ErrPlanNotInCurrent {
		t.Errorf("expected ErrPlanNotInCurrent, got %v", err)
	}
}

func TestQueue_Reset(t *testing.T) {
	tmpDir, cleanup := createTestQueue(t)
	defer cleanup()

	q := NewQueue(tmpDir)

	// Create a current plan
	planPath := createTestPlanFile(t, q.currentDir(), "resetting")
	plan, err := Load(planPath)
	if err != nil {
		t.Fatalf("loading plan: %v", err)
	}

	// Reset it
	if err := q.Reset(plan); err != nil {
		t.Fatalf("resetting plan: %v", err)
	}

	// Verify it moved back to pending
	if _, err := os.Stat(planPath); !os.IsNotExist(err) {
		t.Error("plan file still exists in current")
	}

	expectedNewPath := filepath.Join(q.pendingDir(), "resetting.md")
	if _, err := os.Stat(expectedNewPath); err != nil {
		t.Errorf("plan file not in pending: %v", err)
	}

	// Plan's path should be updated
	if plan.Path != expectedNewPath {
		t.Errorf("plan path not updated: expected %s, got %s", expectedNewPath, plan.Path)
	}
}

func TestQueue_Reset_NotInCurrent(t *testing.T) {
	tmpDir, cleanup := createTestQueue(t)
	defer cleanup()

	q := NewQueue(tmpDir)

	// Create a plan in complete/ (not current/)
	planPath := createTestPlanFile(t, q.completeDir(), "already-done")
	plan, err := Load(planPath)
	if err != nil {
		t.Fatalf("loading plan: %v", err)
	}

	// Try to reset - should fail
	err = q.Reset(plan)
	if err != ErrPlanNotInCurrent {
		t.Errorf("expected ErrPlanNotInCurrent, got %v", err)
	}
}

func TestQueue_Status(t *testing.T) {
	tmpDir, cleanup := createTestQueue(t)
	defer cleanup()

	q := NewQueue(tmpDir)

	// Empty queue
	status, err := q.Status()
	if err != nil {
		t.Fatalf("getting status: %v", err)
	}
	if status.PendingCount != 0 {
		t.Errorf("expected 0 pending, got %d", status.PendingCount)
	}
	if status.CurrentCount != 0 {
		t.Errorf("expected 0 current, got %d", status.CurrentCount)
	}
	if status.CompleteCount != 0 {
		t.Errorf("expected 0 complete, got %d", status.CompleteCount)
	}

	// Add plans to each queue
	createTestPlanFile(t, q.pendingDir(), "pending-1")
	createTestPlanFile(t, q.pendingDir(), "pending-2")
	createTestPlanFile(t, q.currentDir(), "current-1")
	createTestPlanFile(t, q.completeDir(), "complete-1")
	createTestPlanFile(t, q.completeDir(), "complete-2")
	createTestPlanFile(t, q.completeDir(), "complete-3")

	status, err = q.Status()
	if err != nil {
		t.Fatalf("getting status: %v", err)
	}
	if status.PendingCount != 2 {
		t.Errorf("expected 2 pending, got %d", status.PendingCount)
	}
	if status.CurrentCount != 1 {
		t.Errorf("expected 1 current, got %d", status.CurrentCount)
	}
	if status.CompleteCount != 3 {
		t.Errorf("expected 3 complete, got %d", status.CompleteCount)
	}

	// Verify pending plan names
	if len(status.PendingPlans) != 2 {
		t.Errorf("expected 2 pending plan names, got %d", len(status.PendingPlans))
	}

	// Verify current plan name
	if status.CurrentPlan != "current-1" {
		t.Errorf("expected current-1, got %s", status.CurrentPlan)
	}
}

func TestQueue_FullLifecycle(t *testing.T) {
	tmpDir, cleanup := createTestQueue(t)
	defer cleanup()

	q := NewQueue(tmpDir)

	// Create a pending plan
	planPath := createTestPlanFile(t, q.pendingDir(), "lifecycle-test")
	plan, err := Load(planPath)
	if err != nil {
		t.Fatalf("loading plan: %v", err)
	}

	// Verify initial state
	status, _ := q.Status()
	if status.PendingCount != 1 || status.CurrentCount != 0 || status.CompleteCount != 0 {
		t.Errorf("unexpected initial state: pending=%d, current=%d, complete=%d",
			status.PendingCount, status.CurrentCount, status.CompleteCount)
	}

	// Activate
	if err := q.Activate(plan); err != nil {
		t.Fatalf("activate: %v", err)
	}
	status, _ = q.Status()
	if status.PendingCount != 0 || status.CurrentCount != 1 || status.CompleteCount != 0 {
		t.Errorf("unexpected after activate: pending=%d, current=%d, complete=%d",
			status.PendingCount, status.CurrentCount, status.CompleteCount)
	}

	// Reset
	if err := q.Reset(plan); err != nil {
		t.Fatalf("reset: %v", err)
	}
	status, _ = q.Status()
	if status.PendingCount != 1 || status.CurrentCount != 0 || status.CompleteCount != 0 {
		t.Errorf("unexpected after reset: pending=%d, current=%d, complete=%d",
			status.PendingCount, status.CurrentCount, status.CompleteCount)
	}

	// Activate again
	if err := q.Activate(plan); err != nil {
		t.Fatalf("activate again: %v", err)
	}

	// Complete
	if err := q.Complete(plan); err != nil {
		t.Fatalf("complete: %v", err)
	}
	status, _ = q.Status()
	if status.PendingCount != 0 || status.CurrentCount != 0 || status.CompleteCount != 1 {
		t.Errorf("unexpected after complete: pending=%d, current=%d, complete=%d",
			status.PendingCount, status.CurrentCount, status.CompleteCount)
	}
}

func TestQueue_NonExistentDirectory(t *testing.T) {
	q := NewQueue("/non/existent/path")

	// Should return empty, not error
	plans, err := q.Pending()
	if err != nil {
		t.Errorf("expected nil error for non-existent pending, got %v", err)
	}
	if len(plans) != 0 {
		t.Errorf("expected 0 plans, got %d", len(plans))
	}
}

// createTestBundle creates a bundle directory with plan.md in the specified directory.
func createTestBundle(t *testing.T, dir, name string) string {
	t.Helper()

	bundleDir := filepath.Join(dir, name)
	if err := os.MkdirAll(bundleDir, 0755); err != nil {
		t.Fatalf("creating bundle dir %s: %v", name, err)
	}

	content := `# Plan: ` + name + `

**Status:** pending

## Tasks

- [ ] Task 1
- [ ] Task 2
`
	planPath := filepath.Join(bundleDir, "plan.md")
	if err := os.WriteFile(planPath, []byte(content), 0644); err != nil {
		t.Fatalf("creating test plan.md in bundle %s: %v", name, err)
	}
	return bundleDir
}

func TestQueue_Pending_WithBundles(t *testing.T) {
	tmpDir, cleanup := createTestQueue(t)
	defer cleanup()

	q := NewQueue(tmpDir)

	// Create mix of bundles and flat files
	createTestBundle(t, q.pendingDir(), "bundle-a")
	createTestBundle(t, q.pendingDir(), "bundle-b")
	createTestPlanFile(t, q.pendingDir(), "flat-plan")

	plans, err := q.Pending()
	if err != nil {
		t.Fatalf("listing pending: %v", err)
	}
	if len(plans) != 3 {
		t.Errorf("expected 3 plans, got %d", len(plans))
	}

	// Verify sorted by name and correct types
	expectedOrder := []string{"bundle-a", "bundle-b", "flat-plan"}
	expectedBundle := []bool{true, true, false}
	for i, p := range plans {
		if p.Name != expectedOrder[i] {
			t.Errorf("plan %d: expected name %s, got %s", i, expectedOrder[i], p.Name)
		}
		if p.IsBundle() != expectedBundle[i] {
			t.Errorf("plan %d (%s): expected IsBundle=%v, got %v", i, p.Name, expectedBundle[i], p.IsBundle())
		}
	}
}

func TestQueue_Activate_Bundle(t *testing.T) {
	tmpDir, cleanup := createTestQueue(t)
	defer cleanup()

	q := NewQueue(tmpDir)

	// Create a pending bundle
	bundleDir := createTestBundle(t, q.pendingDir(), "my-bundle")
	plan, err := Load(bundleDir)
	if err != nil {
		t.Fatalf("loading bundle: %v", err)
	}

	if !plan.IsBundle() {
		t.Fatal("expected plan to be a bundle")
	}

	// Activate it
	if err := q.Activate(plan); err != nil {
		t.Fatalf("activating bundle: %v", err)
	}

	// Verify bundle moved
	if _, err := os.Stat(bundleDir); !os.IsNotExist(err) {
		t.Error("bundle still exists in pending")
	}

	expectedNewBundleDir := filepath.Join(q.currentDir(), "my-bundle")
	if _, err := os.Stat(expectedNewBundleDir); err != nil {
		t.Errorf("bundle not in current: %v", err)
	}

	// Verify plan.md exists in new location
	expectedNewPath := filepath.Join(expectedNewBundleDir, "plan.md")
	if _, err := os.Stat(expectedNewPath); err != nil {
		t.Errorf("plan.md not in current bundle: %v", err)
	}

	// Plan's paths should be updated
	if plan.BundleDir != expectedNewBundleDir {
		t.Errorf("BundleDir not updated: expected %s, got %s", expectedNewBundleDir, plan.BundleDir)
	}
	if plan.Path != expectedNewPath {
		t.Errorf("Path not updated: expected %s, got %s", expectedNewPath, plan.Path)
	}
}

func TestQueue_Complete_Bundle(t *testing.T) {
	tmpDir, cleanup := createTestQueue(t)
	defer cleanup()

	q := NewQueue(tmpDir)

	// Create a current bundle
	bundleDir := createTestBundle(t, q.currentDir(), "finishing-bundle")
	plan, err := Load(bundleDir)
	if err != nil {
		t.Fatalf("loading bundle: %v", err)
	}

	// Complete it
	if err := q.Complete(plan); err != nil {
		t.Fatalf("completing bundle: %v", err)
	}

	// Verify bundle moved with date suffix
	if _, err := os.Stat(bundleDir); !os.IsNotExist(err) {
		t.Error("bundle still exists in current")
	}

	// Check that it's in complete/ with a date suffix
	expectedDate := time.Now().Format("20060102")
	expectedNewBundleDir := filepath.Join(q.completeDir(), "finishing-bundle-"+expectedDate)
	if _, err := os.Stat(expectedNewBundleDir); err != nil {
		t.Errorf("bundle not in complete with date suffix: %v", err)
	}

	// Plan's paths should be updated
	if plan.BundleDir != expectedNewBundleDir {
		t.Errorf("BundleDir not updated: expected %s, got %s", expectedNewBundleDir, plan.BundleDir)
	}
}

func TestQueue_Complete_Bundle_Collision(t *testing.T) {
	tmpDir, cleanup := createTestQueue(t)
	defer cleanup()

	q := NewQueue(tmpDir)

	expectedDate := time.Now().Format("20060102")

	// Pre-create existing completions with same date
	existingDir1 := filepath.Join(q.completeDir(), "test-bundle-"+expectedDate)
	existingDir2 := filepath.Join(q.completeDir(), "test-bundle-"+expectedDate+"-2")
	os.MkdirAll(existingDir1, 0755)
	os.MkdirAll(existingDir2, 0755)

	// Create a current bundle with same name
	bundleDir := createTestBundle(t, q.currentDir(), "test-bundle")
	plan, err := Load(bundleDir)
	if err != nil {
		t.Fatalf("loading bundle: %v", err)
	}

	// Complete it
	if err := q.Complete(plan); err != nil {
		t.Fatalf("completing bundle: %v", err)
	}

	// Should get suffix -3 due to collisions
	expectedNewBundleDir := filepath.Join(q.completeDir(), "test-bundle-"+expectedDate+"-3")
	if _, err := os.Stat(expectedNewBundleDir); err != nil {
		t.Errorf("bundle not in complete with collision suffix: %v", err)
	}

	if plan.BundleDir != expectedNewBundleDir {
		t.Errorf("BundleDir not updated: expected %s, got %s", expectedNewBundleDir, plan.BundleDir)
	}
}

func TestQueue_Reset_Bundle(t *testing.T) {
	tmpDir, cleanup := createTestQueue(t)
	defer cleanup()

	q := NewQueue(tmpDir)

	// Create a current bundle
	bundleDir := createTestBundle(t, q.currentDir(), "resetting-bundle")
	plan, err := Load(bundleDir)
	if err != nil {
		t.Fatalf("loading bundle: %v", err)
	}

	// Reset it
	if err := q.Reset(plan); err != nil {
		t.Fatalf("resetting bundle: %v", err)
	}

	// Verify bundle moved back to pending
	if _, err := os.Stat(bundleDir); !os.IsNotExist(err) {
		t.Error("bundle still exists in current")
	}

	expectedNewBundleDir := filepath.Join(q.pendingDir(), "resetting-bundle")
	if _, err := os.Stat(expectedNewBundleDir); err != nil {
		t.Errorf("bundle not in pending: %v", err)
	}

	// Plan's paths should be updated
	if plan.BundleDir != expectedNewBundleDir {
		t.Errorf("BundleDir not updated: expected %s, got %s", expectedNewBundleDir, plan.BundleDir)
	}
}

func TestQueue_FullLifecycle_Bundle(t *testing.T) {
	tmpDir, cleanup := createTestQueue(t)
	defer cleanup()

	q := NewQueue(tmpDir)

	// Create a pending bundle
	bundleDir := createTestBundle(t, q.pendingDir(), "lifecycle-bundle")
	plan, err := Load(bundleDir)
	if err != nil {
		t.Fatalf("loading bundle: %v", err)
	}

	// Verify initial state
	status, _ := q.Status()
	if status.PendingCount != 1 || status.CurrentCount != 0 || status.CompleteCount != 0 {
		t.Errorf("unexpected initial state: pending=%d, current=%d, complete=%d",
			status.PendingCount, status.CurrentCount, status.CompleteCount)
	}

	// Activate
	if err := q.Activate(plan); err != nil {
		t.Fatalf("activate: %v", err)
	}
	status, _ = q.Status()
	if status.PendingCount != 0 || status.CurrentCount != 1 || status.CompleteCount != 0 {
		t.Errorf("unexpected after activate: pending=%d, current=%d, complete=%d",
			status.PendingCount, status.CurrentCount, status.CompleteCount)
	}
	if status.CurrentPlan != "lifecycle-bundle" {
		t.Errorf("expected current plan lifecycle-bundle, got %s", status.CurrentPlan)
	}

	// Reset
	if err := q.Reset(plan); err != nil {
		t.Fatalf("reset: %v", err)
	}
	status, _ = q.Status()
	if status.PendingCount != 1 || status.CurrentCount != 0 || status.CompleteCount != 0 {
		t.Errorf("unexpected after reset: pending=%d, current=%d, complete=%d",
			status.PendingCount, status.CurrentCount, status.CompleteCount)
	}

	// Activate again
	if err := q.Activate(plan); err != nil {
		t.Fatalf("activate again: %v", err)
	}

	// Complete
	if err := q.Complete(plan); err != nil {
		t.Fatalf("complete: %v", err)
	}
	status, _ = q.Status()
	if status.PendingCount != 0 || status.CurrentCount != 0 || status.CompleteCount != 1 {
		t.Errorf("unexpected after complete: pending=%d, current=%d, complete=%d",
			status.PendingCount, status.CurrentCount, status.CompleteCount)
	}

	// Verify bundle has date suffix in complete/
	expectedDate := time.Now().Format("20060102")
	if !strings.Contains(plan.BundleDir, expectedDate) {
		t.Errorf("completed bundle should have date suffix, got %s", plan.BundleDir)
	}
}

func TestQueue_UniqueCompleteName(t *testing.T) {
	tmpDir, cleanup := createTestQueue(t)
	defer cleanup()

	q := NewQueue(tmpDir)
	expectedDate := time.Now().Format("20060102")

	// First call should return base name
	name1 := q.uniqueCompleteName("my-plan")
	expected1 := "my-plan-" + expectedDate
	if name1 != expected1 {
		t.Errorf("expected %s, got %s", expected1, name1)
	}

	// Create that directory
	os.MkdirAll(filepath.Join(q.completeDir(), name1), 0755)

	// Second call should return with -2 suffix
	name2 := q.uniqueCompleteName("my-plan")
	expected2 := "my-plan-" + expectedDate + "-2"
	if name2 != expected2 {
		t.Errorf("expected %s, got %s", expected2, name2)
	}

	// Create that too
	os.MkdirAll(filepath.Join(q.completeDir(), name2), 0755)

	// Third call should return with -3 suffix
	name3 := q.uniqueCompleteName("my-plan")
	expected3 := "my-plan-" + expectedDate + "-3"
	if name3 != expected3 {
		t.Errorf("expected %s, got %s", expected3, name3)
	}
}

func TestPlanDir(t *testing.T) {
	// Test with bundle
	bundlePlan := &Plan{
		Path:      "/plans/current/my-bundle/plan.md",
		BundleDir: "/plans/current/my-bundle",
	}
	if got := planDir(bundlePlan); got != "/plans/current/my-bundle" {
		t.Errorf("planDir(bundle) = %s, want /plans/current/my-bundle", got)
	}

	// Test with flat file
	flatPlan := &Plan{
		Path: "/plans/current/my-plan.md",
	}
	if got := planDir(flatPlan); got != "/plans/current" {
		t.Errorf("planDir(flat) = %s, want /plans/current", got)
	}
}
