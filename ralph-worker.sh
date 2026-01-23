#!/bin/bash
set -e

# Ralph Worker Loop
# Picks up epics from Beads and implements all their tasks, creating one PR per epic
#
# Designed to run every 15 minutes on a server via cron:
#   */15 * * * * /path/to/ralph-worker.sh >> /var/log/ralph-worker.log 2>&1
#
# Usage:
#   ./ralph-worker.sh              # Process one epic
#   ./ralph-worker.sh --max 3      # Process up to 3 epics
#   ./ralph-worker.sh --epic bd-a1b2  # Process specific epic
#   ./ralph-worker.sh --dry-run    # Preview without implementing

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Load shared config library
source "$SCRIPT_DIR/lib/config.sh"

# Find project root
PROJECT_ROOT=$(find_project_root)
CONFIG_DIR=$(find_config_dir "$PROJECT_ROOT")

LOCK_FILE="$SCRIPT_DIR/.worker.lock"
LOG_FILE="$SCRIPT_DIR/worker.log"

# Parse arguments
MAX_EPICS=1
SPECIFIC_EPIC=""
DRY_RUN=false
MAX_ITERATIONS_PER_TASK=10
BASE_BRANCH=""

while [[ $# -gt 0 ]]; do
  case $1 in
    --max)
      MAX_EPICS="$2"
      shift 2
      ;;
    --epic)
      SPECIFIC_EPIC="$2"
      shift 2
      ;;
    --dry-run)
      DRY_RUN=true
      shift
      ;;
    --max-iterations)
      MAX_ITERATIONS_PER_TASK="$2"
      shift 2
      ;;
    --base-branch)
      BASE_BRANCH="$2"
      shift 2
      ;;
    --help|-h)
      echo "Ralph Worker - Epic/Task implementation loop"
      echo ""
      echo "Usage:"
      echo "  ./ralph-worker.sh [options]"
      echo ""
      echo "Options:"
      echo "  --max N              Process up to N epics (default: 1)"
      echo "  --epic ID            Process specific epic by ID"
      echo "  --dry-run            Preview without implementing"
      echo "  --max-iterations N   Max iterations per task (default: 10)"
      echo "  --base-branch NAME   Base branch to merge into (default: from config or 'main')"
      echo "  --help, -h           Show this help message"
      exit 0
      ;;
    *)
      echo "Unknown option: $1"
      exit 1
      ;;
  esac
done

# Check dependencies
if ! check_dependencies; then
  exit 1
fi

# Worker-specific dependencies
if ! command -v bd &> /dev/null; then
  log_error "Error: Beads (bd) is not installed"
  echo "Install via: npm install -g @beads/bd"
  exit 1
fi

if ! command -v gh &> /dev/null; then
  log_error "Error: GitHub CLI (gh) is not installed"
  exit 1
fi

if ! command -v jq &> /dev/null; then
  log_error "Error: jq is not installed"
  exit 1
fi

# Setup colors
setup_colors

# Check for existing lock (prevent concurrent workers)
if [ -f "$LOCK_FILE" ]; then
  LOCK_PID=$(cat "$LOCK_FILE" 2>/dev/null)
  if [ -n "$LOCK_PID" ] && kill -0 "$LOCK_PID" 2>/dev/null; then
    log "Worker already running (PID: $LOCK_PID). Exiting."
    exit 0
  else
    log "Stale lock file found. Removing."
    rm -f "$LOCK_FILE"
  fi
fi

# Create lock file
echo $$ > "$LOCK_FILE"
trap "rm -f '$LOCK_FILE'" EXIT

# Get config values
PROJECT_NAME=$(config_get "project.name" "$CONFIG_DIR/config.yaml")
PROJECT_NAME=${PROJECT_NAME:-"Project"}

# Default base branch from config or 'main'
if [ -z "$BASE_BRANCH" ]; then
  BASE_BRANCH=$(config_get "git.base_branch" "$CONFIG_DIR/config.yaml")
  BASE_BRANCH=${BASE_BRANCH:-"main"}
fi

echo -e "${GREEN}========================================"
echo -e "Ralph Worker"
echo -e "========================================${NC}"
echo ""
log "Project: $PROJECT_NAME"
log "Project root: $PROJECT_ROOT"
log "Base branch: $BASE_BRANCH"
log "Max epics: $MAX_EPICS"
log "Max iterations per task: $MAX_ITERATIONS_PER_TASK"
log "Dry run: $DRY_RUN"
if [ -n "$SPECIFIC_EPIC" ]; then
  log "Specific epic: $SPECIFIC_EPIC"
fi
echo ""

cd "$PROJECT_ROOT"

# Ensure we're on base branch and up to date
log "Syncing with remote..."
git fetch origin
git checkout "$BASE_BRANCH" 2>/dev/null || git checkout -b "$BASE_BRANCH" "origin/$BASE_BRANCH"
git pull origin "$BASE_BRANCH"

# Initialize Beads if needed
if [ ! -d ".beads" ]; then
  log_warn "Beads not initialized. Run ralph-discover.sh first."
  exit 0
fi

# Get ready epics (tasks with pr-ready label)
get_ready_epics() {
  bd ready --json 2>/dev/null | jq -r '.[] | select(.labels | contains(["pr-ready"])) | .id' | head -n "$MAX_EPICS"
}

# Get child tasks of an epic
get_epic_tasks() {
  local epic_id=$1
  bd show "$epic_id" --json 2>/dev/null | jq -r '.children[]?.id // empty'
}

if [ -n "$SPECIFIC_EPIC" ]; then
  EPICS="$SPECIFIC_EPIC"
  EPIC_COUNT=1
else
  EPICS=$(get_ready_epics)
  EPIC_COUNT=$(echo "$EPICS" | grep -c . 2>/dev/null || echo 0)
fi

if [ "$EPIC_COUNT" -eq 0 ] || [ -z "$EPICS" ]; then
  log "No ready epics found. Worker complete."
  exit 0
fi

log "Found $EPIC_COUNT ready epic(s)"
echo ""

# Process each epic
EPICS_COMPLETED=0
EPICS_FAILED=0

for EPIC_ID in $EPICS; do
  echo -e "${BLUE}========================================${NC}"
  log "Processing epic: $EPIC_ID"
  echo -e "${BLUE}========================================${NC}"

  # Get epic details
  EPIC_JSON=$(bd show "$EPIC_ID" --json 2>/dev/null)
  EPIC_TITLE=$(echo "$EPIC_JSON" | jq -r '.title' 2>/dev/null)
  EPIC_BODY=$(echo "$EPIC_JSON" | jq -r '.body' 2>/dev/null)
  EPIC_LABELS=$(echo "$EPIC_JSON" | jq -r '.labels | join(",")' 2>/dev/null)

  log "Epic: $EPIC_TITLE"
  log "Labels: $EPIC_LABELS"

  # Get child tasks
  TASKS=$(get_epic_tasks "$EPIC_ID")
  TASK_COUNT=$(echo "$TASKS" | grep -c . 2>/dev/null || echo 0)

  # If no child tasks, treat epic itself as the task
  if [ "$TASK_COUNT" -eq 0 ]; then
    log "Epic has no child tasks - treating epic as single task"
    TASKS="$EPIC_ID"
    TASK_COUNT=1
  else
    log "Found $TASK_COUNT child task(s)"
  fi

  if [ "$DRY_RUN" = true ]; then
    log "[DRY RUN] Would process epic with $TASK_COUNT task(s)"
    echo "$TASKS" | while read -r task_id; do
      [ -n "$task_id" ] && log "  - Task: $task_id"
    done
    continue
  fi

  # Create feature branch for the epic
  BRANCH_NAME="ralph/${EPIC_ID}"
  log "Creating branch: $BRANCH_NAME"
  git checkout "$BASE_BRANCH"
  git pull origin "$BASE_BRANCH"
  git checkout -b "$BRANCH_NAME" 2>/dev/null || git checkout "$BRANCH_NAME"

  # Track completed tasks for this epic
  TASKS_COMPLETED_IN_EPIC=0
  TASKS_FAILED_IN_EPIC=0
  COMMIT_MESSAGES=""

  # Process each task in the epic
  for TASK_ID in $TASKS; do
    [ -z "$TASK_ID" ] && continue

    echo ""
    log "Processing task: $TASK_ID"

    # Get task details
    TASK_JSON=$(bd show "$TASK_ID" --json 2>/dev/null)
    TASK_TITLE=$(echo "$TASK_JSON" | jq -r '.title' 2>/dev/null)
    TASK_BODY=$(echo "$TASK_JSON" | jq -r '.body' 2>/dev/null)

    log "Task: $TASK_TITLE"

    # Write context file for Claude
    cat > "$SCRIPT_DIR/context.json" << EOF
{
  "mode": "worker",
  "epicId": "$EPIC_ID",
  "epicTitle": "$EPIC_TITLE",
  "taskId": "$TASK_ID",
  "taskTitle": "$TASK_TITLE",
  "branchName": "$BRANCH_NAME",
  "projectRoot": "$PROJECT_ROOT",
  "maxIterations": $MAX_ITERATIONS_PER_TASK,
  "iteration": 0,
  "timestamp": "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
}
EOF

    # Write task details for Claude to read
    cat > "$SCRIPT_DIR/.current_task.md" << EOF
# Epic: $EPIC_TITLE

$EPIC_BODY

---

# Current Task: $TASK_TITLE

$TASK_BODY
EOF

    # Run implementation loop for this task
    TASK_COMPLETE=false
    for i in $(seq 1 $MAX_ITERATIONS_PER_TASK); do
      log "  Iteration $i of $MAX_ITERATIONS_PER_TASK"

      # Update iteration in context
      cat > "$SCRIPT_DIR/context.json" << EOF
{
  "mode": "worker",
  "epicId": "$EPIC_ID",
  "epicTitle": "$EPIC_TITLE",
  "taskId": "$TASK_ID",
  "taskTitle": "$TASK_TITLE",
  "branchName": "$BRANCH_NAME",
  "projectRoot": "$PROJECT_ROOT",
  "maxIterations": $MAX_ITERATIONS_PER_TASK,
  "iteration": $i,
  "timestamp": "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
}
EOF

      # Build and run prompt
      PROMPT=$(build_prompt "$SCRIPT_DIR/prompts/base/worker_prompt.md" "$CONFIG_DIR")
      OUTPUT=$(echo "$PROMPT" | claude -p --dangerously-skip-permissions 2>&1 | tee /dev/stderr) || true

      # Check for task completion
      if echo "$OUTPUT" | grep -q "<promise>TASK_COMPLETE</promise>"; then
        TASK_COMPLETE=true
        break
      fi

      # Check for failure
      if echo "$OUTPUT" | grep -q "<promise>TASK_FAILED</promise>"; then
        log "  Task marked as failed"
        break
      fi

      sleep 2
    done

    if [ "$TASK_COMPLETE" = true ]; then
      log "  Task completed!"
      TASKS_COMPLETED_IN_EPIC=$((TASKS_COMPLETED_IN_EPIC + 1))
      COMMIT_MESSAGES="${COMMIT_MESSAGES}\n- ${TASK_TITLE}"

      # Mark task done in Beads
      bd done "$TASK_ID" --comment "Completed as part of epic $EPIC_ID"
    else
      log "  Task failed or timed out"
      TASKS_FAILED_IN_EPIC=$((TASKS_FAILED_IN_EPIC + 1))

      # Add comment to task
      bd comment "$TASK_ID" "Worker failed after $MAX_ITERATIONS_PER_TASK iterations"
    fi
  done

  # Clean up task file
  rm -f "$SCRIPT_DIR/.current_task.md"

  echo ""
  log "Epic task summary: $TASKS_COMPLETED_IN_EPIC completed, $TASKS_FAILED_IN_EPIC failed"

  # Only create PR if at least one task completed
  if [ "$TASKS_COMPLETED_IN_EPIC" -gt 0 ]; then
    # Check if there are actual commits
    COMMIT_COUNT=$(git log "$BASE_BRANCH".."$BRANCH_NAME" --oneline 2>/dev/null | wc -l | tr -d ' ')

    if [ "$COMMIT_COUNT" -gt 0 ]; then
      log "Creating PR with $COMMIT_COUNT commit(s)..."

      # Push branch
      git push -u origin "$BRANCH_NAME"

      # Create PR
      PR_BODY=$(cat <<EOF
## Epic: $EPIC_TITLE

$EPIC_BODY

---

## Changes Made
$(git log "$BASE_BRANCH".."$BRANCH_NAME" --oneline | sed 's/^/- /')

## Tasks Completed
$(echo -e "$COMMIT_MESSAGES")

## Validation
- [ ] Tests pass
- [ ] Lint passes
- [ ] Manual review complete

---

Implemented by Ralph Worker

Epic: \`$EPIC_ID\`
Tasks completed: $TASKS_COMPLETED_IN_EPIC / $TASK_COUNT

Co-Authored-By: Claude <noreply@anthropic.com>
EOF
)

      PR_URL=$(gh pr create \
        --base "$BASE_BRANCH" \
        --head "$BRANCH_NAME" \
        --title "[$EPIC_ID] $EPIC_TITLE" \
        --body "$PR_BODY" 2>&1) || true

      if [ -n "$PR_URL" ] && [[ "$PR_URL" == http* ]]; then
        log "PR created: $PR_URL"

        # Mark epic as done (if all tasks completed)
        if [ "$TASKS_FAILED_IN_EPIC" -eq 0 ]; then
          bd done "$EPIC_ID" --comment "PR created: $PR_URL"
        else
          bd comment "$EPIC_ID" "Partial PR created: $PR_URL ($TASKS_FAILED_IN_EPIC tasks failed)"
        fi

        EPICS_COMPLETED=$((EPICS_COMPLETED + 1))
      else
        log_warn "PR creation may have failed: $PR_URL"
        EPICS_FAILED=$((EPICS_FAILED + 1))
      fi
    else
      log "No commits made - skipping PR creation"
      EPICS_FAILED=$((EPICS_FAILED + 1))
    fi
  else
    log "No tasks completed - skipping PR creation"
    EPICS_FAILED=$((EPICS_FAILED + 1))

    # Clean up empty branch
    git checkout "$BASE_BRANCH"
    git branch -D "$BRANCH_NAME" 2>/dev/null || true
  fi

  # Return to base branch
  git checkout "$BASE_BRANCH"

  echo ""
done

# Cleanup
rm -f "$SCRIPT_DIR/context.json"

echo -e "${GREEN}========================================${NC}"
log "Worker Summary"
echo -e "${GREEN}========================================${NC}"
log "Epics processed: $((EPICS_COMPLETED + EPICS_FAILED))"
log "PRs created: $EPICS_COMPLETED"
log "Failed: $EPICS_FAILED"

# Log to file
{
  echo "---"
  echo "Timestamp: $(date -u +%Y-%m-%dT%H:%M:%SZ)"
  echo "Epics processed: $((EPICS_COMPLETED + EPICS_FAILED))"
  echo "PRs created: $EPICS_COMPLETED"
  echo "Failed: $EPICS_FAILED"
} >> "$LOG_FILE"
