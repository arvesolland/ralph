#!/bin/bash
set -e

# Ralph Worker - File-based task queue
# Works through plans in a structured folder system
#
# Folder structure:
#   plans/
#   ├── pending/      # Plans waiting to be processed (FIFO - oldest first)
#   ├── current/      # Plan currently being worked on (0 or 1 file)
#   └── complete/     # Finished plans with their progress logs
#
# Usage:
#   ./ralph-worker.sh              # Process current or next pending plan
#   ./ralph-worker.sh --status     # Show queue status
#   ./ralph-worker.sh --add file   # Add a plan to pending queue
#   ./ralph-worker.sh --loop       # Keep processing until queue empty

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Load shared config library
if [ -f "$SCRIPT_DIR/lib/config.sh" ]; then
  source "$SCRIPT_DIR/lib/config.sh"
else
  RED='\033[0;31m'
  GREEN='\033[0;32m'
  YELLOW='\033[1;33m'
  BLUE='\033[0;34m'
  NC='\033[0m'
fi

PROJECT_ROOT=$(find_project_root 2>/dev/null || git rev-parse --show-toplevel 2>/dev/null || pwd)
CONFIG_DIR="$PROJECT_ROOT/.ralph"
PLANS_DIR="$PROJECT_ROOT/plans"
PENDING_DIR="$PLANS_DIR/pending"
CURRENT_DIR="$PLANS_DIR/current"
COMPLETED_DIR="$PLANS_DIR/complete"

# Parse arguments
ACTION="work"
LOOP_MODE=false
ADD_FILE=""
MAX_ITERATIONS=50
CREATE_PR=false
MERGE_DIRECT=false
REVIEW_PLAN=false
DELETE_BRANCH=true

# Get default completion mode from config
DEFAULT_COMPLETION_MODE=$(config_get "completion.mode" "$CONFIG_DIR/config.yaml" 2>/dev/null || echo "pr")

while [[ $# -gt 0 ]]; do
  case $1 in
    --status|-s)
      ACTION="status"
      shift
      ;;
    --add|-a)
      ACTION="add"
      ADD_FILE="$2"
      shift 2
      ;;
    --loop|-l)
      LOOP_MODE=true
      shift
      ;;
    --max|-m)
      MAX_ITERATIONS="$2"
      shift 2
      ;;
    --complete|-c)
      ACTION="complete"
      shift
      ;;
    --next|-n)
      ACTION="next"
      shift
      ;;
    --create-pr|--pr)
      CREATE_PR=true
      MERGE_DIRECT=false
      shift
      ;;
    --merge)
      MERGE_DIRECT=true
      CREATE_PR=false
      shift
      ;;
    --no-delete-branch)
      DELETE_BRANCH=false
      shift
      ;;
    --cleanup)
      ACTION="cleanup"
      shift
      ;;
    --reset)
      ACTION="reset"
      shift
      ;;
    --review|-r)
      REVIEW_PLAN=true
      shift
      ;;
    --version|-v)
      echo "Ralph Worker v$(get_ralph_version "$SCRIPT_DIR" 2>/dev/null || echo "unknown")"
      exit 0
      ;;
    --help|-h)
      echo "Ralph Worker - File-based task queue with worktree isolation"
      echo ""
      echo "Usage:"
      echo "  ./ralph-worker.sh              Process current or next plan"
      echo "  ./ralph-worker.sh --status     Show queue status"
      echo "  ./ralph-worker.sh --add FILE   Add plan to pending queue"
      echo "  ./ralph-worker.sh --complete   Mark current plan complete, activate next"
      echo "  ./ralph-worker.sh --next       Activate next pending plan"
      echo "  ./ralph-worker.sh --loop       Process until queue empty"
      echo "  ./ralph-worker.sh --cleanup    Remove orphaned worktrees"
      echo "  ./ralph-worker.sh --reset      Move current plan back to pending"
      echo ""
      echo "Options:"
      echo "  --status, -s       Show queue status"
      echo "  --add, -a FILE     Add a plan file to pending queue"
      echo "  --complete, -c     Complete current plan and activate next"
      echo "  --next, -n         Activate next pending plan"
      echo "  --loop, -l         Keep processing until no more plans"
      echo "  --max, -m N        Max iterations per plan (default: 50)"
      echo "  --review, -r       Run plan reviewer before starting each plan"
      echo "  --pr               Create PR after completion (default)"
      echo "  --merge            Direct merge to base branch (skip PR)"
      echo "  --no-delete-branch Keep feature branch after merge"
      echo "  --cleanup          Remove orphaned worktrees"
      echo "  --reset            Move current plan back to pending (keeps worktree)"
      echo "  --version, -v      Show version"
      echo "  --help, -h         Show this help"
      echo ""
      echo "Completion modes (set via flag or completion.mode in config.yaml):"
      echo "  pr (default)       Push branch and create PR via gh"
      echo "  merge              Merge directly to base branch"
      echo ""
      echo "Folder structure:"
      echo "  plans/pending/    Plans waiting to be processed"
      echo "  plans/current/    Currently active plan (0-1 files)"
      echo "  plans/complete/   Finished plans with logs"
      echo ""
      echo "Worktree isolation:"
      echo "  Each plan executes in its own git worktree at .ralph/worktrees/"
      echo "  This prevents branch switching conflicts in the main worktree."
      exit 0
      ;;
    *)
      echo "Unknown option: $1"
      exit 1
      ;;
  esac
done

# Apply default completion mode if no flag specified
if [[ "$CREATE_PR" == "false" ]] && [[ "$MERGE_DIRECT" == "false" ]]; then
  if [[ "$DEFAULT_COMPLETION_MODE" == "merge" ]]; then
    MERGE_DIRECT=true
  else
    CREATE_PR=true
  fi
fi

# Ensure directories exist
ensure_dirs() {
  mkdir -p "$PENDING_DIR" "$CURRENT_DIR" "$COMPLETED_DIR"
}

# Check if file is a plan (not a progress file or other auxiliary file)
is_plan_file() {
  local file="$1"
  local basename=$(basename "$file")

  # Exclude progress files and hidden files
  # Note: reverse-discovery.md IS a valid plan file (output from ralph-reverse discover)
  if [[ "$basename" == *.progress.md ]] || \
     [[ "$basename" == .* ]]; then
    return 1
  fi
  return 0
}

# Get count of plan files in a directory (excludes progress files)
count_files() {
  local dir="$1"
  local count=0
  shopt -s nullglob
  for f in "$dir"/*.md; do
    [ -f "$f" ] && is_plan_file "$f" && count=$((count + 1))
  done
  shopt -u nullglob
  echo "$count"
}

# Get oldest plan file in a directory (FIFO queue order, excludes progress files)
get_oldest_file() {
  local dir="$1"
  # ls -tr sorts by time, oldest first
  for f in $(ls -tr "$dir"/*.md 2>/dev/null); do
    if [ -f "$f" ] && is_plan_file "$f"; then
      echo "$f"
      return
    fi
  done
}

# Get the current plan file (if any, excludes progress files)
get_current_plan() {
  shopt -s nullglob
  for f in "$CURRENT_DIR"/*.md; do
    if [ -f "$f" ] && is_plan_file "$f"; then
      shopt -u nullglob
      echo "$f"
      return
    fi
  done
  shopt -u nullglob
}

# Check if a plan has incomplete tasks (used for status display only)
has_incomplete_tasks() {
  local plan_file="$1"
  # Look for unchecked markdown checkboxes - best effort for status
  grep -q '^\s*-\s*\[ \]' "$plan_file" 2>/dev/null
}

# Move plan to completed with its progress and feedback files
complete_plan() {
  local plan_file="$1"
  local plan_name=$(basename "$plan_file" .md)
  local plan_dir=$(dirname "$plan_file")
  local progress_file="$plan_dir/${plan_name}.progress.md"
  local feedback_file="$plan_dir/${plan_name}.feedback.md"
  local timestamp=$(date +%Y%m%d-%H%M%S)
  local completed_subdir="$COMPLETED_DIR/${timestamp}-${plan_name}"

  mkdir -p "$completed_subdir"

  # Move the plan
  mv "$plan_file" "$completed_subdir/plan.md"

  # Move the progress file if it exists (plan-specific learnings)
  if [ -f "$progress_file" ]; then
    mv "$progress_file" "$completed_subdir/progress.md"
  fi

  # Move the feedback file if it exists (human input from Slack)
  if [ -f "$feedback_file" ]; then
    mv "$feedback_file" "$completed_subdir/feedback.md"
    echo "  Archived feedback file" >&2
  fi

  # Clean up Slack thread tracking for this plan
  local use_global=$(config_get "slack.global_bot" "$CONFIG_DIR/config.yaml" 2>/dev/null)
  local thread_tracker="$CONFIG_DIR/slack_threads.json"
  local plan_key="$plan_file"

  if [ "$use_global" = "true" ]; then
    thread_tracker="$HOME/.ralph/slack_threads.json"
    # Global mode uses absolute paths
    plan_key="$(cd "$plan_dir" && pwd)/${plan_name}.md"
  fi

  if [ -f "$thread_tracker" ] && command -v jq &> /dev/null; then
    local tmp_file=$(mktemp)
    jq --arg plan "$plan_key" 'del(.[$plan])' "$thread_tracker" > "$tmp_file" && mv "$tmp_file" "$thread_tracker"
    echo "  Cleaned up Slack thread tracking" >&2
  fi

  # If this is a reverse-specs plan, also archive the discovery document
  if [[ "$plan_name" == *"reverse-specs"* ]]; then
    local discovery_file="$plan_dir/reverse-discovery.md"
    local discovery_progress="$plan_dir/reverse-discovery.progress.md"

    if [ -f "$discovery_file" ]; then
      mv "$discovery_file" "$completed_subdir/discovery.md"
      echo "  Archived discovery document" >&2
    fi

    if [ -f "$discovery_progress" ]; then
      mv "$discovery_progress" "$completed_subdir/discovery-progress.md"
    fi
  fi

  echo "$completed_subdir"
}

# Get feature branch name from plan name
get_feature_branch() {
  local plan_file="$1"
  local plan_name=$(basename "$plan_file" .md)
  # Remove timestamp prefix if present (e.g., 20240127-143052-auth -> auth)
  plan_name=$(echo "$plan_name" | sed 's/^[0-9]\{8\}-[0-9]\{6\}-//')
  echo "feat/$plan_name"
}

# Get plan name from plan file (for worktree functions)
get_plan_name() {
  local plan_file="$1"
  basename "$plan_file" .md
}

# DEPRECATED: Old branch-switching approach - kept for reference
# setup_feature_branch() { ... }
# Now replaced by worktree-based isolation. See create_plan_worktree() in lib/worktree.sh

# Setup worktree for plan execution (replaces setup_feature_branch)
# Creates isolated worktree, copies plan file into it
# Returns: worktree path on stdout
setup_plan_worktree() {
  local plan_file="$1"
  local plan_name=$(get_plan_name "$plan_file")
  local base_branch=$(config_get "git.base_branch" "$CONFIG_DIR/config.yaml" 2>/dev/null || echo "main")

  # Check if already locked (prevents double-execution)
  if is_plan_locked "$plan_name"; then
    local existing_path=$(get_worktree_path "$plan_name")
    log_warn "Plan already has active worktree: $existing_path" >&2
    echo "$existing_path"
    return 0
  fi

  # Create worktree (this also creates branch if needed)
  local worktree_path
  worktree_path=$(create_plan_worktree "$plan_name" "$base_branch")

  if [[ -z "$worktree_path" ]] || [[ ! -d "$worktree_path" ]]; then
    log_error "Failed to create worktree for plan: $plan_name" >&2
    return 1
  fi

  # Copy plan file into worktree (keeps original in queue for state tracking)
  cp "$plan_file" "$worktree_path/plan.md"

  # Also copy progress file if it exists
  local progress_file="${plan_file%.md}.progress.md"
  if [[ -f "$progress_file" ]]; then
    cp "$progress_file" "$worktree_path/plan.progress.md"
  fi

  # Initialize worktree (copy .env, install dependencies, run hooks)
  init_worktree "$worktree_path" "$PROJECT_ROOT"

  log_info "Worktree ready: $worktree_path" >&2
  log_info "Plan copied to: $worktree_path/plan.md" >&2

  echo "$worktree_path"
}

# Check for and warn about orphaned files (progress files in wrong places)
check_orphaned_files() {
  local found_orphans=false

  shopt -s nullglob

  # Check for progress files in pending (should never happen)
  for f in "$PENDING_DIR"/*.progress.md; do
    if [ -f "$f" ]; then
      echo -e "${YELLOW}Warning: Found orphaned progress file in pending: $(basename "$f")${NC}" >&2
      echo "  Moving to current directory..." >&2
      mv "$f" "$CURRENT_DIR/" 2>/dev/null || true
      found_orphans=true
    fi
  done

  shopt -u nullglob

  if [ "$found_orphans" = true ]; then
    echo "" >&2
  fi
}

# Move next pending plan to current and setup worktree
# Returns: plan file path in current/ on stdout
activate_next_plan() {
  # First check for orphaned files
  check_orphaned_files

  local next_plan=$(get_oldest_file "$PENDING_DIR")
  if [ -n "$next_plan" ] && [ -f "$next_plan" ]; then
    local plan_name=$(get_plan_name "$next_plan")

    # Check if plan is already locked (worktree exists)
    if is_plan_locked "$plan_name"; then
      log_error "Plan '$plan_name' is already being executed (worktree exists)" >&2
      log_error "Use --cleanup to remove orphaned worktrees, or wait for execution to complete" >&2
      return 1
    fi

    local dest="$CURRENT_DIR/$(basename "$next_plan")"
    mv "$next_plan" "$dest"

    # Commit the plan activation on base branch (keeps queue state tracked)
    git add "$PENDING_DIR" "$CURRENT_DIR" 2>/dev/null || true
    git commit -m "chore: activate plan $plan_name" --allow-empty 2>/dev/null || true

    # Setup worktree for execution (no branch switch in main worktree!)
    local worktree_path
    worktree_path=$(setup_plan_worktree "$dest")

    if [[ -z "$worktree_path" ]]; then
      log_error "Failed to setup worktree, moving plan back to pending" >&2
      mv "$dest" "$next_plan"
      return 1
    fi

    echo "$dest"
  fi
}

# Show queue status
show_status() {
  ensure_dirs

  echo -e "${GREEN}========================================"
  echo -e "Ralph Worker Queue Status"
  echo -e "========================================${NC}"
  echo ""

  local pending_count=$(count_files "$PENDING_DIR")
  local current_plan=$(get_current_plan)
  local completed_count=$(ls -d "$COMPLETED_DIR"/*/ 2>/dev/null | wc -l | tr -d ' ')

  echo -e "${BLUE}Pending:${NC} $pending_count plan(s)"
  if [ "$pending_count" -gt 0 ]; then
    shopt -s nullglob
    for f in "$PENDING_DIR"/*.md; do
      [ -f "$f" ] && is_plan_file "$f" && echo "  - $(basename "$f")"
    done
    shopt -u nullglob
  fi
  echo ""

  echo -e "${BLUE}Current:${NC}"
  if [ -n "$current_plan" ]; then
    local plan_name=$(get_plan_name "$current_plan")
    local task_count=$(grep -c '^\s*-\s*\[ \]' "$current_plan" 2>/dev/null || echo "0")
    local done_count=$(grep -c '^\s*-\s*\[x\]' "$current_plan" 2>/dev/null || echo "0")
    echo "  - $(basename "$current_plan") ($done_count done, $task_count remaining)"

    # Show worktree status
    local worktree_path=$(get_worktree_path "$plan_name")
    if [[ -d "$worktree_path" ]]; then
      local wt_branch=$(get_worktree_branch "$worktree_path")
      echo -e "    ${BLUE}Worktree:${NC} $worktree_path"
      echo -e "    ${BLUE}Branch:${NC} $wt_branch"
    else
      echo -e "    ${YELLOW}Worktree:${NC} not created yet"
    fi
  else
    echo "  (none)"
  fi
  echo ""

  # Show active worktrees
  local worktrees=$(list_plan_worktrees 2>/dev/null)
  if [[ -n "$worktrees" ]]; then
    local wt_count=$(echo "$worktrees" | wc -l | tr -d ' ')
    echo -e "${BLUE}Active Worktrees:${NC} $wt_count"
    for wt in $worktrees; do
      local branch=$(get_worktree_branch "$wt")
      echo "  - $(basename "$wt") ($branch)"
    done
    echo ""
  fi

  echo -e "${BLUE}Completed:${NC} $completed_count plan(s)"
  if [ "$completed_count" -gt 0 ]; then
    ls -d "$COMPLETED_DIR"/*/ 2>/dev/null | tail -5 | while read dir; do
      echo "  - $(basename "$dir")"
    done
    [ "$completed_count" -gt 5 ] && echo "  ... and $((completed_count - 5)) more"
  fi
}

# Add a plan to the queue
add_plan() {
  local file="$1"

  if [ ! -f "$file" ]; then
    echo -e "${RED}Error: File not found: $file${NC}"
    exit 1
  fi

  # Validate it's a plan file, not a progress file
  if ! is_plan_file "$file"; then
    echo -e "${RED}Error: Cannot add '$file' - this appears to be a progress file or auxiliary file, not a plan${NC}"
    exit 1
  fi

  ensure_dirs

  # Add timestamp prefix for ordering
  local timestamp=$(date +%Y%m%d-%H%M%S)
  local basename=$(basename "$file")
  local dest="$PENDING_DIR/${timestamp}-${basename}"

  cp "$file" "$dest"
  echo -e "${GREEN}Added to queue:${NC} $dest"

  show_status
}

# Process a single plan (runs ralph.sh inside the plan's worktree)
process_plan() {
  local plan_file="$1"
  local plan_name=$(get_plan_name "$plan_file")

  echo -e "${BLUE}Processing:${NC} $(basename "$plan_file")"
  echo ""

  # Get or create worktree for this plan
  local worktree_path
  worktree_path=$(get_worktree_path "$plan_name")

  # If worktree doesn't exist, create it
  if [[ ! -d "$worktree_path" ]]; then
    worktree_path=$(setup_plan_worktree "$plan_file")
    if [[ -z "$worktree_path" ]] || [[ ! -d "$worktree_path" ]]; then
      log_error "Failed to setup worktree for plan: $plan_name"
      return 1
    fi
  fi

  echo -e "${BLUE}Executing in worktree:${NC} $worktree_path"
  echo ""

  # Sync feedback file TO worktree (human input from queue directory)
  if [[ -f "${plan_file%.md}.feedback.md" ]]; then
    cp "${plan_file%.md}.feedback.md" "$worktree_path/plan.feedback.md"
    echo -e "${YELLOW}Synced feedback file to worktree${NC}"
  fi

  # Build ralph.sh arguments - use plan.md inside worktree
  local ralph_args=("plan.md" --max "$MAX_ITERATIONS")

  # Don't pass --create-pr to ralph.sh (completion is handled here)
  if [ "$REVIEW_PLAN" = true ]; then
    ralph_args+=(--review-plan)
  fi

  # Run ralph.sh INSIDE the worktree
  # This means ralph.sh operates on the feature branch without switching branches in main worktree
  # Export queue plan path so blocker notifications register with the correct path for feedback sync
  (cd "$worktree_path" && RALPH_QUEUE_PLAN_PATH="$plan_file" "$SCRIPT_DIR/ralph.sh" "${ralph_args[@]}")
  local exit_code=$?

  # Sync plan file back to current/ (so queue state reflects progress)
  if [[ -f "$worktree_path/plan.md" ]] && [[ -f "$plan_file" ]]; then
    cp "$worktree_path/plan.md" "$plan_file"
  fi
  if [[ -f "$worktree_path/plan.progress.md" ]]; then
    cp "$worktree_path/plan.progress.md" "${plan_file%.md}.progress.md" 2>/dev/null || true
  fi
  # Sync feedback file back (agent may have processed items)
  if [[ -f "$worktree_path/plan.feedback.md" ]]; then
    cp "$worktree_path/plan.feedback.md" "${plan_file%.md}.feedback.md" 2>/dev/null || true
  fi
  # Sync blockers tracking file (for deduplication)
  if [[ -f "$worktree_path/plan.blockers" ]]; then
    cp "$worktree_path/plan.blockers" "${plan_file%.md}.blockers" 2>/dev/null || true
  fi

  # If ralph.sh completed successfully (exit 0), trigger completion workflow
  # ralph.sh no longer handles this because it runs inside a worktree
  if [[ "$exit_code" -eq 0 ]]; then
    echo ""
    echo -e "${BLUE}Plan completed - triggering completion workflow...${NC}"
    do_complete
  fi

  return $exit_code
}

# Start Slack bot if configured and not running
# Uses global bot mode (~/.ralph/) to handle multiple repos
start_slack_bot_if_needed() {
  # Check if Slack channel is configured
  local channel=$(config_get "slack.channel" "$CONFIG_DIR/config.yaml" 2>/dev/null)
  if [ -z "$channel" ]; then
    return 0  # No channel configured, skip silently
  fi

  # Check for tokens - try environment first, then global credentials
  local bot_token="${SLACK_BOT_TOKEN:-}"
  local app_token="${SLACK_APP_TOKEN:-}"
  local use_global=$(config_get "slack.global_bot" "$CONFIG_DIR/config.yaml" 2>/dev/null)

  # If no tokens in environment, try loading from global credentials
  if [ -z "$bot_token" ] || [ -z "$app_token" ]; then
    local global_env="$HOME/.ralph/slack.env"
    if [ -f "$global_env" ]; then
      # Source global credentials
      while IFS='=' read -r key value; do
        [[ "$key" =~ ^#.*$ ]] && continue
        [[ -z "$key" ]] && continue
        value=$(echo "$value" | sed 's/^["'"'"']//;s/["'"'"']$//')
        export "$key=$value"
      done < "$global_env"
      bot_token="${SLACK_BOT_TOKEN:-}"
      app_token="${SLACK_APP_TOKEN:-}"
      # Default to global mode when using global credentials
      if [ -z "$use_global" ]; then
        use_global="true"
      fi
    fi
  fi

  # Need both tokens for bot functionality
  if [ -z "$bot_token" ] || [ -z "$app_token" ]; then
    return 0  # Not configured, skip silently
  fi

  # Determine mode and pid file
  local global_flag=""
  local pid_file="$CONFIG_DIR/slack_bot.pid"
  if [ "$use_global" = "true" ]; then
    global_flag="--global"
    pid_file="$HOME/.ralph/slack_bot.pid"
  fi

  # Check if bot is already running
  if [ -f "$pid_file" ]; then
    local pid=$(cat "$pid_file" 2>/dev/null)
    if [ -n "$pid" ] && kill -0 "$pid" 2>/dev/null; then
      log_info "Slack bot already running (PID: $pid)" >&2
      return 0
    fi
    # Stale pid file, remove it
    rm -f "$pid_file"
  fi

  # Find the bot script
  local bot_script="$SCRIPT_DIR/slack-bot/ralph_slack_bot.py"
  if [ ! -f "$bot_script" ]; then
    log_warn "Slack bot script not found: $bot_script" >&2
    return 0
  fi

  # Check if python and slack-bolt are available
  if ! command -v python3 &> /dev/null; then
    log_warn "Python3 not found, cannot start Slack bot" >&2
    return 0
  fi

  # Start the bot in background
  local log_file="$HOME/.ralph/slack_bot.log"
  if [ "$use_global" != "true" ]; then
    log_file="$CONFIG_DIR/slack_bot.log"
  fi
  mkdir -p "$(dirname "$log_file")"

  echo -e "${BLUE}Starting Slack bot...${NC}" >&2
  nohup python3 "$bot_script" $global_flag >> "$log_file" 2>&1 &
  local bot_pid=$!

  # Wait a moment and verify it started
  sleep 2
  if kill -0 "$bot_pid" 2>/dev/null; then
    log_success "Slack bot started (PID: $bot_pid, log: $log_file)" >&2
  else
    log_warn "Slack bot failed to start - check $log_file" >&2
  fi
}

# Main work function
do_work() {
  ensure_dirs

  # Start Slack bot if configured
  start_slack_bot_if_needed

  # Check for orphaned files before starting
  check_orphaned_files

  echo -e "${GREEN}========================================"
  echo -e "Ralph Worker"
  echo -e "========================================${NC}"
  echo ""
  echo "Project: $PROJECT_ROOT"
  echo ""

  # Check for current plan
  local current_plan=$(get_current_plan)

  if [ -n "$current_plan" ]; then
    echo -e "${BLUE}Processing current plan...${NC}"
    process_plan "$current_plan"
    # Note: if plan completes, ralph.sh calls --complete which moves it
    # and activates next. So after this returns, check state again.
    return $?
  fi

  # No current plan - check pending
  local pending_count=$(count_files "$PENDING_DIR")

  if [ "$pending_count" -eq 0 ]; then
    echo -e "${GREEN}Queue empty.${NC} No plans to process."
    return 0
  fi

  echo -e "${BLUE}Activating next plan from queue...${NC}"
  current_plan=$(activate_next_plan)

  if [ -n "$current_plan" ]; then
    echo "  Activated: $(basename "$current_plan")"
    echo ""
    process_plan "$current_plan"
    return $?
  fi
}

# Create PR for completed plan (from worktree)
# Usage: create_pr_for_plan "plan-name" "worktree-path"
# Outputs: PR URL on stdout (if successful)
create_pr_for_plan() {
  local plan_name="$1"
  local worktree_path="$2"
  local feature_branch=$(get_feature_branch_from_name "$plan_name")
  local base_branch=$(config_get "git.base_branch" "$CONFIG_DIR/config.yaml" 2>/dev/null || echo "main")
  local pr_url=""

  echo -e "${BLUE}Creating PR for $feature_branch...${NC}" >&2

  # Push the branch from worktree
  echo "  Pushing branch to origin..." >&2
  if ! git -C "$worktree_path" push -u origin "$feature_branch" 2>&1 >&2; then
    log_warn "Failed to push branch - may already exist on remote"
  fi

  # Create PR using gh (from worktree so it has the right context)
  echo "  Creating PR via gh..." >&2
  local plan_file="$worktree_path/plan.md"
  local pr_title="feat: $plan_name"

  # Extract summary from plan if possible
  local pr_body="## Summary

Completed plan: $plan_name

## Changes

See commits in this PR for details.

---
*Generated by Ralph*"

  if command -v gh &> /dev/null; then
    pr_url=$(cd "$worktree_path" && gh pr create \
      --title "$pr_title" \
      --body "$pr_body" \
      --base "$base_branch" \
      --head "$feature_branch" 2>&2) || {
      log_warn "PR creation failed - may already exist"
      # Try to get existing PR URL
      pr_url=$(cd "$worktree_path" && gh pr view --json url -q .url 2>/dev/null) || true
    }
    if [[ -n "$pr_url" ]]; then
      echo "  PR: $pr_url" >&2
      echo "$pr_url"  # Output URL on stdout for capture
    fi
  else
    log_warn "gh CLI not installed - skipping PR creation"
    echo "  Push completed. Create PR manually at:" >&2
    echo "  https://github.com/$(git remote get-url origin 2>/dev/null | sed 's/.*github.com[:/]\(.*\)\.git/\1/')/compare/$base_branch...$feature_branch" >&2
  fi
}

# Merge feature branch directly to base (from main worktree)
# Usage: merge_plan_branch "plan-name"
merge_plan_branch() {
  local plan_name="$1"
  local feature_branch=$(get_feature_branch_from_name "$plan_name")
  local base_branch=$(config_get "git.base_branch" "$CONFIG_DIR/config.yaml" 2>/dev/null || echo "main")

  echo -e "${BLUE}Merging $feature_branch → $base_branch...${NC}"

  # Ensure we're on base branch in main worktree
  local current_branch=$(git branch --show-current)
  if [[ "$current_branch" != "$base_branch" ]]; then
    echo "  Switching to $base_branch..."
    git checkout "$base_branch" 2>&1 || {
      log_error "Failed to checkout $base_branch"
      return 1
    }
  fi

  # Merge feature branch
  if git merge --ff-only "$feature_branch" 2>/dev/null; then
    echo "  Fast-forward merge successful"
  elif git merge --no-edit "$feature_branch" 2>/dev/null; then
    echo "  Merge commit created"
  else
    log_error "Merge conflict detected"
    git merge --abort 2>/dev/null || true
    echo -e "${YELLOW}  Please resolve manually:${NC}"
    echo "  git checkout $base_branch && git merge $feature_branch"
    echo "  # resolve conflicts"
    echo "  git add . && git merge --continue"
    return 1
  fi

  # Delete feature branch if configured
  if [[ "$DELETE_BRANCH" == "true" ]]; then
    git branch -d "$feature_branch" 2>/dev/null && echo "  Deleted branch: $feature_branch" || true
  fi

  return 0
}

# Complete current plan and activate next
do_complete() {
  ensure_dirs

  local current_plan=$(get_current_plan)

  if [ -z "$current_plan" ]; then
    echo -e "${YELLOW}No current plan to complete.${NC}"
    return 1
  fi

  local plan_name=$(get_plan_name "$current_plan")
  local worktree_path=$(get_worktree_path "$plan_name")
  local base_branch=$(config_get "git.base_branch" "$CONFIG_DIR/config.yaml" 2>/dev/null || echo "main")

  echo -e "${GREEN}Completing:${NC} $(basename "$current_plan")"

  # Sync final state from worktree back to current/ (if worktree exists)
  if [[ -d "$worktree_path" ]]; then
    if [[ -f "$worktree_path/plan.md" ]]; then
      cp "$worktree_path/plan.md" "$current_plan"
    fi
    if [[ -f "$worktree_path/plan.progress.md" ]]; then
      cp "$worktree_path/plan.progress.md" "${current_plan%.md}.progress.md"
    fi
    if [[ -f "$worktree_path/plan.feedback.md" ]]; then
      cp "$worktree_path/plan.feedback.md" "${current_plan%.md}.feedback.md"
    fi
  fi

  # Handle completion mode: PR or merge
  local pr_url=""
  if [[ "$CREATE_PR" == "true" ]]; then
    echo ""
    echo -e "${BLUE}Completion mode: PR${NC}"

    if [[ -d "$worktree_path" ]]; then
      pr_url=$(create_pr_for_plan "$plan_name" "$worktree_path")
    else
      log_warn "Worktree not found - cannot create PR"
      log_warn "Branch may need to be pushed manually"
    fi

    # Send completion notification with PR URL
    send_plan_complete_notification "$plan_name" "$current_plan" "" "$pr_url" "$CONFIG_DIR"

    # Archive plan (PR workflow - don't merge yet)
    local completed_dir=$(complete_plan "$current_plan")
    echo "  Archived to: $completed_dir"

    # Clean up worktree (PR is created, worktree no longer needed)
    if [[ -d "$worktree_path" ]]; then
      echo "  Cleaning up worktree..."
      remove_plan_worktree "$plan_name"
    fi

  elif [[ "$MERGE_DIRECT" == "true" ]]; then
    echo ""
    echo -e "${BLUE}Completion mode: Direct merge${NC}"

    # Merge the feature branch to base
    if ! merge_plan_branch "$plan_name"; then
      log_error "Merge failed - plan not archived"
      return 1
    fi

    # Send completion notification (no PR URL for direct merge)
    send_plan_complete_notification "$plan_name" "$current_plan" "" "" "$CONFIG_DIR"

    # Archive plan after successful merge
    local completed_dir=$(complete_plan "$current_plan")
    echo "  Archived to: $completed_dir"

    # Clean up worktree
    if [[ -d "$worktree_path" ]]; then
      echo "  Cleaning up worktree..."
      remove_plan_worktree "$plan_name"
    fi

    # Pull latest to sync
    echo -e "${BLUE}Pulling latest from remote...${NC}"
    git pull --ff-only 2>/dev/null || git pull --rebase 2>/dev/null || echo "  No remote or pull failed (continuing)"
  fi

  # Commit queue state change
  git add "$CURRENT_DIR" "$COMPLETED_DIR" 2>/dev/null || true
  git commit -m "chore: complete plan $plan_name" --allow-empty 2>/dev/null || true

  # Check for next plan
  local pending_count=$(count_files "$PENDING_DIR")
  if [ "$pending_count" -gt 0 ]; then
    echo ""
    echo -e "${BLUE}Activating next plan...${NC}"
    local next=$(activate_next_plan)
    if [ -n "$next" ]; then
      echo "  Activated: $(basename "$next")"
      echo ""
      echo "Run 'ralph-worker' to continue."
    fi
  else
    echo ""
    echo -e "${GREEN}Queue empty.${NC} No more plans."
  fi
}

# Activate next pending plan
do_next() {
  ensure_dirs

  local current_plan=$(get_current_plan)
  if [ -n "$current_plan" ]; then
    echo -e "${YELLOW}Current plan still active:${NC} $(basename "$current_plan")"
    echo "Use --complete to finish it first, or remove it manually."
    return 1
  fi

  local pending_count=$(count_files "$PENDING_DIR")
  if [ "$pending_count" -eq 0 ]; then
    echo -e "${GREEN}Queue empty.${NC} No pending plans."
    return 0
  fi

  echo -e "${BLUE}Activating next plan...${NC}"
  local next=$(activate_next_plan)
  if [ -n "$next" ]; then
    echo "  Activated: $(basename "$next")"
  fi
}

# Clean up orphaned worktrees
do_cleanup() {
  echo -e "${BLUE}Cleaning up orphaned worktrees...${NC}"
  echo ""

  # First show what worktrees exist
  local worktrees=$(list_plan_worktrees)
  if [[ -z "$worktrees" ]]; then
    echo "No worktrees found."
    git worktree prune 2>/dev/null || true
    return 0
  fi

  echo "Current worktrees:"
  for wt in $worktrees; do
    local branch=$(get_worktree_branch "$wt")
    echo "  - $(basename "$wt") ($branch)"
  done
  echo ""

  # Clean up orphans
  local cleaned=$(cleanup_orphan_worktrees)

  if [[ "$cleaned" -gt 0 ]]; then
    echo -e "${GREEN}Cleaned $cleaned orphaned worktree(s)${NC}"
  else
    echo "No orphaned worktrees found."
  fi

  # Also prune git worktree references
  git worktree prune 2>/dev/null || true
  echo "Git worktree references pruned."
}

# Reset current plan back to pending (start over)
do_reset() {
  ensure_dirs

  local current_plan=$(get_current_plan)

  if [ -z "$current_plan" ]; then
    echo -e "${YELLOW}No current plan to reset.${NC}"
    return 1
  fi

  local plan_name=$(get_plan_name "$current_plan")
  local feature_branch=$(get_feature_branch "$current_plan")
  local worktree_path=$(get_worktree_path "$plan_name")

  echo -e "${BLUE}Resetting plan:${NC} $(basename "$current_plan")"

  # 1. Remove worktree if exists
  if [[ -d "$worktree_path" ]]; then
    echo "  Removing worktree: $worktree_path"
    remove_plan_worktree "$plan_name"
  fi

  # 2. Delete feature branch if exists
  if git show-ref --verify --quiet "refs/heads/$feature_branch" 2>/dev/null; then
    echo "  Deleting branch: $feature_branch"
    git branch -D "$feature_branch" 2>/dev/null || true
  fi

  # 3. Delete progress file (we're starting over)
  local progress_file="${current_plan%.md}.progress.md"
  if [[ -f "$progress_file" ]]; then
    rm "$progress_file"
    echo "  Deleted progress file"
  fi

  # 4. Move plan back to pending (at front of queue with fresh timestamp)
  local timestamp=$(date +%Y%m%d-%H%M%S)
  local basename=$(basename "$current_plan")
  # Remove old timestamp prefix if present
  basename=$(echo "$basename" | sed 's/^[0-9]\{8\}-[0-9]\{6\}-//')
  local dest="$PENDING_DIR/${timestamp}-${basename}"

  mv "$current_plan" "$dest"
  echo "  Moved to: $dest"

  # Commit the reset
  git add "$CURRENT_DIR" "$PENDING_DIR" 2>/dev/null || true
  git commit -m "chore: reset plan $plan_name to pending" --allow-empty 2>/dev/null || true

  echo ""
  echo -e "${GREEN}Plan reset.${NC} Run 'ralph-worker' to start fresh."
}

# Main
case "$ACTION" in
  status)
    show_status
    ;;
  add)
    add_plan "$ADD_FILE"
    ;;
  complete)
    do_complete
    ;;
  next)
    do_next
    ;;
  cleanup)
    do_cleanup
    ;;
  reset)
    do_reset
    ;;
  work)
    if [ "$LOOP_MODE" = true ]; then
      while true; do
        do_work

        # Check if more work to do
        current=$(get_current_plan)
        pending=$(count_files "$PENDING_DIR")

        if [ -z "$current" ] && [ "$pending" -eq 0 ]; then
          echo ""
          echo -e "${GREEN}All plans processed!${NC}"
          break
        fi

        echo ""
        echo "Continuing to next plan..."
        echo ""
        sleep 2
      done
    else
      do_work
    fi
    ;;
esac
