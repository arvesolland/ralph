package runner

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewContext(t *testing.T) {
	t.Run("basic creation", func(t *testing.T) {
		ctx := NewContext(42, "feat/test-plan", "main", 0)

		if ctx.PlanID != 42 {
			t.Errorf("PlanID = %d, want %d", ctx.PlanID, 42)
		}
		if ctx.FeatureBranch != "feat/test-plan" {
			t.Errorf("FeatureBranch = %q, want %q", ctx.FeatureBranch, "feat/test-plan")
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

	t.Run("with custom max iterations", func(t *testing.T) {
		ctx := NewContext(1, "feat/test", "develop", 50)

		if ctx.BaseBranch != "develop" {
			t.Errorf("BaseBranch = %q, want %q", ctx.BaseBranch, "develop")
		}
		if ctx.MaxIterations != 50 {
			t.Errorf("MaxIterations = %d, want %d", ctx.MaxIterations, 50)
		}
	})

	t.Run("with negative max iterations defaults", func(t *testing.T) {
		ctx := NewContext(1, "feat/test", "main", -5)

		if ctx.MaxIterations != DefaultMaxIterations {
			t.Errorf("MaxIterations = %d, want %d", ctx.MaxIterations, DefaultMaxIterations)
		}
	})
}

func TestContext_Increment(t *testing.T) {
	ctx := &Context{
		PlanID:        42,
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
	if next.PlanID != ctx.PlanID {
		t.Errorf("next PlanID = %d, want %d", next.PlanID, ctx.PlanID)
	}
	if next.FeatureBranch != ctx.FeatureBranch {
		t.Errorf("next FeatureBranch = %q, want %q", next.FeatureBranch, ctx.FeatureBranch)
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

	content := `{
  "planId": 42,
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

	if ctx.PlanID != 42 {
		t.Errorf("PlanID = %d, want %d", ctx.PlanID, 42)
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
		PlanID:        7,
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

	if loaded.PlanID != ctx.PlanID {
		t.Errorf("PlanID = %d, want %d", loaded.PlanID, ctx.PlanID)
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

	ctx := &Context{
		PlanID:        42,
		FeatureBranch: "feat/test",
		BaseBranch:    "main",
		Iteration:     3,
		MaxIterations: 30,
	}

	if err := SaveContext(ctx, ctxPath); err != nil {
		t.Fatalf("SaveContext() error = %v", err)
	}

	data, err := os.ReadFile(ctxPath)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}

	content := string(data)
	expectedFields := []string{
		`"planId"`,
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
		PlanID:        99,
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

	if loaded.PlanID != original.PlanID {
		t.Errorf("PlanID mismatch: got %d, want %d", loaded.PlanID, original.PlanID)
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
