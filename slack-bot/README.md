# Ralph Slack Bot

Socket Mode bot for handling human feedback on blockers via Slack thread replies.

## How It Works

1. When Ralph encounters a task requiring human action, it outputs a `<blocker>` marker
2. Ralph sends a Slack notification with the blocker details
3. The notification is registered for thread tracking
4. Humans reply to the Slack thread
5. The bot writes replies to the plan's feedback file
6. Ralph picks up the feedback on the next iteration

## Setup

### 1. Create Slack App

1. Go to [Slack API Apps](https://api.slack.com/apps)
2. Click "Create New App" → "From scratch"
3. Name it "Ralph Bot" and select your workspace

### 2. Enable Socket Mode

1. Go to **Settings** → **Socket Mode**
2. Enable Socket Mode
3. Create an App-Level Token:
   - Token Name: "ralph-socket"
   - Scope: `connections:write`
4. Save the token (starts with `xapp-`)

### 3. Configure Bot Token

1. Go to **Features** → **OAuth & Permissions**
2. Add Bot Token Scopes:
   - `chat:write` - Send messages
   - `channels:history` - Read channel messages
   - `groups:history` - Read private channel messages
3. Install app to workspace
4. Copy the Bot User OAuth Token (starts with `xoxb-`)

### 4. Subscribe to Events

1. Go to **Features** → **Event Subscriptions**
2. Enable Events
3. Subscribe to bot events:
   - `message.channels` - Messages in public channels
   - `message.groups` - Messages in private channels

### 5. Install the Bot

```bash
cd slack-bot
pip install -r requirements.txt
```

### 6. Configure Ralph

Add to your `.ralph/config.yaml`:

```yaml
slack:
  webhook_url: "https://hooks.slack.com/services/..."  # For basic notifications
  channel: "C0123456789"  # Channel ID for blocker notifications (required for thread tracking)
  notify_blocker: true
```

Set environment variables:

```bash
export SLACK_BOT_TOKEN="xoxb-..."   # Bot User OAuth Token
export SLACK_APP_TOKEN="xapp-..."   # App-Level Token (for Socket Mode)
```

### 7. Run the Bot

```bash
# In your project directory
python slack-bot/ralph_slack_bot.py --project-root .
```

Or run as a background service:

```bash
nohup python slack-bot/ralph_slack_bot.py --project-root /path/to/project > slack-bot.log 2>&1 &
```

## Usage Modes

### With Thread Tracking (Recommended)

When both `SLACK_BOT_TOKEN` and `slack.channel` are configured:
- Blocker notifications are sent via Slack API
- Thread IDs are tracked in `.ralph/slack_threads.json`
- Running the bot enables reply handling

### Without Thread Tracking (Webhook Only)

When only `slack.webhook_url` is configured:
- Blocker notifications are sent via webhook
- No thread tracking (can't receive replies)
- Humans must edit feedback file manually

## Files

| File | Purpose |
|------|---------|
| `ralph_slack_bot.py` | Main bot script (Socket Mode) |
| `send_blocker.sh` | CLI tool to send blocker with tracking |
| `requirements.txt` | Python dependencies |

## Thread Tracking

Threads are tracked in `.ralph/slack_threads.json`:

```json
{
  "C0123456789:1234567890.123456": {
    "plan_file": "plans/current/my-plan.md",
    "channel": "C0123456789",
    "thread_ts": "1234567890.123456",
    "created": "2024-01-30T14:32:00Z"
  }
}
```

## Feedback File Format

When the bot receives a thread reply, it writes to `<plan>.feedback.md`:

```markdown
# Feedback: my-plan

## Pending
- [2024-01-30 14:32] @alice: The package is now public, you can continue

## Processed
<!-- Agent moves items here after reading -->
```

## Troubleshooting

### Bot not receiving messages

1. Check Socket Mode is enabled
2. Verify event subscriptions are set up
3. Ensure bot is in the channel (invite with `/invite @Ralph Bot`)

### Thread replies not working

1. Verify `SLACK_BOT_TOKEN` is set and valid
2. Check `slack.channel` is configured with the correct Channel ID
3. Look for errors in bot logs

### Finding Channel ID

1. Right-click the channel in Slack
2. Select "View channel details"
3. Scroll to the bottom - Channel ID is shown there
