#!/usr/bin/env python3
"""
Ralph Slack Bot - Global Socket Mode bot for handling thread replies.

This bot runs as a single instance per machine and handles replies for
multiple Ralph repos. Thread tracking uses absolute paths.

Usage:
    python ralph_slack_bot.py [--global]

Modes:
    --global    Run as global bot from ~/.ralph/ (recommended)
    (default)   Run for current directory only

Environment Variables (or in ~/.ralph/slack.env or ./slack-bot/.env):
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
import fcntl
import atexit

# Possible .env file locations (in order of preference)
ENV_LOCATIONS = [
    Path.home() / ".ralph" / "slack.env",      # Global config
    Path(__file__).parent / ".env",             # Local to script
]

def load_env():
    """Load environment from .env files."""
    for env_file in ENV_LOCATIONS:
        if env_file.exists():
            try:
                from dotenv import load_dotenv
                load_dotenv(env_file)
                return str(env_file)
            except ImportError:
                # Manual parsing fallback
                with open(env_file) as f:
                    for line in f:
                        line = line.strip()
                        if line and not line.startswith('#') and '=' in line:
                            key, value = line.split('=', 1)
                            os.environ[key.strip()] = value.strip().strip('"').strip("'")
                return str(env_file)
    return None

# Load env before other imports that might need tokens
env_loaded = load_env()

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

# Global paths
GLOBAL_RALPH_DIR = Path.home() / ".ralph"
GLOBAL_THREAD_FILE = GLOBAL_RALPH_DIR / "slack_threads.json"
GLOBAL_PID_FILE = GLOBAL_RALPH_DIR / "slack_bot.pid"
GLOBAL_LOG_FILE = GLOBAL_RALPH_DIR / "slack_bot.log"


class RalphSlackBot:
    def __init__(self, global_mode: bool = False):
        self.global_mode = global_mode

        if global_mode:
            self.ralph_dir = GLOBAL_RALPH_DIR
            self.tracker_file = GLOBAL_THREAD_FILE
            self.pid_file = GLOBAL_PID_FILE
        else:
            # Local mode - use current directory's .ralph
            self.ralph_dir = Path.cwd() / ".ralph"
            self.tracker_file = self.ralph_dir / "slack_threads.json"
            self.pid_file = self.ralph_dir / "slack_bot.pid"

        self.ralph_dir.mkdir(parents=True, exist_ok=True)

        self.threads = self._load_threads()
        self._thread_to_plan = self._build_reverse_lookup()

        # Initialize Slack app
        self.app = App(token=os.environ.get("SLACK_BOT_TOKEN"))
        self._setup_handlers()

        # PID file handling
        self._pid_file_handle = None

    def _load_threads(self) -> dict:
        """Load thread tracking data (keyed by plan_file with absolute paths)."""
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
            parts = content.split("## Pending", 1)
            before = parts[0]
            after = parts[1] if len(parts) > 1 else ""

            # Insert after ## Pending line
            new_content = f"{before}## Pending\n- [{timestamp}] @{user}: {message}\n{after.lstrip()}"
        else:
            # No Pending section, add at end
            new_content = content + f"\n## Pending\n- [{timestamp}] @{user}: {message}\n"

        feedback_file.parent.mkdir(parents=True, exist_ok=True)
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

    def _acquire_pid_lock(self) -> bool:
        """Acquire PID file lock. Returns True if successful."""
        try:
            self._pid_file_handle = open(self.pid_file, 'w')
            fcntl.flock(self._pid_file_handle.fileno(), fcntl.LOCK_EX | fcntl.LOCK_NB)
            self._pid_file_handle.write(str(os.getpid()))
            self._pid_file_handle.flush()
            return True
        except (IOError, OSError):
            return False

    def _release_pid_lock(self):
        """Release PID file lock."""
        if self._pid_file_handle:
            try:
                fcntl.flock(self._pid_file_handle.fileno(), fcntl.LOCK_UN)
                self._pid_file_handle.close()
                self.pid_file.unlink(missing_ok=True)
            except Exception:
                pass

    def run(self):
        """Start the bot."""
        app_token = os.environ.get("SLACK_APP_TOKEN")
        if not app_token:
            logger.error("SLACK_APP_TOKEN environment variable not set")
            sys.exit(1)

        if not os.environ.get("SLACK_BOT_TOKEN"):
            logger.error("SLACK_BOT_TOKEN environment variable not set")
            sys.exit(1)

        # Try to acquire lock
        if not self._acquire_pid_lock():
            logger.error(f"Another bot instance is already running (pid file: {self.pid_file})")
            sys.exit(1)

        # Clean up on exit
        atexit.register(self._release_pid_lock)

        mode = "global" if self.global_mode else "local"
        logger.info(f"Starting Ralph Slack Bot ({mode} mode)")
        if env_loaded:
            logger.info(f"Loaded credentials from {env_loaded}")
        logger.info(f"Thread tracking: {self.tracker_file}")
        logger.info(f"PID file: {self.pid_file}")
        logger.info(f"Tracking {len(self.threads)} active plan(s)")

        for plan, info in self.threads.items():
            if isinstance(info, dict) and "thread_ts" in info:
                logger.info(f"  - {plan}")

        handler = SocketModeHandler(self.app, app_token)
        handler.start()


def is_bot_running(global_mode: bool = False) -> bool:
    """Check if bot is already running."""
    pid_file = GLOBAL_PID_FILE if global_mode else (Path.cwd() / ".ralph" / "slack_bot.pid")

    if not pid_file.exists():
        return False

    try:
        pid = int(pid_file.read_text().strip())
        # Check if process is running
        os.kill(pid, 0)
        return True
    except (ValueError, ProcessLookupError, PermissionError):
        # PID file exists but process is dead - clean up
        pid_file.unlink(missing_ok=True)
        return False


def get_bot_pid(global_mode: bool = False) -> int:
    """Get PID of running bot, or None."""
    pid_file = GLOBAL_PID_FILE if global_mode else (Path.cwd() / ".ralph" / "slack_bot.pid")

    if not pid_file.exists():
        return None

    try:
        pid = int(pid_file.read_text().strip())
        os.kill(pid, 0)  # Check if running
        return pid
    except (ValueError, ProcessLookupError, PermissionError):
        return None


def main():
    parser = argparse.ArgumentParser(description="Ralph Slack Bot - Handle thread replies for plan feedback")
    parser.add_argument("--global", dest="global_mode", action="store_true",
                       help="Run as global bot from ~/.ralph/ (handles multiple repos)")
    parser.add_argument("--status", action="store_true",
                       help="Check if bot is running")

    args = parser.parse_args()

    if args.status:
        if is_bot_running(args.global_mode):
            pid = get_bot_pid(args.global_mode)
            print(f"Bot is running (PID: {pid})")
            sys.exit(0)
        else:
            print("Bot is not running")
            sys.exit(1)

    bot = RalphSlackBot(global_mode=args.global_mode)
    bot.run()


if __name__ == "__main__":
    main()
