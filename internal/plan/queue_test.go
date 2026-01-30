package plan

import (
	"os"
	"path/filepath"
	"testing"
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
