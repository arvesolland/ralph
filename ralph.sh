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
REVIEW_PASSES=2
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
      echo "  --review-passes N      Number of review passes (default: 2)"
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

# Get feature branch name from plan
get_feature_branch() {
  local plan_file="$1"
  local plan_name=$(basename "$plan_file" .md)
  # Remove timestamp prefix if present (e.g., 20240127-143052-auth -> auth)
  plan_name=$(echo "$plan_name" | sed 's/^[0-9]\{8\}-[0-9]\{6\}-//')
  echo "feat/$plan_name"
}

FEATURE_BRANCH=$(get_feature_branch "$PLAN_PATH")

echo -e "${GREEN}Ralph - Implementation Loop${NC}"
echo "========================================"
echo "Project: $PROJECT_NAME"
echo "Project root: $PROJECT_ROOT"
echo "Plan file: $PLAN_FILE"
echo "Feature branch: $FEATURE_BRANCH"
if [ "$REVIEW_PLAN" = true ]; then
  echo -e "Plan review: ${YELLOW}enabled ($REVIEW_PASSES passes)${NC}"
fi
echo "Max iterations: $MAX_ITERATIONS"
echo ""

cd "$PROJECT_ROOT"

# Setup feature branch (create or checkout)
echo -e "${BLUE}Setting up feature branch...${NC}"
if git show-ref --verify --quiet "refs/heads/$FEATURE_BRANCH"; then
  echo "  Branch exists, checking out..."
  git checkout "$FEATURE_BRANCH"
  git pull --ff-only 2>/dev/null || true
else
  echo "  Creating branch from $BASE_BRANCH..."
  git checkout -b "$FEATURE_BRANCH" "$BASE_BRANCH" 2>/dev/null || git checkout -b "$FEATURE_BRANCH"
fi
echo "  On branch: $(git branch --show-current)"
echo ""

# ============================================
# Phase 1: Plan Review (if --review-plan)
# ============================================
if [ "$REVIEW_PLAN" = true ]; then
  echo -e "${BLUE}========================================"
  echo -e "Phase 1: Plan Review"
  echo -e "========================================${NC}"
  echo ""

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

      # If plan is in the queue (current folder), trigger completion workflow
      if [[ "$PLAN_PATH" == *"/plans/current/"* ]]; then
        echo ""
        echo "Plan is in queue - triggering completion workflow..."
        if [ "$CREATE_PR" = true ]; then
          "$SCRIPT_DIR/ralph-worker.sh" --complete --create-pr
        else
          "$SCRIPT_DIR/ralph-worker.sh" --complete
        fi
      fi

      exit 0
    else
      log_warn "Verification failed: Plan has incomplete tasks. Continuing..."
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
exit 1
