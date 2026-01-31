// Package notify provides notification functionality for Ralph.
package notify

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/arvesolland/ralph/internal/log"
	"github.com/arvesolland/ralph/internal/plan"
	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
	"github.com/slack-go/slack/socketmode"
)

// BotConfigFilename is the name of the config file for global bot mode.
const BotConfigFilename = "slack.env"

// GlobalBotPath is the default path for global bot configuration.
var GlobalBotPath = filepath.Join(os.Getenv("HOME"), ".ralph")

// SocketModeBot listens for Slack thread replies and writes them to feedback files.
// It uses Slack Socket Mode to receive real-time events.
type SocketModeBot struct {
	// client is the Slack Socket Mode client.
	client *socketmode.Client

	// api is the Slack API client.
	api *slack.Client

	// threadTracker is used to look up plan names from thread timestamps.
	threadTracker *ThreadTracker

	// planBasePath is the base path where plan files are located.
	// Used to construct feedback file paths.
	planBasePath string

	// channelID is the channel ID to listen for messages in.
	channelID string

	// mu protects running state.
	mu sync.Mutex

	// running indicates if the bot is currently running.
	running bool

	// stopCh is used to signal the bot to stop.
	stopCh chan struct{}
}

// BotConfig contains configuration for creating a SocketModeBot.
type BotConfig struct {
	// BotToken is the Slack bot token (xoxb-...).
	BotToken string

	// AppToken is the Slack app-level token for Socket Mode (xapp-...).
	AppToken string

	// ThreadTracker is used to look up plan names from thread timestamps.
	ThreadTracker *ThreadTracker

	// PlanBasePath is the base path where plan files are located.
	PlanBasePath string

	// ChannelID is the channel ID to listen for messages in.
	ChannelID string

	// Debug enables debug logging for the Slack client.
	Debug bool
}

// NewSocketModeBot creates a new Socket Mode bot.
// Returns nil if required configuration is missing.
func NewSocketModeBot(cfg BotConfig) *SocketModeBot {
	if cfg.BotToken == "" || cfg.AppToken == "" {
		return nil
	}

	if cfg.ChannelID == "" {
		log.Debug("SocketModeBot: channel ID required for message filtering")
		return nil
	}

	api := slack.New(
		cfg.BotToken,
		slack.OptionDebug(cfg.Debug),
		slack.OptionAppLevelToken(cfg.AppToken),
	)

	client := socketmode.New(
		api,
		socketmode.OptionDebug(cfg.Debug),
	)

	return &SocketModeBot{
		client:        client,
		api:           api,
		threadTracker: cfg.ThreadTracker,
		planBasePath:  cfg.PlanBasePath,
		channelID:     cfg.ChannelID,
		stopCh:        make(chan struct{}),
	}
}

// Start begins listening for Slack events.
// This method runs in a goroutine and doesn't block.
// Returns an error if the bot is already running.
func (b *SocketModeBot) Start(ctx context.Context) error {
	b.mu.Lock()
	if b.running {
		b.mu.Unlock()
		return fmt.Errorf("bot is already running")
	}
	b.running = true
	b.stopCh = make(chan struct{})
	b.mu.Unlock()

	// Start event handling in a goroutine
	go b.handleEvents(ctx)

	// Start the Socket Mode connection
	go func() {
		if err := b.client.Run(); err != nil {
			log.Error("Socket Mode connection error: %v", err)
			b.mu.Lock()
			b.running = false
			b.mu.Unlock()
		}
	}()

	return nil
}

// Stop gracefully stops the bot.
func (b *SocketModeBot) Stop() {
	b.mu.Lock()
	if !b.running {
		b.mu.Unlock()
		return
	}
	b.running = false
	close(b.stopCh)
	b.mu.Unlock()

	// The client will be closed when the context is cancelled
	log.Info("Socket Mode bot stopped")
}

// IsRunning returns whether the bot is currently running.
func (b *SocketModeBot) IsRunning() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.running
}

// handleEvents processes incoming Socket Mode events.
func (b *SocketModeBot) handleEvents(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			log.Debug("Bot context cancelled, stopping event handler")
			b.Stop()
			return
		case <-b.stopCh:
			log.Debug("Bot stop signal received")
			return
		case evt, ok := <-b.client.Events:
			if !ok {
				log.Debug("Socket Mode events channel closed")
				b.mu.Lock()
				b.running = false
				b.mu.Unlock()
				return
			}
			b.processEvent(evt)
		}
	}
}

// processEvent handles a single Socket Mode event.
func (b *SocketModeBot) processEvent(evt socketmode.Event) {
	switch evt.Type {
	case socketmode.EventTypeConnecting:
		log.Debug("Connecting to Slack Socket Mode...")

	case socketmode.EventTypeConnected:
		log.Info("Connected to Slack Socket Mode")

	case socketmode.EventTypeConnectionError:
		log.Warn("Socket Mode connection error, will attempt to reconnect")

	case socketmode.EventTypeDisconnect:
		log.Debug("Disconnected from Socket Mode")

	case socketmode.EventTypeEventsAPI:
		b.handleEventsAPIEvent(evt)

	default:
		// Acknowledge unknown events
		if evt.Request != nil {
			b.client.Ack(*evt.Request)
		}
	}
}

// handleEventsAPIEvent processes Events API events.
func (b *SocketModeBot) handleEventsAPIEvent(evt socketmode.Event) {
	eventsAPIEvent, ok := evt.Data.(slackevents.EventsAPIEvent)
	if !ok {
		log.Debug("Failed to cast to EventsAPIEvent")
		if evt.Request != nil {
			b.client.Ack(*evt.Request)
		}
		return
	}

	// Acknowledge the event immediately
	if evt.Request != nil {
		b.client.Ack(*evt.Request)
	}

	switch eventsAPIEvent.Type {
	case slackevents.CallbackEvent:
		b.handleCallbackEvent(eventsAPIEvent)
	}
}

// handleCallbackEvent processes callback events from the Events API.
func (b *SocketModeBot) handleCallbackEvent(evt slackevents.EventsAPIEvent) {
	innerEvent := evt.InnerEvent

	switch ev := innerEvent.Data.(type) {
	case *slackevents.MessageEvent:
		b.handleMessageEvent(ev)
	}
}

// handleMessageEvent processes message events.
// Only processes thread replies in tracked threads.
func (b *SocketModeBot) handleMessageEvent(ev *slackevents.MessageEvent) {
	// Ignore messages from bots (including self)
	if ev.BotID != "" || ev.SubType == "bot_message" {
		return
	}

	// Only process messages in the configured channel
	if ev.Channel != b.channelID {
		return
	}

	// Only process thread replies (messages with ThreadTimeStamp that differs from TimeStamp)
	if ev.ThreadTimeStamp == "" || ev.ThreadTimeStamp == ev.TimeStamp {
		return
	}

	// Look up the plan from the thread timestamp
	planName := b.findPlanByThread(ev.ThreadTimeStamp)
	if planName == "" {
		log.Debug("No plan found for thread: %s", ev.ThreadTimeStamp)
		return
	}

	// Write the message to the feedback file
	if err := b.writeFeedback(planName, ev.User, ev.Text); err != nil {
		log.Error("Failed to write feedback: %v", err)
		return
	}

	log.Info("Received thread reply for plan %s from user %s", planName, ev.User)
}

// findPlanByThread looks up the plan name from a thread timestamp.
func (b *SocketModeBot) findPlanByThread(threadTS string) string {
	if b.threadTracker == nil {
		return ""
	}

	// Get all threads and find matching one
	for _, info := range b.threadTracker.List() {
		if info.ThreadTS == threadTS {
			return info.PlanName
		}
	}

	return ""
}

// writeFeedback writes a thread reply to the plan's feedback file.
func (b *SocketModeBot) writeFeedback(planName, userID, text string) error {
	// Get user info for display name
	userName := userID
	if b.api != nil {
		if user, err := b.api.GetUserInfo(userID); err == nil {
			if user.RealName != "" {
				userName = user.RealName
			} else if user.Name != "" {
				userName = user.Name
			}
		}
	}

	// Create a minimal plan for feedback path calculation
	p := &plan.Plan{
		Name: planName,
		Path: filepath.Join(b.planBasePath, planName+".md"),
	}

	// Append to feedback file
	source := fmt.Sprintf("Slack reply from %s", userName)
	return plan.AppendFeedback(p, source, text)
}

// LoadGlobalBotConfig loads bot configuration from the global location (~/.ralph/slack.env).
// Environment variables take precedence over file values.
func LoadGlobalBotConfig() (*BotConfig, error) {
	cfg := &BotConfig{}

	// First try environment variables
	cfg.BotToken = os.Getenv("SLACK_BOT_TOKEN")
	cfg.AppToken = os.Getenv("SLACK_APP_TOKEN")

	// If not in env, try loading from file
	if cfg.BotToken == "" || cfg.AppToken == "" {
		envPath := filepath.Join(GlobalBotPath, BotConfigFilename)
		if err := loadEnvFile(envPath, cfg); err != nil && !os.IsNotExist(err) {
			return nil, fmt.Errorf("loading global bot config: %w", err)
		}
	}

	return cfg, nil
}

// loadEnvFile loads bot configuration from an env file.
// Simple format: KEY=value (one per line, no quotes handling).
func loadEnvFile(path string, cfg *BotConfig) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	lines := splitLines(string(data))
	for _, line := range lines {
		line = trimSpace(line)
		if line == "" || line[0] == '#' {
			continue
		}

		key, value := parseEnvLine(line)
		switch key {
		case "SLACK_BOT_TOKEN":
			if cfg.BotToken == "" {
				cfg.BotToken = value
			}
		case "SLACK_APP_TOKEN":
			if cfg.AppToken == "" {
				cfg.AppToken = value
			}
		}
	}

	return nil
}

// splitLines splits content into lines.
func splitLines(content string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(content); i++ {
		if content[i] == '\n' {
			lines = append(lines, content[start:i])
			start = i + 1
		}
	}
	if start < len(content) {
		lines = append(lines, content[start:])
	}
	return lines
}

// trimSpace removes leading and trailing whitespace.
func trimSpace(s string) string {
	start := 0
	end := len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t' || s[start] == '\r') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t' || s[end-1] == '\r') {
		end--
	}
	return s[start:end]
}

// parseEnvLine parses a KEY=value line.
func parseEnvLine(line string) (key, value string) {
	for i := 0; i < len(line); i++ {
		if line[i] == '=' {
			key = line[:i]
			value = line[i+1:]
			// Strip quotes if present
			if len(value) >= 2 && (value[0] == '"' || value[0] == '\'') && value[len(value)-1] == value[0] {
				value = value[1 : len(value)-1]
			}
			return
		}
	}
	return line, ""
}

// StartBotIfConfigured starts the Socket Mode bot if configuration is available.
// This is a convenience function for auto-starting the bot from worker.
// Returns nil if bot couldn't be started (missing config), or the bot instance if started.
func StartBotIfConfigured(ctx context.Context, threadTracker *ThreadTracker, planBasePath, channelID string) *SocketModeBot {
	cfg, err := LoadGlobalBotConfig()
	if err != nil {
		log.Debug("Failed to load bot config: %v", err)
		return nil
	}

	if cfg.BotToken == "" || cfg.AppToken == "" {
		log.Debug("Bot tokens not configured, Socket Mode bot not started")
		return nil
	}

	cfg.ThreadTracker = threadTracker
	cfg.PlanBasePath = planBasePath
	cfg.ChannelID = channelID

	bot := NewSocketModeBot(*cfg)
	if bot == nil {
		return nil
	}

	if err := bot.Start(ctx); err != nil {
		log.Error("Failed to start Socket Mode bot: %v", err)
		return nil
	}

	return bot
}

// WaitForConnection waits for the bot to connect with a timeout.
// Returns true if connected, false if timeout reached.
func (b *SocketModeBot) WaitForConnection(timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if b.IsRunning() {
			return true
		}
		time.Sleep(100 * time.Millisecond)
	}
	return b.IsRunning()
}
