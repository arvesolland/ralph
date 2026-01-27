#!/bin/bash
set -e

# Ralph Reverse - Codebase-to-Specs Loop
# Analyzes an existing codebase and creates Ralph specs for discovered features
#
# Usage:
#   ./ralph-reverse.sh --discover              # Phase 1: Iterative discovery
#   ./ralph-reverse.sh --generate-plan         # Phase 2: Generate spec-writing plan
#   ./ralph-reverse.sh --auto                  # Run all phases automatically
#   ./ralph-reverse.sh --feature auth          # Single feature mode

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Load shared config library
source "$SCRIPT_DIR/lib/config.sh"

# Find project root
PROJECT_ROOT=$(find_project_root)
CONFIG_DIR=$(find_config_dir "$PROJECT_ROOT")

# Paths
PLANS_DIR="$PROJECT_ROOT/plans"
SPECS_DIR="$PROJECT_ROOT/specs"
DISCOVERY_FILE="$PLANS_DIR/current/reverse-discovery.md"
DISCOVERY_PROGRESS="$PLANS_DIR/current/reverse-discovery.progress.md"

# Defaults
MODE=""
MAX_ITERATIONS=20
SINGLE_FEATURE=""
SINGLE_FEATURE_PATH=""
DRY_RUN=false

# Parse arguments
while [[ $# -gt 0 ]]; do
  case $1 in
    --discover|-d)
      MODE="discover"
      shift
      ;;
    --generate-plan|-g)
      MODE="generate"
      shift
      ;;
    --auto|-a)
      MODE="auto"
      shift
      ;;
    --feature|-f)
      MODE="single"
      SINGLE_FEATURE="$2"
      shift 2
      ;;
    --path|-p)
      SINGLE_FEATURE_PATH="$2"
      shift 2
      ;;
    --max|-m)
      MAX_ITERATIONS="$2"
      shift 2
      ;;
    --dry-run)
      DRY_RUN=true
      shift
      ;;
    --version|-v)
      echo "Ralph Reverse v$(get_ralph_version "$SCRIPT_DIR")"
      exit 0
      ;;
    --help|-h)
      echo "Ralph Reverse - Codebase to Specs Generator"
      echo ""
      echo "Usage:"
      echo "  ./ralph-reverse.sh --discover              Iterative feature discovery"
      echo "  ./ralph-reverse.sh --generate-plan         Generate spec-writing plan"
      echo "  ./ralph-reverse.sh --auto                  Run discovery + generate + worker"
      echo "  ./ralph-reverse.sh --feature NAME          Create spec for single feature"
      echo ""
      echo "Options:"
      echo "  --discover, -d         Run iterative discovery loop"
      echo "  --generate-plan, -g    Generate plan from discovery doc"
      echo "  --auto, -a             Run full pipeline automatically"
      echo "  --feature, -f NAME     Single feature mode"
      echo "  --path, -p PATH        Path hint for single feature (with -f)"
      echo "  --max, -m N            Max discovery iterations (default: 20)"
      echo "  --dry-run              Preview without creating files"
      echo "  --version, -v          Show version"
      echo "  --help, -h             Show this help"
      echo ""
      echo "Workflow:"
      echo "  1. Run --discover to analyze codebase (iterates until ready)"
      echo "  2. Review/edit plans/current/reverse-discovery.md"
      echo "  3. Run --generate-plan to create spec-writing tasks"
      echo "  4. Run ralph-worker.sh to execute tasks"
      echo ""
      echo "Examples:"
      echo "  ./ralph-reverse.sh --discover"
      echo "  ./ralph-reverse.sh --generate-plan"
      echo "  ./ralph-reverse.sh --auto  # Does all steps"
      echo "  ./ralph-reverse.sh --feature auth --path src/auth/"
      exit 0
      ;;
    *)
      echo "Unknown option: $1"
      echo "Use --help for usage information"
      exit 1
      ;;
  esac
done

# Require mode
if [ -z "$MODE" ]; then
  log_error "Error: Mode required (--discover, --generate-plan, --auto, or --feature)"
  echo "Use --help for usage information"
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

echo -e "${GREEN}========================================"
echo -e "Ralph Reverse - Codebase to Specs"
echo -e "========================================${NC}"
echo ""
echo "Project: $PROJECT_NAME"
echo "Project root: $PROJECT_ROOT"
echo "Mode: $MODE"
if [ "$DRY_RUN" = true ]; then
  echo -e "Dry run: ${YELLOW}enabled${NC}"
fi
echo ""

cd "$PROJECT_ROOT"

# Ensure directory structure exists
mkdir -p "$PLANS_DIR/pending" "$PLANS_DIR/current" "$PLANS_DIR/complete"
mkdir -p "$SPECS_DIR"

# ============================================
# Mode: Single Feature
# ============================================
if [ "$MODE" = "single" ]; then
  if [ -z "$SINGLE_FEATURE" ]; then
    log_error "Error: Feature name required with --feature"
    exit 1
  fi

  echo -e "${BLUE}Single Feature Mode: $SINGLE_FEATURE${NC}"
  echo ""

  SPEC_DIR="$SPECS_DIR/$SINGLE_FEATURE"
  SPEC_FILE="$SPEC_DIR/SPEC.md"

  if [ -f "$SPEC_FILE" ]; then
    log_warn "Spec already exists: $SPEC_FILE"
    echo "Use a different name or delete existing spec first."
    exit 1
  fi

  # Write context for single feature spec generation
  cat > "$SCRIPT_DIR/context.json" << EOF
{
  "mode": "single-feature",
  "featureName": "$SINGLE_FEATURE",
  "featurePath": "$SINGLE_FEATURE_PATH",
  "specDir": "$SPEC_DIR",
  "specFile": "$SPEC_FILE",
  "projectRoot": "$PROJECT_ROOT",
  "dryRun": $DRY_RUN,
  "timestamp": "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
}
EOF

  if [ "$DRY_RUN" = true ]; then
    echo "Dry run - would create spec for: $SINGLE_FEATURE"
    echo "Path hint: ${SINGLE_FEATURE_PATH:-"(none, will auto-detect)"}"
    rm -f "$SCRIPT_DIR/context.json"
    exit 0
  fi

  # Build and run prompt
  PROMPT=$(build_prompt "$SCRIPT_DIR/prompts/base/reverse_spec_prompt.md" "$CONFIG_DIR")
  OUTPUT=$(echo "$PROMPT" | claude -p --dangerously-skip-permissions 2>&1 | tee /dev/stderr) || true

  if [ -f "$SPEC_FILE" ]; then
    log_success "Spec created: $SPEC_FILE"
  else
    log_warn "Spec file not created - check output above"
  fi

  rm -f "$SCRIPT_DIR/context.json"
  exit 0
fi

# ============================================
# Mode: Discovery
# ============================================
if [ "$MODE" = "discover" ] || [ "$MODE" = "auto" ]; then
  echo -e "${BLUE}========================================"
  echo -e "Phase 1: Iterative Discovery"
  echo -e "========================================${NC}"
  echo ""

  # Check if discovery already in progress
  if [ -f "$DISCOVERY_FILE" ]; then
    # Check status
    CURRENT_STATUS=$(grep '^\*\*Status:\*\*' "$DISCOVERY_FILE" 2>/dev/null | sed 's/.*\*\* //' | tr -d '[:space:]' || echo "unknown")
    if [ "$CURRENT_STATUS" = "ready" ]; then
      log_success "Discovery already complete (status: ready)"
      echo "Discovery file: $DISCOVERY_FILE"
      echo ""
      if [ "$MODE" = "discover" ]; then
        echo "Next step: ./ralph-reverse.sh --generate-plan"
        exit 0
      fi
      # For auto mode, continue to plan generation
    else
      echo "Resuming existing discovery (status: $CURRENT_STATUS)"
      CURRENT_ITERATION=$(grep '^\*\*Iteration:\*\*' "$DISCOVERY_FILE" 2>/dev/null | sed 's/.*\*\* //' | tr -d '[:space:]' || echo "0")
      echo "Current iteration: $CURRENT_ITERATION"
    fi
  else
    echo "Starting new discovery..."
    CURRENT_ITERATION=0
  fi
  echo ""

  # Discovery loop
  for i in $(seq 1 $MAX_ITERATIONS); do
    ITERATION=$((CURRENT_ITERATION + i))

    echo "========================================"
    echo "Discovery Iteration $ITERATION"
    echo "========================================"
    echo ""

    # Dry run - just show what would happen
    if [ "$DRY_RUN" = true ]; then
      echo "Dry run - would run discovery iteration $ITERATION"
      echo "Discovery file: $DISCOVERY_FILE"
      echo "Progress file: $DISCOVERY_PROGRESS"
      echo ""
      echo "To run for real, remove --dry-run flag"
      rm -f "$SCRIPT_DIR/context.json"
      exit 0
    fi

    # Write context
    cat > "$SCRIPT_DIR/context.json" << EOF
{
  "mode": "discover",
  "discoveryFile": "$DISCOVERY_FILE",
  "progressFile": "$DISCOVERY_PROGRESS",
  "specsDir": "$SPECS_DIR",
  "projectRoot": "$PROJECT_ROOT",
  "iteration": $ITERATION,
  "maxIterations": $MAX_ITERATIONS,
  "dryRun": $DRY_RUN,
  "timestamp": "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
}
EOF

    # Build and run prompt
    PROMPT=$(build_prompt "$SCRIPT_DIR/prompts/base/reverse_discover_prompt.md" "$CONFIG_DIR")
    OUTPUT=$(echo "$PROMPT" | claude -p --dangerously-skip-permissions 2>&1 | tee /dev/stderr) || true

    # Check for completion
    if echo "$OUTPUT" | grep -q "<promise>DISCOVERY_READY</promise>"; then
      echo ""
      log_success "Discovery complete!"

      # Update status in file
      if [ -f "$DISCOVERY_FILE" ]; then
        sed -i '' 's/^\*\*Status:\*\* .*/\*\*Status:\*\* ready/' "$DISCOVERY_FILE" 2>/dev/null || true
      fi

      echo ""
      echo "Discovery file: $DISCOVERY_FILE"

      if [ "$MODE" = "discover" ]; then
        echo ""
        echo "Next steps:"
        echo "  1. Review the discovery document"
        echo "  2. Run: ./ralph-reverse.sh --generate-plan"
        rm -f "$SCRIPT_DIR/context.json"
        exit 0
      fi
      break
    fi

    echo ""
    echo "Discovery not yet complete - continuing..."
    echo "Cooling down before next iteration..."
    sleep 3
  done

  # Check if we exited loop without completion
  if [ "$MODE" = "discover" ]; then
    FINAL_STATUS=$(grep '^\*\*Status:\*\*' "$DISCOVERY_FILE" 2>/dev/null | sed 's/.*\*\* //' | tr -d '[:space:]' || echo "unknown")
    if [ "$FINAL_STATUS" != "ready" ]; then
      echo ""
      log_warn "Max iterations ($MAX_ITERATIONS) reached"
      echo "Discovery may be incomplete. Review: $DISCOVERY_FILE"
      echo "Re-run with higher --max or manually set Status: ready"
      rm -f "$SCRIPT_DIR/context.json"
      exit 1
    fi
  fi
fi

# ============================================
# Mode: Generate Plan
# ============================================
if [ "$MODE" = "generate" ] || [ "$MODE" = "auto" ]; then
  echo -e "${BLUE}========================================"
  echo -e "Phase 2: Generate Spec-Writing Plan"
  echo -e "========================================${NC}"
  echo ""

  # Check discovery file exists and is ready
  if [ ! -f "$DISCOVERY_FILE" ]; then
    log_error "Discovery file not found: $DISCOVERY_FILE"
    echo "Run --discover first"
    exit 1
  fi

  DISCOVERY_STATUS=$(grep '^\*\*Status:\*\*' "$DISCOVERY_FILE" 2>/dev/null | sed 's/.*\*\* //' | tr -d '[:space:]' || echo "unknown")
  if [ "$DISCOVERY_STATUS" != "ready" ]; then
    log_warn "Discovery status is '$DISCOVERY_STATUS', not 'ready'"
    echo "Continue anyway? (y/N)"
    read -r response
    if [[ ! "$response" =~ ^[Yy]$ ]]; then
      exit 1
    fi
  fi

  # Generate timestamp for plan file
  TIMESTAMP=$(date -u +%Y%m%d-%H%M%S)
  PLAN_FILE="$PLANS_DIR/pending/reverse-specs-$TIMESTAMP.md"
  PLAN_PROGRESS="$PLANS_DIR/pending/reverse-specs-$TIMESTAMP.progress.md"

  echo "Generating plan from discovery document..."
  echo "Discovery file: $DISCOVERY_FILE"
  echo "Plan file: $PLAN_FILE"
  echo ""

  # Write context
  cat > "$SCRIPT_DIR/context.json" << EOF
{
  "mode": "generate-plan",
  "discoveryFile": "$DISCOVERY_FILE",
  "planFile": "$PLAN_FILE",
  "progressFile": "$PLAN_PROGRESS",
  "specsDir": "$SPECS_DIR",
  "projectRoot": "$PROJECT_ROOT",
  "dryRun": $DRY_RUN,
  "timestamp": "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
}
EOF

  if [ "$DRY_RUN" = true ]; then
    echo "Dry run - would generate plan at: $PLAN_FILE"
    rm -f "$SCRIPT_DIR/context.json"
    exit 0
  fi

  # Build and run prompt (uses the generate section of reverse_discover_prompt)
  PROMPT=$(build_prompt "$SCRIPT_DIR/prompts/base/reverse_generate_prompt.md" "$CONFIG_DIR")
  OUTPUT=$(echo "$PROMPT" | claude -p --dangerously-skip-permissions 2>&1 | tee /dev/stderr) || true

  if [ -f "$PLAN_FILE" ]; then
    log_success "Plan generated: $PLAN_FILE"
    echo ""

    # Count tasks
    TASK_COUNT=$(grep -c "^### T[0-9]" "$PLAN_FILE" 2>/dev/null || echo "0")
    echo "Tasks created: $TASK_COUNT"

    if [ "$MODE" = "generate" ]; then
      echo ""
      echo "Next step: ./ralph-worker.sh --loop"
    fi
  else
    log_error "Plan file not created"
    rm -f "$SCRIPT_DIR/context.json"
    exit 1
  fi

  rm -f "$SCRIPT_DIR/context.json"
fi

# ============================================
# Mode: Auto (continue to worker)
# ============================================
if [ "$MODE" = "auto" ]; then
  echo ""
  echo -e "${BLUE}========================================"
  echo -e "Phase 3: Execute Plan"
  echo -e "========================================${NC}"
  echo ""

  echo "Starting ralph-worker to process spec-writing tasks..."
  echo ""

  exec "$SCRIPT_DIR/ralph-worker.sh" --loop
fi

echo ""
log_success "Ralph Reverse complete"
