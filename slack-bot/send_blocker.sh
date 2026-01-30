#!/bin/bash
# Send a blocker notification via Slack API and register thread for reply tracking
#
# Usage: send_blocker.sh --channel CHANNEL_ID --plan-file PATH --message "text"
#
# Environment:
#   SLACK_BOT_TOKEN - Bot token (xoxb-...)
#
# This uses the Slack API (not webhook) so we can get the thread_ts for reply tracking.

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Parse arguments
CHANNEL=""
PLAN_FILE=""
MESSAGE=""
PROJECT_ROOT=""

while [[ $# -gt 0 ]]; do
  case $1 in
    --channel)
      CHANNEL="$2"
      shift 2
      ;;
    --plan-file)
      PLAN_FILE="$2"
      shift 2
      ;;
    --message)
      MESSAGE="$2"
      shift 2
      ;;
    --project-root)
      PROJECT_ROOT="$2"
      shift 2
      ;;
    *)
      echo "Unknown option: $1" >&2
      exit 1
      ;;
  esac
done

# Validate
if [[ -z "$CHANNEL" ]] || [[ -z "$PLAN_FILE" ]] || [[ -z "$MESSAGE" ]]; then
  echo "Usage: send_blocker.sh --channel CHANNEL_ID --plan-file PATH --message TEXT" >&2
  exit 1
fi

if [[ -z "$SLACK_BOT_TOKEN" ]]; then
  echo "Error: SLACK_BOT_TOKEN not set" >&2
  exit 1
fi

PROJECT_ROOT="${PROJECT_ROOT:-.}"

# Send message via Slack API
RESPONSE=$(curl -s -X POST "https://slack.com/api/chat.postMessage" \
  -H "Authorization: Bearer $SLACK_BOT_TOKEN" \
  -H "Content-Type: application/json" \
  -d @- <<EOF
{
  "channel": "$CHANNEL",
  "text": "$MESSAGE",
  "unfurl_links": false,
  "unfurl_media": false
}
EOF
)

# Check for success
OK=$(echo "$RESPONSE" | grep -o '"ok":true' || true)
if [[ -z "$OK" ]]; then
  ERROR=$(echo "$RESPONSE" | grep -o '"error":"[^"]*"' | head -1)
  echo "Slack API error: $ERROR" >&2
  exit 1
fi

# Extract thread_ts from response
THREAD_TS=$(echo "$RESPONSE" | grep -o '"ts":"[^"]*"' | head -1 | sed 's/"ts":"//;s/"//')

if [[ -z "$THREAD_TS" ]]; then
  echo "Warning: Could not extract thread_ts from response" >&2
  exit 0
fi

echo "Message sent, thread_ts: $THREAD_TS" >&2

# Register thread for tracking
TRACKER_FILE="$PROJECT_ROOT/.ralph/slack_threads.json"
mkdir -p "$(dirname "$TRACKER_FILE")"

# Create or update tracker file
if [[ -f "$TRACKER_FILE" ]]; then
  THREADS=$(cat "$TRACKER_FILE")
else
  THREADS="{}"
fi

# Add new thread (using simple sed/awk since jq may not be available)
THREAD_KEY="$CHANNEL:$THREAD_TS"
TIMESTAMP=$(date -u +"%Y-%m-%dT%H:%M:%SZ")

# If we have jq, use it for cleaner JSON handling
if command -v jq &> /dev/null; then
  echo "$THREADS" | jq --arg key "$THREAD_KEY" \
    --arg plan "$PLAN_FILE" \
    --arg channel "$CHANNEL" \
    --arg ts "$THREAD_TS" \
    --arg created "$TIMESTAMP" \
    '. + {($key): {"plan_file": $plan, "channel": $channel, "thread_ts": $ts, "created": $created}}' \
    > "$TRACKER_FILE"
else
  # Fallback: simple append (may create duplicate keys, but works)
  if [[ "$THREADS" == "{}" ]]; then
    echo "{\"$THREAD_KEY\": {\"plan_file\": \"$PLAN_FILE\", \"channel\": \"$CHANNEL\", \"thread_ts\": \"$THREAD_TS\", \"created\": \"$TIMESTAMP\"}}" > "$TRACKER_FILE"
  else
    # Remove trailing } and add new entry
    echo "${THREADS%\}}, \"$THREAD_KEY\": {\"plan_file\": \"$PLAN_FILE\", \"channel\": \"$CHANNEL\", \"thread_ts\": \"$THREAD_TS\", \"created\": \"$TIMESTAMP\"}}" > "$TRACKER_FILE"
  fi
fi

echo "Thread registered for reply tracking" >&2
echo "$THREAD_TS"
