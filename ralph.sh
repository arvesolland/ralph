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

echo -e "${GREEN}Ralph - Implementation Loop${NC}"
echo "========================================"
echo "Project: $PROJECT_NAME"
echo "Project root: $PROJECT_ROOT"
echo "Plan file: $PLAN_FILE"
if [ "$REVIEW_PLAN" = true ]; then
  echo -e "Plan review: ${YELLOW}enabled ($REVIEW_PASSES passes)${NC}"
fi
echo "Max iterations: $MAX_ITERATIONS"
echo ""

cd "$PROJECT_ROOT"

# ============================================
# Phase 1: Plan Review (if --review-plan)
# ============================================
if [ "$REVIEW_PLAN" = true ]; then
  echo -e "${BLUE}========================================"
  echo -e "Phase 1: Plan Review"
  echo -e "========================================${NC}"
  echo ""

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

    # Build and run prompt
    PROMPT=$(build_prompt "$SCRIPT_DIR/prompts/base/plan_reviewer_prompt.md" "$CONFIG_DIR")
    OUTPUT=$(echo "$PROMPT" | claude -p --dangerously-skip-permissions 2>&1 | tee /dev/stderr) || true

    echo ""

    if [ "$i" -lt "$REVIEW_PASSES" ]; then
      echo "Cooling down before next review pass..."
      sleep 2
    fi
  done

  echo ""
  log_success "Plan review complete"
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
  "iteration": $i,
  "maxIterations": $MAX_ITERATIONS
}
EOF

  # Build and run prompt
  PROMPT=$(build_prompt "$SCRIPT_DIR/prompts/base/prompt.md" "$CONFIG_DIR")
  OUTPUT=$(echo "$PROMPT" | claude -p --dangerously-skip-permissions 2>&1 | tee /dev/stderr) || true

  if echo "$OUTPUT" | grep -q "<promise>COMPLETE</promise>"; then
    echo ""
    log_success "All tasks complete!"
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
