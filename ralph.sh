#!/bin/bash
set -e

# Ralph - AI Agent Implementation Loop
# Implements tasks from a plan file one at a time
#
# Usage:
#   ./ralph.sh <plan-file> [options]
#   ./ralph.sh docs/plan.md --max 50
#   ./ralph.sh plan.md --review-plan --review-passes 3

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Load shared config library
source "$SCRIPT_DIR/lib/config.sh"

# Find project root
PROJECT_ROOT=$(find_project_root)
CONFIG_DIR=$(find_config_dir "$PROJECT_ROOT")

# Defaults
PLAN_FILE=""
MAX_ITERATIONS=30
REVIEW_PLAN=false
REVIEW_PASSES=5
CREATE_PR=false

# Parse arguments
while [[ $# -gt 0 ]]; do
  case $1 in
    --review-plan|-r)
      REVIEW_PLAN=true
      shift
      ;;
    --review-passes)
      REVIEW_PASSES="$2"
      shift 2
      ;;
    --max|-m)
      MAX_ITERATIONS="$2"
      shift 2
      ;;
    --create-pr|--pr)
      CREATE_PR=true
      shift
      ;;
    --version|-v)
      echo "Ralph v$(get_ralph_version "$SCRIPT_DIR")"
      exit 0
      ;;
    --help|-h)
      echo "Ralph - AI Agent for implementing plans"
      echo ""
      echo "Usage:"
      echo "  ./ralph.sh <plan-file> [options]"
      echo ""
      echo "Arguments:"
      echo "  plan-file              Path to the plan/spec file"
      echo ""
      echo "Options:"
      echo "  --review-plan, -r      Run plan reviewer first to optimize the plan"
      echo "  --review-passes N      Number of review passes (default: 5)"
      echo "  --max, -m N            Max worker iterations (default: 30)"
      echo "  --create-pr, --pr      Create PR via Claude Code after completion"
      echo "  --version, -v          Show version"
      echo "  --help, -h             Show this help message"
      echo ""
      echo "Examples:"
      echo "  ./ralph.sh docs/planning/feature.md"
      echo "  ./ralph.sh plan.md --review-plan"
      echo "  ./ralph.sh plan.md --review-plan --review-passes 3 --max 50"
      exit 0
      ;;
    -*)
      echo "Unknown option: $1"
      echo "Use --help for usage information"
      exit 1
      ;;
    *)
      if [ -z "$PLAN_FILE" ]; then
        PLAN_FILE="$1"
      fi
      shift
      ;;
  esac
done

# Require plan file
if [ -z "$PLAN_FILE" ]; then
  log_error "Error: Plan file required"
  echo "Usage: ./ralph.sh <plan-file> [options]"
  echo "Use --help for more information"
  exit 1
fi

# Resolve plan file path (support relative and absolute)
if [[ "$PLAN_FILE" = /* ]]; then
  PLAN_PATH="$PLAN_FILE"
else
  PLAN_PATH="$PROJECT_ROOT/$PLAN_FILE"
fi

# Verify plan file exists
if [[ ! -f "$PLAN_PATH" ]]; then
  log_error "Plan file not found: $PLAN_PATH"
  exit 1
fi

# Check dependencies
if ! check_dependencies; then
  exit 1
fi

# Setup colors
setup_colors

# Get project name from config
PROJECT_NAME=$(config_get "project.name" "$CONFIG_DIR/config.yaml")
PROJECT_NAME=${PROJECT_NAME:-"Project"}
BASE_BRANCH=$(config_get "git.base_branch" "$CONFIG_DIR/config.yaml")
BASE_BRANCH=${BASE_BRANCH:-"main"}

# Get current branch (ralph.sh now runs in whatever worktree it's invoked in)
# No branch switching - that's handled by ralph-worker.sh via worktrees
CURRENT_BRANCH=$(git branch --show-current 2>/dev/null || echo "unknown")

# Derive feature branch name for context (used in context.json)
get_feature_branch() {
  local plan_file="$1"
  local plan_name=$(basename "$plan_file" .md)
  # Remove timestamp prefix if present (e.g., 20240127-143052-auth -> auth)
  plan_name=$(echo "$plan_name" | sed 's/^[0-9]\{8\}-[0-9]\{6\}-//')
  echo "feat/$plan_name"
}

# Use current branch if on a feature branch, otherwise derive from plan name
if [[ "$CURRENT_BRANCH" == feat/* ]]; then
  FEATURE_BRANCH="$CURRENT_BRANCH"
else
  FEATURE_BRANCH=$(get_feature_branch "$PLAN_PATH")
fi

echo -e "${GREEN}Ralph - Implementation Loop${NC}"
echo "========================================"
echo "Project: $PROJECT_NAME"
echo "Project root: $PROJECT_ROOT"
echo "Plan file: $PLAN_FILE"
echo "Current branch: $CURRENT_BRANCH"
if [[ "$CURRENT_BRANCH" != "$FEATURE_BRANCH" ]]; then
  echo -e "${YELLOW}Warning: Not on expected feature branch ($FEATURE_BRANCH)${NC}"
  echo "  This is OK if running standalone or in a worktree"
fi
if [ "$REVIEW_PLAN" = true ]; then
  echo -e "Plan review: ${YELLOW}enabled ($REVIEW_PASSES passes)${NC}"
fi
echo "Max iterations: $MAX_ITERATIONS"
echo ""

cd "$PROJECT_ROOT"

# Notify plan start (creates Slack thread for all updates)
QUEUE_PLAN_PATH="${RALPH_QUEUE_PLAN_PATH:-$PLAN_PATH}"
# Use queue path for plan name (worktree uses plan.md, but queue has real name)
PLAN_NAME=$(basename "$QUEUE_PLAN_PATH" .md)
PLAN_THREAD_TS=$(send_plan_start_notification "$PLAN_NAME" "$QUEUE_PLAN_PATH" "$MAX_ITERATIONS" "$CONFIG_DIR")

# No branch switching! ralph.sh now runs in whatever directory/worktree it's invoked in
# Branch management is handled by ralph-worker.sh using git worktrees
echo -e "${BLUE}Working on branch:${NC} $CURRENT_BRANCH"
echo ""

# ============================================
# Phase 1: Plan Review (if --review-plan)
# ============================================
if [ "$REVIEW_PLAN" = true ]; then
  echo -e "${BLUE}========================================"
  echo -e "Phase 1: Plan Review"
  echo -e "========================================${NC}"
  echo ""

  # Notify plan review start
  send_plan_progress "Starting plan review ($REVIEW_PASSES passes)" "$QUEUE_PLAN_PATH" "🔍" "$CONFIG_DIR"

  # Capture plan before review for diff
  PLAN_BEFORE_REVIEW=$(cat "$PLAN_PATH")

  for i in $(seq 1 $REVIEW_PASSES); do
    echo -e "${YELLOW}--- Review Pass $i of $REVIEW_PASSES ---${NC}"
    echo ""

    # Write context for plan reviewer
    cat > "$SCRIPT_DIR/context.json" << EOF
{
  "mode": "plan-review",
  "planFile": "$PLAN_FILE",
  "planPath": "$PLAN_PATH",
  "projectRoot": "$PROJECT_ROOT",
  "pass": $i,
  "totalPasses": $REVIEW_PASSES,
  "timestamp": "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
}
EOF

    # Build and run prompt with retry logic
    PROMPT=$(build_prompt "$SCRIPT_DIR/prompts/base/plan_reviewer_prompt.md" "$CONFIG_DIR")
    OUTPUT=$(echo "$PROMPT" | run_claude_with_retry -p --dangerously-skip-permissions) || true

    echo ""

    if [ "$i" -lt "$REVIEW_PASSES" ]; then
      echo "Cooling down before next review pass..."
      sleep 2
    fi
  done

  echo ""
  log_success "Plan review complete"

  # Show review summary
  PLAN_AFTER_REVIEW=$(cat "$PLAN_PATH")
  if [ "$PLAN_BEFORE_REVIEW" = "$PLAN_AFTER_REVIEW" ]; then
    echo -e "${YELLOW}No changes made to plan during review.${NC}"
  else
    # Count lines changed
    BEFORE_LINES=$(echo "$PLAN_BEFORE_REVIEW" | wc -l)
    AFTER_LINES=$(echo "$PLAN_AFTER_REVIEW" | wc -l)
    BEFORE_TASKS=$(echo "$PLAN_BEFORE_REVIEW" | grep -c "^### T" || echo "0")
    AFTER_TASKS=$(echo "$PLAN_AFTER_REVIEW" | grep -c "^### T" || echo "0")

    echo -e "${GREEN}Plan modified during review:${NC}"
    echo "  Lines: $BEFORE_LINES → $AFTER_LINES"
    echo "  Tasks: $BEFORE_TASKS → $AFTER_TASKS"
    echo ""
    echo "Run 'git diff $PLAN_FILE' to see full changes."
  fi
  echo ""
fi

# ============================================
# Phase 2: Implementation Loop
# ============================================
echo -e "${BLUE}========================================"
if [ "$REVIEW_PLAN" = true ]; then
  echo -e "Phase 2: Implementation"
else
  echo -e "Implementation"
fi
echo -e "========================================${NC}"
echo ""

for i in $(seq 1 $MAX_ITERATIONS); do
  echo ""
  echo "========================================"
  echo "Iteration $i of $MAX_ITERATIONS"
  echo "========================================"
  echo ""

  # Notify iteration start (if enabled)
  send_slack_notification "iteration" "Plan *$PLAN_NAME* - iteration $i of $MAX_ITERATIONS" "$CONFIG_DIR"

  # Write context for worker
  cat > "$SCRIPT_DIR/context.json" << EOF
{
  "planFile": "$PLAN_FILE",
  "planPath": "$PLAN_PATH",
  "projectRoot": "$PROJECT_ROOT",
  "featureBranch": "$FEATURE_BRANCH",
  "baseBranch": "$BASE_BRANCH",
  "iteration": $i,
  "maxIterations": $MAX_ITERATIONS
}
EOF

  # Build and run prompt with retry logic
  PROMPT=$(build_prompt "$SCRIPT_DIR/prompts/base/prompt.md" "$CONFIG_DIR")
  OUTPUT=$(echo "$PROMPT" | run_claude_with_retry -p --dangerously-skip-permissions) || true

  if echo "$OUTPUT" | grep -q "<promise>COMPLETE</promise>"; then
    echo ""
    echo -e "${BLUE}Completion signal detected - verifying plan state...${NC}"

    # Verify with haiku to avoid false positives (LLM sometimes mentions the marker without meaning it)
    VERIFY_PROMPT="Read this plan file and determine if ALL tasks are genuinely complete.
Look for task statuses, checkboxes, or any indication of remaining work.

Output ONLY one word: COMPLETE or INCOMPLETE

$(cat "$PLAN_PATH")"

    VERIFY_RESULT=$(run_claude_simple_with_retry "$VERIFY_PROMPT" --model haiku -p | tr -d '[:space:]')

    if [ "$VERIFY_RESULT" = "COMPLETE" ]; then
      log_success "Verified: All tasks complete!"
      echo "Ralph finished successfully"
      echo "Plan file: $PLAN_FILE"
      rm -f "$SCRIPT_DIR/context.json"

      # Notify completion (to plan thread)
      send_plan_complete_notification "$PLAN_NAME" "$QUEUE_PLAN_PATH" "$i" "$CONFIG_DIR"

      # Completion workflow is handled by ralph-worker.sh when running in worktree mode
      # ralph-worker.sh will detect exit code 0 and call do_complete

      exit 0
    else
      log_warn "Verification failed: Plan has incomplete tasks."

      # Get detailed explanation of what's incomplete
      EXPLAIN_PROMPT="Read this plan file and explain what tasks are NOT complete.
Be specific: list the task IDs/names and what criteria are not met.
If all tasks appear complete, explain why you flagged it as incomplete.

$(cat "$PLAN_PATH")"

      EXPLANATION=$(run_claude_simple_with_retry "$EXPLAIN_PROMPT" --model haiku -p 2>/dev/null || echo "Could not get detailed explanation")

      # Write explanation to feedback file so agent sees it next iteration
      FEEDBACK_FILE="${PLAN_PATH%.md}.feedback.md"
      TIMESTAMP=$(date "+%Y-%m-%d %H:%M")

      if [ ! -f "$FEEDBACK_FILE" ]; then
        cat > "$FEEDBACK_FILE" << EOF
# Feedback: $(basename "${PLAN_PATH%.md}")

## Pending

## Processed
EOF
      fi

      # Add verification failure to Pending section
      # Format: timestamp header, then indented explanation block
      {
        # Read file, insert after ## Pending
        while IFS= read -r line || [ -n "$line" ]; do
          echo "$line"
          if [ "$line" = "## Pending" ]; then
            echo "- [$TIMESTAMP] **Verification failed:**"
            echo "$EXPLANATION" | sed 's/^/  /'
            echo ""
          fi
        done
      } < "$FEEDBACK_FILE" > "${FEEDBACK_FILE}.tmp" && mv "${FEEDBACK_FILE}.tmp" "$FEEDBACK_FILE"

      echo ""
      echo -e "${YELLOW}Verification details written to: $FEEDBACK_FILE${NC}"
      echo -e "${YELLOW}The agent will see this on the next iteration.${NC}"
      echo ""
      echo "Continuing..."
    fi
  fi

  # Check for blocker (human input needed)
  if echo "$OUTPUT" | grep -q "<blocker>"; then
    BLOCKER_CONTENT=$(extract_blocker "$OUTPUT")
    if [ -n "$BLOCKER_CONTENT" ]; then
      BLOCKER_HASH=$(blocker_hash "$BLOCKER_CONTENT")

      # Use queue path for blocker tracking (consistent across worktree iterations)
      BLOCKER_PLAN_PATH="${RALPH_QUEUE_PLAN_PATH:-$PLAN_PATH}"

      # Only notify if this is a new blocker (avoid spamming same message)
      if ! blocker_already_notified "$BLOCKER_HASH" "$BLOCKER_PLAN_PATH" "$CONFIG_DIR"; then
        echo ""
        log_warn "Blocker detected - human input required"
        echo -e "${YELLOW}$BLOCKER_CONTENT${NC}"

        # Send Slack notification with blocker details (with thread tracking if API configured)
        # Use queue path for thread registration (so bot writes to correct location for sync)
        # Falls back to PLAN_PATH when running standalone (not via ralph-worker.sh)
        BLOCKER_PLAN_PATH="${RALPH_QUEUE_PLAN_PATH:-$PLAN_PATH}"
        BLOCKER_MSG=$(echo "$BLOCKER_CONTENT" | head -5 | tr '\n' ' ' | sed 's/  */ /g')
        THREAD_TS=$(send_blocker_notification "Plan *$PLAN_NAME* needs human input (iteration $i):\n$BLOCKER_MSG\n\n_Reply to this thread to provide feedback._" "$BLOCKER_PLAN_PATH" "$CONFIG_DIR")

        mark_blocker_notified "$BLOCKER_HASH" "$BLOCKER_PLAN_PATH" "$CONFIG_DIR"
        echo ""
        if [ -n "$THREAD_TS" ]; then
          echo "Slack notification sent (thread: $THREAD_TS). Reply to the thread or edit: ${PLAN_PATH%.md}.feedback.md"
        else
          echo "Slack notification sent. Add response to: ${PLAN_PATH%.md}.feedback.md"
        fi
      fi
    fi
  fi

  echo ""
  echo "Cooling down before next iteration..."
  sleep 3
done

echo ""
log_warn "Max iterations ($MAX_ITERATIONS) reached"
echo "Check plan file for remaining tasks: $PLAN_FILE"
rm -f "$SCRIPT_DIR/context.json"

# Notify max iterations reached (to plan thread)
send_plan_error_notification "Plan *$PLAN_NAME* hit max iterations ($MAX_ITERATIONS) without completing" "$QUEUE_PLAN_PATH" "$CONFIG_DIR"

exit 1
