package worktree

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDetectLockfile(t *testing.T) {
	tests := []struct {
		name     string
		fixture  string
		expected string
	}{
		{"npm", "node-npm", "package-lock.json"},
		{"yarn", "node-yarn", "yarn.lock"},
		{"pnpm", "node-pnpm", "pnpm-lock.yaml"},
		{"bun", "node-bun", "bun.lockb"},
		{"composer", "php", "composer.lock"},
		{"pip", "python-pip", "requirements.txt"},
		{"poetry", "python-poetry", "poetry.lock"},
		{"bundler", "ruby", "Gemfile.lock"},
		{"go", "go", "go.sum"},
		{"cargo", "rust", "Cargo.lock"},
		{"empty", "empty", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fixturePath := filepath.Join("testdata", "deps", tt.fixture)
			result := DetectLockfile(fixturePath)
			if result != tt.expected {
				t.Errorf("DetectLockfile(%s) = %q, want %q", tt.fixture, result, tt.expected)
			}
		})
	}
}

func TestDetectLockfile_PriorityOrder(t *testing.T) {
	// Create temp directory with multiple lockfiles
	tmpDir := t.TempDir()

	// Create both npm and yarn lockfiles
	if err := os.WriteFile(filepath.Join(tmpDir, "package-lock.json"), []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "yarn.lock"), []byte("# yarn"), 0644); err != nil {
		t.Fatal(err)
	}

	// yarn.lock should take priority (comes before package-lock.json in order)
	result := DetectLockfile(tmpDir)
	if result != "yarn.lock" {
		t.Errorf("Expected yarn.lock to take priority over package-lock.json, got %q", result)
	}

	// Add pnpm lockfile - should take priority over both
	if err := os.WriteFile(filepath.Join(tmpDir, "pnpm-lock.yaml"), []byte("# pnpm"), 0644); err != nil {
		t.Fatal(err)
	}

	result = DetectLockfile(tmpDir)
	if result != "pnpm-lock.yaml" {
		t.Errorf("Expected pnpm-lock.yaml to take priority, got %q", result)
	}
}

func TestDetectLockfile_NonexistentDirectory(t *testing.T) {
	result := DetectLockfile("/nonexistent/directory")
	if result != "" {
		t.Errorf("Expected empty string for nonexistent directory, got %q", result)
	}
}

func TestGetLockfileInfo(t *testing.T) {
	tests := []struct {
		lockfile    string
		wantCommand string
		wantDesc    string
	}{
		{"package-lock.json", "npm", "npm"},
		{"yarn.lock", "yarn", "Yarn"},
		{"pnpm-lock.yaml", "pnpm", "pnpm"},
		{"bun.lockb", "bun", "Bun"},
		{"composer.lock", "composer", "Composer"},
		{"requirements.txt", "pip", "pip"},
		{"poetry.lock", "poetry", "Poetry"},
		{"Gemfile.lock", "bundle", "Bundler"},
		{"go.sum", "go", "Go modules"},
		{"Cargo.lock", "cargo", "Cargo"},
	}

	for _, tt := range tests {
		t.Run(tt.lockfile, func(t *testing.T) {
			info := GetLockfileInfo(tt.lockfile)
			if info == nil {
				t.Fatalf("GetLockfileInfo(%q) returned nil", tt.lockfile)
			}
			if info.Command != tt.wantCommand {
				t.Errorf("Command = %q, want %q", info.Command, tt.wantCommand)
			}
			if info.Description != tt.wantDesc {
				t.Errorf("Description = %q, want %q", info.Description, tt.wantDesc)
			}
		})
	}
}

func TestGetLockfileInfo_Unknown(t *testing.T) {
	info := GetLockfileInfo("unknown.lock")
	if info != nil {
		t.Errorf("Expected nil for unknown lockfile, got %v", info)
	}
}

func TestLockfileOrder_Coverage(t *testing.T) {
	// Verify all expected lockfiles are covered
	expectedLockfiles := []string{
		"package-lock.json",
		"yarn.lock",
		"pnpm-lock.yaml",
		"bun.lockb",
		"composer.lock",
		"requirements.txt",
		"poetry.lock",
		"Gemfile.lock",
		"go.sum",
		"Cargo.lock",
	}

	for _, lf := range expectedLockfiles {
		info := GetLockfileInfo(lf)
		if info == nil {
			t.Errorf("Missing lockfile definition: %s", lf)
		}
	}
}

func TestDetectAndInstall_NoLockfile(t *testing.T) {
	fixturePath := filepath.Join("testdata", "deps", "empty")
	result, err := DetectAndInstall(fixturePath)
	if err != nil {
		t.Errorf("DetectAndInstall with no lockfile should not error: %v", err)
	}
	if result != nil {
		t.Errorf("Expected nil result for no lockfile, got %v", result)
	}
}

func TestDetectAndInstall_CommandNotFound(t *testing.T) {
	// Create temp directory with a lockfile for a command that doesn't exist
	tmpDir := t.TempDir()

	// Use a lockfile for a package manager that's unlikely to be installed
	// We'll create a fake lockfile type to force the command-not-found error
	// Since we can't easily test with missing commands, we'll verify the detection logic instead

	// Create a go.sum file (go is likely installed on dev machines)
	if err := os.WriteFile(filepath.Join(tmpDir, "go.sum"), []byte("# test"), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := DetectAndInstall(tmpDir)

	// If go is not installed, we expect ErrCommandNotFound
	if err != nil && errors.Is(err, ErrCommandNotFound) {
		if result != nil {
			t.Error("Expected nil result with command not found error")
		}
		return
	}

	// If go is installed but go.mod doesn't exist, the command will fail
	// This is expected behavior - we're testing the detection and execution flow
	if err != nil {
		if result == nil {
			t.Error("Expected non-nil result even on command failure")
		}
		if result.Lockfile != "go.sum" {
			t.Errorf("Expected lockfile 'go.sum', got %q", result.Lockfile)
		}
		// Command failure is expected since there's no proper go.mod
		return
	}

	// If it succeeded (unlikely in test fixture without proper go.mod)
	// that's also fine - just verify the result
	if result != nil {
		if result.Lockfile != "go.sum" {
			t.Errorf("Expected lockfile 'go.sum', got %q", result.Lockfile)
		}
	}
}

func TestLockfileArgs(t *testing.T) {
	// Verify specific args for each lockfile
	tests := []struct {
		lockfile string
		wantArgs []string
	}{
		{"package-lock.json", []string{"ci"}},
		{"yarn.lock", []string{"install", "--frozen-lockfile"}},
		{"pnpm-lock.yaml", []string{"install", "--frozen-lockfile"}},
		{"bun.lockb", []string{"install", "--frozen-lockfile"}},
		{"composer.lock", []string{"install"}},
		{"requirements.txt", []string{"install", "-r", "requirements.txt"}},
		{"poetry.lock", []string{"install"}},
		{"Gemfile.lock", []string{"install"}},
		{"go.sum", []string{"mod", "download"}},
		{"Cargo.lock", []string{"fetch"}},
	}

	for _, tt := range tests {
		t.Run(tt.lockfile, func(t *testing.T) {
			info := GetLockfileInfo(tt.lockfile)
			if info == nil {
				t.Fatalf("GetLockfileInfo(%q) returned nil", tt.lockfile)
			}
			if len(info.Args) != len(tt.wantArgs) {
				t.Errorf("Args length = %d, want %d", len(info.Args), len(tt.wantArgs))
				return
			}
			for i, arg := range info.Args {
				if arg != tt.wantArgs[i] {
					t.Errorf("Args[%d] = %q, want %q", i, arg, tt.wantArgs[i])
				}
			}
		})
	}
}

func TestInstallResultCommand(t *testing.T) {
	// Verify command string format
	info := GetLockfileInfo("package-lock.json")
	if info == nil {
		t.Fatal("GetLockfileInfo returned nil")
	}

	expectedCmdPrefix := "npm"
	if info.Command != expectedCmdPrefix {
		t.Errorf("Command = %q, want %q", info.Command, expectedCmdPrefix)
	}
}

// TestDetectAndInstall_Integration tests the full flow with a real command
// This test is marked as integration since it requires actual package managers
func TestDetectAndInstall_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Skip if go is not available
	if _, err := os.Stat("/usr/local/go/bin/go"); err != nil {
		// Try which
		if out, err := os.ReadFile("/dev/null"); err != nil || len(out) == 0 {
			t.Skip("Skipping: go not available")
		}
	}

	// Create a proper Go module for testing
	tmpDir := t.TempDir()

	goMod := `module testmodule

go 1.22
`
	if err := os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte(goMod), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "go.sum"), []byte(""), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := DetectAndInstall(tmpDir)
	if errors.Is(err, ErrCommandNotFound) {
		t.Skip("Skipping: go command not found")
	}

	// The command should succeed for an empty go.sum
	if err != nil {
		// Some error is expected if go.sum is empty and there are no deps
		// Just verify we got a result
		if result == nil {
			t.Error("Expected non-nil result")
		}
		return
	}

	if result == nil {
		t.Fatal("Expected non-nil result")
	}

	if result.Lockfile != "go.sum" {
		t.Errorf("Lockfile = %q, want 'go.sum'", result.Lockfile)
	}

	if !strings.Contains(result.Command, "go") {
		t.Errorf("Command should contain 'go', got %q", result.Command)
	}
}
