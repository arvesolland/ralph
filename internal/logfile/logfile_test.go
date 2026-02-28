package logfile

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNew_CreatesLogFile(t *testing.T) {
	dir := t.TempDir()
	logsDir := filepath.Join(dir, "logs")

	lf, err := New(Options{
		LogDir: logsDir,
		Prefix: "test",
	})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	defer lf.Close()

	// Check file was created
	if _, err := os.Stat(lf.Path()); os.IsNotExist(err) {
		t.Errorf("log file not created at %s", lf.Path())
	}

	// Verify path structure
	if !strings.HasPrefix(filepath.Base(lf.Path()), "test-") {
		t.Errorf("log file name should start with 'test-', got %s", filepath.Base(lf.Path()))
	}
	if !strings.HasSuffix(lf.Path(), ".log") {
		t.Errorf("log file should end with .log, got %s", lf.Path())
	}
}

func TestNew_CustomPath(t *testing.T) {
	dir := t.TempDir()
	customPath := filepath.Join(dir, "custom", "my.log")

	lf, err := New(Options{
		CustomPath: customPath,
	})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	defer lf.Close()

	if lf.Path() != customPath {
		t.Errorf("Path() = %s, want %s", lf.Path(), customPath)
	}

	if _, err := os.Stat(customPath); os.IsNotExist(err) {
		t.Errorf("custom log file not created at %s", customPath)
	}
}

func TestNew_TeesStdout(t *testing.T) {
	dir := t.TempDir()
	logsDir := filepath.Join(dir, "logs")

	lf, err := New(Options{
		LogDir: logsDir,
		Prefix: "tee-test",
	})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	// Write to os.Stdout (which is now the pipe)
	testMsg := "hello from stdout\n"
	fmt.Fprint(os.Stdout, testMsg)

	// Close to flush
	lf.Close()

	// Read the log file
	content, err := os.ReadFile(lf.Path())
	if err != nil {
		t.Fatalf("reading log file: %v", err)
	}

	if !strings.Contains(string(content), "hello from stdout") {
		t.Errorf("log file should contain stdout output, got: %s", string(content))
	}
}

func TestNew_TeesStderr(t *testing.T) {
	dir := t.TempDir()
	logsDir := filepath.Join(dir, "logs")

	lf, err := New(Options{
		LogDir: logsDir,
		Prefix: "tee-test",
	})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	// Write to os.Stderr (which is now the pipe)
	testMsg := "hello from stderr\n"
	fmt.Fprint(os.Stderr, testMsg)

	// Close to flush
	lf.Close()

	// Read the log file
	content, err := os.ReadFile(lf.Path())
	if err != nil {
		t.Fatalf("reading log file: %v", err)
	}

	if !strings.Contains(string(content), "hello from stderr") {
		t.Errorf("log file should contain stderr output, got: %s", string(content))
	}
}

func TestNew_RestoresStdoutStderr(t *testing.T) {
	origOut := os.Stdout
	origErr := os.Stderr

	dir := t.TempDir()
	logsDir := filepath.Join(dir, "logs")

	lf, err := New(Options{
		LogDir: logsDir,
		Prefix: "restore-test",
	})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	// os.Stdout/Stderr should be replaced
	if os.Stdout == origOut {
		t.Error("os.Stdout should be replaced during logging")
	}
	if os.Stderr == origErr {
		t.Error("os.Stderr should be replaced during logging")
	}

	lf.Close()

	// After close, should be restored
	if os.Stdout != origOut {
		t.Error("os.Stdout should be restored after Close()")
	}
	if os.Stderr != origErr {
		t.Error("os.Stderr should be restored after Close()")
	}
}

func TestNew_MissingLogDir(t *testing.T) {
	_, err := New(Options{
		Prefix: "test",
	})
	if err == nil {
		t.Error("expected error when LogDir is empty")
	}
}

func TestNew_MissingPrefix(t *testing.T) {
	dir := t.TempDir()
	_, err := New(Options{
		LogDir: dir,
	})
	if err == nil {
		t.Error("expected error when Prefix is empty")
	}
}

func TestCleanup_KeepsMaxFiles(t *testing.T) {
	dir := t.TempDir()

	// Create 15 log files with different timestamps
	for i := 0; i < 15; i++ {
		ts := time.Date(2025, 1, 1, 0, 0, i, 0, time.UTC).Format("20060102-150405")
		name := fmt.Sprintf("worker-%s.log", ts)
		if err := os.WriteFile(filepath.Join(dir, name), []byte("test"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	Cleanup(dir, "worker", 10)

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}

	count := 0
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "worker-") && strings.HasSuffix(e.Name(), ".log") {
			count++
		}
	}

	if count != 10 {
		t.Errorf("expected 10 files after cleanup, got %d", count)
	}
}

func TestCleanup_RemovesOldest(t *testing.T) {
	dir := t.TempDir()

	// Create 5 log files
	names := []string{
		"plan-1-20250101-000001.log",
		"plan-1-20250101-000002.log",
		"plan-1-20250101-000003.log",
		"plan-1-20250101-000004.log",
		"plan-1-20250101-000005.log",
	}
	for _, name := range names {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("test"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	Cleanup(dir, "plan-1", 3)

	// Check that oldest 2 are gone
	for _, name := range names[:2] {
		if _, err := os.Stat(filepath.Join(dir, name)); !os.IsNotExist(err) {
			t.Errorf("expected %s to be removed", name)
		}
	}

	// Check that newest 3 remain
	for _, name := range names[2:] {
		if _, err := os.Stat(filepath.Join(dir, name)); os.IsNotExist(err) {
			t.Errorf("expected %s to remain", name)
		}
	}
}

func TestCleanup_IgnoresOtherPrefixes(t *testing.T) {
	dir := t.TempDir()

	// Create files with different prefixes
	files := []string{
		"worker-20250101-000001.log",
		"worker-20250101-000002.log",
		"worker-20250101-000003.log",
		"plan-1-20250101-000001.log",
		"plan-1-20250101-000002.log",
	}
	for _, name := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("test"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	Cleanup(dir, "worker", 1)

	// Worker files: only 1 should remain
	entries, _ := os.ReadDir(dir)
	workerCount := 0
	planCount := 0
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "worker-") {
			workerCount++
		}
		if strings.HasPrefix(e.Name(), "plan-1-") {
			planCount++
		}
	}

	if workerCount != 1 {
		t.Errorf("expected 1 worker file, got %d", workerCount)
	}
	if planCount != 2 {
		t.Errorf("expected 2 plan files (untouched), got %d", planCount)
	}
}

func TestCleanup_NoopWhenFewFiles(t *testing.T) {
	dir := t.TempDir()

	// Create 3 files (under limit of 10)
	for i := 0; i < 3; i++ {
		name := fmt.Sprintf("worker-%d.log", i)
		if err := os.WriteFile(filepath.Join(dir, name), []byte("test"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	Cleanup(dir, "worker", 10)

	entries, _ := os.ReadDir(dir)
	if len(entries) != 3 {
		t.Errorf("expected 3 files (no cleanup needed), got %d", len(entries))
	}
}

func TestPlanPrefix(t *testing.T) {
	if got := PlanPrefix(56); got != "plan-56" {
		t.Errorf("PlanPrefix(56) = %q, want %q", got, "plan-56")
	}
}

func TestWorkerPrefix(t *testing.T) {
	if got := WorkerPrefix(); got != "worker" {
		t.Errorf("WorkerPrefix() = %q, want %q", got, "worker")
	}
}
