package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunStatus_NoPlanDirectory(t *testing.T) {
	// Create temp directory without plans/
	tmpDir := t.TempDir()
	oldWd, _ := os.Getwd()
	defer os.Chdir(oldWd)
	os.Chdir(tmpDir)

	// Capture output
	var buf bytes.Buffer
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := runStatus(nil, nil)

	w.Close()
	buf.ReadFrom(r)
	os.Stdout = oldStdout

	if err != nil {
		t.Errorf("expected no error, got: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "No plans directory found") {
		t.Errorf("expected 'No plans directory found' message, got: %s", output)
	}
}

func TestRunStatus_EmptyQueue(t *testing.T) {
	// Create temp directory with empty plans structure
	tmpDir := t.TempDir()
	os.MkdirAll(filepath.Join(tmpDir, "plans", "pending"), 0755)
	os.MkdirAll(filepath.Join(tmpDir, "plans", "current"), 0755)
	os.MkdirAll(filepath.Join(tmpDir, "plans", "complete"), 0755)

	oldWd, _ := os.Getwd()
	defer os.Chdir(oldWd)
	os.Chdir(tmpDir)

	// Capture output
	var buf bytes.Buffer
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := runStatus(nil, nil)

	w.Close()
	buf.ReadFrom(r)
	os.Stdout = oldStdout

	if err != nil {
		t.Errorf("expected no error, got: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "Queue Status") {
		t.Errorf("expected 'Queue Status' header, got: %s", output)
	}
	if !strings.Contains(output, "Current: (none)") && !strings.Contains(output, "Current:\x1b[0m (none)") {
		// Account for both colored and non-colored output
		if !strings.Contains(output, "(none)") {
			t.Errorf("expected '(none)' for current plan, got: %s", output)
		}
	}
	if !strings.Contains(output, "Pending: 0 plan(s)") && !strings.Contains(output, "0 plan(s)") {
		t.Errorf("expected '0 plan(s)' for pending, got: %s", output)
	}
}

func TestRunStatus_WithCurrentPlan(t *testing.T) {
	// Create temp directory with a current plan
	tmpDir := t.TempDir()
	os.MkdirAll(filepath.Join(tmpDir, "plans", "pending"), 0755)
	os.MkdirAll(filepath.Join(tmpDir, "plans", "current"), 0755)
	os.MkdirAll(filepath.Join(tmpDir, "plans", "complete"), 0755)

	// Create a plan in current/
	planContent := `# Plan: Test Plan
**Status:** open

## Tasks
- [ ] Task 1
`
	os.WriteFile(filepath.Join(tmpDir, "plans", "current", "test-plan.md"), []byte(planContent), 0644)

	oldWd, _ := os.Getwd()
	defer os.Chdir(oldWd)
	os.Chdir(tmpDir)

	// Capture output
	var buf bytes.Buffer
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := runStatus(nil, nil)

	w.Close()
	buf.ReadFrom(r)
	os.Stdout = oldStdout

	if err != nil {
		t.Errorf("expected no error, got: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "test-plan") {
		t.Errorf("expected 'test-plan' in output, got: %s", output)
	}
	if !strings.Contains(output, "feat/test-plan") {
		t.Errorf("expected 'feat/test-plan' branch in output, got: %s", output)
	}
}

func TestRunStatus_WithPendingPlans(t *testing.T) {
	// Create temp directory with pending plans
	tmpDir := t.TempDir()
	os.MkdirAll(filepath.Join(tmpDir, "plans", "pending"), 0755)
	os.MkdirAll(filepath.Join(tmpDir, "plans", "current"), 0755)
	os.MkdirAll(filepath.Join(tmpDir, "plans", "complete"), 0755)

	// Create plans in pending/
	planContent := `# Plan: Test
**Status:** pending
`
	os.WriteFile(filepath.Join(tmpDir, "plans", "pending", "alpha.md"), []byte(planContent), 0644)
	os.WriteFile(filepath.Join(tmpDir, "plans", "pending", "beta.md"), []byte(planContent), 0644)

	oldWd, _ := os.Getwd()
	defer os.Chdir(oldWd)
	os.Chdir(tmpDir)

	// Capture output
	var buf bytes.Buffer
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := runStatus(nil, nil)

	w.Close()
	buf.ReadFrom(r)
	os.Stdout = oldStdout

	if err != nil {
		t.Errorf("expected no error, got: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "2 plan(s)") {
		t.Errorf("expected '2 plan(s)' for pending count, got: %s", output)
	}
	if !strings.Contains(output, "alpha") {
		t.Errorf("expected 'alpha' plan listed, got: %s", output)
	}
	if !strings.Contains(output, "beta") {
		t.Errorf("expected 'beta' plan listed, got: %s", output)
	}
}

func TestRunStatus_OutputFormat(t *testing.T) {
	// Create temp directory with full queue
	tmpDir := t.TempDir()
	os.MkdirAll(filepath.Join(tmpDir, "plans", "pending"), 0755)
	os.MkdirAll(filepath.Join(tmpDir, "plans", "current"), 0755)
	os.MkdirAll(filepath.Join(tmpDir, "plans", "complete"), 0755)

	planContent := `# Plan: Test
**Status:** pending
`
	// 1 pending
	os.WriteFile(filepath.Join(tmpDir, "plans", "pending", "pending-plan.md"), []byte(planContent), 0644)
	// 1 current
	os.WriteFile(filepath.Join(tmpDir, "plans", "current", "current-plan.md"), []byte(planContent), 0644)
	// 2 complete
	os.WriteFile(filepath.Join(tmpDir, "plans", "complete", "done1.md"), []byte(planContent), 0644)
	os.WriteFile(filepath.Join(tmpDir, "plans", "complete", "done2.md"), []byte(planContent), 0644)

	oldWd, _ := os.Getwd()
	defer os.Chdir(oldWd)
	os.Chdir(tmpDir)

	// Capture output
	var buf bytes.Buffer
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := runStatus(nil, nil)

	w.Close()
	buf.ReadFrom(r)
	os.Stdout = oldStdout

	if err != nil {
		t.Errorf("expected no error, got: %v", err)
	}

	output := buf.String()

	// Check all sections present
	if !strings.Contains(output, "Queue Status") {
		t.Errorf("missing 'Queue Status' header")
	}
	if !strings.Contains(output, "Current:") {
		t.Errorf("missing 'Current:' section")
	}
	if !strings.Contains(output, "Pending:") {
		t.Errorf("missing 'Pending:' section")
	}
	if !strings.Contains(output, "Complete:") {
		t.Errorf("missing 'Complete:' section")
	}
	if !strings.Contains(output, "Worktrees") {
		t.Errorf("missing 'Worktrees' section")
	}

	// Check counts
	if !strings.Contains(output, "current-plan") {
		t.Errorf("missing current plan name")
	}
	if !strings.Contains(output, "1 plan(s)") {
		t.Errorf("missing '1 plan(s)' for pending")
	}
	if !strings.Contains(output, "2 plan(s)") {
		t.Errorf("missing '2 plan(s)' for complete")
	}
}

func TestRunStatus_ExitCode(t *testing.T) {
	// Create temp directory with valid structure
	tmpDir := t.TempDir()
	os.MkdirAll(filepath.Join(tmpDir, "plans", "pending"), 0755)
	os.MkdirAll(filepath.Join(tmpDir, "plans", "current"), 0755)
	os.MkdirAll(filepath.Join(tmpDir, "plans", "complete"), 0755)

	oldWd, _ := os.Getwd()
	defer os.Chdir(oldWd)
	os.Chdir(tmpDir)

	// Redirect stdout to suppress output
	oldStdout := os.Stdout
	_, w, _ := os.Pipe()
	os.Stdout = w

	err := runStatus(nil, nil)

	w.Close()
	os.Stdout = oldStdout

	// Should return nil (exit code 0)
	if err != nil {
		t.Errorf("expected exit code 0 (nil error), got error: %v", err)
	}
}
