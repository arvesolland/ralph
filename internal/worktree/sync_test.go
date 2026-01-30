package worktree

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/arvesolland/ralph/internal/config"
	"github.com/arvesolland/ralph/internal/plan"
)

func TestSyncToWorktree(t *testing.T) {
	// Create temp directories for main and worktree
	mainDir := t.TempDir()
	worktreeDir := t.TempDir()

	// Create plans/current/ directory in main
	plansDir := filepath.Join(mainDir, "plans", "current")
	if err := os.MkdirAll(plansDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create plan file
	planPath := filepath.Join(plansDir, "test-plan.md")
	planContent := "# Test Plan\n\n**Status:** pending\n"
	if err := os.WriteFile(planPath, []byte(planContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Create progress file
	progressPath := filepath.Join(plansDir, "test-plan.progress.md")
	progressContent := "# Progress: test-plan\n\nSome progress...\n"
	if err := os.WriteFile(progressPath, []byte(progressContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Create feedback file
	feedbackPath := filepath.Join(plansDir, "test-plan.feedback.md")
	feedbackContent := "# Feedback: test-plan\n\n## Pending\n\n## Processed\n"
	if err := os.WriteFile(feedbackPath, []byte(feedbackContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Create plan struct
	p := &plan.Plan{
		Path: planPath,
		Name: "test-plan",
	}

	// Sync to worktree
	cfg := &config.Config{}
	if err := SyncToWorktree(p, worktreeDir, cfg, mainDir); err != nil {
		t.Fatalf("SyncToWorktree failed: %v", err)
	}

	// Verify files were copied
	dstPlanPath := filepath.Join(worktreeDir, "plans", "current", "test-plan.md")
	if content, err := os.ReadFile(dstPlanPath); err != nil {
		t.Errorf("Plan file not copied: %v", err)
	} else if string(content) != planContent {
		t.Errorf("Plan content mismatch: got %q, want %q", string(content), planContent)
	}

	dstProgressPath := filepath.Join(worktreeDir, "plans", "current", "test-plan.progress.md")
	if content, err := os.ReadFile(dstProgressPath); err != nil {
		t.Errorf("Progress file not copied: %v", err)
	} else if string(content) != progressContent {
		t.Errorf("Progress content mismatch: got %q, want %q", string(content), progressContent)
	}

	dstFeedbackPath := filepath.Join(worktreeDir, "plans", "current", "test-plan.feedback.md")
	if content, err := os.ReadFile(dstFeedbackPath); err != nil {
		t.Errorf("Feedback file not copied: %v", err)
	} else if string(content) != feedbackContent {
		t.Errorf("Feedback content mismatch: got %q, want %q", string(content), feedbackContent)
	}
}

func TestSyncToWorktree_WithEnvFiles(t *testing.T) {
	mainDir := t.TempDir()
	worktreeDir := t.TempDir()

	// Create .env file in main
	envContent := "DATABASE_URL=postgres://localhost/test\n"
	envPath := filepath.Join(mainDir, ".env")
	if err := os.WriteFile(envPath, []byte(envContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Create .env.local file
	envLocalContent := "SECRET_KEY=test123\n"
	envLocalPath := filepath.Join(mainDir, ".env.local")
	if err := os.WriteFile(envLocalPath, []byte(envLocalContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Create plan file
	plansDir := filepath.Join(mainDir, "plans", "current")
	if err := os.MkdirAll(plansDir, 0755); err != nil {
		t.Fatal(err)
	}
	planPath := filepath.Join(plansDir, "test-plan.md")
	if err := os.WriteFile(planPath, []byte("# Test Plan\n"), 0644); err != nil {
		t.Fatal(err)
	}

	p := &plan.Plan{
		Path: planPath,
		Name: "test-plan",
	}

	// Config with env files to copy
	cfg := &config.Config{
		Worktree: config.WorktreeConfig{
			CopyEnvFiles: ".env, .env.local",
		},
	}

	if err := SyncToWorktree(p, worktreeDir, cfg, mainDir); err != nil {
		t.Fatalf("SyncToWorktree failed: %v", err)
	}

	// Verify .env copied
	dstEnvPath := filepath.Join(worktreeDir, ".env")
	if content, err := os.ReadFile(dstEnvPath); err != nil {
		t.Errorf(".env not copied: %v", err)
	} else if string(content) != envContent {
		t.Errorf(".env content mismatch: got %q, want %q", string(content), envContent)
	}

	// Verify .env.local copied
	dstEnvLocalPath := filepath.Join(worktreeDir, ".env.local")
	if content, err := os.ReadFile(dstEnvLocalPath); err != nil {
		t.Errorf(".env.local not copied: %v", err)
	} else if string(content) != envLocalContent {
		t.Errorf(".env.local content mismatch: got %q, want %q", string(content), envLocalContent)
	}
}

func TestSyncToWorktree_MissingFiles(t *testing.T) {
	mainDir := t.TempDir()
	worktreeDir := t.TempDir()

	// Create only the plan file (no progress or feedback)
	plansDir := filepath.Join(mainDir, "plans", "current")
	if err := os.MkdirAll(plansDir, 0755); err != nil {
		t.Fatal(err)
	}
	planPath := filepath.Join(plansDir, "test-plan.md")
	if err := os.WriteFile(planPath, []byte("# Test Plan\n"), 0644); err != nil {
		t.Fatal(err)
	}

	p := &plan.Plan{
		Path: planPath,
		Name: "test-plan",
	}

	cfg := &config.Config{}

	// Should not error even if progress/feedback files don't exist
	if err := SyncToWorktree(p, worktreeDir, cfg, mainDir); err != nil {
		t.Fatalf("SyncToWorktree should not error for missing optional files: %v", err)
	}

	// Plan file should be copied
	dstPlanPath := filepath.Join(worktreeDir, "plans", "current", "test-plan.md")
	if _, err := os.Stat(dstPlanPath); err != nil {
		t.Errorf("Plan file should be copied: %v", err)
	}

	// Progress and feedback should not exist
	dstProgressPath := filepath.Join(worktreeDir, "plans", "current", "test-plan.progress.md")
	if _, err := os.Stat(dstProgressPath); !os.IsNotExist(err) {
		t.Error("Progress file should not exist")
	}

	dstFeedbackPath := filepath.Join(worktreeDir, "plans", "current", "test-plan.feedback.md")
	if _, err := os.Stat(dstFeedbackPath); !os.IsNotExist(err) {
		t.Error("Feedback file should not exist")
	}
}

func TestSyncFromWorktree(t *testing.T) {
	mainDir := t.TempDir()
	worktreeDir := t.TempDir()

	// Create directory structure in worktree
	worktreePlansDir := filepath.Join(worktreeDir, "plans", "current")
	if err := os.MkdirAll(worktreePlansDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create updated plan file in worktree
	planContent := "# Test Plan\n\n**Status:** complete\n"
	planPath := filepath.Join(worktreePlansDir, "test-plan.md")
	if err := os.WriteFile(planPath, []byte(planContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Create updated progress file in worktree
	progressContent := "# Progress\n\n## Iteration 1\nDid stuff\n"
	progressPath := filepath.Join(worktreePlansDir, "test-plan.progress.md")
	if err := os.WriteFile(progressPath, []byte(progressContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Create destination directory in main
	mainPlansDir := filepath.Join(mainDir, "plans", "current")
	if err := os.MkdirAll(mainPlansDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Plan struct points to main worktree location
	mainPlanPath := filepath.Join(mainPlansDir, "test-plan.md")
	p := &plan.Plan{
		Path: mainPlanPath,
		Name: "test-plan",
	}

	if err := SyncFromWorktree(p, worktreeDir, mainDir); err != nil {
		t.Fatalf("SyncFromWorktree failed: %v", err)
	}

	// Verify plan file was copied back
	if content, err := os.ReadFile(mainPlanPath); err != nil {
		t.Errorf("Plan file not copied back: %v", err)
	} else if string(content) != planContent {
		t.Errorf("Plan content mismatch: got %q, want %q", string(content), planContent)
	}

	// Verify progress file was copied back
	mainProgressPath := filepath.Join(mainPlansDir, "test-plan.progress.md")
	if content, err := os.ReadFile(mainProgressPath); err != nil {
		t.Errorf("Progress file not copied back: %v", err)
	} else if string(content) != progressContent {
		t.Errorf("Progress content mismatch: got %q, want %q", string(content), progressContent)
	}
}

func TestSyncFromWorktree_MissingFiles(t *testing.T) {
	mainDir := t.TempDir()
	worktreeDir := t.TempDir()

	// Create only plan file in worktree (no progress)
	worktreePlansDir := filepath.Join(worktreeDir, "plans", "current")
	if err := os.MkdirAll(worktreePlansDir, 0755); err != nil {
		t.Fatal(err)
	}
	planPath := filepath.Join(worktreePlansDir, "test-plan.md")
	if err := os.WriteFile(planPath, []byte("# Test Plan\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create destination directory in main
	mainPlansDir := filepath.Join(mainDir, "plans", "current")
	if err := os.MkdirAll(mainPlansDir, 0755); err != nil {
		t.Fatal(err)
	}

	mainPlanPath := filepath.Join(mainPlansDir, "test-plan.md")
	p := &plan.Plan{
		Path: mainPlanPath,
		Name: "test-plan",
	}

	// Should not error for missing progress file
	if err := SyncFromWorktree(p, worktreeDir, mainDir); err != nil {
		t.Fatalf("SyncFromWorktree should not error for missing optional files: %v", err)
	}

	// Plan should be copied
	if _, err := os.Stat(mainPlanPath); err != nil {
		t.Errorf("Plan file should be copied: %v", err)
	}
}

func TestCopyFile(t *testing.T) {
	tmpDir := t.TempDir()

	// Create source file with specific permissions
	srcPath := filepath.Join(tmpDir, "source.txt")
	content := "test content"
	if err := os.WriteFile(srcPath, []byte(content), 0640); err != nil {
		t.Fatal(err)
	}

	// Copy to destination
	dstPath := filepath.Join(tmpDir, "subdir", "dest.txt")
	if err := copyFile(srcPath, dstPath); err != nil {
		t.Fatalf("copyFile failed: %v", err)
	}

	// Verify content
	if got, err := os.ReadFile(dstPath); err != nil {
		t.Fatalf("Failed to read destination: %v", err)
	} else if string(got) != content {
		t.Errorf("Content mismatch: got %q, want %q", string(got), content)
	}

	// Verify permissions preserved
	srcInfo, _ := os.Stat(srcPath)
	dstInfo, _ := os.Stat(dstPath)
	if srcInfo.Mode() != dstInfo.Mode() {
		t.Errorf("Permissions not preserved: src %v, dst %v", srcInfo.Mode(), dstInfo.Mode())
	}
}

func TestCopyFile_NonExistent(t *testing.T) {
	tmpDir := t.TempDir()

	srcPath := filepath.Join(tmpDir, "nonexistent.txt")
	dstPath := filepath.Join(tmpDir, "dest.txt")

	err := copyFile(srcPath, dstPath)
	if !os.IsNotExist(err) {
		t.Errorf("Expected os.ErrNotExist, got: %v", err)
	}
}

func TestParseEnvFileList(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "single file",
			input:    ".env",
			expected: []string{".env"},
		},
		{
			name:     "multiple files",
			input:    ".env, .env.local, .env.test",
			expected: []string{".env", ".env.local", ".env.test"},
		},
		{
			name:     "with extra whitespace",
			input:    "  .env  ,  .env.local  ",
			expected: []string{".env", ".env.local"},
		},
		{
			name:     "empty string",
			input:    "",
			expected: nil,
		},
		{
			name:     "only whitespace",
			input:    "   ",
			expected: []string{},
		},
		{
			name:     "empty entries",
			input:    ".env, , .env.local",
			expected: []string{".env", ".env.local"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseEnvFileList(tt.input)

			if len(result) != len(tt.expected) {
				t.Errorf("Length mismatch: got %d, want %d", len(result), len(tt.expected))
				return
			}

			for i, v := range result {
				if v != tt.expected[i] {
					t.Errorf("Index %d: got %q, want %q", i, v, tt.expected[i])
				}
			}
		})
	}
}

func TestSyncToWorktree_PreservesPermissions(t *testing.T) {
	mainDir := t.TempDir()
	worktreeDir := t.TempDir()

	// Create plan file with specific permissions
	plansDir := filepath.Join(mainDir, "plans", "current")
	if err := os.MkdirAll(plansDir, 0755); err != nil {
		t.Fatal(err)
	}
	planPath := filepath.Join(plansDir, "test-plan.md")
	if err := os.WriteFile(planPath, []byte("# Test Plan\n"), 0640); err != nil {
		t.Fatal(err)
	}

	p := &plan.Plan{
		Path: planPath,
		Name: "test-plan",
	}

	cfg := &config.Config{}
	if err := SyncToWorktree(p, worktreeDir, cfg, mainDir); err != nil {
		t.Fatalf("SyncToWorktree failed: %v", err)
	}

	// Verify permissions preserved
	dstPlanPath := filepath.Join(worktreeDir, "plans", "current", "test-plan.md")
	srcInfo, _ := os.Stat(planPath)
	dstInfo, err := os.Stat(dstPlanPath)
	if err != nil {
		t.Fatalf("Failed to stat destination: %v", err)
	}

	if srcInfo.Mode() != dstInfo.Mode() {
		t.Errorf("Permissions not preserved: src %v, dst %v", srcInfo.Mode(), dstInfo.Mode())
	}
}
