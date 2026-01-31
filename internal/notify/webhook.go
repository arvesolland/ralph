// Package notify handles Slack notifications for Ralph.
package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/arvesolland/ralph/internal/log"
	"github.com/arvesolland/ralph/internal/plan"
	"github.com/arvesolland/ralph/internal/runner"
)

// Notifier defines the interface for sending notifications.
type Notifier interface {
	// Start sends a notification when a plan starts.
	Start(p *plan.Plan) error

	// Complete sends a notification when a plan completes.
	Complete(p *plan.Plan, prURL string) error

	// Blocker sends a notification when a blocker is encountered.
	Blocker(p *plan.Plan, blocker *runner.Blocker) error

	// Error sends a notification when an error occurs.
	Error(p *plan.Plan, err error) error

	// Iteration sends a notification for each iteration (if enabled).
	Iteration(p *plan.Plan, iteration, maxIterations int) error
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
func (w *WebhookNotifier) Start(p *plan.Plan) error {
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
func (w *WebhookNotifier) Complete(p *plan.Plan, prURL string) error {
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

// Blocker sends a notification when a blocker is encountered.
func (w *WebhookNotifier) Blocker(p *plan.Plan, blocker *runner.Blocker) error {
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
func (w *WebhookNotifier) Error(p *plan.Plan, err error) error {
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
func (w *WebhookNotifier) Iteration(p *plan.Plan, iteration, maxIterations int) error {
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

// sendAsync sends the message asynchronously.
// Errors are logged but not returned.
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

// Start does nothing.
func (n *NoopNotifier) Start(p *plan.Plan) error { return nil }

// Complete does nothing.
func (n *NoopNotifier) Complete(p *plan.Plan, prURL string) error { return nil }

// Blocker does nothing.
func (n *NoopNotifier) Blocker(p *plan.Plan, blocker *runner.Blocker) error { return nil }

// Error does nothing.
func (n *NoopNotifier) Error(p *plan.Plan, err error) error { return nil }

// Iteration does nothing.
func (n *NoopNotifier) Iteration(p *plan.Plan, iteration, maxIterations int) error { return nil }

// Ensure NoopNotifier implements Notifier.
var _ Notifier = (*NoopNotifier)(nil)

// Ensure WebhookNotifier implements Notifier.
var _ Notifier = (*WebhookNotifier)(nil)
