# Feature: Slack Integration

**ID:** F1.5
**Status:** planned
**Requires:** F1.3

## Summary

Slack notifications via webhook and Bot API, plus Socket Mode for handling thread replies. This absorbs the separate Python slack bot into the Go binary, eliminating the need for a separate process.

## Goals

- Send notifications via webhook (simple, no auth)
- Send notifications via Bot API (thread tracking)
- Track threads per plan (replies go to same thread)
- Handle Socket Mode for real-time reply processing
- Convert thread replies to feedback file entries
- Support global bot mode (~/.ralph/ for multi-repo)
- Deduplicate blocker notifications
- Async notifications (don't block execution)

## Non-Goals

- Full Slack app functionality
- Slash commands
- Interactive buttons/modals
- Message editing/deletion

## Design

### Notifier Interface

```go
type Notifier interface {
    Start(plan *Plan) error
    Complete(plan *Plan, prURL string) error
    Blocker(plan *Plan, blocker *Blocker) error
    Error(plan *Plan, err error) error
    Iteration(plan *Plan, iteration int) error
}

type SlackNotifier struct {
    config      *SlackConfig
    threads     *ThreadTracker
    bot         *SocketModeBot // nil if not configured
}
```

### Notification Modes

1. **Webhook Only**: Fire-and-forget POST to webhook URL
2. **Bot API**: Use bot token, track threads, support replies
3. **Socket Mode**: Real-time event handling for replies

### Thread Tracking

```go
type ThreadTracker struct {
    path    string // .ralph/slack_threads.json
    threads map[string]*ThreadInfo
}

type ThreadInfo struct {
    PlanName        string   `json:"plan_name"`
    ThreadTS        string   `json:"thread_ts"`
    ChannelID       string   `json:"channel_id"`
    NotifiedBlockers []string `json:"notified_blockers"` // Hashes
}
```

### Socket Mode Bot

```go
type SocketModeBot struct {
    client    *slack.Client
    socket    *socketmode.Client
    tracker   *ThreadTracker
    repoPath  string // For writing feedback files
}

func (b *SocketModeBot) Start(ctx context.Context) error
func (b *SocketModeBot) handleMessage(ev *slackevents.MessageEvent)
```

When a message is received in a tracked thread:
1. Look up plan from thread tracker
2. Append message to plan's feedback file
3. Acknowledge to Slack

### Global Bot Mode

When `slack.global_bot: true`:
- Bot config at `~/.ralph/slack.env`
- Thread tracker at `~/.ralph/slack_threads.json`
- Single bot handles multiple repos
- Feedback written to correct repo based on thread → plan mapping

### Message Formats

**Start:**
```
:rocket: Starting plan: *{plan_name}*
Branch: `{branch}`
```

**Complete:**
```
:white_check_mark: Completed: *{plan_name}*
PR: {pr_url}
```

**Blocker:**
```
:warning: Blocker in *{plan_name}*

{blocker_description}

*Action needed:* {action}
*To resume:* {resume}

Reply to this thread to provide input.
```

**Error:**
```
:x: Error in *{plan_name}*
{error_message}
```

### Key Files

| File | Purpose |
|------|---------|
| `internal/notify/notifier.go` | Notifier interface |
| `internal/notify/slack.go` | Slack implementation |
| `internal/notify/webhook.go` | Webhook-only notifier |
| `internal/notify/threads.go` | Thread tracking |
| `internal/notify/bot.go` | Socket Mode bot |
| `internal/notify/feedback.go` | Feedback file writing |

## Gotchas

- Webhook notifications are async - errors logged, not propagated
- Bot token and App token are different - both needed for Socket Mode
- Thread TS must be saved immediately after first message
- Global bot needs repo path in thread tracker to find feedback file
- Blocker hash deduplication prevents spam on retries
- Socket Mode requires constant connection - handle reconnects

---

## Changelog

- 2026-01-31: Initial spec
