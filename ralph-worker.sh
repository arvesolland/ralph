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

# Move plan to completed with its progress file
complete_plan() {
  local plan_file="$1"
  local plan_name=$(basename "$plan_file" .md)
  local plan_dir=$(dirname "$plan_file")
  local progress_file="$plan_dir/${plan_name}.progress.md"
  local timestamp=$(date +%Y%m%d-%H%M%S)
  local completed_subdir="$COMPLETED_DIR/${timestamp}-${plan_name}"

  mkdir -p "$completed_subdir"

  # Move the plan
  mv "$plan_file" "$completed_subdir/plan.md"

  # Move the progress file if it exists (plan-specific learnings)
  if [ -f "$progress_file" ]; then
    mv "$progress_file" "$completed_subdir/progress.md"
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

# Create or checkout feature branch for plan
setup_feature_branch() {
  local plan_file="$1"
  local branch_name=$(get_feature_branch "$plan_file")
  local base_branch=$(config_get "git.base_branch" "$CONFIG_DIR/config.yaml" 2>/dev/null || echo "main")

  # All output to stderr to avoid polluting stdout (which is captured for return values)
  {
    echo -e "${BLUE}Setting up feature branch: $branch_name${NC}"

    if git show-ref --verify --quiet "refs/heads/$branch_name"; then
      echo "  Branch exists, checking out..."
      git checkout "$branch_name" 2>&1
      git pull --ff-only 2>&1 || true
    else
      echo "  Creating branch from $base_branch..."
      git checkout -b "$branch_name" "$base_branch" 2>&1 || git checkout -b "$branch_name" 2>&1
    fi

    echo "  On branch: $(git branch --show-current)"
  } >&2
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

# Move next pending plan to current
activate_next_plan() {
  # First check for orphaned files
  check_orphaned_files

  local next_plan=$(get_oldest_file "$PENDING_DIR")
  if [ -n "$next_plan" ] && [ -f "$next_plan" ]; then
    local dest="$CURRENT_DIR/$(basename "$next_plan")"
    mv "$next_plan" "$dest"

    # Setup feature branch for this plan
    setup_feature_branch "$dest"

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

# Process a single plan
process_plan() {
  local plan_file="$1"
  local plan_name=$(basename "$plan_file")

  echo -e "${BLUE}Processing:${NC} $plan_name"
  echo ""

  # Build ralph.sh arguments
  local ralph_args=("$plan_file" --max "$MAX_ITERATIONS")

  # Pass through CREATE_PR flag if set
  if [ "$CREATE_PR" = true ]; then
    ralph_args+=(--create-pr)
  fi

  # Run ralph.sh on the plan
  # ralph.sh will call ralph-worker.sh --complete when it catches <promise>COMPLETE</promise>
  "$SCRIPT_DIR/ralph.sh" "${ralph_args[@]}"
  local exit_code=$?

  # ralph.sh handles completion detection via COMPLETE marker
  # If it returns 0, plan was completed and moved by the completion hook
  # If it returns 1, max iterations reached - plan still in current/
  return $exit_code
}

# Main work function
do_work() {
  ensure_dirs

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

# Create PR via Claude Code
create_pr_with_claude() {
  local plan_file="$1"
  local feature_branch=$(git branch --show-current)
  local base_branch=$(config_get "git.base_branch" "$CONFIG_DIR/config.yaml" 2>/dev/null || echo "main")

  echo -e "${BLUE}Creating PR via Claude Code...${NC}"
  echo "  Feature branch: $feature_branch"
  echo "  Base branch: $base_branch"
  echo ""

  # Build prompt for Claude Code
  local prompt="Create a pull request for the completed plan.

## Branch Info
- Feature branch: $feature_branch
- Target branch: $base_branch

## Plan File
Read the completed plan at: $plan_file

## Instructions
1. Run \`git log $base_branch..$feature_branch --oneline\` to see commits in this branch
2. Run \`git diff $base_branch...$feature_branch --stat\` to see files changed
3. Read the plan file for context and completed tasks

Create a PR with:
- **Title:** Clear, concise summary of the feature/fix (from plan name or context)
- **Description:**
  - Summary of what was implemented (from completed tasks)
  - Key changes (from git diff)
  - Any gotchas or notes (from progress file if exists)

Use \`gh pr create\` to create the PR targeting $base_branch."

  # Run Claude Code to create PR (with retry logic)
  echo "$prompt" | run_claude_with_retry -p --dangerously-skip-permissions || true

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
