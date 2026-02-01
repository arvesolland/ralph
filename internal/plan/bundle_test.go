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

func TestMigrateToBundles(t *testing.T) {
	t.Run("migrates flat files to bundles", func(t *testing.T) {
		tmpDir := t.TempDir()
		pendingDir := filepath.Join(tmpDir, "pending")
		os.MkdirAll(pendingDir, 0755)

		// Create a flat plan file
		planContent := "# Plan: test-plan\n\n**Status:** pending\n"
		os.WriteFile(filepath.Join(pendingDir, "test-plan.md"), []byte(planContent), 0644)

		// Create associated progress file
		progressContent := "# Progress\n\nSome progress\n"
		os.WriteFile(filepath.Join(pendingDir, "test-plan.progress.md"), []byte(progressContent), 0644)

		// Create associated feedback file
		feedbackContent := "# Feedback\n\n## Pending\n\n## Processed\n"
		os.WriteFile(filepath.Join(pendingDir, "test-plan.feedback.md"), []byte(feedbackContent), 0644)

		// Migrate
		err := MigrateToBundles(tmpDir)
		if err != nil {
			t.Fatalf("MigrateToBundles failed: %v", err)
		}

		// Verify bundle directory was created
		bundleDir := filepath.Join(pendingDir, "test-plan")
		if _, err := os.Stat(bundleDir); os.IsNotExist(err) {
			t.Error("bundle directory was not created")
		}

		// Verify files are in bundle
		if _, err := os.Stat(filepath.Join(bundleDir, "plan.md")); os.IsNotExist(err) {
			t.Error("plan.md not found in bundle")
		}
		if _, err := os.Stat(filepath.Join(bundleDir, "progress.md")); os.IsNotExist(err) {
			t.Error("progress.md not found in bundle")
		}
		if _, err := os.Stat(filepath.Join(bundleDir, "feedback.md")); os.IsNotExist(err) {
			t.Error("feedback.md not found in bundle")
		}

		// Verify original files are gone
		if _, err := os.Stat(filepath.Join(pendingDir, "test-plan.md")); !os.IsNotExist(err) {
			t.Error("original plan file still exists")
		}
		if _, err := os.Stat(filepath.Join(pendingDir, "test-plan.progress.md")); !os.IsNotExist(err) {
			t.Error("original progress file still exists")
		}
		if _, err := os.Stat(filepath.Join(pendingDir, "test-plan.feedback.md")); !os.IsNotExist(err) {
			t.Error("original feedback file still exists")
		}

		// Verify contents were preserved
		content, _ := os.ReadFile(filepath.Join(bundleDir, "plan.md"))
		if string(content) != planContent {
			t.Errorf("plan.md content was modified: got %q, want %q", string(content), planContent)
		}
		content, _ = os.ReadFile(filepath.Join(bundleDir, "progress.md"))
		if string(content) != progressContent {
			t.Errorf("progress.md content was modified: got %q, want %q", string(content), progressContent)
		}
		content, _ = os.ReadFile(filepath.Join(bundleDir, "feedback.md"))
		if string(content) != feedbackContent {
			t.Errorf("feedback.md content was modified: got %q, want %q", string(content), feedbackContent)
		}
	})

	t.Run("creates scaffolded files when missing", func(t *testing.T) {
		tmpDir := t.TempDir()
		pendingDir := filepath.Join(tmpDir, "pending")
		os.MkdirAll(pendingDir, 0755)

		// Create only a flat plan file (no progress or feedback)
		planContent := "# Plan: lonely-plan\n\n**Status:** pending\n"
		os.WriteFile(filepath.Join(pendingDir, "lonely-plan.md"), []byte(planContent), 0644)

		// Migrate
		err := MigrateToBundles(tmpDir)
		if err != nil {
			t.Fatalf("MigrateToBundles failed: %v", err)
		}

		bundleDir := filepath.Join(pendingDir, "lonely-plan")

		// Verify scaffolded progress file was created
		progressContent, err := os.ReadFile(filepath.Join(bundleDir, "progress.md"))
		if err != nil {
			t.Fatal("progress.md was not created")
		}
		if !strings.Contains(string(progressContent), "# Progress: lonely-plan") {
			t.Error("scaffolded progress.md missing expected header")
		}

		// Verify scaffolded feedback file was created
		feedbackContent, err := os.ReadFile(filepath.Join(bundleDir, "feedback.md"))
		if err != nil {
			t.Fatal("feedback.md was not created")
		}
		if !strings.Contains(string(feedbackContent), "# Feedback: lonely-plan") {
			t.Error("scaffolded feedback.md missing expected header")
		}
	})

	t.Run("skips existing bundles", func(t *testing.T) {
		tmpDir := t.TempDir()
		pendingDir := filepath.Join(tmpDir, "pending")
		os.MkdirAll(pendingDir, 0755)

		// Create an existing bundle (directory with plan.md)
		bundleDir := filepath.Join(pendingDir, "existing-bundle")
		os.MkdirAll(bundleDir, 0755)
		originalContent := "# Plan: existing-bundle\n\nOriginal content\n"
		os.WriteFile(filepath.Join(bundleDir, "plan.md"), []byte(originalContent), 0644)

		// Migrate
		err := MigrateToBundles(tmpDir)
		if err != nil {
			t.Fatalf("MigrateToBundles failed: %v", err)
		}

		// Verify bundle was not modified
		content, _ := os.ReadFile(filepath.Join(bundleDir, "plan.md"))
		if string(content) != originalContent {
			t.Errorf("existing bundle was modified: got %q, want %q", string(content), originalContent)
		}
	})

	t.Run("migrates across all subdirectories", func(t *testing.T) {
		tmpDir := t.TempDir()

		// Create subdirectories
		for _, subdir := range []string{"pending", "current", "complete"} {
			dir := filepath.Join(tmpDir, subdir)
			os.MkdirAll(dir, 0755)

			// Add a flat plan file in each
			planContent := "# Plan: " + subdir + "-plan\n\n**Status:** " + subdir + "\n"
			os.WriteFile(filepath.Join(dir, subdir+"-plan.md"), []byte(planContent), 0644)
		}

		// Migrate
		err := MigrateToBundles(tmpDir)
		if err != nil {
			t.Fatalf("MigrateToBundles failed: %v", err)
		}

		// Verify bundles were created in each subdirectory
		for _, subdir := range []string{"pending", "current", "complete"} {
			bundleDir := filepath.Join(tmpDir, subdir, subdir+"-plan")
			if _, err := os.Stat(bundleDir); os.IsNotExist(err) {
				t.Errorf("bundle not created in %s", subdir)
			}
			if _, err := os.Stat(filepath.Join(bundleDir, "plan.md")); os.IsNotExist(err) {
				t.Errorf("plan.md not found in %s bundle", subdir)
			}
		}
	})

	t.Run("handles missing subdirectories gracefully", func(t *testing.T) {
		tmpDir := t.TempDir()
		// Don't create any subdirectories

		// Should not error
		err := MigrateToBundles(tmpDir)
		if err != nil {
			t.Fatalf("MigrateToBundles failed on empty plansDir: %v", err)
		}
	})

	t.Run("skips non-md files", func(t *testing.T) {
		tmpDir := t.TempDir()
		pendingDir := filepath.Join(tmpDir, "pending")
		os.MkdirAll(pendingDir, 0755)

		// Create a non-md file
		os.WriteFile(filepath.Join(pendingDir, "notes.txt"), []byte("some notes"), 0644)

		// Create a plan file
		os.WriteFile(filepath.Join(pendingDir, "real-plan.md"), []byte("# Plan\n"), 0644)

		err := MigrateToBundles(tmpDir)
		if err != nil {
			t.Fatalf("MigrateToBundles failed: %v", err)
		}

		// notes.txt should still exist
		if _, err := os.Stat(filepath.Join(pendingDir, "notes.txt")); os.IsNotExist(err) {
			t.Error("non-md file was incorrectly removed")
		}

		// real-plan should be migrated
		if _, err := os.Stat(filepath.Join(pendingDir, "real-plan", "plan.md")); os.IsNotExist(err) {
			t.Error("plan file was not migrated")
		}
	})
}
