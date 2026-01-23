#!/bin/bash
set -e

# Ralph Discovery Loop
# Analyzes codebase for improvement opportunities and creates Beads tasks
# Run daily via cron: 0 6 * * * /path/to/ralph-discover.sh
#
# Usage:
#   ./ralph-discover.sh              # Full discovery
#   ./ralph-discover.sh --dry-run    # Preview without creating tasks
#   ./ralph-discover.sh --category tests  # Only discover missing tests

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Load shared config library
source "$SCRIPT_DIR/lib/config.sh"

# Find project root
PROJECT_ROOT=$(find_project_root)
CONFIG_DIR=$(find_config_dir "$PROJECT_ROOT")

LOG_FILE="$SCRIPT_DIR/discover.log"

# Parse arguments
DRY_RUN=false
CATEGORY=""

while [[ $# -gt 0 ]]; do
  case $1 in
    --dry-run)
      DRY_RUN=true
      shift
      ;;
    --category)
      CATEGORY="$2"
      shift 2
      ;;
    --help|-h)
      echo "Ralph Discovery - Codebase analysis and task creation"
      echo ""
      echo "Usage:"
      echo "  ./ralph-discover.sh [options]"
      echo ""
      echo "Options:"
      echo "  --dry-run            Preview findings without creating tasks"
      echo "  --category TYPE      Only analyze specific category:"
      echo "                       tests, refactor, security, performance, docs"
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

# Check for Beads
if ! command -v bd &> /dev/null; then
  log_error "Error: Beads (bd) is not installed"
  echo ""
  echo "Install via npm:"
  echo "  npm install -g @beads/bd"
  exit 1
fi

# Setup colors
setup_colors

# Get project name from config
PROJECT_NAME=$(config_get "project.name" "$CONFIG_DIR/config.yaml")
PROJECT_NAME=${PROJECT_NAME:-"Project"}

echo -e "${GREEN}========================================"
echo -e "Ralph Discovery Loop"
echo -e "========================================${NC}"
echo ""
echo "Project: $PROJECT_NAME"
echo "Project root: $PROJECT_ROOT"
echo "Dry run: $DRY_RUN"
if [ -n "$CATEGORY" ]; then
  echo "Category filter: $CATEGORY"
fi
echo ""

cd "$PROJECT_ROOT"

# Initialize Beads if not already initialized
if [ ! -d ".beads" ]; then
  log_info "Initializing Beads..."
  bd init --stealth  # Use stealth mode to keep .beads local
fi

# Write context file
cat > "$SCRIPT_DIR/context.json" << EOF
{
  "mode": "discover",
  "projectRoot": "$PROJECT_ROOT",
  "dryRun": $DRY_RUN,
  "category": "$CATEGORY",
  "timestamp": "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
}
EOF

log_info "Starting codebase analysis..."
echo ""

# Build and run prompt
PROMPT=$(build_prompt "$SCRIPT_DIR/prompts/base/discover_prompt.md" "$CONFIG_DIR")
OUTPUT=$(echo "$PROMPT" | claude -p --dangerously-skip-permissions 2>&1 | tee /dev/stderr)

# Log the run
{
  echo "---"
  echo "Timestamp: $(date -u +%Y-%m-%dT%H:%M:%SZ)"
  echo "Dry run: $DRY_RUN"
  echo "Category: ${CATEGORY:-all}"
  echo ""
} >> "$LOG_FILE"

# Check for completion
if echo "$OUTPUT" | grep -q "<promise>DISCOVERY_COMPLETE</promise>"; then
  echo ""
  echo -e "${GREEN}========================================"
  echo -e "Discovery Complete!"
  echo -e "========================================${NC}"
  echo ""

  # Show pending tasks
  log_info "Ready tasks for Ralph Worker:"
  bd ready --json 2>/dev/null | head -20 || echo "No ready tasks"

  rm -f "$SCRIPT_DIR/context.json"
  exit 0
fi

echo ""
log_warn "Discovery finished (check output above)"
rm -f "$SCRIPT_DIR/context.json"
