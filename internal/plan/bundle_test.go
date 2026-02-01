package plan

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCreateBundle(t *testing.T) {
	// Create temp directory for tests
	tmpDir := t.TempDir()
	plansDir := tmpDir

	// Create pending directory
	pendingDir := filepath.Join(plansDir, "pending")
	if err := os.MkdirAll(pendingDir, 0755); err != nil {
		t.Fatalf("failed to create pending dir: %v", err)
	}

	t.Run("creates bundle with all files", func(t *testing.T) {
		plan, err := CreateBundle(plansDir, "my-feature")
		if err != nil {
			t.Fatalf("CreateBundle failed: %v", err)
		}

		// Verify Plan fields
		if plan.Name != "my-feature" {
			t.Errorf("Name = %q, want %q", plan.Name, "my-feature")
		}
		if !plan.IsBundle() {
			t.Error("IsBundle() = false, want true")
		}
		if plan.Status != "pending" {
			t.Errorf("Status = %q, want %q", plan.Status, "pending")
		}
		if plan.Branch != "feat/my-feature" {
			t.Errorf("Branch = %q, want %q", plan.Branch, "feat/my-feature")
		}

		bundleDir := filepath.Join(pendingDir, "my-feature")
		if plan.BundleDir != bundleDir {
			t.Errorf("BundleDir = %q, want %q", plan.BundleDir, bundleDir)
		}

		// Verify files exist
		files := []string{"plan.md", "progress.md", "feedback.md"}
		for _, f := range files {
			path := filepath.Join(bundleDir, f)
			if _, err := os.Stat(path); os.IsNotExist(err) {
				t.Errorf("file %s does not exist", f)
			}
		}
	})

	t.Run("fails on duplicate name", func(t *testing.T) {
		_, err := CreateBundle(plansDir, "my-feature")
		if err == nil {
			t.Error("expected error for duplicate name, got nil")
		}
		if !strings.Contains(err.Error(), "already exists") {
			t.Errorf("error = %q, want to contain 'already exists'", err.Error())
		}
	})

	t.Run("fails on empty name", func(t *testing.T) {
		_, err := CreateBundle(plansDir, "")
		if err == nil {
			t.Error("expected error for empty name, got nil")
		}
		if !strings.Contains(err.Error(), "cannot be empty") {
			t.Errorf("error = %q, want to contain 'cannot be empty'", err.Error())
		}
	})

	t.Run("fails on invalid name", func(t *testing.T) {
		_, err := CreateBundle(plansDir, "!!!")
		if err == nil {
			t.Error("expected error for invalid name, got nil")
		}
		if !strings.Contains(err.Error(), "empty directory name") {
			t.Errorf("error = %q, want to contain 'empty directory name'", err.Error())
		}
	})

	t.Run("sanitizes name with spaces", func(t *testing.T) {
		plan, err := CreateBundle(plansDir, "My New Feature")
		if err != nil {
			t.Fatalf("CreateBundle failed: %v", err)
		}

		// Name in Plan should be the sanitized directory name
		if plan.Name != "my-new-feature" {
			t.Errorf("Name = %q, want %q", plan.Name, "my-new-feature")
		}

		bundleDir := filepath.Join(pendingDir, "my-new-feature")
		if _, err := os.Stat(bundleDir); os.IsNotExist(err) {
			t.Errorf("bundle directory not created at %s", bundleDir)
		}
	})
}

func TestScaffoldPlan(t *testing.T) {
	tmpDir := t.TempDir()

	err := scaffoldPlan(tmpDir, "test-plan")
	if err != nil {
		t.Fatalf("scaffoldPlan failed: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(tmpDir, "plan.md"))
	if err != nil {
		t.Fatalf("failed to read plan.md: %v", err)
	}

	contentStr := string(content)

	// Verify content
	if !strings.Contains(contentStr, "# Plan: test-plan") {
		t.Error("plan.md missing title")
	}
	if !strings.Contains(contentStr, "**Status:** pending") {
		t.Error("plan.md missing status")
	}
	if !strings.Contains(contentStr, "## Tasks") {
		t.Error("plan.md missing Tasks section")
	}
	if !strings.Contains(contentStr, "## Discovered") {
		t.Error("plan.md missing Discovered section")
	}
}

func TestScaffoldProgress(t *testing.T) {
	tmpDir := t.TempDir()

	err := scaffoldProgress(tmpDir, "test-plan")
	if err != nil {
		t.Fatalf("scaffoldProgress failed: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(tmpDir, "progress.md"))
	if err != nil {
		t.Fatalf("failed to read progress.md: %v", err)
	}

	contentStr := string(content)

	// Verify content
	if !strings.Contains(contentStr, "# Progress: test-plan") {
		t.Error("progress.md missing title")
	}
	if !strings.Contains(contentStr, "Iteration log") {
		t.Error("progress.md missing iteration log header")
	}
	if !strings.Contains(contentStr, "### Iteration N") {
		t.Error("progress.md missing format example")
	}
}

func TestScaffoldFeedback(t *testing.T) {
	tmpDir := t.TempDir()

	err := scaffoldFeedback(tmpDir, "test-plan")
	if err != nil {
		t.Fatalf("scaffoldFeedback failed: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(tmpDir, "feedback.md"))
	if err != nil {
		t.Fatalf("failed to read feedback.md: %v", err)
	}

	contentStr := string(content)

	// Verify content
	if !strings.Contains(contentStr, "# Feedback: test-plan") {
		t.Error("feedback.md missing title")
	}
	if !strings.Contains(contentStr, "## Pending") {
		t.Error("feedback.md missing Pending section")
	}
	if !strings.Contains(contentStr, "## Processed") {
		t.Error("feedback.md missing Processed section")
	}
}

func TestSanitizeBundleName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"my-feature", "my-feature"},
		{"My Feature", "my-feature"},
		{"my_feature", "my-feature"},
		{"My New Feature", "my-new-feature"},
		{"feature!!!", "feature"},
		{"  spaces  ", "spaces"},
		{"v2.0-release", "v2.0-release"},
		{"UPPERCASE", "uppercase"},
		{"mix--hyphens", "mix-hyphens"},
		{"-leading-trailing-", "leading-trailing"},
		{"!!!", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := sanitizeBundleName(tt.input)
			if got != tt.want {
				t.Errorf("sanitizeBundleName(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
