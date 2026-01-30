#!/usr/bin/env python3
"""
Ralph Slack Bot - Socket Mode bot for handling blocker replies.

This bot listens for thread replies to blocker notifications and writes
them to the appropriate feedback file so Ralph can act on human input.

Usage:
    python ralph_slack_bot.py [--project-root /path/to/project]

Environment Variables (or in slack-bot/.env):
    SLACK_BOT_TOKEN - Bot token (starts with xoxb-)
    SLACK_APP_TOKEN - App-level token for Socket Mode (starts with xapp-)

Setup:
    1. Create Slack app at https://api.slack.com/apps
    2. Enable Socket Mode (Settings > Socket Mode)
    3. Create App-Level Token with connections:write scope
    4. Add Bot Token Scopes: chat:write, channels:history, groups:history, users:read
    5. Subscribe to Events: message.channels, message.groups
    6. Install app to workspace
"""

import os
import sys
import json
import argparse
import logging
from datetime import datetime
from pathlib import Path

# Load .env file if present
SCRIPT_DIR = Path(__file__).parent
ENV_FILE = SCRIPT_DIR / ".env"
if ENV_FILE.exists():
    try:
        from dotenv import load_dotenv
        load_dotenv(ENV_FILE)
        print(f"Loaded environment from {ENV_FILE}")
    except ImportError:
        # Manual .env parsing fallback
        with open(ENV_FILE) as f:
            for line in f:
                line = line.strip()
                if line and not line.startswith('#') and '=' in line:
                    key, value = line.split('=', 1)
                    os.environ[key.strip()] = value.strip().strip('"').strip("'")
        print(f"Loaded environment from {ENV_FILE} (manual parsing)")

try:
    from slack_bolt import App
    from slack_bolt.adapter.socket_mode import SocketModeHandler
except ImportError:
    print("Error: slack-bolt not installed. Run: pip install slack-bolt")
    sys.exit(1)

# Configure logging
logging.basicConfig(
    level=logging.INFO,
    format='%(asctime)s - %(levelname)s - %(message)s'
)
logger = logging.getLogger(__name__)

# Thread tracking file - maps plan_file to thread info
THREAD_TRACKER_FILE = ".ralph/slack_threads.json"


class RalphSlackBot:
    def __init__(self, project_root: str = None):
        self.project_root = Path(project_root) if project_root else Path.cwd()
        self.tracker_file = self.project_root / THREAD_TRACKER_FILE
        self.threads = self._load_threads()
        self._thread_to_plan = self._build_reverse_lookup()

        # Initialize Slack app
        self.app = App(token=os.environ.get("SLACK_BOT_TOKEN"))
        self._setup_handlers()

    def _load_threads(self) -> dict:
        """Load thread tracking data (keyed by plan_file)."""
        if self.tracker_file.exists():
            try:
                return json.loads(self.tracker_file.read_text())
            except json.JSONDecodeError:
                logger.warning("Could not parse thread tracker file, starting fresh")
        return {}

    def _build_reverse_lookup(self) -> dict:
        """Build reverse lookup: channel:thread_ts -> plan_file."""
        reverse = {}
        for plan_file, info in self.threads.items():
            if isinstance(info, dict) and "thread_ts" in info and "channel" in info:
                key = f"{info['channel']}:{info['thread_ts']}"
                reverse[key] = plan_file
        return reverse

    def _save_threads(self):
        """Save thread tracking data."""
        self.tracker_file.parent.mkdir(parents=True, exist_ok=True)
        self.tracker_file.write_text(json.dumps(self.threads, indent=2))
        self._thread_to_plan = self._build_reverse_lookup()

    def reload_threads(self):
        """Reload threads from disk (in case they changed externally)."""
        self.threads = self._load_threads()
        self._thread_to_plan = self._build_reverse_lookup()
        logger.info(f"Reloaded {len(self.threads)} tracked plans")

    def _get_feedback_file(self, plan_file: str) -> Path:
        """Get the feedback file path for a plan."""
        plan_path = Path(plan_file)
        return plan_path.parent / f"{plan_path.stem}.feedback.md"

    def _write_feedback(self, plan_file: str, user: str, message: str):
        """Write feedback to the plan's feedback file."""
        feedback_file = self._get_feedback_file(plan_file)
        timestamp = datetime.now().strftime("%Y-%m-%d %H:%M")

        # Create or update feedback file
        if feedback_file.exists():
            content = feedback_file.read_text()
        else:
            plan_name = Path(plan_file).stem
            content = f"# Feedback: {plan_name}\n\n## Pending\n\n## Processed\n"

        # Find the Pending section and add the feedback
        if "## Pending" in content:
            parts = content.split("## Pending")
            before = parts[0]
            after = parts[1] if len(parts) > 1 else ""

            # Find where Pending section ends (next ## or end of file)
            if "## Processed" in after:
                pending_content, rest = after.split("## Processed", 1)
                new_content = f"{before}## Pending{pending_content}- [{timestamp}] @{user}: {message}\n\n## Processed{rest}"
            else:
                new_content = f"{before}## Pending{after}\n- [{timestamp}] @{user}: {message}\n"
        else:
            # No Pending section, add at end
            new_content = content + f"\n## Pending\n- [{timestamp}] @{user}: {message}\n"

        feedback_file.write_text(new_content)
        logger.info(f"Wrote feedback to {feedback_file}")
        return feedback_file

    def _setup_handlers(self):
        """Set up Slack event handlers."""

        @self.app.event("message")
        def handle_message(event, say, client):
            """Handle incoming messages - look for thread replies."""
            # Only process thread replies
            thread_ts = event.get("thread_ts")
            if not thread_ts:
                return

            channel = event.get("channel")
            thread_key = f"{channel}:{thread_ts}"

            # Reload threads in case they changed (new plans started)
            self.reload_threads()

            # Check if this is a reply to a tracked plan thread
            if thread_key not in self._thread_to_plan:
                return

            # Ignore bot messages (including our own)
            if event.get("bot_id") or event.get("subtype") == "bot_message":
                return

            plan_file = self._thread_to_plan[thread_key]
            user = event.get("user", "unknown")
            text = event.get("text", "")

            if not text.strip():
                return

            # Get user info for display name
            try:
                user_info = client.users_info(user=user)
                display_name = user_info["user"]["profile"].get("display_name") or user_info["user"]["name"]
            except Exception:
                display_name = user

            logger.info(f"Received reply from {display_name} for plan {plan_file}: {text[:50]}...")

            # Write to feedback file
            feedback_file = self._write_feedback(plan_file, display_name, text)

            # Acknowledge in thread
            try:
                say(
                    text=f":white_check_mark: Feedback recorded in `{feedback_file.name}`. Ralph will pick this up on the next iteration.",
                    thread_ts=thread_ts
                )
            except Exception as e:
                logger.error(f"Failed to send acknowledgment: {e}")

    def run(self):
        """Start the bot."""
        app_token = os.environ.get("SLACK_APP_TOKEN")
        if not app_token:
            logger.error("SLACK_APP_TOKEN environment variable not set")
            sys.exit(1)

        if not os.environ.get("SLACK_BOT_TOKEN"):
            logger.error("SLACK_BOT_TOKEN environment variable not set")
            sys.exit(1)

        logger.info(f"Starting Ralph Slack Bot (project: {self.project_root})")
        logger.info(f"Tracking {len(self.threads)} active plan(s)")
        for plan, info in self.threads.items():
            if isinstance(info, dict) and "thread_ts" in info:
                logger.info(f"  - {plan} -> thread {info['thread_ts']}")

        handler = SocketModeHandler(self.app, app_token)
        handler.start()


def main():
    parser = argparse.ArgumentParser(description="Ralph Slack Bot - Handle thread replies for plan feedback")
    parser.add_argument("--project-root", default=".", help="Project root directory")

    args = parser.parse_args()

    bot = RalphSlackBot(project_root=args.project_root)
    bot.run()


if __name__ == "__main__":
    main()
