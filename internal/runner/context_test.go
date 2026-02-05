package runner

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/arvesolland/ralph/internal/plan"
)

func TestNewContext(t *testing.T) {
	t.Run("with bundle and relative paths", func(t *testing.T) {
		p := &plan.Plan{
			Path:      "/repo/plans/current/test-plan/plan.md",
			Name:      "test-plan",
			Branch:    "feat/test-plan",
			BundleDir: "/repo/plans/current/test-plan",
		}
		ctx := NewContext(p, "main", 0, "/repo")

		if ctx.PlanFile != "plans/current/test-plan/plan.md" {
			t.Errorf("PlanFile = %q, want %q", ctx.PlanFile, "plans/current/test-plan/plan.md")
		}
		if ctx.PlanDir != "plans/current/test-plan" {
			t.Errorf("PlanDir = %q, want %q", ctx.PlanDir, "plans/current/test-plan")
		}
		if ctx.FeatureBranch != p.Branch {
			t.Errorf("FeatureBranch = %q, want %q", ctx.FeatureBranch, p.Branch)
		}
		if ctx.BaseBranch != "main" {
			t.Errorf("BaseBranch = %q, want %q", ctx.BaseBranch, "main")
		}
		if ctx.Iteration != 1 {
			t.Errorf("Iteration = %d, want %d", ctx.Iteration, 1)
		}
		if ctx.MaxIterations != DefaultMaxIterations {
			t.Errorf("MaxIterations = %d, want %d", ctx.MaxIterations, DefaultMaxIterations)
		}
	})

	t.Run("with flat file and relative paths", func(t *testing.T) {
		p := &plan.Plan{
			Path:   "/repo/plans/current/test-plan.md",
			Name:   "test-plan",
			Branch: "feat/test-plan",
			// No BundleDir = flat file
		}
		ctx := NewContext(p, "main", 0, "/repo")

		if ctx.PlanFile != "plans/current/test-plan.md" {
			t.Errorf("PlanFile = %q, want %q", ctx.PlanFile, "plans/current/test-plan.md")
		}
		// For flat files, PlanDir is the file path (same as PlanFile) so plan.Load() works
		if ctx.PlanDir != "plans/current/test-plan.md" {
			t.Errorf("PlanDir = %q, want %q", ctx.PlanDir, "plans/current/test-plan.md")
		}
	})

	t.Run("with empty workDir uses absolute paths", func(t *testing.T) {
		p := &plan.Plan{
			Path:      "/repo/plans/current/test-plan/plan.md",
			Name:      "test-plan",
			Branch:    "feat/test-plan",
			BundleDir: "/repo/plans/current/test-plan",
		}
		ctx := NewContext(p, "main", 0, "")

		// When workDir is empty, paths should remain absolute
		if ctx.PlanFile != "/repo/plans/current/test-plan/plan.md" {
			t.Errorf("PlanFile = %q, want %q", ctx.PlanFile, "/repo/plans/current/test-plan/plan.md")
		}
		if ctx.PlanDir != "/repo/plans/current/test-plan" {
			t.Errorf("PlanDir = %q, want %q", ctx.PlanDir, "/repo/plans/current/test-plan")
		}
	})

	t.Run("with custom max iterations", func(t *testing.T) {
		p := &plan.Plan{
			Path:      "/repo/plans/current/test-plan/plan.md",
			BundleDir: "/repo/plans/current/test-plan",
		}
		ctx := NewContext(p, "develop", 50, "/repo")

		if ctx.BaseBranch != "develop" {
			t.Errorf("BaseBranch = %q, want %q", ctx.BaseBranch, "develop")
		}
		if ctx.MaxIterations != 50 {
			t.Errorf("MaxIterations = %d, want %d", ctx.MaxIterations, 50)
		}
	})

	t.Run("with negative max iterations defaults", func(t *testing.T) {
		p := &plan.Plan{
			Path:      "/repo/plans/current/test-plan/plan.md",
			BundleDir: "/repo/plans/current/test-plan",
		}
		ctx := NewContext(p, "main", -5, "/repo")

		if ctx.MaxIterations != DefaultMaxIterations {
			t.Errorf("MaxIterations = %d, want %d", ctx.MaxIterations, DefaultMaxIterations)
		}
	})
}

func TestContext_Increment(t *testing.T) {
	ctx := &Context{
		PlanFile:      "plans/current/test-plan/plan.md",
		PlanDir:       "plans/current/test-plan",
		FeatureBranch: "feat/test",
		BaseBranch:    "main",
		Iteration:     5,
		MaxIterations: 30,
	}

	next := ctx.Increment()

	// Original should be unchanged
	if ctx.Iteration != 5 {
		t.Errorf("original Iteration = %d, want %d", ctx.Iteration, 5)
	}

	// New context should have incremented iteration
	if next.Iteration != 6 {
		t.Errorf("next Iteration = %d, want %d", next.Iteration, 6)
	}

	// Other fields should be copied
	if next.PlanFile != ctx.PlanFile {
		t.Errorf("next PlanFile = %q, want %q", next.PlanFile, ctx.PlanFile)
	}
	if next.PlanDir != ctx.PlanDir {
		t.Errorf("next PlanDir = %q, want %q", next.PlanDir, ctx.PlanDir)
	}
	if next.MaxIterations != ctx.MaxIterations {
		t.Errorf("next MaxIterations = %d, want %d", next.MaxIterations, ctx.MaxIterations)
	}
}

func TestContext_IsMaxReached(t *testing.T) {
	tests := []struct {
		iteration int
		max       int
		expected  bool
	}{
		{1, 30, false},
		{30, 30, false},
		{31, 30, true},
		{50, 30, true},
	}

	for _, tt := range tests {
		ctx := &Context{
			Iteration:     tt.iteration,
			MaxIterations: tt.max,
		}
		got := ctx.IsMaxReached()
		if got != tt.expected {
			t.Errorf("IsMaxReached() with iteration=%d, max=%d = %v, want %v",
				tt.iteration, tt.max, got, tt.expected)
		}
	}
}

func TestLoadContext_Success(t *testing.T) {
	tmpDir := t.TempDir()
	ctxPath := filepath.Join(tmpDir, "context.json")

	// Write a valid context file with planDir
	content := `{
  "planFile": "plans/current/test-plan/plan.md",
  "planDir": "plans/current/test-plan",
  "featureBranch": "feat/test-plan",
  "baseBranch": "main",
  "iteration": 5,
  "maxIterations": 30
}`
	if err := os.WriteFile(ctxPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	ctx, err := LoadContext(ctxPath)
	if err != nil {
		t.Fatalf("LoadContext() error = %v", err)
	}

	if ctx.PlanFile != "plans/current/test-plan/plan.md" {
		t.Errorf("PlanFile = %q, want %q", ctx.PlanFile, "plans/current/test-plan/plan.md")
	}
	if ctx.PlanDir != "plans/current/test-plan" {
		t.Errorf("PlanDir = %q, want %q", ctx.PlanDir, "plans/current/test-plan")
	}
	if ctx.FeatureBranch != "feat/test-plan" {
		t.Errorf("FeatureBranch = %q, want %q", ctx.FeatureBranch, "feat/test-plan")
	}
	if ctx.BaseBranch != "main" {
		t.Errorf("BaseBranch = %q, want %q", ctx.BaseBranch, "main")
	}
	if ctx.Iteration != 5 {
		t.Errorf("Iteration = %d, want %d", ctx.Iteration, 5)
	}
	if ctx.MaxIterations != 30 {
		t.Errorf("MaxIterations = %d, want %d", ctx.MaxIterations, 30)
	}
}

func TestLoadContext_BackwardCompatibility(t *testing.T) {
	// Test loading old context.json files that don't have planDir field
	tmpDir := t.TempDir()
	ctxPath := filepath.Join(tmpDir, "context.json")

	// Write an old-format context file WITHOUT planDir
	content := `{
  "planFile": "plans/current/test-plan/plan.md",
  "featureBranch": "feat/test-plan",
  "baseBranch": "main",
  "iteration": 5,
  "maxIterations": 30
}`
	if err := os.WriteFile(ctxPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	ctx, err := LoadContext(ctxPath)
	if err != nil {
		t.Fatalf("LoadContext() error = %v", err)
	}

	// Should load successfully with PlanDir as empty string
	if ctx.PlanFile != "plans/current/test-plan/plan.md" {
		t.Errorf("PlanFile = %q, want %q", ctx.PlanFile, "plans/current/test-plan/plan.md")
	}
	if ctx.PlanDir != "" {
		t.Errorf("PlanDir = %q, want empty string for old context files", ctx.PlanDir)
	}
	if ctx.Iteration != 5 {
		t.Errorf("Iteration = %d, want %d", ctx.Iteration, 5)
	}
}

func TestLoadContext_NonexistentFile(t *testing.T) {
	_, err := LoadContext("/nonexistent/context.json")
	if err == nil {
		t.Error("LoadContext() expected error for nonexistent file")
	}
}

func TestLoadContext_InvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	ctxPath := filepath.Join(tmpDir, "context.json")

	if err := os.WriteFile(ctxPath, []byte("not valid json"), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	_, err := LoadContext(ctxPath)
	if err == nil {
		t.Error("LoadContext() expected error for invalid JSON")
	}
}

func TestSaveContext_Success(t *testing.T) {
	tmpDir := t.TempDir()
	ctxPath := filepath.Join(tmpDir, "subdir", "context.json")

	ctx := &Context{
		PlanFile:      "/plans/current/my-plan.md",
		FeatureBranch: "feat/my-plan",
		BaseBranch:    "develop",
		Iteration:     10,
		MaxIterations: 50,
	}

	if err := SaveContext(ctx, ctxPath); err != nil {
		t.Fatalf("SaveContext() error = %v", err)
	}

	// Read back and verify
	loaded, err := LoadContext(ctxPath)
	if err != nil {
		t.Fatalf("LoadContext() error = %v", err)
	}

	if loaded.PlanFile != ctx.PlanFile {
		t.Errorf("PlanFile = %q, want %q", loaded.PlanFile, ctx.PlanFile)
	}
	if loaded.FeatureBranch != ctx.FeatureBranch {
		t.Errorf("FeatureBranch = %q, want %q", loaded.FeatureBranch, ctx.FeatureBranch)
	}
	if loaded.BaseBranch != ctx.BaseBranch {
		t.Errorf("BaseBranch = %q, want %q", loaded.BaseBranch, ctx.BaseBranch)
	}
	if loaded.Iteration != ctx.Iteration {
		t.Errorf("Iteration = %d, want %d", loaded.Iteration, ctx.Iteration)
	}
	if loaded.MaxIterations != ctx.MaxIterations {
		t.Errorf("MaxIterations = %d, want %d", loaded.MaxIterations, ctx.MaxIterations)
	}
}

func TestSaveContext_Overwrite(t *testing.T) {
	tmpDir := t.TempDir()
	ctxPath := filepath.Join(tmpDir, "context.json")

	ctx1 := &Context{Iteration: 1, MaxIterations: 30}
	ctx2 := &Context{Iteration: 5, MaxIterations: 30}

	if err := SaveContext(ctx1, ctxPath); err != nil {
		t.Fatalf("SaveContext(ctx1) error = %v", err)
	}

	if err := SaveContext(ctx2, ctxPath); err != nil {
		t.Fatalf("SaveContext(ctx2) error = %v", err)
	}

	loaded, err := LoadContext(ctxPath)
	if err != nil {
		t.Fatalf("LoadContext() error = %v", err)
	}

	if loaded.Iteration != 5 {
		t.Errorf("Iteration = %d, want %d (should be overwritten)", loaded.Iteration, 5)
	}
}

func TestSaveContext_AtomicWrite(t *testing.T) {
	tmpDir := t.TempDir()
	ctxPath := filepath.Join(tmpDir, "context.json")

	ctx := &Context{Iteration: 1, MaxIterations: 30}

	if err := SaveContext(ctx, ctxPath); err != nil {
		t.Fatalf("SaveContext() error = %v", err)
	}

	// Check that no temp file remains
	tempPath := ctxPath + ".tmp"
	if _, err := os.Stat(tempPath); !os.IsNotExist(err) {
		t.Errorf("temp file %q should not exist after successful save", tempPath)
	}
}

func TestContextPath(t *testing.T) {
	tests := []struct {
		worktreePath string
		expected     string
	}{
		{"/repo/.ralph/worktrees/test-plan", "/repo/.ralph/worktrees/test-plan/.ralph/context.json"},
		{"/home/user/project", "/home/user/project/.ralph/context.json"},
		{".", ".ralph/context.json"},
	}

	for _, tt := range tests {
		got := ContextPath(tt.worktreePath)
		if got != tt.expected {
			t.Errorf("ContextPath(%q) = %q, want %q", tt.worktreePath, got, tt.expected)
		}
	}
}

func TestJSONSerialization(t *testing.T) {
	tmpDir := t.TempDir()
	ctxPath := filepath.Join(tmpDir, "context.json")

	// Test that all fields serialize correctly to expected JSON keys
	ctx := &Context{
		PlanFile:      "plans/current/test-plan/plan.md",
		PlanDir:       "plans/current/test-plan",
		FeatureBranch: "feat/test",
		BaseBranch:    "main",
		Iteration:     3,
		MaxIterations: 30,
	}

	if err := SaveContext(ctx, ctxPath); err != nil {
		t.Fatalf("SaveContext() error = %v", err)
	}

	// Read raw JSON and verify field names
	data, err := os.ReadFile(ctxPath)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}

	content := string(data)
	expectedFields := []string{
		`"planFile"`,
		`"planDir"`,
		`"featureBranch"`,
		`"baseBranch"`,
		`"iteration"`,
		`"maxIterations"`,
	}

	for _, field := range expectedFields {
		if !contains(content, field) {
			t.Errorf("JSON output should contain %s, got: %s", field, content)
		}
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestRoundTrip(t *testing.T) {
	tmpDir := t.TempDir()
	ctxPath := filepath.Join(tmpDir, "context.json")

	original := &Context{
		PlanFile:      "plans/current/complex-plan/plan.md",
		PlanDir:       "plans/current/complex-plan",
		FeatureBranch: "feat/complex-plan",
		BaseBranch:    "main",
		Iteration:     15,
		MaxIterations: 100,
	}

	if err := SaveContext(original, ctxPath); err != nil {
		t.Fatalf("SaveContext() error = %v", err)
	}

	loaded, err := LoadContext(ctxPath)
	if err != nil {
		t.Fatalf("LoadContext() error = %v", err)
	}

	// Verify all fields round-trip correctly
	if loaded.PlanFile != original.PlanFile {
		t.Errorf("PlanFile mismatch: got %q, want %q", loaded.PlanFile, original.PlanFile)
	}
	if loaded.PlanDir != original.PlanDir {
		t.Errorf("PlanDir mismatch: got %q, want %q", loaded.PlanDir, original.PlanDir)
	}
	if loaded.FeatureBranch != original.FeatureBranch {
		t.Errorf("FeatureBranch mismatch: got %q, want %q", loaded.FeatureBranch, original.FeatureBranch)
	}
	if loaded.BaseBranch != original.BaseBranch {
		t.Errorf("BaseBranch mismatch: got %q, want %q", loaded.BaseBranch, original.BaseBranch)
	}
	if loaded.Iteration != original.Iteration {
		t.Errorf("Iteration mismatch: got %d, want %d", loaded.Iteration, original.Iteration)
	}
	if loaded.MaxIterations != original.MaxIterations {
		t.Errorf("MaxIterations mismatch: got %d, want %d", loaded.MaxIterations, original.MaxIterations)
	}
}
