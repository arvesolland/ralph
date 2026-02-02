package notify

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/arvesolland/ralph/internal/plan"
	"github.com/arvesolland/ralph/internal/runner"
)

func TestNewWebhookNotifier_EmptyURL(t *testing.T) {
	n := NewWebhookNotifier("")
	if n != nil {
		t.Error("expected nil notifier for empty URL")
	}
}

func TestNewWebhookNotifier_ValidURL(t *testing.T) {
	n := NewWebhookNotifier("https://hooks.slack.com/test")
	if n == nil {
		t.Fatal("expected non-nil notifier")
	}
	if n.webhookURL != "https://hooks.slack.com/test" {
		t.Errorf("expected webhook URL to be set, got %s", n.webhookURL)
	}
	if n.httpClient == nil {
		t.Error("expected http client to be set")
	}
}

func TestWebhookNotifier_Start(t *testing.T) {
	var received slackMessage
	var mu sync.Mutex
	done := make(chan struct{})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Errorf("failed to decode request: %v", err)
		}
		close(done)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	n := NewWebhookNotifier(server.URL)
	p := &plan.Plan{Name: "test-plan", Branch: "feat/test-plan"}

	err := n.Start(p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Wait for async send
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for notification")
	}

	mu.Lock()
	defer mu.Unlock()

	if len(received.Blocks) != 2 {
		t.Errorf("expected 2 blocks, got %d", len(received.Blocks))
	}

	// Verify content
	if received.Blocks[0].Text == nil {
		t.Fatal("expected text in first block")
	}
	if received.Blocks[0].Text.Type != "mrkdwn" {
		t.Errorf("expected mrkdwn type, got %s", received.Blocks[0].Text.Type)
	}
}

func TestWebhookNotifier_Complete_WithPR(t *testing.T) {
	var received slackMessage
	var mu sync.Mutex
	done := make(chan struct{})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		json.NewDecoder(r.Body).Decode(&received)
		close(done)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	n := NewWebhookNotifier(server.URL)
	p := &plan.Plan{Name: "test-plan", Branch: "feat/test-plan"}

	err := n.Complete(p, "https://github.com/owner/repo/pull/123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for notification")
	}

	mu.Lock()
	defer mu.Unlock()

	// Should have fields including PR URL
	if len(received.Blocks) < 2 {
		t.Fatalf("expected at least 2 blocks, got %d", len(received.Blocks))
	}

	found := false
	for _, block := range received.Blocks {
		for _, field := range block.Fields {
			if field.Text != "" {
				found = true
			}
		}
	}
	if !found {
		t.Error("expected fields in message")
	}
}

func TestWebhookNotifier_Complete_NoPR(t *testing.T) {
	var received slackMessage
	var mu sync.Mutex
	done := make(chan struct{})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		json.NewDecoder(r.Body).Decode(&received)
		close(done)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	n := NewWebhookNotifier(server.URL)
	p := &plan.Plan{Name: "test-plan", Branch: "feat/test-plan"}

	err := n.Complete(p, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for notification")
	}

	mu.Lock()
	defer mu.Unlock()

	if len(received.Blocks) != 2 {
		t.Errorf("expected 2 blocks, got %d", len(received.Blocks))
	}
}

func TestWebhookNotifier_Blocker(t *testing.T) {
	var received slackMessage
	var mu sync.Mutex
	done := make(chan struct{})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		json.NewDecoder(r.Body).Decode(&received)
		close(done)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	n := NewWebhookNotifier(server.URL)
	p := &plan.Plan{Name: "test-plan", Branch: "feat/test-plan"}
	blocker := &runner.Blocker{
		Content:     "Full blocker content",
		Description: "Need human input",
		Action:      "Click the button",
		Resume:      "Will continue after",
		Hash:        "abc12345",
	}

	err := n.Blocker(p, blocker)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for notification")
	}

	mu.Lock()
	defer mu.Unlock()

	// Should have header + description + action + resume blocks
	if len(received.Blocks) < 4 {
		t.Errorf("expected at least 4 blocks, got %d", len(received.Blocks))
	}
}

func TestWebhookNotifier_Blocker_Nil(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not send request for nil blocker")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	n := NewWebhookNotifier(server.URL)
	p := &plan.Plan{Name: "test-plan"}

	err := n.Blocker(p, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Give time for async send (should not happen)
	time.Sleep(100 * time.Millisecond)
}

func TestWebhookNotifier_Error(t *testing.T) {
	var received slackMessage
	var mu sync.Mutex
	done := make(chan struct{})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		json.NewDecoder(r.Body).Decode(&received)
		close(done)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	n := NewWebhookNotifier(server.URL)
	p := &plan.Plan{Name: "test-plan"}

	err := n.Error(p, errors.New("something went wrong"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for notification")
	}

	mu.Lock()
	defer mu.Unlock()

	if len(received.Blocks) != 2 {
		t.Errorf("expected 2 blocks, got %d", len(received.Blocks))
	}
}

func TestWebhookNotifier_Error_Nil(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not send request for nil error")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	n := NewWebhookNotifier(server.URL)
	p := &plan.Plan{Name: "test-plan"}

	err := n.Error(p, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	time.Sleep(100 * time.Millisecond)
}

func TestWebhookNotifier_Error_TruncatesLongMessage(t *testing.T) {
	var received slackMessage
	var mu sync.Mutex
	done := make(chan struct{})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		json.NewDecoder(r.Body).Decode(&received)
		close(done)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	n := NewWebhookNotifier(server.URL)
	p := &plan.Plan{Name: "test-plan"}

	// Create error message > 500 chars
	longErr := make([]byte, 600)
	for i := range longErr {
		longErr[i] = 'a'
	}

	err := n.Error(p, errors.New(string(longErr)))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for notification")
	}

	mu.Lock()
	defer mu.Unlock()

	// Error message should be truncated
	if len(received.Blocks) < 2 {
		t.Fatal("expected at least 2 blocks")
	}
	errorBlock := received.Blocks[1]
	if errorBlock.Text == nil {
		t.Fatal("expected text in error block")
	}
	// Should contain "..." indicating truncation
	if len(errorBlock.Text.Text) > 600 {
		t.Errorf("error message should be truncated, got length %d", len(errorBlock.Text.Text))
	}
}

func TestWebhookNotifier_Iteration(t *testing.T) {
	var received slackMessage
	var mu sync.Mutex
	done := make(chan struct{})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		json.NewDecoder(r.Body).Decode(&received)
		close(done)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	n := NewWebhookNotifier(server.URL)
	p := &plan.Plan{Name: "test-plan"}

	err := n.Iteration(p, 5, 30)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for notification")
	}

	mu.Lock()
	defer mu.Unlock()

	if len(received.Blocks) != 1 {
		t.Errorf("expected 1 block, got %d", len(received.Blocks))
	}
}

func TestWebhookNotifier_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	n := NewWebhookNotifier(server.URL)
	p := &plan.Plan{Name: "test-plan"}

	// Should not return error (async and swallowed)
	err := n.Start(p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Give time for async send
	time.Sleep(100 * time.Millisecond)
}

func TestWebhookNotifier_Send_ContentType(t *testing.T) {
	var contentType string
	done := make(chan struct{})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		contentType = r.Header.Get("Content-Type")
		close(done)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	n := NewWebhookNotifier(server.URL)
	p := &plan.Plan{Name: "test-plan"}

	n.Start(p)

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for notification")
	}

	if contentType != "application/json" {
		t.Errorf("expected Content-Type application/json, got %s", contentType)
	}
}

func TestNoopNotifier(t *testing.T) {
	n := &NoopNotifier{}
	p := &plan.Plan{Name: "test"}

	if err := n.Start(p); err != nil {
		t.Errorf("Start: unexpected error: %v", err)
	}
	if err := n.Complete(p, ""); err != nil {
		t.Errorf("Complete: unexpected error: %v", err)
	}
	if err := n.Blocker(p, &runner.Blocker{}); err != nil {
		t.Errorf("Blocker: unexpected error: %v", err)
	}
	if err := n.Error(p, errors.New("test")); err != nil {
		t.Errorf("Error: unexpected error: %v", err)
	}
	if err := n.Iteration(p, 1, 10); err != nil {
		t.Errorf("Iteration: unexpected error: %v", err)
	}
}

func TestNotifierInterface(t *testing.T) {
	// Ensure both types implement Notifier
	var _ Notifier = (*WebhookNotifier)(nil)
	var _ Notifier = (*NoopNotifier)(nil)
}

// TestBlockerFlow_ClaudeOutputToSlackMessage tests the complete flow from
// Claude output containing <blocker> tags through to the final Slack message.
// This is an integration test that verifies:
// 1. Blocker is correctly parsed from Claude output
// 2. Parsed blocker is sent to Slack webhook
// 3. Slack message contains all blocker fields (Description, Action, Resume)
func TestBlockerFlow_ClaudeOutputToSlackMessage(t *testing.T) {
	tests := []struct {
		name           string
		claudeOutput   string
		wantDesc       string
		wantAction     string
		wantResume     string
		wantBlockCount int // minimum number of blocks in Slack message
	}{
		{
			name: "full structured blocker",
			claudeOutput: `I've analyzed the codebase and found an issue.

<blocker>
The GitHub package needs to be made public before we can proceed.
Action: Go to https://github.com/org/repo/packages → Settings → Change visibility to Public
Resume: Once public, I will verify anonymous pull works and complete the deployment.
</blocker>

I'll wait for this to be resolved.`,
			wantDesc:       "The GitHub package needs to be made public before we can proceed.",
			wantAction:     "Go to https://github.com/org/repo/packages → Settings → Change visibility to Public",
			wantResume:     "Once public, I will verify anonymous pull works and complete the deployment.",
			wantBlockCount: 4, // header + desc + action + resume
		},
		{
			name: "blocker with explicit Description field",
			claudeOutput: `<blocker>
Description: The API credentials have expired.
Action: Generate new credentials at https://api.example.com/settings
Resume: Will update the config and retry the API call.
</blocker>`,
			wantDesc:       "The API credentials have expired.",
			wantAction:     "Generate new credentials at https://api.example.com/settings",
			wantResume:     "Will update the config and retry the API call.",
			wantBlockCount: 4,
		},
		{
			name: "blocker with multiline description",
			claudeOutput: `<blocker>
The deployment is blocked for multiple reasons:
1. The secret key is missing
2. The database needs migration
3. The cache needs to be cleared
Action: Run the setup script: ./scripts/setup.sh
Resume: Will continue with deployment after setup completes.
</blocker>`,
			wantDesc:       "The deployment is blocked for multiple reasons:\n1. The secret key is missing\n2. The database needs migration\n3. The cache needs to be cleared",
			wantAction:     "Run the setup script: ./scripts/setup.sh",
			wantResume:     "Will continue with deployment after setup completes.",
			wantBlockCount: 4,
		},
		{
			name: "blocker with only description and action",
			claudeOutput: `<blocker>
Need approval to proceed with the merge.
Action: Review and approve the PR at https://github.com/org/repo/pull/123
</blocker>`,
			wantDesc:       "Need approval to proceed with the merge.",
			wantAction:     "Review and approve the PR at https://github.com/org/repo/pull/123",
			wantResume:     "",
			wantBlockCount: 3, // header + desc + action (no resume)
		},
		{
			name: "simple blocker without structured fields",
			claudeOutput: `<blocker>
Waiting for human confirmation to delete the production database.
</blocker>`,
			wantDesc:       "Waiting for human confirmation to delete the production database.",
			wantAction:     "",
			wantResume:     "",
			wantBlockCount: 2, // header + desc only
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Step 1: Parse blocker from Claude output (simulates runner behavior)
			blocker := runner.ExtractBlocker(tt.claudeOutput)
			if blocker == nil {
				t.Fatal("expected blocker to be extracted from Claude output")
			}

			// Verify parsing is correct
			if blocker.Description != tt.wantDesc {
				t.Errorf("parsed Description = %q, want %q", blocker.Description, tt.wantDesc)
			}
			if blocker.Action != tt.wantAction {
				t.Errorf("parsed Action = %q, want %q", blocker.Action, tt.wantAction)
			}
			if blocker.Resume != tt.wantResume {
				t.Errorf("parsed Resume = %q, want %q", blocker.Resume, tt.wantResume)
			}

			// Step 2: Send to Slack webhook and capture the message
			var received slackMessage
			var mu sync.Mutex
			done := make(chan struct{})

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				defer mu.Unlock()
				if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
					t.Errorf("failed to decode webhook request: %v", err)
				}
				close(done)
				w.WriteHeader(http.StatusOK)
			}))
			defer server.Close()

			notifier := NewWebhookNotifier(server.URL)
			p := &plan.Plan{Name: "test-plan", Branch: "feat/test-plan"}

			// Step 3: Send the blocker notification
			err := notifier.Blocker(p, blocker)
			if err != nil {
				t.Fatalf("Blocker notification failed: %v", err)
			}

			// Wait for async send
			select {
			case <-done:
			case <-time.After(2 * time.Second):
				t.Fatal("timeout waiting for webhook")
			}

			mu.Lock()
			defer mu.Unlock()

			// Step 4: Verify Slack message structure
			if len(received.Blocks) < tt.wantBlockCount {
				t.Errorf("expected at least %d blocks, got %d", tt.wantBlockCount, len(received.Blocks))
			}

			// Verify the message contains the blocker content
			msgJSON, _ := json.Marshal(received)
			msgStr := string(msgJSON)

			// For multiline descriptions, check for key parts rather than exact match
			// (JSON encoding escapes newlines)
			if tt.wantDesc != "" {
				// Check for first line of description
				firstLine := tt.wantDesc
				if idx := indexOf(tt.wantDesc, "\n"); idx > 0 {
					firstLine = tt.wantDesc[:idx]
				}
				if !containsString(msgStr, firstLine) {
					t.Errorf("Slack message should contain description starting with %q", firstLine)
				}
			}
			if tt.wantAction != "" && !containsString(msgStr, tt.wantAction) {
				t.Errorf("Slack message should contain action %q", tt.wantAction)
			}
			if tt.wantResume != "" && !containsString(msgStr, tt.wantResume) {
				t.Errorf("Slack message should contain resume %q", tt.wantResume)
			}

			// Verify it mentions the plan
			if !containsString(msgStr, "test-plan") {
				t.Error("Slack message should mention the plan name")
			}
		})
	}
}

// containsString checks if s contains substr (simple substring match)
func containsString(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// indexOf returns the index of substr in s, or -1 if not found
func indexOf(s, substr string) int {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
