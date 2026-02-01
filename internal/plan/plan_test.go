package plan

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad_ValidPlan(t *testing.T) {
	path := filepath.Join("testdata", "valid-plan.md")
	p, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if p.Name != "valid-plan" {
		t.Errorf("Name = %q, want %q", p.Name, "valid-plan")
	}

	if p.Status != "open" {
		t.Errorf("Status = %q, want %q", p.Status, "open")
	}

	if p.Branch != "feat/valid-plan" {
		t.Errorf("Branch = %q, want %q", p.Branch, "feat/valid-plan")
	}

	if p.Content == "" {
		t.Error("Content should not be empty")
	}

	// Path should be absolute
	if !filepath.IsAbs(p.Path) {
		t.Errorf("Path = %q, want absolute path", p.Path)
	}

	// Flat file should not be a bundle
	if p.IsBundle() {
		t.Error("IsBundle() = true, want false for flat file")
	}
	if p.BundleDir != "" {
		t.Errorf("BundleDir = %q, want empty for flat file", p.BundleDir)
	}
}

func TestLoad_Bundle(t *testing.T) {
	path := filepath.Join("testdata", "test-bundle")
	p, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if p.Name != "test-bundle" {
		t.Errorf("Name = %q, want %q", p.Name, "test-bundle")
	}

	if p.Status != "open" {
		t.Errorf("Status = %q, want %q", p.Status, "open")
	}

	if p.Branch != "feat/test-bundle" {
		t.Errorf("Branch = %q, want %q", p.Branch, "feat/test-bundle")
	}

	// Should be a bundle
	if !p.IsBundle() {
		t.Error("IsBundle() = false, want true for bundle")
	}

	// BundleDir should be set
	if p.BundleDir == "" {
		t.Error("BundleDir should not be empty for bundle")
	}
	if !filepath.IsAbs(p.BundleDir) {
		t.Errorf("BundleDir = %q, want absolute path", p.BundleDir)
	}

	// Path should point to plan.md inside bundle
	if !filepath.IsAbs(p.Path) {
		t.Errorf("Path = %q, want absolute path", p.Path)
	}
	if filepath.Base(p.Path) != "plan.md" {
		t.Errorf("Path = %q, want to end with plan.md", p.Path)
	}
}

func TestLoad_MissingStatus(t *testing.T) {
	path := filepath.Join("testdata", "no-status.md")
	p, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if p.Name != "no-status" {
		t.Errorf("Name = %q, want %q", p.Name, "no-status")
	}

	// Should default to "pending" when status is missing
	if p.Status != "pending" {
		t.Errorf("Status = %q, want %q (default)", p.Status, "pending")
	}

	if p.Branch != "feat/no-status" {
		t.Errorf("Branch = %q, want %q", p.Branch, "feat/no-status")
	}
}

func TestLoad_SpecialCharactersInName(t *testing.T) {
	path := filepath.Join("testdata", "my plan (v2).md")
	p, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	// Name preserves original filename (minus extension)
	if p.Name != "my plan (v2)" {
		t.Errorf("Name = %q, want %q", p.Name, "my plan (v2)")
	}

	// Branch should be sanitized
	if p.Branch != "feat/my-plan-v2" {
		t.Errorf("Branch = %q, want %q", p.Branch, "feat/my-plan-v2")
	}
}

func TestLoad_NonexistentFile(t *testing.T) {
	_, err := Load("testdata/does-not-exist.md")
	if err == nil {
		t.Error("Load() expected error for nonexistent file, got nil")
	}
	if !os.IsNotExist(err) {
		t.Errorf("Expected os.IsNotExist error, got %v", err)
	}
}

func TestDeriveName(t *testing.T) {
	tests := []struct {
		path     string
		isBundle bool
		want     string
	}{
		// Flat files (isBundle=false)
		{"go-rewrite.md", false, "go-rewrite"},
		{"plans/current/my-plan.md", false, "my-plan"},
		{"/absolute/path/test.md", false, "test"},
		{"plan.with.dots.md", false, "plan.with.dots"},
		{"no-extension", false, "no-extension"},
		// Bundles (isBundle=true)
		{"plans/pending/my-bundle", true, "my-bundle"},
		{"/absolute/path/feature-x", true, "feature-x"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := deriveName(tt.path, tt.isBundle)
			if got != tt.want {
				t.Errorf("deriveName(%q, %v) = %q, want %q", tt.path, tt.isBundle, got, tt.want)
			}
		})
	}
}

func TestExtractStatus(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{
			name:    "standard status",
			content: "# Plan\n**Status:** open\n\nContent here",
			want:    "open",
		},
		{
			name:    "complete status",
			content: "**Status:** complete",
			want:    "complete",
		},
		{
			name:    "pending status",
			content: "**Status:** pending",
			want:    "pending",
		},
		{
			name:    "status with extra whitespace",
			content: "**Status:**   in_progress  ",
			want:    "in_progress",
		},
		{
			name:    "no status defaults to pending",
			content: "# Plan\n\nNo status here",
			want:    "pending",
		},
		{
			name:    "status is case insensitive",
			content: "**Status:** OPEN",
			want:    "open",
		},
		{
			name:    "task status not confused with plan status",
			content: "# Plan\n\n**Status:** open\n\n### T1\n**Status:** complete",
			want:    "open",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractStatus(tt.content)
			if got != tt.want {
				t.Errorf("extractStatus() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSanitizeBranchName(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		{"go-rewrite", "go-rewrite"},
		{"my plan (v2)", "my-plan-v2"},
		{"feature_with_underscores", "feature-with-underscores"},
		{"UPPERCASE", "uppercase"},
		{"special!@#$chars", "specialchars"},
		{"  spaces  around  ", "spaces-around"},
		{"multiple---hyphens", "multiple-hyphens"},
		{"trailing-hyphen-", "trailing-hyphen"},
		{"-leading-hyphen", "leading-hyphen"},
		{"numbers123work", "numbers123work"},
		{"MixedCase_and-Stuff (v3)", "mixedcase-and-stuff-v3"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitizeBranchName(tt.name)
			if got != tt.want {
				t.Errorf("sanitizeBranchName(%q) = %q, want %q", tt.name, got, tt.want)
			}
		})
	}
}

func TestDeriveBranch(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		{"go-rewrite", "feat/go-rewrite"},
		{"my plan (v2)", "feat/my-plan-v2"},
		{"simple", "feat/simple"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := deriveBranch(tt.name)
			if got != tt.want {
				t.Errorf("deriveBranch(%q) = %q, want %q", tt.name, got, tt.want)
			}
		})
	}
}
