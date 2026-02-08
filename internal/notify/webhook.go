// Package notify handles Slack notifications for Ralph.
package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/arvesolland/ralph/internal/config"
	"github.com/arvesolland/ralph/internal/log"
)

// PlanInfo holds plan details needed for notifications.
type PlanInfo struct {
	Name   string
	Branch string
}

// Blocker holds blocker details needed for notifications.
type Blocker struct {
	Content     string
	Description string
	Action      string
	Resume      string
	Hash        string
}

// ProgressPhase represents the current phase of plan execution.
type ProgressPhase string

const (
	PhaseInitializing ProgressPhase = "initializing"
	PhaseRunning      ProgressPhase = "running"
	PhaseVerifying    ProgressPhase = "verifying"
	PhaseBlocked      ProgressPhase = "blocked"
	PhaseComplete     ProgressPhase = "complete"
	PhaseError        ProgressPhase = "error"
)

// ProgressStatus represents the current progress of a plan execution.
type ProgressStatus struct {
	// Iteration is the current iteration number.
	Iteration int

	// MaxIterations is the maximum number of iterations.
	MaxIterations int

	// Phase is the current execution phase.
	Phase ProgressPhase

	// Message is an optional status message.
	Message string
}

// Notifier defines the interface for sending notifications.
type Notifier interface {
	// Start sends a notification when a plan starts.
	Start(p PlanInfo) error

	// Complete sends a notification when a plan completes.
	Complete(p PlanInfo, prURL string) error

	// Blocker sends a notification when a blocker is encountered.
	BlockerNotify(p PlanInfo, blocker *Blocker) error

	// Error sends a notification when an error occurs.
	Error(p PlanInfo, err error) error

	// Iteration sends a notification for each iteration (if enabled).
	Iteration(p PlanInfo, iteration, maxIterations int) error

	// UpdateProgress updates the parent message with current progress.
	// This provides a "living status card" that shows the current state.
	UpdateProgress(p PlanInfo, status *ProgressStatus) error
}

// WebhookNotifier sends notifications via Slack incoming webhooks.
type WebhookNotifier struct {
	webhookURL string
	httpClient *http.Client
}

// NewWebhookNotifier creates a new WebhookNotifier.
// Returns nil if webhookURL is empty (notifications disabled).
func NewWebhookNotifier(webhookURL string) *WebhookNotifier {
	if webhookURL == "" {
		return nil
	}
	return &WebhookNotifier{
		webhookURL: webhookURL,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// slackMessage represents a Slack webhook message payload.
type slackMessage struct {
	Text        string       `json:"text,omitempty"`
	Blocks      []slackBlock `json:"blocks,omitempty"`
	Attachments []attachment `json:"attachments,omitempty"`
}

// slackBlock represents a Slack Block Kit block.
type slackBlock struct {
	Type   string      `json:"type"`
	Text   *slackText  `json:"text,omitempty"`
	Fields []slackText `json:"fields,omitempty"`
}

// slackText represents text content in Slack.
type slackText struct {
	Type string `json:"type"` // "plain_text" or "mrkdwn"
	Text string `json:"text"`
}

// attachment represents a Slack attachment.
type attachment struct {
	Color  string       `json:"color,omitempty"`
	Blocks []slackBlock `json:"blocks,omitempty"`
}

// Start sends a notification when a plan starts.
func (w *WebhookNotifier) Start(p PlanInfo) error {
	msg := slackMessage{
		Blocks: []slackBlock{
			{
				Type: "section",
				Text: &slackText{
					Type: "mrkdwn",
					Text: fmt.Sprintf(":rocket: *Plan Started*\n`%s`", p.Name),
				},
			},
			{
				Type: "section",
				Fields: []slackText{
					{Type: "mrkdwn", Text: fmt.Sprintf("*Branch:*\n`%s`", p.Branch)},
				},
			},
		},
	}

	w.sendAsync(msg)
	return nil
}

// Complete sends a notification when a plan completes.
func (w *WebhookNotifier) Complete(p PlanInfo, prURL string) error {
	text := fmt.Sprintf(":white_check_mark: *Plan Complete*\n`%s`", p.Name)

	fields := []slackText{
		{Type: "mrkdwn", Text: fmt.Sprintf("*Branch:*\n`%s`", p.Branch)},
	}

	if prURL != "" {
		fields = append(fields, slackText{
			Type: "mrkdwn",
			Text: fmt.Sprintf("*Pull Request:*\n<%s|View PR>", prURL),
		})
	}

	msg := slackMessage{
		Blocks: []slackBlock{
			{
				Type: "section",
				Text: &slackText{Type: "mrkdwn", Text: text},
			},
			{
				Type:   "section",
				Fields: fields,
			},
		},
	}

	w.sendAsync(msg)
	return nil
}

// BlockerNotify sends a notification when a blocker is encountered.
func (w *WebhookNotifier) BlockerNotify(p PlanInfo, blocker *Blocker) error {
	if blocker == nil {
		return nil
	}

	blockerText := blocker.Description
	if blockerText == "" {
		blockerText = blocker.Content
	}

	blocks := []slackBlock{
		{
			Type: "section",
			Text: &slackText{
				Type: "mrkdwn",
				Text: fmt.Sprintf(":warning: *Human Input Required*\n`%s`", p.Name),
			},
		},
		{
			Type: "section",
			Text: &slackText{
				Type: "mrkdwn",
				Text: fmt.Sprintf("*Description:*\n%s", blockerText),
			},
		},
	}

	if blocker.Action != "" {
		blocks = append(blocks, slackBlock{
			Type: "section",
			Text: &slackText{
				Type: "mrkdwn",
				Text: fmt.Sprintf("*Action Required:*\n%s", blocker.Action),
			},
		})
	}

	if blocker.Resume != "" {
		blocks = append(blocks, slackBlock{
			Type: "section",
			Text: &slackText{
				Type: "mrkdwn",
				Text: fmt.Sprintf("*On Resume:*\n%s", blocker.Resume),
			},
		})
	}

	msg := slackMessage{
		Blocks: blocks,
	}

	w.sendAsync(msg)
	return nil
}

// Error sends a notification when an error occurs.
func (w *WebhookNotifier) Error(p PlanInfo, err error) error {
	if err == nil {
		return nil
	}

	errMsg := err.Error()
	if len(errMsg) > 500 {
		errMsg = errMsg[:500] + "..."
	}

	msg := slackMessage{
		Blocks: []slackBlock{
			{
				Type: "section",
				Text: &slackText{
					Type: "mrkdwn",
					Text: fmt.Sprintf(":x: *Plan Error*\n`%s`", p.Name),
				},
			},
			{
				Type: "section",
				Text: &slackText{
					Type: "mrkdwn",
					Text: fmt.Sprintf("*Error:*\n```%s```", errMsg),
				},
			},
		},
	}

	w.sendAsync(msg)
	return nil
}

// Iteration sends a notification for each iteration (if enabled).
func (w *WebhookNotifier) Iteration(p PlanInfo, iteration, maxIterations int) error {
	msg := slackMessage{
		Blocks: []slackBlock{
			{
				Type: "section",
				Text: &slackText{
					Type: "mrkdwn",
					Text: fmt.Sprintf(":hourglass_flowing_sand: *Iteration %d/%d*\n`%s`", iteration, maxIterations, p.Name),
				},
			},
		},
	}

	w.sendAsync(msg)
	return nil
}

// UpdateProgress is a no-op for webhooks since they don't support message updates.
func (w *WebhookNotifier) UpdateProgress(p PlanInfo, status *ProgressStatus) error {
	return nil
}

// sendAsync sends the message asynchronously.
func (w *WebhookNotifier) sendAsync(msg slackMessage) {
	go func() {
		if err := w.send(msg); err != nil {
			log.Debug("Failed to send Slack notification: %v", err)
		}
	}()
}

// send sends the message synchronously.
func (w *WebhookNotifier) send(msg slackMessage) error {
	body, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, w.webhookURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := w.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	return nil
}

// NoopNotifier is a Notifier that does nothing.
// Used when notifications are disabled.
type NoopNotifier struct{}

func (n *NoopNotifier) Start(p PlanInfo) error                           { return nil }
func (n *NoopNotifier) Complete(p PlanInfo, prURL string) error          { return nil }
func (n *NoopNotifier) BlockerNotify(p PlanInfo, blocker *Blocker) error { return nil }
func (n *NoopNotifier) Error(p PlanInfo, err error) error                { return nil }
func (n *NoopNotifier) Iteration(p PlanInfo, iteration, maxIterations int) error {
	return nil
}
func (n *NoopNotifier) UpdateProgress(p PlanInfo, status *ProgressStatus) error { return nil }

// Ensure NoopNotifier implements Notifier.
var _ Notifier = (*NoopNotifier)(nil)

// Ensure WebhookNotifier implements Notifier.
var _ Notifier = (*WebhookNotifier)(nil)

// NewNotifier creates a Notifier based on the configuration.
// Returns a SlackNotifier if bot_token is configured, falls back to WebhookNotifier,
// and returns NoopNotifier if neither is configured.
func NewNotifier(cfg *config.Config, tracker *ThreadTracker) Notifier {
	if cfg == nil {
		return &NoopNotifier{}
	}

	// Check for custom API URL (for testing)
	apiURL := os.Getenv("SLACK_API_URL")

	// Determine bot token - use global config if global_bot is true
	botToken := cfg.Slack.BotToken
	if cfg.Slack.GlobalBot && botToken == "" {
		// Load from global config (~/.ralph/slack.env)
		globalCfg, err := LoadGlobalBotConfig()
		if err != nil {
			log.Debug("Failed to load global bot config: %v", err)
		} else if globalCfg.BotToken != "" {
			botToken = globalCfg.BotToken
			log.Debug("Using global bot token from ~/.ralph/slack.env")
		}
	}

	// Try Slack Bot API first
	if botToken != "" && cfg.Slack.Channel != "" {
		return NewSlackNotifier(SlackNotifierConfig{
			BotToken:      botToken,
			Channel:       cfg.Slack.Channel,
			ThreadTracker: tracker,
			WebhookURL:    cfg.Slack.WebhookURL,
			APIURL:        apiURL,
			RepoName:      cfg.Project.Name,
		})
	}

	// Fall back to webhook
	if cfg.Slack.WebhookURL != "" {
		notifier := NewWebhookNotifier(cfg.Slack.WebhookURL)
		if notifier != nil {
			return notifier
		}
	}

	// No Slack configured
	return &NoopNotifier{}
}
