#!/bin/bash
set -e

# Ralph Update
# Updates Ralph scripts while preserving project configuration
#
# Usage:
#   ./scripts/ralph/ralph-update.sh
#
# Preserved (never overwritten):
#   - .ralph/config.yaml
#   - .ralph/principles.md
#   - .ralph/patterns.md
#   - .ralph/boundaries.md
#   - .ralph/tech-stack.md
#   - scripts/ralph/progress.txt
#
# Updated:
#   - scripts/ralph/*.sh (core scripts)
#   - scripts/ralph/lib/*.sh
#   - scripts/ralph/prompts/base/*.md

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"

RALPH_REPO="${RALPH_REPO:-https://raw.githubusercontent.com/arvesolland/ralph/main}"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

echo -e "${GREEN}========================================"
echo -e "Ralph Update"
echo -e "========================================${NC}"
echo ""
echo "Project root: $PROJECT_ROOT"
echo "Source: $RALPH_REPO"
echo ""

# Function to download a file
download_file() {
  local src="$1"
  local dest="$2"

  curl -fsSL "$RALPH_REPO/$src" -o "$dest" 2>/dev/null || {
    echo -e "${RED}  Failed to download: $src${NC}"
    return 1
  }
}

# Update core scripts
echo -e "${BLUE}Updating core scripts...${NC}"
SCRIPTS=(
  "ralph.sh"
  "ralph-worker.sh"
  "ralph-init.sh"
  "ralph-update.sh"
)

for script in "${SCRIPTS[@]}"; do
  echo -n "  - $script "
  if download_file "$script" "$SCRIPT_DIR/$script"; then
    echo -e "${GREEN}✓${NC}"
  fi
done

# Update lib
echo -e "${BLUE}Updating lib...${NC}"
mkdir -p "$SCRIPT_DIR/lib"
echo -n "  - lib/config.sh "
if download_file "lib/config.sh" "$SCRIPT_DIR/lib/config.sh"; then
  echo -e "${GREEN}✓${NC}"
fi

# Update prompts
echo -e "${BLUE}Updating base prompts...${NC}"
mkdir -p "$SCRIPT_DIR/prompts/base"
PROMPTS=(
  "prompts/base/prompt.md"
  "prompts/base/plan_reviewer_prompt.md"
)

for prompt in "${PROMPTS[@]}"; do
  echo -n "  - $prompt "
  if download_file "$prompt" "$SCRIPT_DIR/$prompt"; then
    echo -e "${GREEN}✓${NC}"
  fi
done

# Make scripts executable
chmod +x "$SCRIPT_DIR/"*.sh
chmod +x "$SCRIPT_DIR/lib/"*.sh 2>/dev/null || true

# Show preserved files
echo ""
echo -e "${YELLOW}Preserved (not modified):${NC}"
echo "  - .ralph/config.yaml"
echo "  - .ralph/principles.md"
echo "  - .ralph/patterns.md"
echo "  - .ralph/boundaries.md"
echo "  - .ralph/tech-stack.md"
echo "  - scripts/ralph/progress.txt"

echo ""
echo -e "${GREEN}Update complete!${NC}"
