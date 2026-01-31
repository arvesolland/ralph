package notify

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/arvesolland/ralph/internal/plan"
	"github.com/arvesolland/ralph/internal/runner"
	"github.com/slack-go/slack"
)

// mockSlackServer creates a mock Slack API server for testing.
type mockSlackServer struct {
	*httptest.Server
	mu       sync.Mutex
	messages []mockMessage
}

type mockMessage struct {
	Channel  string
	Text     string
	Blocks   []json.RawMessage
	ThreadTS string
}

func newMockSlackServer() *mockSlackServer {
	m := &mockSlackServer{
		messages: make([]mockMessage, 0),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/chat.postMessage", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		m.mu.Lock()
		defer m.mu.Unlock()

		msg := mockMessage{
			Channel:  r.FormValue("channel"),
			Text:     r.FormValue("text"),
			ThreadTS: r.FormValue("thread_ts"),
		}

		// Parse blocks if present
		if blocksStr := r.FormValue("blocks"); blocksStr != "" {
			var blocks []json.RawMessage
			if err := json.Unmarshal([]byte(blocksStr), &blocks); err == nil {
				msg.Blocks = blocks
			}
		}

		m.messages = append(m.messages, msg)

		// Return a successful response with a timestamp
		// Use a simple map for the response
		resp := map[string]interface{}{
			"ok":      true,
			"ts":      "1234567890.123456",
			"channel": msg.Channel,
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})

	m.Server = httptest.NewServer(mux)
	return m
}

func (m *mockSlackServer) getMessages() []mockMessage {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]mockMessage, len(m.messages))
	copy(result, m.messages)
	return result
}

func TestNewSlackNotifier_WithBotToken(t *testing.T) {
	cfg := SlackNotifierConfig{
		BotToken: "xoxb-test-token",
		Channel:  "C12345",
	}

	notifier := NewSlackNotifier(cfg)
	if notifier == nil {
		t.Fatal("expected notifier to be created")
	}

	_, ok := notifier.(*SlackNotifier)
	if !ok {
		t.Errorf("expected *SlackNotifier, got %T", notifier)
	}
}

func TestNewSlackNotifier_FallbackToWebhook(t *testing.T) {
	cfg := SlackNotifierConfig{
		WebhookURL: "https://hooks.slack.com/test",
	}

	notifier := NewSlackNotifier(cfg)
	if notifier == nil {
		t.Fatal("expected notifier to be created")
	}

	_, ok := notifier.(*WebhookNotifier)
	if !ok {
		t.Errorf("expected *WebhookNotifier, got %T", notifier)
	}
}

func TestNewSlackNotifier_NoConfig(t *testing.T) {
	cfg := SlackNotifierConfig{}

	notifier := NewSlackNotifier(cfg)
	if notifier == nil {
		t.Fatal("expected notifier to be created")
	}

	_, ok := notifier.(*NoopNotifier)
	if !ok {
		t.Errorf("expected *NoopNotifier, got %T", notifier)
	}
}

func TestNewSlackNotifier_BotTokenWithoutChannel(t *testing.T) {
	cfg := SlackNotifierConfig{
		BotToken:   "xoxb-test-token",
		WebhookURL: "https://hooks.slack.com/test",
	}

	notifier := NewSlackNotifier(cfg)
	if notifier == nil {
		t.Fatal("expected notifier to be created")
	}

	// Should fall back to webhook since no channel
	_, ok := notifier.(*WebhookNotifier)
	if !ok {
		t.Errorf("expected *WebhookNotifier when channel missing, got %T", notifier)
	}
}

func TestSlackNotifier_Start(t *testing.T) {
	server := newMockSlackServer()
	defer server.Close()

	// Create a client that uses our mock server
	client := slack.New("xoxb-test-token", slack.OptionAPIURL(server.URL+"/"))

	// Create a temp file for thread tracking
	tmpDir := t.TempDir()
	tracker, err := NewThreadTracker(filepath.Join(tmpDir, "threads.json"))
	if err != nil {
		t.Fatalf("failed to create thread tracker: %v", err)
	}

	notifier := &SlackNotifier{
		client:        client,
		channel:       "C12345",
		threadTracker: tracker,
	}

	p := &plan.Plan{
		Name:   "test-plan",
		Branch: "feat/test-plan",
	}

	err = notifier.Start(p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Give async operation time to complete
	time.Sleep(100 * time.Millisecond)

	msgs := server.getMessages()
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}

	if msgs[0].Channel != "C12345" {
		t.Errorf("expected channel C12345, got %s", msgs[0].Channel)
	}

	// Verify thread was saved
	info := tracker.Get("test-plan")
	if info == nil {
		t.Error("expected thread info to be saved")
	} else if info.ThreadTS != "1234567890.123456" {
		t.Errorf("expected ThreadTS 1234567890.123456, got %s", info.ThreadTS)
	}
}

func TestSlackNotifier_Complete(t *testing.T) {
	server := newMockSlackServer()
	defer server.Close()

	client := slack.New("xoxb-test-token", slack.OptionAPIURL(server.URL+"/"))

	tmpDir := t.TempDir()
	tracker, err := NewThreadTracker(filepath.Join(tmpDir, "threads.json"))
	if err != nil {
		t.Fatalf("failed to create thread tracker: %v", err)
	}

	// Pre-populate thread info
	tracker.Set("test-plan", &ThreadInfo{
		PlanName:  "test-plan",
		ThreadTS:  "1234567890.000000",
		ChannelID: "C12345",
	})

	notifier := &SlackNotifier{
		client:        client,
		channel:       "C12345",
		threadTracker: tracker,
	}

	p := &plan.Plan{
		Name:   "test-plan",
		Branch: "feat/test-plan",
	}

	err = notifier.Complete(p, "https://github.com/test/pr/1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Give async operation time to complete
	time.Sleep(100 * time.Millisecond)

	msgs := server.getMessages()
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}

	// Verify it was sent as a thread reply
	if msgs[0].ThreadTS != "1234567890.000000" {
		t.Errorf("expected thread reply, got ThreadTS=%s", msgs[0].ThreadTS)
	}
}

func TestSlackNotifier_Blocker(t *testing.T) {
	server := newMockSlackServer()
	defer server.Close()

	client := slack.New("xoxb-test-token", slack.OptionAPIURL(server.URL+"/"))

	tmpDir := t.TempDir()
	tracker, err := NewThreadTracker(filepath.Join(tmpDir, "threads.json"))
	if err != nil {
		t.Fatalf("failed to create thread tracker: %v", err)
	}

	// Pre-populate thread info
	tracker.Set("test-plan", &ThreadInfo{
		PlanName:  "test-plan",
		ThreadTS:  "1234567890.000000",
		ChannelID: "C12345",
	})

	notifier := &SlackNotifier{
		client:        client,
		channel:       "C12345",
		threadTracker: tracker,
	}

	p := &plan.Plan{
		Name:   "test-plan",
		Branch: "feat/test-plan",
	}

	blocker := &runner.Blocker{
		Content:     "Package needs to be made public",
		Description: "Package needs to be made public",
		Action:      "Go to GitHub and make it public",
		Resume:      "Will verify package is accessible",
		Hash:        "abc12345",
	}

	err = notifier.Blocker(p, blocker)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Give async operation time to complete
	time.Sleep(100 * time.Millisecond)

	msgs := server.getMessages()
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}

	// Verify blocker was marked as notified
	if !tracker.HasNotifiedBlocker("test-plan", "abc12345") {
		t.Error("expected blocker to be marked as notified")
	}
}

func TestSlackNotifier_Blocker_Deduplication(t *testing.T) {
	server := newMockSlackServer()
	defer server.Close()

	client := slack.New("xoxb-test-token", slack.OptionAPIURL(server.URL+"/"))

	tmpDir := t.TempDir()
	tracker, err := NewThreadTracker(filepath.Join(tmpDir, "threads.json"))
	if err != nil {
		t.Fatalf("failed to create thread tracker: %v", err)
	}

	// Pre-populate thread info with already notified blocker
	tracker.Set("test-plan", &ThreadInfo{
		PlanName:         "test-plan",
		ThreadTS:         "1234567890.000000",
		ChannelID:        "C12345",
		NotifiedBlockers: []string{"abc12345"},
	})

	notifier := &SlackNotifier{
		client:        client,
		channel:       "C12345",
		threadTracker: tracker,
	}

	p := &plan.Plan{
		Name:   "test-plan",
		Branch: "feat/test-plan",
	}

	blocker := &runner.Blocker{
		Content: "Package needs to be made public",
		Hash:    "abc12345", // Same hash as already notified
	}

	err = notifier.Blocker(p, blocker)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Give async operation time to complete
	time.Sleep(100 * time.Millisecond)

	msgs := server.getMessages()
	if len(msgs) != 0 {
		t.Errorf("expected no messages (duplicate blocker), got %d", len(msgs))
	}
}

func TestSlackNotifier_Blocker_Nil(t *testing.T) {
	server := newMockSlackServer()
	defer server.Close()

	client := slack.New("xoxb-test-token", slack.OptionAPIURL(server.URL+"/"))

	notifier := &SlackNotifier{
		client:  client,
		channel: "C12345",
	}

	p := &plan.Plan{
		Name:   "test-plan",
		Branch: "feat/test-plan",
	}

	err := notifier.Blocker(p, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Give async operation time to complete
	time.Sleep(100 * time.Millisecond)

	msgs := server.getMessages()
	if len(msgs) != 0 {
		t.Errorf("expected no messages for nil blocker, got %d", len(msgs))
	}
}

func TestSlackNotifier_Error(t *testing.T) {
	server := newMockSlackServer()
	defer server.Close()

	client := slack.New("xoxb-test-token", slack.OptionAPIURL(server.URL+"/"))

	notifier := &SlackNotifier{
		client:  client,
		channel: "C12345",
	}

	p := &plan.Plan{
		Name:   "test-plan",
		Branch: "feat/test-plan",
	}

	err := notifier.Error(p, runner.ErrRateLimit)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Give async operation time to complete
	time.Sleep(100 * time.Millisecond)

	msgs := server.getMessages()
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
}

func TestSlackNotifier_Error_TruncatesLongMessage(t *testing.T) {
	server := newMockSlackServer()
	defer server.Close()

	client := slack.New("xoxb-test-token", slack.OptionAPIURL(server.URL+"/"))

	notifier := &SlackNotifier{
		client:  client,
		channel: "C12345",
	}

	p := &plan.Plan{
		Name:   "test-plan",
		Branch: "feat/test-plan",
	}

	longError := strings.Repeat("a", 600)
	err := notifier.Error(p, &mockError{msg: longError})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Give async operation time to complete
	time.Sleep(100 * time.Millisecond)

	msgs := server.getMessages()
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
}

type mockError struct {
	msg string
}

func (e *mockError) Error() string {
	return e.msg
}

func TestSlackNotifier_Iteration(t *testing.T) {
	server := newMockSlackServer()
	defer server.Close()

	client := slack.New("xoxb-test-token", slack.OptionAPIURL(server.URL+"/"))

	notifier := &SlackNotifier{
		client:  client,
		channel: "C12345",
	}

	p := &plan.Plan{
		Name:   "test-plan",
		Branch: "feat/test-plan",
	}

	err := notifier.Iteration(p, 5, 30)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Give async operation time to complete
	time.Sleep(100 * time.Millisecond)

	msgs := server.getMessages()
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
}

func TestSlackNotifier_PostMessageInThread_NoThread(t *testing.T) {
	server := newMockSlackServer()
	defer server.Close()

	client := slack.New("xoxb-test-token", slack.OptionAPIURL(server.URL+"/"))

	// No thread tracker - should post to channel directly
	notifier := &SlackNotifier{
		client:  client,
		channel: "C12345",
	}

	p := &plan.Plan{
		Name:   "test-plan",
		Branch: "feat/test-plan",
	}

	err := notifier.Iteration(p, 1, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Give async operation time to complete
	time.Sleep(100 * time.Millisecond)

	msgs := server.getMessages()
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}

	// Should have no thread_ts since no tracker
	if msgs[0].ThreadTS != "" {
		t.Errorf("expected no ThreadTS, got %s", msgs[0].ThreadTS)
	}
}

func TestSlackNotifierConfig(t *testing.T) {
	tests := []struct {
		name       string
		cfg        SlackNotifierConfig
		isSlack    bool
		isWebhook  bool
		isNoop     bool
	}{
		{
			name: "bot token and channel",
			cfg: SlackNotifierConfig{
				BotToken: "xoxb-test",
				Channel:  "C12345",
			},
			isSlack: true,
		},
		{
			name: "bot token without channel falls back to webhook",
			cfg: SlackNotifierConfig{
				BotToken:   "xoxb-test",
				WebhookURL: "https://hooks.slack.com/test",
			},
			isWebhook: true,
		},
		{
			name: "only webhook",
			cfg: SlackNotifierConfig{
				WebhookURL: "https://hooks.slack.com/test",
			},
			isWebhook: true,
		},
		{
			name:   "no config",
			cfg:    SlackNotifierConfig{},
			isNoop: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			notifier := NewSlackNotifier(tc.cfg)
			_, isSlack := notifier.(*SlackNotifier)
			_, isWebhook := notifier.(*WebhookNotifier)
			_, isNoop := notifier.(*NoopNotifier)

			if tc.isSlack && !isSlack {
				t.Errorf("expected *SlackNotifier")
			}
			if tc.isWebhook && !isWebhook {
				t.Errorf("expected *WebhookNotifier")
			}
			if tc.isNoop && !isNoop {
				t.Errorf("expected *NoopNotifier")
			}
		})
	}
}

func TestSlackNotifier_WithThreadTracker(t *testing.T) {
	tmpDir := t.TempDir()
	trackerPath := filepath.Join(tmpDir, "threads.json")

	tracker, err := NewThreadTracker(trackerPath)
	if err != nil {
		t.Fatalf("failed to create tracker: %v", err)
	}

	cfg := SlackNotifierConfig{
		BotToken:      "xoxb-test",
		Channel:       "C12345",
		ThreadTracker: tracker,
	}

	notifier := NewSlackNotifier(cfg)
	slackNotifier, ok := notifier.(*SlackNotifier)
	if !ok {
		t.Fatal("expected SlackNotifier")
	}

	if slackNotifier.threadTracker == nil {
		t.Error("expected thread tracker to be set")
	}
}

func TestSlackNotifierInterface(t *testing.T) {
	// Verify SlackNotifier implements Notifier interface
	var _ Notifier = (*SlackNotifier)(nil)
}

func TestSlackNotifier_CompleteWithoutPR(t *testing.T) {
	server := newMockSlackServer()
	defer server.Close()

	client := slack.New("xoxb-test-token", slack.OptionAPIURL(server.URL+"/"))

	notifier := &SlackNotifier{
		client:  client,
		channel: "C12345",
	}

	p := &plan.Plan{
		Name:   "test-plan",
		Branch: "feat/test-plan",
	}

	err := notifier.Complete(p, "") // Empty PR URL
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Give async operation time to complete
	time.Sleep(100 * time.Millisecond)

	msgs := server.getMessages()
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
}

func TestSlackNotifier_Error_Nil(t *testing.T) {
	server := newMockSlackServer()
	defer server.Close()

	client := slack.New("xoxb-test-token", slack.OptionAPIURL(server.URL+"/"))

	notifier := &SlackNotifier{
		client:  client,
		channel: "C12345",
	}

	p := &plan.Plan{
		Name:   "test-plan",
		Branch: "feat/test-plan",
	}

	err := notifier.Error(p, nil) // Nil error
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Give async operation time to complete
	time.Sleep(100 * time.Millisecond)

	msgs := server.getMessages()
	if len(msgs) != 0 {
		t.Errorf("expected no messages for nil error, got %d", len(msgs))
	}
}

func TestSlackNotifier_ThreadTrackerPersistence(t *testing.T) {
	server := newMockSlackServer()
	defer server.Close()

	tmpDir := t.TempDir()
	trackerPath := filepath.Join(tmpDir, "threads.json")

	client := slack.New("xoxb-test-token", slack.OptionAPIURL(server.URL+"/"))

	tracker, err := NewThreadTracker(trackerPath)
	if err != nil {
		t.Fatalf("failed to create tracker: %v", err)
	}

	notifier := &SlackNotifier{
		client:        client,
		channel:       "C12345",
		threadTracker: tracker,
	}

	p := &plan.Plan{
		Name:   "test-plan",
		Branch: "feat/test-plan",
	}

	// Start should save thread info
	err = notifier.Start(p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Give async operation time to complete
	time.Sleep(100 * time.Millisecond)

	// Verify file was created
	if _, err := os.Stat(trackerPath); os.IsNotExist(err) {
		t.Error("expected threads file to be created")
	}

	// Create a new tracker from the same file
	tracker2, err := NewThreadTracker(trackerPath)
	if err != nil {
		t.Fatalf("failed to create second tracker: %v", err)
	}

	// Verify data was persisted
	info := tracker2.Get("test-plan")
	if info == nil {
		t.Error("expected thread info to be persisted")
	} else if info.ThreadTS != "1234567890.123456" {
		t.Errorf("expected ThreadTS 1234567890.123456, got %s", info.ThreadTS)
	}
}
