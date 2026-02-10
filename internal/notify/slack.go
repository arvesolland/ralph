package notify

import (
	"fmt"
	"strings"

	"github.com/arvesolland/ralph/internal/log"
	"github.com/slack-go/slack"
)

// SlackNotifier sends notifications via the Slack Bot API with thread tracking.
// If bot_token is not configured, it falls back to WebhookNotifier.
type SlackNotifier struct {
	client        *slack.Client
	channel       string
	threadTracker *ThreadTracker
	repoName      string
	teamURL       string
}

// SlackNotifierConfig contains configuration for creating a SlackNotifier.
type SlackNotifierConfig struct {
	BotToken      string
	Channel       string
	WebhookURL    string
	ThreadTracker *ThreadTracker
	RepoName      string
	// APIURL is an optional custom Slack API URL (for testing).
	// If empty, the default Slack API URL is used.
	APIURL string
}

// NewSlackNotifier creates a new SlackNotifier.
// If botToken is empty, falls back to WebhookNotifier using webhookURL.
// Returns nil if neither botToken nor webhookURL is configured.
func NewSlackNotifier(cfg SlackNotifierConfig) Notifier {
	// If bot token is configured, use Bot API
	if cfg.BotToken != "" && cfg.Channel != "" {
		opts := []slack.Option{}
		if cfg.APIURL != "" {
			opts = append(opts, slack.OptionAPIURL(cfg.APIURL))
		}
		client := slack.New(cfg.BotToken, opts...)

		// Fetch workspace URL via auth.test for constructing thread permalinks
		var teamURL string
		resp, err := client.AuthTest()
		if err != nil {
			log.Debug("Failed to get workspace URL via auth.test: %v", err)
		} else if resp != nil {
			teamURL = resp.URL
		}

		return &SlackNotifier{
			client:        client,
			channel:       cfg.Channel,
			threadTracker: cfg.ThreadTracker,
			repoName:      cfg.RepoName,
			teamURL:       teamURL,
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
func (s *SlackNotifier) Start(p PlanInfo) error {
	// Build initial status card with progress display
	status := &ProgressStatus{
		Iteration:     0,
		MaxIterations: 0,
		Phase:         PhaseInitializing,
	}
	blocks := s.buildProgressBlocks(p, status)

	// Post message to channel (this creates the thread)
	_, ts, err := s.postMessage(blocks)
	if err != nil {
		log.Debug("Failed to send Slack start notification: %v", err)
		return nil
	}

	// Save thread info for future messages and updates
	if s.threadTracker != nil && ts != "" {
		info := &ThreadInfo{
			PlanName:      p.Name,
			ThreadTS:      ts,
			ChannelID:     s.channel,
			MessageTS:     ts,
			LastPhase:     string(PhaseInitializing),
			LastIteration: 1,
		}
		if err := s.threadTracker.Set(p.Name, info); err != nil {
			log.Debug("Failed to save thread info: %v", err)
		}
	}

	return nil
}

// Complete sends a notification when a plan completes.
func (s *SlackNotifier) Complete(p PlanInfo, prURL string) error {
	var text string
	if s.repoName != "" {
		text = fmt.Sprintf(":white_check_mark: *Plan Complete*\n`%s` · `%s`", s.repoName, p.Name)
	} else {
		text = fmt.Sprintf(":white_check_mark: *Plan Complete*\n`%s`", p.Name)
	}

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

// BlockerNotify sends a notification when a blocker is encountered.
func (s *SlackNotifier) BlockerNotify(p PlanInfo, blocker *Blocker) error {
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

	var headerText string
	if s.repoName != "" {
		headerText = fmt.Sprintf(":warning: *Human Input Required*\n`%s` · `%s`", s.repoName, p.Name)
	} else {
		headerText = fmt.Sprintf(":warning: *Human Input Required*\n`%s`", p.Name)
	}

	blocks := []slack.Block{
		slack.NewSectionBlock(
			slack.NewTextBlockObject(slack.MarkdownType, headerText, false, false),
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
func (s *SlackNotifier) Error(p PlanInfo, err error) error {
	if err == nil {
		return nil
	}

	errMsg := err.Error()
	if len(errMsg) > 500 {
		errMsg = errMsg[:500] + "..."
	}

	var headerText string
	if s.repoName != "" {
		headerText = fmt.Sprintf(":x: *Plan Error*\n`%s` · `%s`", s.repoName, p.Name)
	} else {
		headerText = fmt.Sprintf(":x: *Plan Error*\n`%s`", p.Name)
	}

	blocks := []slack.Block{
		slack.NewSectionBlock(
			slack.NewTextBlockObject(slack.MarkdownType, headerText, false, false),
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
func (s *SlackNotifier) Iteration(p PlanInfo, iteration, maxIterations int) error {
	blocks := []slack.Block{
		slack.NewSectionBlock(
			slack.NewTextBlockObject(slack.MarkdownType, fmt.Sprintf(":hourglass_flowing_sand: *Iteration %d/%d*\n`%s`", iteration, maxIterations, p.Name), false, false),
			nil, nil,
		),
	}

	s.postMessageInThread(p.Name, blocks)
	return nil
}

// UpdateProgress updates the parent message with current progress status.
func (s *SlackNotifier) UpdateProgress(p PlanInfo, status *ProgressStatus) error {
	if s.threadTracker == nil {
		return nil
	}

	info := s.threadTracker.Get(p.Name)
	if info == nil || info.MessageTS == "" {
		log.Debug("No message to update for plan: %s", p.Name)
		return nil
	}

	// Check if we should update (avoid redundant updates)
	changed, err := s.threadTracker.UpdateProgress(p.Name, status.Iteration, string(status.Phase), status.TasksDone)
	if err != nil {
		log.Debug("Failed to check progress change: %v", err)
	}
	if !changed {
		log.Debug("Progress unchanged, skipping update")
		return nil
	}

	// Build updated blocks
	blocks := s.buildProgressBlocks(p, status)

	// Update the message asynchronously
	go func() {
		_, _, _, err := s.client.UpdateMessage(
			info.ChannelID,
			info.MessageTS,
			slack.MsgOptionBlocks(blocks...),
		)
		if err != nil {
			log.Debug("Failed to update Slack progress: %v", err)
		}
	}()

	return nil
}

// buildProgressBlocks creates the Block Kit blocks for a progress status card.
func (s *SlackNotifier) buildProgressBlocks(p PlanInfo, status *ProgressStatus) []slack.Block {
	var emoji string
	var phaseText string
	switch status.Phase {
	case PhaseInitializing:
		emoji = ":rocket:"
		phaseText = "Initializing"
	case PhaseRunning:
		emoji = ":hourglass_flowing_sand:"
		phaseText = "Running"
	case PhaseVerifying:
		emoji = ":mag:"
		phaseText = "Verifying"
	case PhaseBlocked:
		emoji = ":warning:"
		phaseText = "Blocked"
	case PhaseComplete:
		emoji = ":white_check_mark:"
		phaseText = "Complete"
	case PhaseError:
		emoji = ":x:"
		phaseText = "Error"
	default:
		emoji = ":hourglass_flowing_sand:"
		phaseText = "Running"
	}

	// Build progress bar (10 segments) based on task completion or iteration
	var progressText string
	if status.TasksTotal > 0 {
		// Task-based progress
		filled := int(float64(status.TasksDone) / float64(status.TasksTotal) * 10)
		if filled > 10 {
			filled = 10
		}
		progressBar := strings.Repeat("\u2588", filled) + strings.Repeat("\u2591", 10-filled)
		progressText = fmt.Sprintf("%s %d/%d tasks", progressBar, status.TasksDone, status.TasksTotal)
		if status.Iteration > 0 {
			progressText += fmt.Sprintf(" (iteration %d)", status.Iteration)
		}
	} else if status.MaxIterations > 0 {
		// Fallback: iteration-based progress
		filled := int(float64(status.Iteration) / float64(status.MaxIterations) * 10)
		if filled > 10 {
			filled = 10
		}
		progressBar := strings.Repeat("\u2588", filled) + strings.Repeat("\u2591", 10-filled)
		progressText = fmt.Sprintf("%s %d/%d", progressBar, status.Iteration, status.MaxIterations)
	}

	// Build header text
	var headerText string
	if s.repoName != "" {
		headerText = fmt.Sprintf("%s *Plan %s*\n`%s` · `%s`", emoji, phaseText, s.repoName, p.Name)
	} else {
		headerText = fmt.Sprintf("%s *Plan %s*\n`%s`", emoji, phaseText, p.Name)
	}

	// Build fields
	fields := []*slack.TextBlockObject{
		slack.NewTextBlockObject(slack.MarkdownType, fmt.Sprintf("*Branch:*\n`%s`", p.Branch), false, false),
	}

	if progressText != "" {
		fields = append(fields, slack.NewTextBlockObject(slack.MarkdownType, fmt.Sprintf("*Progress:*\n%s", progressText), false, false))
	}

	if status.Message != "" {
		fields = append(fields, slack.NewTextBlockObject(slack.MarkdownType, fmt.Sprintf("*Status:*\n%s", status.Message), false, false))
	}

	blocks := []slack.Block{
		slack.NewSectionBlock(
			slack.NewTextBlockObject(slack.MarkdownType, headerText, false, false),
			nil, nil,
		),
		slack.NewSectionBlock(nil, fields, nil),
	}

	return blocks
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

// SeedThread pre-populates the thread tracker with an existing Slack thread.
// This is used when resuming a plan that already has a thread URL stored in ATM.
func (s *SlackNotifier) SeedThread(planName, channelID, threadTS string) error {
	if s.threadTracker == nil {
		return nil
	}

	// Only seed if we don't already have a thread for this plan
	if existing := s.threadTracker.Get(planName); existing != nil {
		return nil
	}

	info := &ThreadInfo{
		PlanName:  planName,
		ThreadTS:  threadTS,
		ChannelID: channelID,
		MessageTS: threadTS,
	}
	return s.threadTracker.Set(planName, info)
}

// TeamURL returns the Slack workspace base URL (e.g. "https://myteam.slack.com/").
func (s *SlackNotifier) TeamURL() string {
	return s.teamURL
}

// GetThreadURL returns the Slack thread permalink for a plan, or empty string if unavailable.
func (s *SlackNotifier) GetThreadURL(planName string) string {
	if s.teamURL == "" || s.threadTracker == nil {
		return ""
	}
	info := s.threadTracker.Get(planName)
	if info == nil || info.ThreadTS == "" || info.ChannelID == "" {
		return ""
	}
	return BuildSlackThreadURL(s.teamURL, info.ChannelID, info.ThreadTS)
}

// Ensure SlackNotifier implements Notifier.
var _ Notifier = (*SlackNotifier)(nil)
