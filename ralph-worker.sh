#!/bin/bash
set -e

# Ralph Worker - File-based task queue
# Works through plans in a structured folder system
#
# Folder structure:
#   .ralph/plans/
#   ├── pending/      # Plans waiting to be processed (oldest first)
#   ├── current/      # Plan currently being worked on (0 or 1 file)
#   └── completed/    # Finished plans with their progress logs
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
MAX_ITERATIONS=30
CREATE_PR=false

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
      shift
      ;;
    --version|-v)
      echo "Ralph Worker v$(get_ralph_version "$SCRIPT_DIR" 2>/dev/null || echo "unknown")"
      exit 0
      ;;
    --help|-h)
      echo "Ralph Worker - File-based task queue"
      echo ""
      echo "Usage:"
      echo "  ./ralph-worker.sh              Process current or next plan"
      echo "  ./ralph-worker.sh --status     Show queue status"
      echo "  ./ralph-worker.sh --add FILE   Add plan to pending queue"
      echo "  ./ralph-worker.sh --complete   Mark current plan complete, activate next"
      echo "  ./ralph-worker.sh --next       Activate next pending plan"
      echo "  ./ralph-worker.sh --loop       Process until queue empty"
      echo ""
      echo "Options:"
      echo "  --status, -s       Show queue status"
      echo "  --add, -a FILE     Add a plan file to pending queue"
      echo "  --complete, -c     Complete current plan and activate next"
      echo "  --next, -n         Activate next pending plan"
      echo "  --loop, -l         Keep processing until no more plans"
      echo "  --max, -m N        Max iterations per plan (default: 30)"
      echo "  --create-pr, --pr  Create PR via Claude Code after plan completion"
      echo "  --version, -v      Show version"
      echo "  --help, -h         Show this help"
      echo ""
      echo "Folder structure:"
      echo "  plans/pending/    Plans waiting to be processed"
      echo "  plans/current/    Currently active plan (0-1 files)"
      echo "  plans/complete/   Finished plans with logs"
      exit 0
      ;;
    *)
      echo "Unknown option: $1"
      exit 1
      ;;
  esac
done

# Ensure directories exist
ensure_dirs() {
  mkdir -p "$PENDING_DIR" "$CURRENT_DIR" "$COMPLETED_DIR"
}

# Get count of files in a directory
count_files() {
  local dir="$1"
  find "$dir" -maxdepth 1 -type f -name "*.md" 2>/dev/null | wc -l | tr -d ' '
}

# Get oldest file in a directory
get_oldest_file() {
  local dir="$1"
  ls -t "$dir"/*.md 2>/dev/null | tail -1
}

# Get the current plan file (if any)
get_current_plan() {
  local files=($(ls "$CURRENT_DIR"/*.md 2>/dev/null))
  if [ ${#files[@]} -gt 0 ]; then
    echo "${files[0]}"
  fi
}

# Check if a plan has incomplete tasks (used for status display only)
has_incomplete_tasks() {
  local plan_file="$1"
  # Look for unchecked markdown checkboxes - best effort for status
  grep -q '^\s*-\s*\[ \]' "$plan_file" 2>/dev/null
}

# Move plan to completed with progress snapshot
complete_plan() {
  local plan_file="$1"
  local plan_name=$(basename "$plan_file" .md)
  local timestamp=$(date +%Y%m%d-%H%M%S)
  local completed_subdir="$COMPLETED_DIR/${timestamp}-${plan_name}"

  mkdir -p "$completed_subdir"

  # Move the plan
  mv "$plan_file" "$completed_subdir/plan.md"

  # Copy relevant progress entries
  if [ -f "$SCRIPT_DIR/progress.txt" ]; then
    # Extract entries related to this plan (if tagged) or just copy recent
    cp "$SCRIPT_DIR/progress.txt" "$completed_subdir/progress-snapshot.txt"
  fi

  echo "$completed_subdir"
}

# Move next pending plan to current
activate_next_plan() {
  local next_plan=$(get_oldest_file "$PENDING_DIR")
  if [ -n "$next_plan" ] && [ -f "$next_plan" ]; then
    mv "$next_plan" "$CURRENT_DIR/"
    echo "$CURRENT_DIR/$(basename "$next_plan")"
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
    for f in "$PENDING_DIR"/*.md; do
      [ -f "$f" ] && echo "  - $(basename "$f")"
    done
  fi
  echo ""

  echo -e "${BLUE}Current:${NC}"
  if [ -n "$current_plan" ]; then
    local task_count=$(grep -c '^\s*-\s*\[ \]' "$current_plan" 2>/dev/null || echo "0")
    local done_count=$(grep -c '^\s*-\s*\[x\]' "$current_plan" 2>/dev/null || echo "0")
    echo "  - $(basename "$current_plan") ($done_count done, $task_count remaining)"
  else
    echo "  (none)"
  fi
  echo ""

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

  ensure_dirs

  # Add timestamp prefix for ordering
  local timestamp=$(date +%Y%m%d-%H%M%S)
  local basename=$(basename "$file")
  local dest="$PENDING_DIR/${timestamp}-${basename}"

  cp "$file" "$dest"
  echo -e "${GREEN}Added to queue:${NC} $dest"

  show_status
}

# Process a single plan
process_plan() {
  local plan_file="$1"
  local plan_name=$(basename "$plan_file")

  echo -e "${BLUE}Processing:${NC} $plan_name"
  echo ""

  # Run ralph.sh on the plan
  # ralph.sh will call ralph-worker.sh --complete when it catches <promise>COMPLETE</promise>
  "$SCRIPT_DIR/ralph.sh" "$plan_file" --max "$MAX_ITERATIONS"
  local exit_code=$?

  # ralph.sh handles completion detection via COMPLETE marker
  # If it returns 0, plan was completed and moved by the completion hook
  # If it returns 1, max iterations reached - plan still in current/
  return $exit_code
}

# Main work function
do_work() {
  ensure_dirs

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

# Create PR via Claude Code
create_pr_with_claude() {
  local plan_file="$1"

  echo -e "${BLUE}Creating PR via Claude Code...${NC}"
  echo ""

  # Build prompt for Claude Code
  local prompt="Create a pull request for the work completed in this branch.

Read the completed plan file at: $plan_file

Use the plan's Context section and completed tasks to write a clear PR description.
The PR title should reflect the main goal from the plan.
Include a summary of what was implemented based on the completed tasks.

Create the PR targeting the base branch this feature branch was created from."

  # Run Claude Code to create PR
  echo "$prompt" | claude -p --dangerously-skip-permissions 2>&1 | tee /dev/stderr

  echo ""
}

# Complete current plan and activate next
do_complete() {
  ensure_dirs

  local current_plan=$(get_current_plan)

  if [ -z "$current_plan" ]; then
    echo -e "${YELLOW}No current plan to complete.${NC}"
    return 1
  fi

  # Store plan path before moving (for PR creation)
  local plan_for_pr="$current_plan"

  echo -e "${GREEN}Completing:${NC} $(basename "$current_plan")"
  local completed_dir=$(complete_plan "$current_plan")
  echo "  Archived to: $completed_dir"

  # Create PR if flag is set
  if [ "$CREATE_PR" = true ]; then
    echo ""
    create_pr_with_claude "$completed_dir/plan.md"
  fi

  # Check for next plan
  local pending_count=$(count_files "$PENDING_DIR")
  if [ "$pending_count" -gt 0 ]; then
    echo ""
    echo -e "${BLUE}Activating next plan...${NC}"
    local next=$(activate_next_plan)
    if [ -n "$next" ]; then
      echo "  Activated: $(basename "$next")"
      echo ""
      echo "Run 'ralph-worker' or 'ralph plans/current/*.md' to continue."
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
