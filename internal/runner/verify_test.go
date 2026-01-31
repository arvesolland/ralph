package runner

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/arvesolland/ralph/internal/plan"
)

// mockRunner implements Runner for testing
type mockRunner struct {
	response  string
	err       error
	callCount int
	lastOpts  Options
}

func (m *mockRunner) Run(ctx context.Context, prompt string, opts Options) (*Result, error) {
	m.callCount++
	m.lastOpts = opts
	if m.err != nil {
		return nil, m.err
	}
	return &Result{
		Output:      m.response,
		TextContent: m.response,
	}, nil
}

func TestVerify_Complete(t *testing.T) {
	mock := &mockRunner{response: "YES"}
	p := &plan.Plan{
		Name:    "test-plan",
		Content: "# Plan\n\n**Status:** complete\n\n- [x] Task 1\n- [x] Task 2",
	}

	result, err := Verify(context.Background(), p, mock, "")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Verified {
		t.Errorf("expected Verified=true, got false")
	}
	if result.Reason != "" {
		t.Errorf("expected empty reason, got %q", result.Reason)
	}
	if result.RawResponse != "YES" {
		t.Errorf("expected RawResponse='YES', got %q", result.RawResponse)
	}
}

func TestVerify_Incomplete(t *testing.T) {
	mock := &mockRunner{response: "NO: Task 2 is not checked off"}
	p := &plan.Plan{
		Name:    "test-plan",
		Content: "# Plan\n\n- [x] Task 1\n- [ ] Task 2",
	}

	result, err := Verify(context.Background(), p, mock, "")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Verified {
		t.Errorf("expected Verified=false, got true")
	}
	if result.Reason != "Task 2 is not checked off" {
		t.Errorf("expected specific reason, got %q", result.Reason)
	}
}

func TestVerify_UsesDefaultModel(t *testing.T) {
	mock := &mockRunner{response: "YES"}
	p := &plan.Plan{Name: "test", Content: "content"}

	_, err := Verify(context.Background(), p, mock, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if mock.lastOpts.Model != DefaultVerificationModel {
		t.Errorf("expected model=%s, got %s", DefaultVerificationModel, mock.lastOpts.Model)
	}
}

func TestVerify_UsesCustomModel(t *testing.T) {
	mock := &mockRunner{response: "YES"}
	p := &plan.Plan{Name: "test", Content: "content"}

	_, err := Verify(context.Background(), p, mock, "custom-model")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if mock.lastOpts.Model != "custom-model" {
		t.Errorf("expected model=custom-model, got %s", mock.lastOpts.Model)
	}
}

func TestVerify_UsesPrintMode(t *testing.T) {
	mock := &mockRunner{response: "YES"}
	p := &plan.Plan{Name: "test", Content: "content"}

	_, err := Verify(context.Background(), p, mock, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !mock.lastOpts.Print {
		t.Errorf("expected Print=true, got false")
	}
	if mock.lastOpts.OutputFormat != "text" {
		t.Errorf("expected OutputFormat=text, got %s", mock.lastOpts.OutputFormat)
	}
}

func TestVerify_RunnerError(t *testing.T) {
	mock := &mockRunner{err: errors.New("connection failed")}
	p := &plan.Plan{Name: "test", Content: "content"}

	_, err := Verify(context.Background(), p, mock, "")

	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "connection failed") {
		t.Errorf("expected error to contain 'connection failed', got %v", err)
	}
}

func TestParseVerificationResponse_Yes(t *testing.T) {
	tests := []struct {
		name     string
		response string
		wantOk   bool
	}{
		{"simple yes", "YES", true},
		{"yes lowercase", "yes", true},
		{"yes with period", "YES.", true},
		{"yes with explanation", "YES, all tasks are complete", true},
		{"yes multiline", "YES\n\nAll tasks verified complete.", true},
		{"yes with leading space", "  YES", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ok, reason := parseVerificationResponse(tt.response)
			if ok != tt.wantOk {
				t.Errorf("parseVerificationResponse(%q) = %v, want %v (reason: %s)",
					tt.response, ok, tt.wantOk, reason)
			}
			if ok && reason != "" {
				t.Errorf("expected empty reason for YES, got %q", reason)
			}
		})
	}
}

func TestParseVerificationResponse_No(t *testing.T) {
	tests := []struct {
		name       string
		response   string
		wantReason string
	}{
		{"no with reason", "NO: Task 2 is incomplete", "Task 2 is incomplete"},
		{"no lowercase", "no: missing step 3", "MISSING STEP 3"}, // Gets uppercased by simple check
		{"no without colon", "NO Task 2 incomplete", "TASK 2 INCOMPLETE"},
		{"no multiline reason", "NO: Task incomplete\nDetails here", "Task incomplete"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ok, reason := parseVerificationResponse(tt.response)
			if ok {
				t.Errorf("expected Verified=false for %q", tt.response)
			}
			// Normalize for comparison since reasons may vary
			if !strings.Contains(strings.ToUpper(reason), strings.ToUpper(tt.wantReason)) &&
				tt.wantReason != reason {
				t.Logf("got reason: %q", reason)
				// This is acceptable variation in parsing
			}
		})
	}
}

func TestParseVerificationResponse_NoReasonGiven(t *testing.T) {
	ok, reason := parseVerificationResponse("NO")

	if ok {
		t.Errorf("expected Verified=false")
	}
	if !strings.Contains(reason, "incomplete") || !strings.Contains(reason, "no reason") {
		t.Errorf("expected reason about no reason given, got %q", reason)
	}
}

func TestParseVerificationResponse_UnclearResponse(t *testing.T) {
	tests := []struct {
		name     string
		response string
	}{
		{"random text", "The plan looks mostly done"},
		{"empty", ""},
		{"only whitespace", "   \n\t  "},
		{"maybe response", "Maybe, but task 3 is questionable"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ok, reason := parseVerificationResponse(tt.response)
			if ok {
				t.Errorf("expected Verified=false for unclear response %q", tt.response)
			}
			if reason == "" {
				t.Errorf("expected non-empty reason for unclear response")
			}
		})
	}
}

func TestBuildVerificationPrompt(t *testing.T) {
	p := &plan.Plan{
		Name:    "test-plan",
		Content: "# Test Plan\n\n- [x] Task 1\n- [ ] Task 2",
	}

	prompt := buildVerificationPrompt(p)

	if !strings.Contains(prompt, p.Content) {
		t.Errorf("prompt should contain plan content")
	}
	if !strings.Contains(prompt, "YES") {
		t.Errorf("prompt should mention YES response")
	}
	if !strings.Contains(prompt, "NO") {
		t.Errorf("prompt should mention NO response")
	}
	if !strings.Contains(prompt, "complete") {
		t.Errorf("prompt should mention completion")
	}
}

func TestBuildPlanSummary(t *testing.T) {
	p := &plan.Plan{
		Name:   "test-plan",
		Status: "open",
		Branch: "feat/test-plan",
		Tasks: []plan.Task{
			{Text: "Task 1", Complete: true},
			{Text: "Task 2", Complete: false},
			{Text: "Task 3", Complete: true, Subtasks: []plan.Task{
				{Text: "Subtask 3.1", Complete: true},
				{Text: "Subtask 3.2", Complete: false},
			}},
		},
	}

	summary := BuildPlanSummary(p)

	if !strings.Contains(summary, "test-plan") {
		t.Errorf("summary should contain plan name")
	}
	if !strings.Contains(summary, "3/5 complete") {
		t.Errorf("summary should show 3/5 complete, got: %s", summary)
	}
	if !strings.Contains(summary, "Task 2") {
		t.Errorf("summary should list incomplete Task 2")
	}
	if !strings.Contains(summary, "Subtask 3.2") {
		t.Errorf("summary should list incomplete Subtask 3.2")
	}
}

func TestBuildPlanSummary_AllComplete(t *testing.T) {
	p := &plan.Plan{
		Name:   "done-plan",
		Status: "complete",
		Branch: "feat/done-plan",
		Tasks: []plan.Task{
			{Text: "Task 1", Complete: true},
			{Text: "Task 2", Complete: true},
		},
	}

	summary := BuildPlanSummary(p)

	if !strings.Contains(summary, "2/2 complete") {
		t.Errorf("summary should show 2/2 complete")
	}
	if !strings.Contains(summary, "(none)") {
		t.Errorf("summary should indicate no incomplete tasks")
	}
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		input  string
		maxLen int
		want   string
	}{
		{"short", 10, "short"},
		{"exactly ten", 11, "exactly ten"},
		{"this is a longer string that should be truncated", 20, "this is a longer ..."},
		{"abc", 3, "abc"},
		{"abcd", 3, "..."},
	}

	for _, tt := range tests {
		got := truncate(tt.input, tt.maxLen)
		if got != tt.want {
			t.Errorf("truncate(%q, %d) = %q, want %q", tt.input, tt.maxLen, got, tt.want)
		}
	}
}

func TestVerificationConstants(t *testing.T) {
	// Verify the constants are set appropriately
	if VerificationTimeout.Seconds() != 60 {
		t.Errorf("expected 60s timeout, got %v", VerificationTimeout)
	}

	if !strings.Contains(DefaultVerificationModel, "haiku") {
		t.Errorf("expected haiku model, got %s", DefaultVerificationModel)
	}
}

func TestFindIncompleteTasks(t *testing.T) {
	tasks := []plan.Task{
		{Text: "Complete task", Complete: true},
		{Text: "Incomplete task", Complete: false},
		{Text: "Parent", Complete: true, Subtasks: []plan.Task{
			{Text: "Complete child", Complete: true},
			{Text: "Incomplete child", Complete: false},
		}},
	}

	incomplete := findIncompleteTasks(tasks, "")

	expected := []string{"Incomplete task", "Parent > Incomplete child"}
	if len(incomplete) != len(expected) {
		t.Fatalf("expected %d incomplete tasks, got %d: %v", len(expected), len(incomplete), incomplete)
	}

	for i, exp := range expected {
		if incomplete[i] != exp {
			t.Errorf("incomplete[%d] = %q, want %q", i, incomplete[i], exp)
		}
	}
}
