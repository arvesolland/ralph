# Ralph Slack Bot

Socket Mode bot for handling human feedback via Slack thread replies. Supports both per-repo and global (multi-repo) modes.

## How It Works

1. When Ralph encounters a task requiring human action, it outputs a `<blocker>` marker
2. Ralph sends a Slack notification (creates a thread)
3. All plan updates go to that thread (progress, blockers, completion)
4. Humans reply to the Slack thread
5. The bot writes replies to the plan's feedback file
6. Ralph picks up the feedback on the next iteration

## Quick Start (Global Mode - Recommended)

```bash
# 1. Install dependencies
pip install -r slack-bot/requirements.txt

# 2. Create global credentials file
mkdir -p ~/.ralph
cat > ~/.ralph/slack.env << 'EOF'
SLACK_BOT_TOKEN=xoxb-your-token
SLACK_APP_TOKEN=xapp-your-token
EOF

# 3. Configure your project's .ralph/config.yaml
slack:
  channel: "C0123456789"  # Your channel ID
  global_bot: true        # Use single bot for all repos

# 4. Bot auto-starts when ralph-worker.sh runs!
```

## Modes

### Global Mode (`--global` or `global_bot: true`)

- Single bot instance at `~/.ralph/`
- Handles multiple repos simultaneously
- Thread tracking uses absolute paths
- Credentials from `~/.ralph/slack.env`
- **Recommended for machines running multiple Ralph projects**

### Local Mode (default)

- One bot per repo at `.ralph/`
- Only handles that repo
- Credentials from `./slack-bot/.env` or environment

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
   - `users:read` - Get user display names
3. Install app to workspace
4. Copy the Bot User OAuth Token (starts with `xoxb-`)

### 4. Subscribe to Events

1. Go to **Features** → **Event Subscriptions**
2. Enable Events
3. Subscribe to bot events:
   - `message.channels` - Messages in public channels
   - `message.groups` - Messages in private channels

### 5. Configure Ralph

Add to your `.ralph/config.yaml`:

```yaml
slack:
  channel: "C0123456789"    # Channel ID (required)
  global_bot: true          # Use global bot mode
  notify_start: true
  notify_complete: true
  notify_blocker: true
  notify_error: true
```

## Running the Bot

### Auto-Start (Recommended)

When `ralph-worker.sh` runs, it automatically starts the bot if:
- `SLACK_BOT_TOKEN` and `SLACK_APP_TOKEN` are available
- `slack.channel` is configured
- Bot isn't already running

### Manual Start

```bash
# Global mode (handles all repos)
python slack-bot/ralph_slack_bot.py --global

# Local mode (current repo only)
python slack-bot/ralph_slack_bot.py

# Check status
python slack-bot/ralph_slack_bot.py --status
python slack-bot/ralph_slack_bot.py --global --status
```

## Files

| File | Purpose |
|------|---------|
| `~/.ralph/slack.env` | Global credentials |
| `~/.ralph/slack_threads.json` | Global thread tracking |
| `~/.ralph/slack_bot.pid` | Global bot PID file |
| `~/.ralph/slack_bot.log` | Global bot log |

## Thread Tracking

Threads are tracked with absolute paths (global mode):

```json
{
  "/Users/dev/project-a/plans/current/my-plan.md": {
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

### Bot not starting automatically

1. Check `SLACK_BOT_TOKEN` and `SLACK_APP_TOKEN` are set (or in `~/.ralph/slack.env`)
2. Check `slack.channel` is configured
3. Check logs: `tail -f ~/.ralph/slack_bot.log`

### Bot not receiving messages

1. Check Socket Mode is enabled in Slack app settings
2. Verify event subscriptions are set up
3. Ensure bot is in the channel (invite with `/invite @Ralph Bot`)

### Finding Channel ID

1. Right-click the channel in Slack
2. Select "View channel details"
3. Scroll to the bottom - Channel ID is shown there
