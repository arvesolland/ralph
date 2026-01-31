package notify

import (
	"fmt"

	"github.com/arvesolland/ralph/internal/log"
	"github.com/arvesolland/ralph/internal/plan"
	"github.com/arvesolland/ralph/internal/runner"
	"github.com/slack-go/slack"
)

// SlackNotifier sends notifications via the Slack Bot API with thread tracking.
// If bot_token is not configured, it falls back to WebhookNotifier.
type SlackNotifier struct {
	client        *slack.Client
	channel       string
	threadTracker *ThreadTracker

	// fallback is used when bot_token is not configured
	fallback *WebhookNotifier
}

// SlackNotifierConfig contains configuration for creating a SlackNotifier.
type SlackNotifierConfig struct {
	BotToken      string
	Channel       string
	WebhookURL    string
	ThreadTracker *ThreadTracker
}

// NewSlackNotifier creates a new SlackNotifier.
// If botToken is empty, falls back to WebhookNotifier using webhookURL.
// Returns nil if neither botToken nor webhookURL is configured.
func NewSlackNotifier(cfg SlackNotifierConfig) Notifier {
	// If bot token is configured, use Bot API
	if cfg.BotToken != "" && cfg.Channel != "" {
		return &SlackNotifier{
			client:        slack.New(cfg.BotToken),
			channel:       cfg.Channel,
			threadTracker: cfg.ThreadTracker,
		}
	}

	// Fall back to webhook
	if cfg.WebhookURL != "" {
		return NewWebhookNotifier(cfg.WebhookURL)
	}

	// No configuration, return noop
	return &NoopNotifier{}
}

// Start sends a notification when a plan starts and creates a new thread.
func (s *SlackNotifier) Start(p *plan.Plan) error {
	blocks := []slack.Block{
		slack.NewSectionBlock(
			slack.NewTextBlockObject(slack.MarkdownType, fmt.Sprintf(":rocket: *Plan Started*\n`%s`", p.Name), false, false),
			nil, nil,
		),
		slack.NewSectionBlock(nil,
			[]*slack.TextBlockObject{
				slack.NewTextBlockObject(slack.MarkdownType, fmt.Sprintf("*Branch:*\n`%s`", p.Branch), false, false),
			},
			nil,
		),
	}

	// Post message to channel (this creates the thread)
	_, ts, err := s.postMessage(blocks)
	if err != nil {
		log.Debug("Failed to send Slack start notification: %v", err)
		return nil // Don't fail plan execution for notification errors
	}

	// Save thread info for future messages
	if s.threadTracker != nil && ts != "" {
		info := &ThreadInfo{
			PlanName:  p.Name,
			ThreadTS:  ts,
			ChannelID: s.channel,
		}
		if err := s.threadTracker.Set(p.Name, info); err != nil {
			log.Debug("Failed to save thread info: %v", err)
		}
	}

	return nil
}

// Complete sends a notification when a plan completes.
func (s *SlackNotifier) Complete(p *plan.Plan, prURL string) error {
	text := fmt.Sprintf(":white_check_mark: *Plan Complete*\n`%s`", p.Name)

	fields := []*slack.TextBlockObject{
		slack.NewTextBlockObject(slack.MarkdownType, fmt.Sprintf("*Branch:*\n`%s`", p.Branch), false, false),
	}

	if prURL != "" {
		fields = append(fields, slack.NewTextBlockObject(
			slack.MarkdownType,
			fmt.Sprintf("*Pull Request:*\n<%s|View PR>", prURL),
			false, false,
		))
	}

	blocks := []slack.Block{
		slack.NewSectionBlock(
			slack.NewTextBlockObject(slack.MarkdownType, text, false, false),
			nil, nil,
		),
		slack.NewSectionBlock(nil, fields, nil),
	}

	s.postMessageInThread(p.Name, blocks)
	return nil
}

// Blocker sends a notification when a blocker is encountered.
// Uses blocker hash deduplication to prevent duplicate notifications.
func (s *SlackNotifier) Blocker(p *plan.Plan, blocker *runner.Blocker) error {
	if blocker == nil {
		return nil
	}

	// Check if this blocker has already been notified
	if s.threadTracker != nil {
		if s.threadTracker.HasNotifiedBlocker(p.Name, blocker.Hash) {
			log.Debug("Blocker already notified (hash: %s), skipping", blocker.Hash)
			return nil
		}
	}

	blockerText := blocker.Description
	if blockerText == "" {
		blockerText = blocker.Content
	}

	blocks := []slack.Block{
		slack.NewSectionBlock(
			slack.NewTextBlockObject(slack.MarkdownType, fmt.Sprintf(":warning: *Human Input Required*\n`%s`", p.Name), false, false),
			nil, nil,
		),
		slack.NewSectionBlock(
			slack.NewTextBlockObject(slack.MarkdownType, fmt.Sprintf("*Description:*\n%s", blockerText), false, false),
			nil, nil,
		),
	}

	if blocker.Action != "" {
		blocks = append(blocks, slack.NewSectionBlock(
			slack.NewTextBlockObject(slack.MarkdownType, fmt.Sprintf("*Action Required:*\n%s", blocker.Action), false, false),
			nil, nil,
		))
	}

	if blocker.Resume != "" {
		blocks = append(blocks, slack.NewSectionBlock(
			slack.NewTextBlockObject(slack.MarkdownType, fmt.Sprintf("*On Resume:*\n%s", blocker.Resume), false, false),
			nil, nil,
		))
	}

	s.postMessageInThread(p.Name, blocks)

	// Mark blocker as notified
	if s.threadTracker != nil {
		if _, err := s.threadTracker.AddNotifiedBlocker(p.Name, blocker.Hash); err != nil {
			log.Debug("Failed to mark blocker as notified: %v", err)
		}
	}

	return nil
}

// Error sends a notification when an error occurs.
func (s *SlackNotifier) Error(p *plan.Plan, err error) error {
	if err == nil {
		return nil
	}

	errMsg := err.Error()
	if len(errMsg) > 500 {
		errMsg = errMsg[:500] + "..."
	}

	blocks := []slack.Block{
		slack.NewSectionBlock(
			slack.NewTextBlockObject(slack.MarkdownType, fmt.Sprintf(":x: *Plan Error*\n`%s`", p.Name), false, false),
			nil, nil,
		),
		slack.NewSectionBlock(
			slack.NewTextBlockObject(slack.MarkdownType, fmt.Sprintf("*Error:*\n```%s```", errMsg), false, false),
			nil, nil,
		),
	}

	s.postMessageInThread(p.Name, blocks)
	return nil
}

// Iteration sends a notification for each iteration (if enabled).
func (s *SlackNotifier) Iteration(p *plan.Plan, iteration, maxIterations int) error {
	blocks := []slack.Block{
		slack.NewSectionBlock(
			slack.NewTextBlockObject(slack.MarkdownType, fmt.Sprintf(":hourglass_flowing_sand: *Iteration %d/%d*\n`%s`", iteration, maxIterations, p.Name), false, false),
			nil, nil,
		),
	}

	s.postMessageInThread(p.Name, blocks)
	return nil
}

// postMessage posts a message to the channel and returns the channel ID and timestamp.
func (s *SlackNotifier) postMessage(blocks []slack.Block) (string, string, error) {
	channel, ts, err := s.client.PostMessage(
		s.channel,
		slack.MsgOptionBlocks(blocks...),
	)
	return channel, ts, err
}

// postMessageInThread posts a message as a reply to the plan's thread.
// If no thread exists for the plan, posts to the channel directly.
func (s *SlackNotifier) postMessageInThread(planName string, blocks []slack.Block) {
	go func() {
		var threadTS string
		if s.threadTracker != nil {
			if info := s.threadTracker.Get(planName); info != nil {
				threadTS = info.ThreadTS
			}
		}

		opts := []slack.MsgOption{slack.MsgOptionBlocks(blocks...)}
		if threadTS != "" {
			opts = append(opts, slack.MsgOptionTS(threadTS))
		}

		_, _, err := s.client.PostMessage(s.channel, opts...)
		if err != nil {
			log.Debug("Failed to send Slack notification: %v", err)
		}
	}()
}

// Ensure SlackNotifier implements Notifier.
var _ Notifier = (*SlackNotifier)(nil)
