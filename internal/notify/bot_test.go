package notify

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/arvesolland/ralph/internal/plan"
)

func TestNewSocketModeBot_MissingBotToken(t *testing.T) {
	cfg := BotConfig{
		AppToken:  "xapp-test",
		ChannelID: "C123",
	}
	bot := NewSocketModeBot(cfg)
	if bot != nil {
		t.Error("expected nil bot when BotToken is missing")
	}
}

func TestNewSocketModeBot_MissingAppToken(t *testing.T) {
	cfg := BotConfig{
		BotToken:  "xoxb-test",
		ChannelID: "C123",
	}
	bot := NewSocketModeBot(cfg)
	if bot != nil {
		t.Error("expected nil bot when AppToken is missing")
	}
}

func TestNewSocketModeBot_MissingChannelID(t *testing.T) {
	cfg := BotConfig{
		BotToken: "xoxb-test",
		AppToken: "xapp-test",
	}
	bot := NewSocketModeBot(cfg)
	if bot != nil {
		t.Error("expected nil bot when ChannelID is missing")
	}
}

func TestNewSocketModeBot_ValidConfig(t *testing.T) {
	cfg := BotConfig{
		BotToken:  "xoxb-test",
		AppToken:  "xapp-test",
		ChannelID: "C123",
	}
	bot := NewSocketModeBot(cfg)
	if bot == nil {
		t.Fatal("expected non-nil bot with valid config")
	}
	if bot.channelID != "C123" {
		t.Errorf("expected channelID C123, got %s", bot.channelID)
	}
}

func TestSocketModeBot_IsRunning(t *testing.T) {
	cfg := BotConfig{
		BotToken:  "xoxb-test",
		AppToken:  "xapp-test",
		ChannelID: "C123",
	}
	bot := NewSocketModeBot(cfg)
	if bot == nil {
		t.Fatal("expected non-nil bot")
	}

	// Initially not running
	if bot.IsRunning() {
		t.Error("expected bot to not be running initially")
	}
}

func TestSocketModeBot_Stop_NotRunning(t *testing.T) {
	cfg := BotConfig{
		BotToken:  "xoxb-test",
		AppToken:  "xapp-test",
		ChannelID: "C123",
	}
	bot := NewSocketModeBot(cfg)
	if bot == nil {
		t.Fatal("expected non-nil bot")
	}

	// Stop when not running should not panic
	bot.Stop()
}

func TestBotConfig_Fields(t *testing.T) {
	cfg := BotConfig{
		BotToken:     "xoxb-test",
		AppToken:     "xapp-test",
		ChannelID:    "C123",
		PlanBasePath: "/plans",
		Debug:        true,
	}

	if cfg.BotToken != "xoxb-test" {
		t.Errorf("unexpected BotToken: %s", cfg.BotToken)
	}
	if cfg.AppToken != "xapp-test" {
		t.Errorf("unexpected AppToken: %s", cfg.AppToken)
	}
	if cfg.ChannelID != "C123" {
		t.Errorf("unexpected ChannelID: %s", cfg.ChannelID)
	}
	if cfg.PlanBasePath != "/plans" {
		t.Errorf("unexpected PlanBasePath: %s", cfg.PlanBasePath)
	}
	if !cfg.Debug {
		t.Error("expected Debug to be true")
	}
}

func TestSocketModeBot_FindPlanByThread(t *testing.T) {
	tmpDir := t.TempDir()
	trackerPath := filepath.Join(tmpDir, "threads.json")

	tracker, err := NewThreadTracker(trackerPath)
	if err != nil {
		t.Fatalf("failed to create tracker: %v", err)
	}

	// Add a thread
	if err := tracker.Set("test-plan", &ThreadInfo{
		ThreadTS:  "1234567890.123456",
		ChannelID: "C123",
	}); err != nil {
		t.Fatalf("failed to set thread info: %v", err)
	}

	cfg := BotConfig{
		BotToken:      "xoxb-test",
		AppToken:      "xapp-test",
		ChannelID:     "C123",
		ThreadTracker: tracker,
	}
	bot := NewSocketModeBot(cfg)
	if bot == nil {
		t.Fatal("expected non-nil bot")
	}

	// Test finding existing thread
	planName := bot.findPlanByThread("1234567890.123456")
	if planName != "test-plan" {
		t.Errorf("expected test-plan, got %s", planName)
	}

	// Test non-existent thread
	planName = bot.findPlanByThread("9999999999.999999")
	if planName != "" {
		t.Errorf("expected empty string for non-existent thread, got %s", planName)
	}
}

func TestSocketModeBot_FindPlanByThread_NilTracker(t *testing.T) {
	cfg := BotConfig{
		BotToken:  "xoxb-test",
		AppToken:  "xapp-test",
		ChannelID: "C123",
	}
	bot := NewSocketModeBot(cfg)
	if bot == nil {
		t.Fatal("expected non-nil bot")
	}

	// Should return empty string when tracker is nil
	planName := bot.findPlanByThread("1234567890.123456")
	if planName != "" {
		t.Errorf("expected empty string when tracker is nil, got %s", planName)
	}
}

func TestSocketModeBot_WriteFeedback(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := BotConfig{
		BotToken:     "xoxb-test",
		AppToken:     "xapp-test",
		ChannelID:    "C123",
		PlanBasePath: tmpDir,
	}
	bot := NewSocketModeBot(cfg)
	if bot == nil {
		t.Fatal("expected non-nil bot")
	}

	// Write feedback
	err := bot.writeFeedback("test-plan", "U123", "Test feedback message")
	if err != nil {
		t.Fatalf("writeFeedback failed: %v", err)
	}

	// Verify feedback file was created
	feedbackPath := filepath.Join(tmpDir, "test-plan.feedback.md")
	content, err := os.ReadFile(feedbackPath)
	if err != nil {
		t.Fatalf("failed to read feedback file: %v", err)
	}

	contentStr := string(content)
	if !contains(contentStr, "Test feedback message") {
		t.Errorf("feedback file missing message, got: %s", contentStr)
	}
	if !contains(contentStr, "Slack reply from U123") {
		t.Errorf("feedback file missing source, got: %s", contentStr)
	}
}

func TestLoadGlobalBotConfig_FromEnv(t *testing.T) {
	// Save and restore env vars
	oldBot := os.Getenv("SLACK_BOT_TOKEN")
	oldApp := os.Getenv("SLACK_APP_TOKEN")
	defer func() {
		os.Setenv("SLACK_BOT_TOKEN", oldBot)
		os.Setenv("SLACK_APP_TOKEN", oldApp)
	}()

	os.Setenv("SLACK_BOT_TOKEN", "xoxb-from-env")
	os.Setenv("SLACK_APP_TOKEN", "xapp-from-env")

	cfg, err := LoadGlobalBotConfig()
	if err != nil {
		t.Fatalf("LoadGlobalBotConfig failed: %v", err)
	}

	if cfg.BotToken != "xoxb-from-env" {
		t.Errorf("expected BotToken from env, got: %s", cfg.BotToken)
	}
	if cfg.AppToken != "xapp-from-env" {
		t.Errorf("expected AppToken from env, got: %s", cfg.AppToken)
	}
}

func TestLoadEnvFile(t *testing.T) {
	tmpDir := t.TempDir()
	envPath := filepath.Join(tmpDir, "slack.env")

	// Create env file
	content := `# Comment line
SLACK_BOT_TOKEN=xoxb-from-file
SLACK_APP_TOKEN=xapp-from-file
OTHER_VAR=ignored
`
	if err := os.WriteFile(envPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write env file: %v", err)
	}

	cfg := &BotConfig{}
	if err := loadEnvFile(envPath, cfg); err != nil {
		t.Fatalf("loadEnvFile failed: %v", err)
	}

	if cfg.BotToken != "xoxb-from-file" {
		t.Errorf("expected xoxb-from-file, got: %s", cfg.BotToken)
	}
	if cfg.AppToken != "xapp-from-file" {
		t.Errorf("expected xapp-from-file, got: %s", cfg.AppToken)
	}
}

func TestLoadEnvFile_QuotedValues(t *testing.T) {
	tmpDir := t.TempDir()
	envPath := filepath.Join(tmpDir, "slack.env")

	// Create env file with quoted values
	content := `SLACK_BOT_TOKEN="xoxb-quoted"
SLACK_APP_TOKEN='xapp-single-quoted'
`
	if err := os.WriteFile(envPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write env file: %v", err)
	}

	cfg := &BotConfig{}
	if err := loadEnvFile(envPath, cfg); err != nil {
		t.Fatalf("loadEnvFile failed: %v", err)
	}

	if cfg.BotToken != "xoxb-quoted" {
		t.Errorf("expected xoxb-quoted, got: %s", cfg.BotToken)
	}
	if cfg.AppToken != "xapp-single-quoted" {
		t.Errorf("expected xapp-single-quoted, got: %s", cfg.AppToken)
	}
}

func TestLoadEnvFile_NonExistent(t *testing.T) {
	cfg := &BotConfig{}
	err := loadEnvFile("/nonexistent/path/slack.env", cfg)
	if err == nil {
		t.Error("expected error for non-existent file")
	}
}

func TestLoadEnvFile_EnvTakesPrecedence(t *testing.T) {
	tmpDir := t.TempDir()
	envPath := filepath.Join(tmpDir, "slack.env")

	// Create env file
	content := `SLACK_BOT_TOKEN=xoxb-from-file
SLACK_APP_TOKEN=xapp-from-file
`
	if err := os.WriteFile(envPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write env file: %v", err)
	}

	// Pre-set one value (simulating env var)
	cfg := &BotConfig{
		BotToken: "xoxb-from-env",
	}
	if err := loadEnvFile(envPath, cfg); err != nil {
		t.Fatalf("loadEnvFile failed: %v", err)
	}

	// Pre-set value should be preserved
	if cfg.BotToken != "xoxb-from-env" {
		t.Errorf("expected xoxb-from-env (preserved), got: %s", cfg.BotToken)
	}
	// File value should be used for the other
	if cfg.AppToken != "xapp-from-file" {
		t.Errorf("expected xapp-from-file, got: %s", cfg.AppToken)
	}
}

func TestParseEnvLine(t *testing.T) {
	tests := []struct {
		line      string
		wantKey   string
		wantValue string
	}{
		{"KEY=value", "KEY", "value"},
		{"KEY=value with spaces", "KEY", "value with spaces"},
		{"KEY=\"quoted value\"", "KEY", "quoted value"},
		{"KEY='single quoted'", "KEY", "single quoted"},
		{"KEY=", "KEY", ""},
		{"NOEQUALS", "NOEQUALS", ""},
		{"KEY==value", "KEY", "=value"},
	}

	for _, tt := range tests {
		t.Run(tt.line, func(t *testing.T) {
			key, value := parseEnvLine(tt.line)
			if key != tt.wantKey {
				t.Errorf("key: expected %q, got %q", tt.wantKey, key)
			}
			if value != tt.wantValue {
				t.Errorf("value: expected %q, got %q", tt.wantValue, value)
			}
		})
	}
}

func TestSplitLines(t *testing.T) {
	tests := []struct {
		content string
		want    []string
	}{
		{"", nil},
		{"single", []string{"single"}},
		{"line1\nline2", []string{"line1", "line2"}},
		{"line1\nline2\n", []string{"line1", "line2"}}, // trailing newline doesn't create empty line
		{"line1\n\nline3", []string{"line1", "", "line3"}},
	}

	for _, tt := range tests {
		t.Run(tt.content, func(t *testing.T) {
			got := splitLines(tt.content)
			if len(got) != len(tt.want) {
				t.Errorf("length: expected %d, got %d (%v)", len(tt.want), len(got), got)
				return
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("line %d: expected %q, got %q", i, tt.want[i], got[i])
				}
			}
		})
	}
}

func TestTrimSpace(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"hello", "hello"},
		{" hello", "hello"},
		{"hello ", "hello"},
		{" hello ", "hello"},
		{"\thello\t", "hello"},
		{"  \t hello \t  ", "hello"},
		{"", ""},
		{"   ", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := trimSpace(tt.input)
			if got != tt.want {
				t.Errorf("expected %q, got %q", tt.want, got)
			}
		})
	}
}

func TestStartBotIfConfigured_NoConfig(t *testing.T) {
	// Save and restore env vars
	oldBot := os.Getenv("SLACK_BOT_TOKEN")
	oldApp := os.Getenv("SLACK_APP_TOKEN")
	defer func() {
		os.Setenv("SLACK_BOT_TOKEN", oldBot)
		os.Setenv("SLACK_APP_TOKEN", oldApp)
	}()

	// Clear env vars
	os.Setenv("SLACK_BOT_TOKEN", "")
	os.Setenv("SLACK_APP_TOKEN", "")

	// Override global path to use temp dir
	tmpDir := t.TempDir()
	oldGlobalPath := GlobalBotPath
	GlobalBotPath = tmpDir
	defer func() { GlobalBotPath = oldGlobalPath }()

	ctx := context.Background()
	bot := StartBotIfConfigured(ctx, nil, tmpDir, "C123")
	if bot != nil {
		t.Error("expected nil bot when no config available")
		bot.Stop()
	}
}

func TestSocketModeBot_WithThreadTracker(t *testing.T) {
	tmpDir := t.TempDir()
	trackerPath := filepath.Join(tmpDir, "threads.json")

	tracker, err := NewThreadTracker(trackerPath)
	if err != nil {
		t.Fatalf("failed to create tracker: %v", err)
	}

	cfg := BotConfig{
		BotToken:      "xoxb-test",
		AppToken:      "xapp-test",
		ChannelID:     "C123",
		ThreadTracker: tracker,
		PlanBasePath:  tmpDir,
	}

	bot := NewSocketModeBot(cfg)
	if bot == nil {
		t.Fatal("expected non-nil bot")
	}

	if bot.threadTracker != tracker {
		t.Error("threadTracker not set correctly")
	}
	if bot.planBasePath != tmpDir {
		t.Errorf("planBasePath: expected %s, got %s", tmpDir, bot.planBasePath)
	}
}

func TestSocketModeBot_WaitForConnection_NotRunning(t *testing.T) {
	cfg := BotConfig{
		BotToken:  "xoxb-test",
		AppToken:  "xapp-test",
		ChannelID: "C123",
	}
	bot := NewSocketModeBot(cfg)
	if bot == nil {
		t.Fatal("expected non-nil bot")
	}

	// Bot is not running, should timeout quickly
	start := time.Now()
	connected := bot.WaitForConnection(100 * time.Millisecond)
	elapsed := time.Since(start)

	if connected {
		t.Error("expected false when bot is not running")
	}
	if elapsed < 100*time.Millisecond {
		t.Errorf("WaitForConnection returned too quickly: %v", elapsed)
	}
}

func TestSocketModeBot_Start_AlreadyRunning(t *testing.T) {
	cfg := BotConfig{
		BotToken:  "xoxb-test",
		AppToken:  "xapp-test",
		ChannelID: "C123",
	}
	bot := NewSocketModeBot(cfg)
	if bot == nil {
		t.Fatal("expected non-nil bot")
	}

	// Manually set running state
	bot.mu.Lock()
	bot.running = true
	bot.mu.Unlock()

	// Try to start again
	ctx := context.Background()
	err := bot.Start(ctx)
	if err == nil {
		t.Error("expected error when starting already running bot")
	}
	if err.Error() != "bot is already running" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestGlobalBotPath(t *testing.T) {
	// GlobalBotPath should be ~/.ralph
	home := os.Getenv("HOME")
	expected := filepath.Join(home, ".ralph")

	// The actual GlobalBotPath is set at package init time,
	// so we just verify it follows the expected pattern
	if !filepath.IsAbs(GlobalBotPath) {
		t.Errorf("GlobalBotPath should be absolute: %s", GlobalBotPath)
	}
	if GlobalBotPath != expected {
		t.Logf("GlobalBotPath: %s (expected %s based on HOME)", GlobalBotPath, expected)
	}
}

func TestBotConfigFilename(t *testing.T) {
	if BotConfigFilename != "slack.env" {
		t.Errorf("expected slack.env, got: %s", BotConfigFilename)
	}
}

// Helper for feedback test
func contains(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestWriteFeedback_Integration(t *testing.T) {
	tmpDir := t.TempDir()

	// Create thread tracker
	trackerPath := filepath.Join(tmpDir, "threads.json")
	tracker, err := NewThreadTracker(trackerPath)
	if err != nil {
		t.Fatalf("failed to create tracker: %v", err)
	}

	// Set up a thread
	if err := tracker.Set("integration-plan", &ThreadInfo{
		ThreadTS:  "1234567890.123456",
		ChannelID: "C123",
	}); err != nil {
		t.Fatalf("failed to set thread: %v", err)
	}

	// Create plan base path
	planDir := filepath.Join(tmpDir, "plans", "current")
	if err := os.MkdirAll(planDir, 0755); err != nil {
		t.Fatalf("failed to create plan dir: %v", err)
	}

	cfg := BotConfig{
		BotToken:      "xoxb-test",
		AppToken:      "xapp-test",
		ChannelID:     "C123",
		ThreadTracker: tracker,
		PlanBasePath:  planDir,
	}

	bot := NewSocketModeBot(cfg)
	if bot == nil {
		t.Fatal("expected non-nil bot")
	}

	// Write feedback
	err = bot.writeFeedback("integration-plan", "U456", "Integration test message")
	if err != nil {
		t.Fatalf("writeFeedback failed: %v", err)
	}

	// Verify feedback file
	p := &plan.Plan{
		Name: "integration-plan",
		Path: filepath.Join(planDir, "integration-plan.md"),
	}
	feedbackContent, err := plan.ReadFeedback(p)
	if err != nil {
		t.Fatalf("ReadFeedback failed: %v", err)
	}

	if !contains(feedbackContent, "Integration test message") {
		t.Errorf("feedback should contain message, got: %s", feedbackContent)
	}
}
