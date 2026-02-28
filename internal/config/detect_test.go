package config

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestDetect_NodeJS(t *testing.T) {
	dir := filepath.Join("testdata", "detect", "node")
	cfg, err := Detect(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Language != "node" {
		t.Errorf("expected language 'node', got '%s'", cfg.Language)
	}
	if cfg.Commands.Test != "npm test" {
		t.Errorf("expected test command 'npm test', got '%s'", cfg.Commands.Test)
	}
	if cfg.Commands.Lint != "npm run lint" {
		t.Errorf("expected lint command 'npm run lint', got '%s'", cfg.Commands.Lint)
	}
	if cfg.Commands.Build != "npm run build" {
		t.Errorf("expected build command 'npm run build', got '%s'", cfg.Commands.Build)
	}
	if cfg.Commands.Dev != "npm run dev" {
		t.Errorf("expected dev command 'npm run dev', got '%s'", cfg.Commands.Dev)
	}
	if cfg.PackageJSON == nil {
		t.Error("expected PackageJSON to be populated")
	}
	if cfg.PackageJSON.Name != "test-node-project" {
		t.Errorf("expected package name 'test-node-project', got '%s'", cfg.PackageJSON.Name)
	}
}

func TestDetect_NodeJS_NextJS(t *testing.T) {
	dir := filepath.Join("testdata", "detect", "node-nextjs")
	cfg, err := Detect(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Language != "node" {
		t.Errorf("expected language 'node', got '%s'", cfg.Language)
	}
	if cfg.Framework != "nextjs" {
		t.Errorf("expected framework 'nextjs', got '%s'", cfg.Framework)
	}
}

func TestDetect_Go(t *testing.T) {
	dir := filepath.Join("testdata", "detect", "go")
	cfg, err := Detect(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Language != "go" {
		t.Errorf("expected language 'go', got '%s'", cfg.Language)
	}
	if cfg.Commands.Test != "go test ./..." {
		t.Errorf("expected test command 'go test ./...', got '%s'", cfg.Commands.Test)
	}
	if cfg.Commands.Build != "go build ./..." {
		t.Errorf("expected build command 'go build ./...', got '%s'", cfg.Commands.Build)
	}
}

func TestDetect_Python_Pyproject(t *testing.T) {
	dir := filepath.Join("testdata", "detect", "python")
	cfg, err := Detect(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Language != "python" {
		t.Errorf("expected language 'python', got '%s'", cfg.Language)
	}
	if cfg.Commands.Test != "pytest" {
		t.Errorf("expected test command 'pytest', got '%s'", cfg.Commands.Test)
	}
}

func TestDetect_Python_Requirements(t *testing.T) {
	dir := filepath.Join("testdata", "detect", "python-requirements")
	cfg, err := Detect(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Language != "python" {
		t.Errorf("expected language 'python', got '%s'", cfg.Language)
	}
	if cfg.Commands.Test != "pytest" {
		t.Errorf("expected test command 'pytest', got '%s'", cfg.Commands.Test)
	}
}

func TestDetect_PHP(t *testing.T) {
	dir := filepath.Join("testdata", "detect", "php")
	cfg, err := Detect(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Language != "php" {
		t.Errorf("expected language 'php', got '%s'", cfg.Language)
	}
	if cfg.Commands.Test != "vendor/bin/phpunit" {
		t.Errorf("expected test command 'vendor/bin/phpunit', got '%s'", cfg.Commands.Test)
	}
}

func TestDetect_PHP_Laravel(t *testing.T) {
	dir := filepath.Join("testdata", "detect", "php-laravel")
	cfg, err := Detect(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Language != "php" {
		t.Errorf("expected language 'php', got '%s'", cfg.Language)
	}
	if cfg.Framework != "laravel" {
		t.Errorf("expected framework 'laravel', got '%s'", cfg.Framework)
	}
	if cfg.Commands.Test != "php artisan test" {
		t.Errorf("expected test command 'php artisan test', got '%s'", cfg.Commands.Test)
	}
}

func TestDetect_Rust(t *testing.T) {
	dir := filepath.Join("testdata", "detect", "rust")
	cfg, err := Detect(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Language != "rust" {
		t.Errorf("expected language 'rust', got '%s'", cfg.Language)
	}
	if cfg.Commands.Test != "cargo test" {
		t.Errorf("expected test command 'cargo test', got '%s'", cfg.Commands.Test)
	}
	if cfg.Commands.Build != "cargo build" {
		t.Errorf("expected build command 'cargo build', got '%s'", cfg.Commands.Build)
	}
	if cfg.Commands.Lint != "cargo clippy" {
		t.Errorf("expected lint command 'cargo clippy', got '%s'", cfg.Commands.Lint)
	}
}

func TestDetect_Ruby(t *testing.T) {
	dir := filepath.Join("testdata", "detect", "ruby")
	cfg, err := Detect(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Language != "ruby" {
		t.Errorf("expected language 'ruby', got '%s'", cfg.Language)
	}
	if cfg.Commands.Test != "bundle exec rspec" {
		t.Errorf("expected test command 'bundle exec rspec', got '%s'", cfg.Commands.Test)
	}
}

func TestDetect_Ruby_Rails(t *testing.T) {
	dir := filepath.Join("testdata", "detect", "ruby-rails")
	cfg, err := Detect(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Language != "ruby" {
		t.Errorf("expected language 'ruby', got '%s'", cfg.Language)
	}
	if cfg.Framework != "rails" {
		t.Errorf("expected framework 'rails', got '%s'", cfg.Framework)
	}
	if cfg.Commands.Test != "bundle exec rails test" {
		t.Errorf("expected test command 'bundle exec rails test', got '%s'", cfg.Commands.Test)
	}
}

func TestDetect_Empty(t *testing.T) {
	dir := filepath.Join("testdata", "detect", "empty")
	cfg, err := Detect(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Language != "" {
		t.Errorf("expected empty language, got '%s'", cfg.Language)
	}
}

func TestDetect_NonExistentDir(t *testing.T) {
	dir := filepath.Join("testdata", "detect", "nonexistent")
	cfg, err := Detect(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Non-existent directory should return empty config (not error)
	if cfg.Language != "" {
		t.Errorf("expected empty language, got '%s'", cfg.Language)
	}
}

// initGitRepo creates a git repo with initial commit in the given dir.
func initGitRepo(t *testing.T, dir, branch string) {
	t.Helper()
	cmds := [][]string{
		{"git", "init", "-b", branch},
		{"git", "config", "user.email", "test@test.com"},
		{"git", "config", "user.name", "Test"},
		{"git", "commit", "--allow-empty", "-m", "init"},
	}
	for _, args := range cmds {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("command %v failed: %v\n%s", args, err, out)
		}
	}
}

func TestDetectDefaultBranch_Main(t *testing.T) {
	// Create a bare "remote" repo with main as default
	bare := t.TempDir()
	initGitRepo(t, bare, "main")

	// Create a clone so origin/HEAD is set
	clone := t.TempDir()
	cmd := exec.Command("git", "clone", bare, clone)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("clone failed: %v\n%s", err, out)
	}

	branch := DetectDefaultBranch(clone)
	if branch != "main" {
		t.Errorf("expected 'main', got '%s'", branch)
	}
}

func TestDetectDefaultBranch_Master(t *testing.T) {
	// Create a bare "remote" repo with master as default
	bare := t.TempDir()
	initGitRepo(t, bare, "master")

	// Create a clone so origin/HEAD is set
	clone := t.TempDir()
	cmd := exec.Command("git", "clone", bare, clone)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("clone failed: %v\n%s", err, out)
	}

	branch := DetectDefaultBranch(clone)
	if branch != "master" {
		t.Errorf("expected 'master', got '%s'", branch)
	}
}

func TestDetectDefaultBranch_NoRemote(t *testing.T) {
	// Create a local repo with no remote
	dir := t.TempDir()
	initGitRepo(t, dir, "develop")

	branch := DetectDefaultBranch(dir)
	if branch != "main" {
		t.Errorf("expected fallback 'main', got '%s'", branch)
	}
}

func TestDetectDefaultBranch_NotGitDir(t *testing.T) {
	// Create a plain directory (not a git repo)
	dir := t.TempDir()

	branch := DetectDefaultBranch(dir)
	if branch != "main" {
		t.Errorf("expected fallback 'main', got '%s'", branch)
	}
}

func TestDetectDefaultBranch_NonExistentDir(t *testing.T) {
	dir := filepath.Join(os.TempDir(), "ralph-nonexistent-test-dir-xyz")

	branch := DetectDefaultBranch(dir)
	if branch != "main" {
		t.Errorf("expected fallback 'main', got '%s'", branch)
	}
}
