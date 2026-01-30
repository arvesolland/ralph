#!/bin/bash
# Ralph Configuration Library
# Shared functions for loading config and building prompts
#
# Environment variables for retry/timeout configuration:
#   RALPH_MAX_RETRIES     - Max retry attempts (default: 5)
#   RALPH_RETRY_DELAY     - Base delay between retries in seconds (default: 5)
#   RALPH_GRACE_PERIOD    - Seconds to wait after completion before killing hung process (default: 5)
#   RALPH_SIMPLE_TIMEOUT  - Timeout for simple verification calls in seconds (default: 60)

# Get Ralph version
get_ralph_version() {
  local script_dir="$1"
  local version_file="$script_dir/VERSION"

  if [ -f "$version_file" ]; then
    cat "$version_file" | tr -d '\n'
  else
    echo "unknown"
  fi
}

# Find project root (git root or current dir)
find_project_root() {
  git rev-parse --show-toplevel 2>/dev/null || pwd
}

# Find Ralph scripts directory
find_ralph_dir() {
  local project_root="$1"

  # Check common locations
  if [ -d "$project_root/scripts/ralph" ]; then
    echo "$project_root/scripts/ralph"
  elif [ -d "$project_root/.ralph/scripts" ]; then
    echo "$project_root/.ralph/scripts"
  else
    echo ""
  fi
}

# Find Ralph config directory
find_config_dir() {
  local project_root="$1"

  if [ -d "$project_root/.ralph" ]; then
    echo "$project_root/.ralph"
  else
    echo ""
  fi
}

# Load a config value from YAML (simple parser, no yq dependency)
# Usage: config_get "project.name" "/path/to/config.yaml"
config_get() {
  local key="$1"
  local config_file="$2"

  if [ ! -f "$config_file" ]; then
    echo ""
    return
  fi

  # Simple YAML parsing for top-level and nested keys
  # Handles: key: value and parent.child: value
  local IFS='.'
  read -ra parts <<< "$key"

  if [ ${#parts[@]} -eq 1 ]; then
    # Top-level key
    grep "^${parts[0]}:" "$config_file" 2>/dev/null | sed 's/^[^:]*: *//' | sed 's/^"//' | sed 's/"$//'
  else
    # Nested key (e.g., project.name)
    # Find the parent section, then the child key
    awk -v parent="${parts[0]}" -v child="${parts[1]}" '
      /^[a-z]/ { section = $0; gsub(/:.*/, "", section) }
      section == parent && /^  / {
        key = $0
        gsub(/^  /, "", key)
        gsub(/:.*/, "", key)
        if (key == child) {
          val = $0
          gsub(/^[^:]*: */, "", val)
          gsub(/^"/, "", val)
          gsub(/"$/, "", val)
          print val
          exit
        }
      }
    ' "$config_file"
  fi
}

# Load file contents or return empty string
load_file_or_empty() {
  local file_path="$1"
  if [ -f "$file_path" ]; then
    cat "$file_path"
  else
    echo ""
  fi
}

# Build prompt by replacing placeholders with config values
# Usage: build_prompt "/path/to/base/prompt.md" "/path/to/.ralph"
build_prompt() {
  local base_prompt="$1"
  local config_dir="$2"
  local config_file="$config_dir/config.yaml"

  if [ ! -f "$base_prompt" ]; then
    echo "Error: Base prompt not found: $base_prompt" >&2
    return 1
  fi

  # Load override files
  local principles=$(load_file_or_empty "$config_dir/principles.md")
  local patterns=$(load_file_or_empty "$config_dir/patterns.md")
  local boundaries=$(load_file_or_empty "$config_dir/boundaries.md")
  local tech_stack=$(load_file_or_empty "$config_dir/tech-stack.md")

  # Load config values
  local project_name=$(config_get "project.name" "$config_file")
  local project_description=$(config_get "project.description" "$config_file")
  local test_command=$(config_get "commands.test" "$config_file")
  local lint_command=$(config_get "commands.lint" "$config_file")
  local build_command=$(config_get "commands.build" "$config_file")
  local dev_command=$(config_get "commands.dev" "$config_file")

  # Default values
  project_name=${project_name:-"Project"}
  test_command=${test_command:-"npm test"}
  lint_command=${lint_command:-"npm run lint"}

  # Read base prompt and replace placeholders
  local prompt_content
  prompt_content=$(cat "$base_prompt")

  # Replace placeholders
  prompt_content="${prompt_content//\{\{PROJECT_NAME\}\}/$project_name}"
  prompt_content="${prompt_content//\{\{PROJECT_DESCRIPTION\}\}/$project_description}"
  prompt_content="${prompt_content//\{\{PRINCIPLES\}\}/$principles}"
  prompt_content="${prompt_content//\{\{PATTERNS\}\}/$patterns}"
  prompt_content="${prompt_content//\{\{BOUNDARIES\}\}/$boundaries}"
  prompt_content="${prompt_content//\{\{TECH_STACK\}\}/$tech_stack}"
  prompt_content="${prompt_content//\{\{TEST_COMMAND\}\}/$test_command}"
  prompt_content="${prompt_content//\{\{LINT_COMMAND\}\}/$lint_command}"
  prompt_content="${prompt_content//\{\{BUILD_COMMAND\}\}/$build_command}"
  prompt_content="${prompt_content//\{\{DEV_COMMAND\}\}/$dev_command}"

  echo "$prompt_content"
}

# Colors for output
setup_colors() {
  if [ -t 1 ]; then
    RED='\033[0;31m'
    GREEN='\033[0;32m'
    YELLOW='\033[1;33m'
    BLUE='\033[0;34m'
    NC='\033[0m'
  else
    RED=''
    GREEN=''
    YELLOW=''
    BLUE=''
    NC=''
  fi
}

# Logging with timestamp
log() {
  echo "[$(date -u +%Y-%m-%dT%H:%M:%SZ)] $1"
}

log_success() {
  setup_colors
  echo -e "${GREEN}$1${NC}"
}

log_error() {
  setup_colors
  echo -e "${RED}$1${NC}" >&2
}

log_warn() {
  setup_colors
  echo -e "${YELLOW}$1${NC}"
}

log_info() {
  setup_colors
  echo -e "${BLUE}$1${NC}"
}

# ============================================
# Slack Notifications
# ============================================
# Send notification to Slack webhook if configured
# Usage: send_slack_notification "event_type" "message" ["config_dir"]
#
# Event types: start, complete, iteration, error, blocker
# Config in .ralph/config.yaml:
#   slack:
#     webhook_url: "https://hooks.slack.com/services/..."
#     notify_start: true      # notify on plan start (default: true)
#     notify_complete: true   # notify on plan completion (default: true)
#     notify_iteration: false # notify on each iteration (default: false)
#     notify_error: true      # notify on errors (default: true)
#     notify_blocker: true    # notify when human input needed (default: true)
#
send_slack_notification() {
  local event_type="$1"
  local message="$2"
  local config_dir="${3:-$CONFIG_DIR}"
  local config_file="$config_dir/config.yaml"

  # Get webhook URL - skip if not configured
  local webhook_url=$(config_get "slack.webhook_url" "$config_file")
  if [ -z "$webhook_url" ]; then
    return 0
  fi

  # Check if this event type should be notified
  local should_notify="true"
  case "$event_type" in
    start)
      should_notify=$(config_get "slack.notify_start" "$config_file")
      should_notify=${should_notify:-true}
      ;;
    complete)
      should_notify=$(config_get "slack.notify_complete" "$config_file")
      should_notify=${should_notify:-true}
      ;;
    iteration)
      should_notify=$(config_get "slack.notify_iteration" "$config_file")
      should_notify=${should_notify:-false}
      ;;
    error)
      should_notify=$(config_get "slack.notify_error" "$config_file")
      should_notify=${should_notify:-true}
      ;;
    blocker)
      should_notify=$(config_get "slack.notify_blocker" "$config_file")
      should_notify=${should_notify:-true}
      ;;
  esac

  if [ "$should_notify" != "true" ]; then
    return 0
  fi

  # Get project name for context
  local project_name=$(config_get "project.name" "$config_file")
  project_name=${project_name:-"Ralph"}

  # Choose emoji based on event type
  local emoji="🤖"
  case "$event_type" in
    start)    emoji="🚀" ;;
    complete) emoji="✅" ;;
    iteration) emoji="🔄" ;;
    error)    emoji="❌" ;;
    blocker)  emoji="🛑" ;;
  esac

  # Build JSON payload
  local payload=$(cat <<EOF
{
  "text": "$emoji *[$project_name]* $message",
  "unfurl_links": false,
  "unfurl_media": false
}
EOF
)

  # Send to Slack (async, don't block on failure)
  curl -s -X POST -H 'Content-type: application/json' \
    --data "$payload" \
    "$webhook_url" >/dev/null 2>&1 &
}

# ============================================
# Slack Bot Notifications with Thread Tracking
# ============================================
# All plan notifications go to a single thread per plan.
# This enables:
# - All updates in one place (start, progress, blockers, completion)
# - Reply to any message to provide feedback
#
# Thread tracking files:
# - .ralph/slack_threads.json - maps plan files to their Slack threads
#

# Send a message to Slack via API (with optional thread)
# Usage: slack_post_message "message" "emoji" ["thread_ts"]
# Returns: message ts on stdout
slack_post_message() {
  local message="$1"
  local emoji="${2:-🤖}"
  local thread_ts="$3"
  local config_dir="${4:-$CONFIG_DIR}"
  local config_file="$config_dir/config.yaml"

  # Load credentials from global file if needed
  load_slack_credentials

  local bot_token="${SLACK_BOT_TOKEN:-}"
  local channel=$(config_get "slack.channel" "$config_file")

  if [ -z "$bot_token" ] || [ -z "$channel" ]; then
    return 1
  fi

  local project_name=$(config_get "project.name" "$config_file")
  project_name=${project_name:-"Ralph"}

  local full_message=$(printf '%s' "$emoji *[$project_name]* $message")

  # Escape for JSON
  full_message=$(echo "$full_message" | sed 's/\\/\\\\/g' | sed 's/"/\\"/g' | sed ':a;N;$!ba;s/\n/\\n/g')

  # Build JSON payload
  local payload="{\"channel\": \"$channel\", \"text\": \"$full_message\", \"unfurl_links\": false, \"unfurl_media\": false"
  if [ -n "$thread_ts" ]; then
    payload="$payload, \"thread_ts\": \"$thread_ts\""
  fi
  payload="$payload}"

  local response=$(curl -s -X POST "https://slack.com/api/chat.postMessage" \
    -H "Authorization: Bearer $bot_token" \
    -H "Content-Type: application/json" \
    -d "$payload" 2>/dev/null)

  if echo "$response" | grep -q '"ok":true'; then
    echo "$response" | grep -o '"ts":"[^"]*"' | head -1 | sed 's/"ts":"//;s/"//'
    return 0
  fi

  return 1
}

# Load Slack credentials from global file if not in environment
# This is called early to ensure tokens are available
load_slack_credentials() {
  if [ -n "$SLACK_BOT_TOKEN" ] && [ -n "$SLACK_APP_TOKEN" ]; then
    return 0  # Already have credentials
  fi

  local global_env="$HOME/.ralph/slack.env"
  if [ -f "$global_env" ]; then
    while IFS='=' read -r key value; do
      [[ "$key" =~ ^#.*$ ]] && continue
      [[ -z "$key" ]] && continue
      value=$(echo "$value" | sed 's/^["'"'"']//;s/["'"'"']$//')
      export "$key=$value"
    done < "$global_env"
  fi
}

# Check if we should use global mode (explicit config or using global credentials)
should_use_global_bot() {
  local config_dir="${1:-$CONFIG_DIR}"
  local config_file="$config_dir/config.yaml"

  # Explicit config takes precedence
  local use_global=$(config_get "slack.global_bot" "$config_file")
  if [ "$use_global" = "true" ]; then
    echo "true"
    return
  fi
  if [ "$use_global" = "false" ]; then
    echo "false"
    return
  fi

  # Default to global if global credentials exist and no local ones
  if [ -f "$HOME/.ralph/slack.env" ]; then
    echo "true"
  else
    echo "false"
  fi
}

# Get the thread tracker file path (global or local based on config)
# Usage: get_thread_tracker_file "config_dir"
get_thread_tracker_file() {
  local config_dir="${1:-$CONFIG_DIR}"

  if [ "$(should_use_global_bot "$config_dir")" = "true" ]; then
    echo "$HOME/.ralph/slack_threads.json"
  else
    echo "$config_dir/slack_threads.json"
  fi
}

# Get or create a Slack thread for a plan
# Usage: get_plan_thread "plan_file" "config_dir"
# Returns: thread_ts on stdout (empty if no thread exists)
get_plan_thread() {
  local plan_file="$1"
  local config_dir="${2:-$CONFIG_DIR}"
  local tracker_file=$(get_thread_tracker_file "$config_dir")

  # Convert to absolute path for global mode
  if [[ "$plan_file" != /* ]]; then
    plan_file="$(cd "$(dirname "$plan_file")" && pwd)/$(basename "$plan_file")"
  fi

  if [ ! -f "$tracker_file" ]; then
    echo ""
    return
  fi

  # Look up thread for this plan
  if command -v jq &> /dev/null; then
    jq -r --arg plan "$plan_file" '.[$plan].thread_ts // empty' "$tracker_file" 2>/dev/null
  else
    grep -o "\"$plan_file\"[^}]*\"thread_ts\":\"[^\"]*\"" "$tracker_file" 2>/dev/null | grep -o '"thread_ts":"[^"]*"' | sed 's/"thread_ts":"//;s/"//'
  fi
}

# Register a plan's Slack thread
# Usage: register_plan_thread "plan_file" "channel" "thread_ts" "config_dir"
register_plan_thread() {
  local plan_file="$1"
  local channel="$2"
  local thread_ts="$3"
  local config_dir="${4:-$CONFIG_DIR}"
  local tracker_file=$(get_thread_tracker_file "$config_dir")

  # Convert to absolute path for global mode
  if [[ "$plan_file" != /* ]]; then
    plan_file="$(cd "$(dirname "$plan_file")" && pwd)/$(basename "$plan_file")"
  fi

  mkdir -p "$(dirname "$tracker_file")"

  local threads="{}"
  if [ -f "$tracker_file" ]; then
    threads=$(cat "$tracker_file" 2>/dev/null || echo "{}")
  fi

  local timestamp=$(date -u +"%Y-%m-%dT%H:%M:%SZ")

  if command -v jq &> /dev/null; then
    echo "$threads" | jq --arg plan "$plan_file" \
      --arg channel "$channel" \
      --arg ts "$thread_ts" \
      --arg created "$timestamp" \
      '. + {($plan): {"channel": $channel, "thread_ts": $ts, "created": $created}}' \
      > "$tracker_file"
  else
    if [ "$threads" = "{}" ]; then
      echo "{\"$plan_file\": {\"channel\": \"$channel\", \"thread_ts\": \"$thread_ts\", \"created\": \"$timestamp\"}}" > "$tracker_file"
    else
      echo "${threads%\}}, \"$plan_file\": {\"channel\": \"$channel\", \"thread_ts\": \"$thread_ts\", \"created\": \"$timestamp\"}}" > "$tracker_file"
    fi
  fi

  log_info "Registered Slack thread $thread_ts for plan: $plan_file" >&2
}

# Send plan start notification and create thread
# Usage: send_plan_start "plan_name" "plan_file" "max_iterations" "config_dir"
# Returns: thread_ts on stdout
send_plan_start_notification() {
  local plan_name="$1"
  local plan_file="$2"
  local max_iterations="$3"
  local config_dir="${4:-$CONFIG_DIR}"
  local config_file="$config_dir/config.yaml"

  local should_notify=$(config_get "slack.notify_start" "$config_file")
  should_notify=${should_notify:-true}
  if [ "$should_notify" != "true" ]; then
    return 0
  fi

  local channel=$(config_get "slack.channel" "$config_file")
  local message="Starting plan: *$plan_name* (max $max_iterations iterations)"

  local thread_ts=$(slack_post_message "$message" "🚀" "" "$config_dir")

  if [ -n "$thread_ts" ] && [ -n "$channel" ]; then
    register_plan_thread "$plan_file" "$channel" "$thread_ts" "$config_dir"
    echo "$thread_ts"
  else
    # Fallback to webhook
    send_slack_notification "start" "Starting plan: *$plan_name* (max $max_iterations iterations)" "$config_dir"
  fi
}

# Send progress update to plan thread
# Usage: send_plan_progress "message" "plan_file" "emoji" "config_dir"
send_plan_progress() {
  local message="$1"
  local plan_file="$2"
  local emoji="${3:-🔄}"
  local config_dir="${4:-$CONFIG_DIR}"

  local thread_ts=$(get_plan_thread "$plan_file" "$config_dir")

  if [ -n "$thread_ts" ]; then
    slack_post_message "$message" "$emoji" "$thread_ts" "$config_dir" >/dev/null
  fi
}

# Send blocker notification to plan thread
# Usage: send_blocker_notification "message" "plan_file" "config_dir"
send_blocker_notification() {
  local message="$1"
  local plan_file="$2"
  local config_dir="${3:-$CONFIG_DIR}"
  local config_file="$config_dir/config.yaml"

  local should_notify=$(config_get "slack.notify_blocker" "$config_file")
  should_notify=${should_notify:-true}
  if [ "$should_notify" != "true" ]; then
    return 0
  fi

  local thread_ts=$(get_plan_thread "$plan_file" "$config_dir")

  if [ -n "$thread_ts" ]; then
    # Post to existing plan thread
    slack_post_message "$message\n\n_Reply to this thread to provide feedback._" "🛑" "$thread_ts" "$config_dir" >/dev/null
    echo "$thread_ts"
  else
    # No plan thread - create one for this blocker
    local project_name=$(config_get "project.name" "$config_file")
    project_name=${project_name:-"Ralph"}
    local channel=$(config_get "slack.channel" "$config_file")

    local new_ts=$(slack_post_message "$message\n\n_Reply to this thread to provide feedback._" "🛑" "" "$config_dir")

    if [ -n "$new_ts" ] && [ -n "$channel" ]; then
      register_plan_thread "$plan_file" "$channel" "$new_ts" "$config_dir"
      echo "$new_ts"
    else
      # Fallback to webhook
      send_slack_notification "blocker" "$message" "$config_dir"
    fi
  fi
}

# Send completion notification to plan thread
# Usage: send_plan_complete "plan_name" "plan_file" "iterations" "config_dir"
send_plan_complete_notification() {
  local plan_name="$1"
  local plan_file="$2"
  local iterations="$3"
  local config_dir="${4:-$CONFIG_DIR}"
  local config_file="$config_dir/config.yaml"

  local should_notify=$(config_get "slack.notify_complete" "$config_file")
  should_notify=${should_notify:-true}
  if [ "$should_notify" != "true" ]; then
    return 0
  fi

  local thread_ts=$(get_plan_thread "$plan_file" "$config_dir")
  local message="Plan *$plan_name* completed successfully after $iterations iterations!"

  if [ -n "$thread_ts" ]; then
    slack_post_message "$message" "✅" "$thread_ts" "$config_dir" >/dev/null
  else
    send_slack_notification "complete" "$message" "$config_dir"
  fi
}

# Send error notification to plan thread
# Usage: send_plan_error "message" "plan_file" "config_dir"
send_plan_error_notification() {
  local message="$1"
  local plan_file="$2"
  local config_dir="${3:-$CONFIG_DIR}"
  local config_file="$config_dir/config.yaml"

  local should_notify=$(config_get "slack.notify_error" "$config_file")
  should_notify=${should_notify:-true}
  if [ "$should_notify" != "true" ]; then
    return 0
  fi

  local thread_ts=$(get_plan_thread "$plan_file" "$config_dir")

  if [ -n "$thread_ts" ]; then
    slack_post_message "$message" "❌" "$thread_ts" "$config_dir" >/dev/null
  else
    send_slack_notification "error" "$message" "$config_dir"
  fi
}

# ============================================
# Blocker Detection
# ============================================
# Extract blocker information from Claude output
# Blockers use format: <blocker>description</blocker>
# Returns: blocker content or empty string
#
extract_blocker() {
  local output="$1"

  # Extract content between <blocker> tags (handles multiline)
  echo "$output" | grep -ozP '<blocker>[\s\S]*?</blocker>' 2>/dev/null \
    | sed 's/<blocker>//g' | sed 's/<\/blocker>//g' | tr '\0' '\n' | head -1
}

# Get short hash of blocker content (for deduplication)
blocker_hash() {
  local content="$1"
  echo "$content" | md5sum 2>/dev/null | cut -c1-8 || echo "$content" | shasum | cut -c1-8
}

# Check if blocker has already been notified (stored in slack_threads.json)
# Returns: 0 if already notified, 1 if new
blocker_already_notified() {
  local blocker_hash="$1"
  local plan_file="$2"
  local config_dir="${3:-$CONFIG_DIR}"
  local tracker_file="$config_dir/slack_threads.json"

  if [ ! -f "$tracker_file" ]; then
    return 1  # New blocker
  fi

  # Check if this blocker hash is in the notified_blockers array for this plan
  if command -v jq &> /dev/null; then
    if jq -e --arg plan "$plan_file" --arg hash "$blocker_hash" \
       '.[$plan].notified_blockers // [] | index($hash) != null' "$tracker_file" >/dev/null 2>&1; then
      return 0  # Already notified
    fi
  else
    if grep -q "\"$blocker_hash\"" "$tracker_file" 2>/dev/null; then
      return 0  # Simple check
    fi
  fi

  return 1  # New blocker
}

# Mark blocker as notified (stored in slack_threads.json)
mark_blocker_notified() {
  local blocker_hash="$1"
  local plan_file="$2"
  local config_dir="${3:-$CONFIG_DIR}"
  local tracker_file="$config_dir/slack_threads.json"

  if [ ! -f "$tracker_file" ]; then
    return 0
  fi

  if command -v jq &> /dev/null; then
    local tmp_file=$(mktemp)
    jq --arg plan "$plan_file" --arg hash "$blocker_hash" \
       'if .[$plan] then .[$plan].notified_blockers = ((.[$plan].notified_blockers // []) + [$hash] | unique) else . end' \
       "$tracker_file" > "$tmp_file" && mv "$tmp_file" "$tracker_file"
  fi
  # Without jq, we just skip tracking (may cause duplicate notifications)
}

# Run claude with retry logic for transient errors
# Usage: echo "$PROMPT" | run_claude_with_retry [claude_args...]
# Returns: Claude output on success, exits with error after max retries
#
# WORKAROUND: Uses stream-json output with jq filtering for real-time streaming
# and timeout-based hang detection (GitHub Issue #19060).
# Credit: Matt Pollock for the jq streaming approach.
run_claude_with_retry() {
  local max_retries=${RALPH_MAX_RETRIES:-5}
  local base_delay=${RALPH_RETRY_DELAY:-5}
  local grace_period=${RALPH_GRACE_PERIOD:-5}
  local attempt=1
  local prompt

  # Read prompt from stdin
  prompt=$(cat)

  # jq filter to extract streaming text from assistant messages (credit: Matt Pollock)
  # - Selects assistant messages and extracts text content
  # - Fixes line endings for proper terminal display
  local stream_text='select(.type == "assistant").message.content[]? | select(.type == "text").text // empty'

  # jq filter to extract final result
  local final_result='select(.type == "result").result // empty'

  # Check if jq is available for streaming output
  local has_jq=false
  if command -v jq &> /dev/null; then
    has_jq=true
  fi

  while [ $attempt -le $max_retries ]; do
    local temp_output=$(mktemp)
    local temp_prompt=$(mktemp)
    local stream_pid=""
    local result_received=false
    local has_error=false
    local error_msg=""

    # Save prompt to file for the background process
    echo "$prompt" > "$temp_prompt"

    if [ "$has_jq" = true ]; then
      # Stream with jq for real-time readable output
      # Pipeline: claude -> tee (capture ALL for error detection) -> grep (filter JSON) -> jq (display)
      # Note: --verbose is required when using --output-format stream-json with -p
      # IMPORTANT: tee must come BEFORE grep so error messages are captured for retry detection
      claude "$@" --verbose --output-format stream-json < "$temp_prompt" 2>&1 \
        | tee "$temp_output" \
        | grep --line-buffered '^{' \
        | jq --unbuffered -rj "$stream_text" >&2 &
      stream_pid=$!

      # Monitor for completion and handle hanging
      (
        local result_found=false
        while kill -0 $stream_pid 2>/dev/null; do
          sleep 1
          if [ -f "$temp_output" ] && grep -q '"type":"result"' "$temp_output" 2>/dev/null; then
            if [ "$result_found" = false ]; then
              result_found=true
              sleep $grace_period
              # If pipeline still running after grace period, kill it
              if kill -0 $stream_pid 2>/dev/null; then
                # Kill the whole pipeline process group
                kill $stream_pid 2>/dev/null || true
                sleep 1
                kill -9 $stream_pid 2>/dev/null || true
              fi
              break
            fi
          fi
        done
      ) &
      local monitor_pid=$!

      # Wait for streaming pipeline to finish
      wait $stream_pid 2>/dev/null || true

      # Stop the monitor
      kill $monitor_pid 2>/dev/null || true
      wait $monitor_pid 2>/dev/null || true

      echo "" >&2  # Newline after streaming output
    else
      # Fallback: no jq, just capture output with timeout monitoring
      # Note: --verbose is required when using --output-format stream-json with -p
      claude "$@" --verbose --output-format stream-json < "$temp_prompt" > "$temp_output" 2>&1 &
      local claude_pid=$!

      # Monitor for completion and handle hanging
      (
        local result_found=false
        while kill -0 $claude_pid 2>/dev/null; do
          sleep 1
          if [ -f "$temp_output" ] && grep -q '"type":"result"' "$temp_output" 2>/dev/null; then
            if [ "$result_found" = false ]; then
              result_found=true
              sleep $grace_period
              if kill -0 $claude_pid 2>/dev/null; then
                log_warn "Claude CLI hanging after completion - terminating" >&2
                kill $claude_pid 2>/dev/null || true
                sleep 1
                kill -9 $claude_pid 2>/dev/null || true
              fi
              break
            fi
          fi
        done
      ) &
      local monitor_pid=$!

      wait $claude_pid 2>/dev/null || true
      kill $monitor_pid 2>/dev/null || true
      wait $monitor_pid 2>/dev/null || true
    fi

    # Read final output
    local output=$(cat "$temp_output" 2>/dev/null || true)

    # Check if result was received (successful completion)
    if grep -q '"type":"result"' "$temp_output" 2>/dev/null; then
      result_received=true
    fi

    # Check for known transient errors (only in non-JSON lines - actual CLI errors are plain text)
    # This avoids false positives when the agent's response contains these strings
    if grep -v '^{' "$temp_output" 2>/dev/null | grep -qE "No messages returned|promise rejected|ECONNRESET|ETIMEDOUT|rate limit|overloaded"; then
      has_error=true
      error_msg=$(grep -v '^{' "$temp_output" 2>/dev/null | grep -oE "No messages returned|promise rejected|ECONNRESET|ETIMEDOUT|rate limit|overloaded" | head -1)
    fi

    # Cleanup temp files
    rm -f "$temp_output" "$temp_prompt"

    # Check if we should retry (only check non-JSON lines for errors to avoid false positives from agent response)
    if [ "$has_error" = true ] || { [ "$result_received" = false ] && [ -n "$output" ] && echo "$output" | grep -v '^{' | grep -qE "error|Error|ERROR"; }; then
      if [ $attempt -lt $max_retries ]; then
        local delay=$((base_delay * attempt))
        log_warn "Claude CLI error (attempt $attempt/$max_retries): $error_msg" >&2
        log_warn "Retrying in ${delay}s..." >&2
        sleep $delay
        attempt=$((attempt + 1))
        continue
      else
        log_error "Claude CLI failed after $max_retries attempts" >&2
        echo "$output"
        return 1
      fi
    fi

    # Success - output the full response (stream-json format)
    echo "$output"
    return 0
  done
}

# Run claude with retry (simple version for verification calls)
# Usage: run_claude_simple_with_retry "prompt" [claude_args...]
# Uses timeout to handle Claude Code CLI hanging bug (GitHub Issue #19060)
run_claude_simple_with_retry() {
  local prompt="$1"
  shift
  local max_retries=${RALPH_MAX_RETRIES:-5}
  local base_delay=${RALPH_RETRY_DELAY:-5}
  local timeout_secs=${RALPH_SIMPLE_TIMEOUT:-60}
  local attempt=1

  while [ $attempt -le $max_retries ]; do
    local temp_output=$(mktemp)
    local claude_pid
    local timed_out=false

    # Run claude in background
    echo "$prompt" | claude "$@" > "$temp_output" 2>&1 &
    claude_pid=$!

    # Wait with timeout
    local waited=0
    while kill -0 $claude_pid 2>/dev/null && [ $waited -lt $timeout_secs ]; do
      sleep 1
      waited=$((waited + 1))
    done

    # Check if still running (timed out)
    if kill -0 $claude_pid 2>/dev/null; then
      timed_out=true
      log_warn "Claude verification timed out after ${timeout_secs}s - terminating" >&2
      kill $claude_pid 2>/dev/null || true
      sleep 1
      kill -9 $claude_pid 2>/dev/null || true
    fi

    wait $claude_pid 2>/dev/null || true
    local exit_code=$?
    local output=$(cat "$temp_output" 2>/dev/null || true)
    rm -f "$temp_output"

    # Check for errors (empty output, non-zero exit, or timeout)
    if [ -z "$output" ] || [ $exit_code -ne 0 ] || [ "$timed_out" = true ]; then
      if [ $attempt -lt $max_retries ]; then
        local delay=$((base_delay * attempt))
        log_warn "Claude verification error (attempt $attempt/$max_retries). Retrying in ${delay}s..." >&2
        sleep $delay
        attempt=$((attempt + 1))
        continue
      else
        log_error "Claude verification failed after $max_retries attempts" >&2
        return 1
      fi
    fi

    echo "$output"
    return 0
  done
}

# Check required dependencies
check_dependencies() {
  local missing=()

  if ! command -v claude &> /dev/null; then
    missing+=("claude (Claude Code CLI)")
  fi

  if ! command -v git &> /dev/null; then
    missing+=("git")
  fi

  if [ ${#missing[@]} -gt 0 ]; then
    log_error "Missing required dependencies:"
    for dep in "${missing[@]}"; do
      echo "  - $dep"
    done
    return 1
  fi

  return 0
}

# Check optional dependencies and warn
check_optional_dependencies() {
  local warnings=()

  if ! command -v gh &> /dev/null; then
    warnings+=("gh (GitHub CLI) - useful for PR creation")
  fi

  if [ ${#warnings[@]} -gt 0 ]; then
    log_warn "Optional dependencies not found:"
    for warn in "${warnings[@]}"; do
      echo "  - $warn"
    done
  fi
}

# ============================================
# Worktree Management
# ============================================
# Source worktree library if available
SCRIPT_DIR_CONFIG="${BASH_SOURCE[0]%/*}"
if [[ -f "$SCRIPT_DIR_CONFIG/worktree.sh" ]]; then
  source "$SCRIPT_DIR_CONFIG/worktree.sh"
fi
