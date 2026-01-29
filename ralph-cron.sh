#!/bin/bash
# Ralph Cron Wrapper - Run ralph-worker with flock to prevent overlap
#
# Usage:
#   ./ralph-cron.sh <project-dir> [ralph-worker options]
#
# Crontab (every 5 minutes):
#   */5 * * * * /path/to/ralph-cron.sh /path/to/project --loop >> /tmp/ralph.log 2>&1
#
# tmux loop:
#   while true; do caffeinate -i /path/to/ralph-cron.sh /path/to/project --loop; sleep 300; done
#
# Requirements:
#   - flock (brew install flock on macOS)
#
# Options after project path are passed through to ralph-worker.sh

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
LOCK_FILE="/tmp/ralph-worker.lock"

# First arg is project directory, rest are ralph-worker options
PROJECT_DIR="$1"
shift

if [ -z "$PROJECT_DIR" ]; then
  echo "Usage: $0 <project-dir> [ralph-worker options]"
  echo "Example: $0 /path/to/project --loop"
  exit 1
fi

cd "$PROJECT_DIR" || exit 1

# Pull latest changes (plans, code)
echo "[$(date)] Pulling latest..."
git pull --ff-only 2>/dev/null || git pull --rebase 2>/dev/null || echo "Pull failed (continuing anyway)"

# Use flock to ensure only one instance runs
# -n = non-blocking (exit immediately if lock held)
# --review = run plan reviewer before each plan
# --loop = keep processing until queue empty
exec flock -n "$LOCK_FILE" "$SCRIPT_DIR/ralph-worker.sh" --review "$@"
