package worker

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/arvesolland/ralph/internal/git"
)

func TestIsGHInstalled(t *testing.T) {
	result := isGHInstalled()
	t.Logf("gh installed: %v", result)
}

func TestExtractPRURL(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "standard URL",
			input:    "https://github.com/arvesolland/ralph/pull/123",
			expected: "https://github.com/arvesolland/ralph/pull/123",
		},
		{
			name:     "URL in text",
			input:    "Created PR: https://github.com/owner/repo/pull/456\nDone.",
			expected: "https://github.com/owner/repo/pull/456",
		},
		{
			name:     "no URL",
			input:    "Something went wrong",
			expected: "",
		},
		{
			name:     "URL with different owner/repo",
			input:    "https://github.com/some-org/some-repo/pull/789",
			expected: "https://github.com/some-org/some-repo/pull/789",
		},
		{
			name:     "multiple URLs (returns first)",
			input:    "First: https://github.com/a/b/pull/1 Second: https://github.com/c/d/pull/2",
			expected: "https://github.com/a/b/pull/1",
		},
		{
			name:     "similar but not PR URL",
			input:    "https://github.com/owner/repo/issues/123",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractPRURL(tt.input)
			if result != tt.expected {
				t.Errorf("extractPRURL(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestPRURLRegex(t *testing.T) {
	validURLs := []string{
		"https://github.com/owner/repo/pull/1",
		"https://github.com/a/b/pull/123",
		"https://github.com/some-org/some-repo/pull/99999",
	}

	for _, url := range validURLs {
		if !prURLRegex.MatchString(url) {
			t.Errorf("prURLRegex should match %q", url)
		}
	}

	invalidURLs := []string{
		"http://github.com/owner/repo/pull/1", // http not https
		"https://github.com/owner/repo/issues/1",
		"https://github.com/owner/repo/pull/",    // no number
		"https://gitlab.com/owner/repo/pull/123", // not github
	}

	for _, url := range invalidURLs {
		if prURLRegex.MatchString(url) {
			t.Errorf("prURLRegex should not match %q", url)
		}
	}
}

func TestLogManualPRInstructionsSimple(t *testing.T) {
	// Just verify it doesn't panic
	logManualPRInstructionsSimple("test-plan", "feat/test-plan")
}

func TestPushBranch(t *testing.T) {
	mockGit := &mockGitForCompletion{
		pushError: nil,
	}

	err := pushBranch(mockGit, "feat/test")
	if err != nil {
		t.Errorf("pushBranch() error = %v, want nil", err)
	}

	if mockGit.pushedBranch != "feat/test" {
		t.Errorf("pushBranch() pushed branch = %q, want %q", mockGit.pushedBranch, "feat/test")
	}
}

func TestPushBranch_Error(t *testing.T) {
	mockGit := &mockGitForCompletion{
		pushError: ErrPushFailed,
	}

	err := pushBranch(mockGit, "feat/test")
	if err == nil {
		t.Error("pushBranch() should return error when push fails")
	}
}

// mockGitForCompletion is a minimal mock for testing completion functions
type mockGitForCompletion struct {
	git.Git
	pushError    error
	pushedBranch string
	workDir      string
}

func (m *mockGitForCompletion) PushWithUpstream(remote, branch string) error {
	m.pushedBranch = branch
	return m.pushError
}

func (m *mockGitForCompletion) WorkDir() string {
	return m.workDir
}

func TestCreatePRBasic_GHNotInstalled(t *testing.T) {
	originalPath := os.Getenv("PATH")
	defer os.Setenv("PATH", originalPath)

	os.Setenv("PATH", "")

	_, err := createPRBasic("test-plan", "/tmp")
	if err != ErrGHNotInstalled {
		t.Errorf("createPRBasic() error = %v, want ErrGHNotInstalled", err)
	}
}

func TestCreatePRBasic_Fallback(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	tmpDir, err := os.MkdirTemp("", "completion-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	mockGH := filepath.Join(tmpDir, "gh")
	mockScript := `#!/bin/bash
echo "https://github.com/test/repo/pull/123"
`
	if err := os.WriteFile(mockGH, []byte(mockScript), 0755); err != nil {
		t.Fatalf("failed to write mock gh: %v", err)
	}

	originalPath := os.Getenv("PATH")
	defer os.Setenv("PATH", originalPath)
	os.Setenv("PATH", tmpDir+":"+originalPath)

	prURL, err := createPRBasic("test-feature", tmpDir)
	if err != nil {
		t.Errorf("createPRBasic() error = %v", err)
	}

	if prURL != "https://github.com/test/repo/pull/123" {
		t.Errorf("createPRBasic() prURL = %q, want %q", prURL, "https://github.com/test/repo/pull/123")
	}
}

func TestCompletionErrors(t *testing.T) {
	if ErrGHNotInstalled.Error() != "gh CLI not installed" {
		t.Errorf("ErrGHNotInstalled = %v, want 'gh CLI not installed'", ErrGHNotInstalled)
	}

	if ErrPushFailed.Error() != "failed to push branch" {
		t.Errorf("ErrPushFailed = %v, want 'failed to push branch'", ErrPushFailed)
	}

	if ErrPRCreateFailed.Error() != "failed to create PR" {
		t.Errorf("ErrPRCreateFailed = %v, want 'failed to create PR'", ErrPRCreateFailed)
	}

	if ErrMergeConflict.Error() != "merge conflict" {
		t.Errorf("ErrMergeConflict = %v, want 'merge conflict'", ErrMergeConflict)
	}

	if ErrCheckoutFailed.Error() != "failed to checkout branch" {
		t.Errorf("ErrCheckoutFailed = %v, want 'failed to checkout branch'", ErrCheckoutFailed)
	}

	if ErrMergeFailed.Error() != "failed to merge branch" {
		t.Errorf("ErrMergeFailed = %v, want 'failed to merge branch'", ErrMergeFailed)
	}
}

// mockGitForMerge is a mock Git implementation for testing merge completion
type mockGitForMerge struct {
	git.Git
	checkoutError       error
	mergeError          error
	pushError           error
	deleteBranchError   error
	deleteRemoteError   error
	currentBranch       string
	checkedOutBranch    string
	mergedBranch        string
	deletedBranch       string
	deletedRemoteBranch string
}

func (m *mockGitForMerge) Checkout(branch string) error {
	m.checkedOutBranch = branch
	m.currentBranch = branch
	return m.checkoutError
}

func (m *mockGitForMerge) Merge(branch string, noFastForward bool) error {
	m.mergedBranch = branch
	return m.mergeError
}

func (m *mockGitForMerge) Push() error {
	return m.pushError
}

func (m *mockGitForMerge) DeleteBranch(name string, force bool) error {
	m.deletedBranch = name
	return m.deleteBranchError
}

func (m *mockGitForMerge) DeleteRemoteBranch(remote, branch string) error {
	m.deletedRemoteBranch = branch
	return m.deleteRemoteError
}

func TestCompleteMerge_Success(t *testing.T) {
	mock := &mockGitForMerge{}
	err := CompleteMerge("feat/test-feature", "main", mock)
	if err != nil {
		t.Errorf("CompleteMerge() error = %v, want nil", err)
	}

	if mock.checkedOutBranch != "main" {
		t.Errorf("should checkout base branch, got %q", mock.checkedOutBranch)
	}

	if mock.mergedBranch != "feat/test-feature" {
		t.Errorf("should merge feature branch, got %q", mock.mergedBranch)
	}

	if mock.deletedBranch != "feat/test-feature" {
		t.Errorf("should delete local feature branch, got %q", mock.deletedBranch)
	}

	if mock.deletedRemoteBranch != "feat/test-feature" {
		t.Errorf("should delete remote feature branch, got %q", mock.deletedRemoteBranch)
	}
}

func TestCompleteMerge_CheckoutFails(t *testing.T) {
	mock := &mockGitForMerge{
		checkoutError: git.ErrBranchNotFound,
	}

	err := CompleteMerge("feat/test-feature", "main", mock)
	if err == nil {
		t.Error("CompleteMerge() should return error when checkout fails")
	}

	if !strings.Contains(err.Error(), "failed to checkout") {
		t.Errorf("error should mention checkout failure, got: %v", err)
	}
}

func TestCompleteMerge_MergeConflict(t *testing.T) {
	mock := &mockGitForMerge{
		mergeError: git.ErrMergeConflict,
	}

	err := CompleteMerge("feat/test-feature", "main", mock)
	if err == nil {
		t.Error("CompleteMerge() should return error on merge conflict")
	}

	if !strings.Contains(err.Error(), "merge conflict") {
		t.Errorf("error should mention merge conflict, got: %v", err)
	}
}

func TestCompleteMerge_MergeFails(t *testing.T) {
	mock := &mockGitForMerge{
		mergeError: errors.New("some git error"),
	}

	err := CompleteMerge("feat/test-feature", "main", mock)
	if err == nil {
		t.Error("CompleteMerge() should return error on merge failure")
	}

	if !strings.Contains(err.Error(), "failed to merge") {
		t.Errorf("error should mention merge failure, got: %v", err)
	}
}

func TestCompleteMerge_PushFails(t *testing.T) {
	mock := &mockGitForMerge{
		pushError: errors.New("push rejected"),
	}

	err := CompleteMerge("feat/test-feature", "main", mock)
	if err == nil {
		t.Error("CompleteMerge() should return error on push failure")
	}

	if !strings.Contains(err.Error(), "failed to push") {
		t.Errorf("error should mention push failure, got: %v", err)
	}
}

func TestCompleteMerge_DeleteBranchFails(t *testing.T) {
	mock := &mockGitForMerge{
		deleteBranchError: errors.New("branch in use"),
	}

	// Should NOT fail - just log warning
	err := CompleteMerge("feat/test-feature", "main", mock)
	if err != nil {
		t.Errorf("CompleteMerge() should not fail when branch delete fails, got: %v", err)
	}
}

func TestCompleteMerge_DeleteRemoteBranchFails(t *testing.T) {
	mock := &mockGitForMerge{
		deleteRemoteError: errors.New("remote branch not found"),
	}

	// Should NOT fail - just log warning
	err := CompleteMerge("feat/test-feature", "main", mock)
	if err != nil {
		t.Errorf("CompleteMerge() should not fail when remote branch delete fails, got: %v", err)
	}
}

func TestCompleteMerge_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	tmpDir, err := os.MkdirTemp("", "merge-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	repoDir := filepath.Join(tmpDir, "repo")
	if err := os.MkdirAll(repoDir, 0755); err != nil {
		t.Fatalf("failed to create repo dir: %v", err)
	}

	runGitCmd := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = repoDir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=Test",
			"GIT_AUTHOR_EMAIL=test@test.com",
			"GIT_COMMITTER_NAME=Test",
			"GIT_COMMITTER_EMAIL=test@test.com",
		)
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v failed: %v\n%s", args, err, output)
		}
	}

	runGitCmd("init", "-b", "main")

	testFile := filepath.Join(repoDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("initial"), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}
	runGitCmd("add", ".")
	runGitCmd("commit", "-m", "initial commit")

	runGitCmd("checkout", "-b", "feat/test-feature")
	if err := os.WriteFile(testFile, []byte("feature change"), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}
	runGitCmd("add", ".")
	runGitCmd("commit", "-m", "feature commit")

	runGitCmd("checkout", "main")

	g := git.NewGit(repoDir)

	err = g.Checkout("main")
	if err != nil {
		t.Fatalf("checkout failed: %v", err)
	}

	err = g.Merge("feat/test-feature", true)
	if err != nil {
		t.Fatalf("merge failed: %v", err)
	}

	content, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("failed to read test file: %v", err)
	}

	if string(content) != "feature change" {
		t.Errorf("file content = %q, want %q", string(content), "feature change")
	}

	err = g.DeleteBranch("feat/test-feature", true)
	if err != nil {
		t.Errorf("delete branch failed: %v", err)
	}

	exists, err := g.BranchExists("feat/test-feature")
	if err != nil {
		t.Fatalf("branch exists check failed: %v", err)
	}
	if exists {
		t.Error("feature branch should be deleted")
	}

	t.Logf("Successfully merged feat/test-feature into main")
}

func TestBranchToWorktreeName(t *testing.T) {
	tests := []struct {
		branch   string
		expected string
	}{
		{"feat/my-plan", "feat-my-plan"},
		{"feat/nested/branch", "feat-nested-branch"},
		{"main", "main"},
		{"feature", "feature"},
	}

	for _, tt := range tests {
		got := branchToWorktreeName(tt.branch)
		if got != tt.expected {
			t.Errorf("branchToWorktreeName(%q) = %q, want %q", tt.branch, got, tt.expected)
		}
	}
}
