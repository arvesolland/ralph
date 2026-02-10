package notify

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/slack-go/slack"
)

// mockSlackServer creates a mock Slack API server for testing.
type mockSlackServer struct {
	*httptest.Server
	mu       sync.Mutex
	messages []mockMessage
	updates  []mockUpdate
}

type mockMessage struct {
	Channel  string
	Text     string
	Blocks   []json.RawMessage
	ThreadTS string
}

type mockUpdate struct {
	Channel   string
	Timestamp string
	Blocks    []json.RawMessage
}

func newMockSlackServer() *mockSlackServer {
	m := &mockSlackServer{
		messages: make([]mockMessage, 0),
		updates:  make([]mockUpdate, 0),
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
		resp := map[string]interface{}{
			"ok":      true,
			"ts":      "1234567890.123456",
			"channel": msg.Channel,
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	})

	mux.HandleFunc("/auth.test", func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{
			"ok":       true,
			"url":      "https://testteam.slack.com/",
			"team":     "testteam",
			"user":     "testbot",
			"team_id":  "T12345",
			"user_id":  "U12345",
			"bot_id":   "B12345",
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	})

	mux.HandleFunc("/chat.update", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		m.mu.Lock()
		defer m.mu.Unlock()

		update := mockUpdate{
			Channel:   r.FormValue("channel"),
			Timestamp: r.FormValue("ts"),
		}

		// Parse blocks if present
		if blocksStr := r.FormValue("blocks"); blocksStr != "" {
			var blocks []json.RawMessage
			if err := json.Unmarshal([]byte(blocksStr), &blocks); err == nil {
				update.Blocks = blocks
			}
		}

		m.updates = append(m.updates, update)

		// Return a successful response
		resp := map[string]interface{}{
			"ok":      true,
			"ts":      update.Timestamp,
			"channel": update.Channel,
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
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

func (m *mockSlackServer) getUpdates() []mockUpdate {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]mockUpdate, len(m.updates))
	copy(result, m.updates)
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

	client := slack.New("xoxb-test-token", slack.OptionAPIURL(server.URL+"/"))

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

	p := PlanInfo{
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
	_ = tracker.Set("test-plan", &ThreadInfo{
		PlanName:  "test-plan",
		ThreadTS:  "1234567890.000000",
		ChannelID: "C12345",
	})

	notifier := &SlackNotifier{
		client:        client,
		channel:       "C12345",
		threadTracker: tracker,
	}

	p := PlanInfo{
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

func TestSlackNotifier_BlockerNotify(t *testing.T) {
	server := newMockSlackServer()
	defer server.Close()

	client := slack.New("xoxb-test-token", slack.OptionAPIURL(server.URL+"/"))

	tmpDir := t.TempDir()
	tracker, err := NewThreadTracker(filepath.Join(tmpDir, "threads.json"))
	if err != nil {
		t.Fatalf("failed to create thread tracker: %v", err)
	}

	// Pre-populate thread info
	_ = tracker.Set("test-plan", &ThreadInfo{
		PlanName:  "test-plan",
		ThreadTS:  "1234567890.000000",
		ChannelID: "C12345",
	})

	notifier := &SlackNotifier{
		client:        client,
		channel:       "C12345",
		threadTracker: tracker,
	}

	p := PlanInfo{
		Name:   "test-plan",
		Branch: "feat/test-plan",
	}

	blocker := &Blocker{
		Content:     "Package needs to be made public",
		Description: "Package needs to be made public",
		Action:      "Go to GitHub and make it public",
		Resume:      "Will verify package is accessible",
		Hash:        "abc12345",
	}

	err = notifier.BlockerNotify(p, blocker)
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

func TestSlackNotifier_BlockerNotify_Deduplication(t *testing.T) {
	server := newMockSlackServer()
	defer server.Close()

	client := slack.New("xoxb-test-token", slack.OptionAPIURL(server.URL+"/"))

	tmpDir := t.TempDir()
	tracker, err := NewThreadTracker(filepath.Join(tmpDir, "threads.json"))
	if err != nil {
		t.Fatalf("failed to create thread tracker: %v", err)
	}

	// Pre-populate thread info with already notified blocker
	_ = tracker.Set("test-plan", &ThreadInfo{
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

	p := PlanInfo{
		Name:   "test-plan",
		Branch: "feat/test-plan",
	}

	blocker := &Blocker{
		Content: "Package needs to be made public",
		Hash:    "abc12345", // Same hash as already notified
	}

	err = notifier.BlockerNotify(p, blocker)
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

func TestSlackNotifier_BlockerNotify_Nil(t *testing.T) {
	server := newMockSlackServer()
	defer server.Close()

	client := slack.New("xoxb-test-token", slack.OptionAPIURL(server.URL+"/"))

	notifier := &SlackNotifier{
		client:  client,
		channel: "C12345",
	}

	p := PlanInfo{
		Name:   "test-plan",
		Branch: "feat/test-plan",
	}

	err := notifier.BlockerNotify(p, nil)
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

	p := PlanInfo{
		Name:   "test-plan",
		Branch: "feat/test-plan",
	}

	err := notifier.Error(p, errors.New("rate limit exceeded"))
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

	p := PlanInfo{
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

	p := PlanInfo{
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

	p := PlanInfo{
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
		name      string
		cfg       SlackNotifierConfig
		isSlack   bool
		isWebhook bool
		isNoop    bool
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

	p := PlanInfo{
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

	p := PlanInfo{
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

	p := PlanInfo{
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

func TestSlackNotifier_UpdateProgress(t *testing.T) {
	server := newMockSlackServer()
	defer server.Close()

	client := slack.New("xoxb-test-token", slack.OptionAPIURL(server.URL+"/"))

	tmpDir := t.TempDir()
	tracker, err := NewThreadTracker(filepath.Join(tmpDir, "threads.json"))
	if err != nil {
		t.Fatalf("failed to create thread tracker: %v", err)
	}

	// Pre-populate thread info
	_ = tracker.Set("test-plan", &ThreadInfo{
		PlanName:  "test-plan",
		ThreadTS:  "1234567890.000000",
		MessageTS: "1234567890.000000",
		ChannelID: "C12345",
	})

	notifier := &SlackNotifier{
		client:        client,
		channel:       "C12345",
		threadTracker: tracker,
	}

	p := PlanInfo{
		Name:   "test-plan",
		Branch: "feat/test-plan",
	}

	status := &ProgressStatus{
		Iteration:     5,
		MaxIterations: 30,
		Phase:         PhaseRunning,
		Message:       "Processing task 3",
	}

	err = notifier.UpdateProgress(p, status)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Give async operation time to complete
	time.Sleep(100 * time.Millisecond)

	updates := server.getUpdates()
	if len(updates) != 1 {
		t.Fatalf("expected 1 update, got %d", len(updates))
	}

	// Verify the update targeted the correct message
	if updates[0].Channel != "C12345" {
		t.Errorf("expected channel C12345, got %s", updates[0].Channel)
	}
	if updates[0].Timestamp != "1234567890.000000" {
		t.Errorf("expected timestamp 1234567890.000000, got %s", updates[0].Timestamp)
	}

	// Verify tracker was updated
	info := tracker.Get("test-plan")
	if info.LastIteration != 5 {
		t.Errorf("expected LastIteration 5, got %d", info.LastIteration)
	}
	if info.LastPhase != "running" {
		t.Errorf("expected LastPhase 'running', got %s", info.LastPhase)
	}
}

func TestSlackNotifier_UpdateProgress_Deduplication(t *testing.T) {
	server := newMockSlackServer()
	defer server.Close()

	client := slack.New("xoxb-test-token", slack.OptionAPIURL(server.URL+"/"))

	tmpDir := t.TempDir()
	tracker, err := NewThreadTracker(filepath.Join(tmpDir, "threads.json"))
	if err != nil {
		t.Fatalf("failed to create thread tracker: %v", err)
	}

	// Pre-populate thread info with existing progress
	_ = tracker.Set("test-plan", &ThreadInfo{
		PlanName:      "test-plan",
		ThreadTS:      "1234567890.000000",
		MessageTS:     "1234567890.000000",
		ChannelID:     "C12345",
		LastIteration: 5,
		LastPhase:     "running",
	})

	notifier := &SlackNotifier{
		client:        client,
		channel:       "C12345",
		threadTracker: tracker,
	}

	p := PlanInfo{
		Name:   "test-plan",
		Branch: "feat/test-plan",
	}

	// Send same progress - should be deduplicated
	status := &ProgressStatus{
		Iteration:     5,
		MaxIterations: 30,
		Phase:         PhaseRunning,
	}

	err = notifier.UpdateProgress(p, status)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Give async operation time to complete
	time.Sleep(100 * time.Millisecond)

	updates := server.getUpdates()
	if len(updates) != 0 {
		t.Errorf("expected 0 updates (deduplicated), got %d", len(updates))
	}
}

func TestSlackNotifier_UpdateProgress_NoThread(t *testing.T) {
	server := newMockSlackServer()
	defer server.Close()

	client := slack.New("xoxb-test-token", slack.OptionAPIURL(server.URL+"/"))

	tmpDir := t.TempDir()
	tracker, err := NewThreadTracker(filepath.Join(tmpDir, "threads.json"))
	if err != nil {
		t.Fatalf("failed to create thread tracker: %v", err)
	}

	// No thread info set

	notifier := &SlackNotifier{
		client:        client,
		channel:       "C12345",
		threadTracker: tracker,
	}

	p := PlanInfo{
		Name:   "test-plan",
		Branch: "feat/test-plan",
	}

	status := &ProgressStatus{
		Iteration:     5,
		MaxIterations: 30,
		Phase:         PhaseRunning,
	}

	// Should not error even if no thread exists
	err = notifier.UpdateProgress(p, status)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Give async operation time to complete
	time.Sleep(100 * time.Millisecond)

	updates := server.getUpdates()
	if len(updates) != 0 {
		t.Errorf("expected 0 updates (no thread), got %d", len(updates))
	}
}

func TestBuildProgressBlocks(t *testing.T) {
	notifier := &SlackNotifier{}

	testCases := []struct {
		name          string
		phase         ProgressPhase
		expectedEmoji string
		expectedPhase string
	}{
		{"initializing", PhaseInitializing, ":rocket:", "Initializing"},
		{"running", PhaseRunning, ":hourglass_flowing_sand:", "Running"},
		{"verifying", PhaseVerifying, ":mag:", "Verifying"},
		{"blocked", PhaseBlocked, ":warning:", "Blocked"},
		{"complete", PhaseComplete, ":white_check_mark:", "Complete"},
		{"error", PhaseError, ":x:", "Error"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			p := PlanInfo{
				Name:   "test-plan",
				Branch: "feat/test-plan",
			}

			status := &ProgressStatus{
				Iteration:     5,
				MaxIterations: 10,
				Phase:         tc.phase,
			}

			blocks := notifier.buildProgressBlocks(p, status)

			if len(blocks) < 2 {
				t.Fatalf("expected at least 2 blocks, got %d", len(blocks))
			}

			// Check first block contains expected emoji and phase
			firstBlock, ok := blocks[0].(*slack.SectionBlock)
			if !ok {
				t.Fatal("first block is not a SectionBlock")
			}

			text := firstBlock.Text.Text
			if !strings.Contains(text, tc.expectedEmoji) {
				t.Errorf("expected emoji %s in text, got: %s", tc.expectedEmoji, text)
			}
			if !strings.Contains(text, tc.expectedPhase) {
				t.Errorf("expected phase %s in text, got: %s", tc.expectedPhase, text)
			}
		})
	}
}

func TestBuildProgressBlocks_ProgressBar(t *testing.T) {
	notifier := &SlackNotifier{}

	p := PlanInfo{
		Name:   "test-plan",
		Branch: "feat/test-plan",
	}

	testCases := []struct {
		iteration     int
		maxIterations int
		expectedBar   string
	}{
		{0, 10, "\u2591\u2591\u2591\u2591\u2591\u2591\u2591\u2591\u2591\u2591"},
		{5, 10, "\u2588\u2588\u2588\u2588\u2588\u2591\u2591\u2591\u2591\u2591"},
		{10, 10, "\u2588\u2588\u2588\u2588\u2588\u2588\u2588\u2588\u2588\u2588"},
		{3, 30, "\u2588\u2591\u2591\u2591\u2591\u2591\u2591\u2591\u2591\u2591"},
	}

	for _, tc := range testCases {
		t.Run(strings.ReplaceAll(tc.expectedBar, "\u2591", "0"), func(t *testing.T) {
			status := &ProgressStatus{
				Iteration:     tc.iteration,
				MaxIterations: tc.maxIterations,
				Phase:         PhaseRunning,
			}

			blocks := notifier.buildProgressBlocks(p, status)

			// Check second block (fields) contains progress bar
			secondBlock, ok := blocks[1].(*slack.SectionBlock)
			if !ok {
				t.Fatal("second block is not a SectionBlock")
			}

			found := false
			for _, field := range secondBlock.Fields {
				if strings.Contains(field.Text, tc.expectedBar) {
					found = true
					break
				}
			}

			if !found {
				t.Errorf("expected progress bar %s not found in fields", tc.expectedBar)
			}
		})
	}
}
