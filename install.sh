#!/bin/bash
set -e

# Ralph Installer
# Downloads and installs Ralph scripts into the current project
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/USER/ralph/main/install.sh | bash
#   curl -fsSL https://raw.githubusercontent.com/USER/ralph/main/install.sh | bash -s -- --ai
#
# Or from local clone:
#   ./install.sh
#   ./install.sh --ai

# Configuration - update this to your repo URL
RALPH_REPO="${RALPH_REPO:-https://raw.githubusercontent.com/arvesolland/ralph/main}"
RALPH_VERSION="${RALPH_VERSION:-main}"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

# Parse arguments
AI_INIT=false
LOCAL_INSTALL=false

while [[ $# -gt 0 ]]; do
  case $1 in
    --ai)
      AI_INIT=true
      shift
      ;;
    --local)
      LOCAL_INSTALL=true
      shift
      ;;
    --help|-h)
      echo "Ralph Installer"
      echo ""
      echo "Usage:"
      echo "  ./install.sh [options]"
      echo ""
      echo "Options:"
      echo "  --ai       Run AI-assisted configuration after install"
      echo "  --local    Install from local directory (for development)"
      echo "  --help     Show this help message"
      exit 0
      ;;
    *)
      echo "Unknown option: $1"
      exit 1
      ;;
  esac
done

# Find project root
find_project_root() {
  git rev-parse --show-toplevel 2>/dev/null || pwd
}

PROJECT_ROOT=$(find_project_root)
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

echo -e "${GREEN}========================================"
echo -e "Ralph Installer"
echo -e "========================================${NC}"
echo ""
echo "Project root: $PROJECT_ROOT"
echo ""

# Create directory structure
echo -e "${BLUE}Creating directory structure...${NC}"
mkdir -p "$PROJECT_ROOT/scripts/ralph/lib"
mkdir -p "$PROJECT_ROOT/scripts/ralph/prompts/base"
mkdir -p "$PROJECT_ROOT/.ralph"

# Function to copy or download a file
get_file() {
  local src="$1"
  local dest="$2"

  if [ "$LOCAL_INSTALL" = true ]; then
    # Copy from local source
    if [ -f "$SCRIPT_DIR/$src" ]; then
      cp "$SCRIPT_DIR/$src" "$dest"
    else
      echo -e "${RED}Error: Local file not found: $SCRIPT_DIR/$src${NC}"
      return 1
    fi
  else
    # Download from remote
    curl -fsSL "$RALPH_REPO/$src" -o "$dest" 2>/dev/null || {
      echo -e "${RED}Error: Failed to download $src${NC}"
      return 1
    }
  fi
}

# Install core scripts
echo -e "${BLUE}Installing core scripts...${NC}"
SCRIPTS=(
  "ralph.sh"
  "ralph-init.sh"
  "ralph-update.sh"
  "lib/config.sh"
)

for script in "${SCRIPTS[@]}"; do
  echo "  - $script"
  get_file "$script" "$PROJECT_ROOT/scripts/ralph/$script"
done

# Make scripts executable
chmod +x "$PROJECT_ROOT/scripts/ralph/"*.sh
chmod +x "$PROJECT_ROOT/scripts/ralph/lib/"*.sh 2>/dev/null || true

# Install base prompts
echo -e "${BLUE}Installing base prompts...${NC}"
PROMPTS=(
  "prompts/base/prompt.md"
  "prompts/base/plan_reviewer_prompt.md"
)

for prompt in "${PROMPTS[@]}"; do
  echo "  - $prompt"
  get_file "$prompt" "$PROJECT_ROOT/scripts/ralph/$prompt"
done

# Create config stubs if they don't exist
echo -e "${BLUE}Creating configuration files...${NC}"

if [ ! -f "$PROJECT_ROOT/.ralph/config.yaml" ]; then
  cat > "$PROJECT_ROOT/.ralph/config.yaml" << 'EOF'
# Ralph Configuration
# Edit this file to customize Ralph for your project

project:
  name: "My Project"
  description: "A brief description of your project"

# Git settings
git:
  base_branch: "main"

# Commands for validation
commands:
  test: "npm test"
  lint: "npm run lint"
  build: "npm run build"
  dev: "npm run dev"
EOF
  echo "  - .ralph/config.yaml (created)"
else
  echo "  - .ralph/config.yaml (exists, skipped)"
fi

if [ ! -f "$PROJECT_ROOT/.ralph/principles.md" ]; then
  cat > "$PROJECT_ROOT/.ralph/principles.md" << 'EOF'
# Project Principles

Add your project's core development principles here. These will be injected into all Ralph prompts.

## Examples

- Keep functions small and focused
- Write tests for all new features
- Prefer composition over inheritance
- Never commit secrets or credentials
EOF
  echo "  - .ralph/principles.md (created)"
else
  echo "  - .ralph/principles.md (exists, skipped)"
fi

if [ ! -f "$PROJECT_ROOT/.ralph/patterns.md" ]; then
  cat > "$PROJECT_ROOT/.ralph/patterns.md" << 'EOF'
# Code Patterns

Document the coding patterns used in this project. These will be injected into Ralph prompts.

## Examples

- Use `async/await` instead of callbacks
- Follow naming convention: `camelCase` for functions, `PascalCase` for classes
- Error handling: always use try/catch with proper error types
EOF
  echo "  - .ralph/patterns.md (created)"
else
  echo "  - .ralph/patterns.md (exists, skipped)"
fi

if [ ! -f "$PROJECT_ROOT/.ralph/boundaries.md" ]; then
  cat > "$PROJECT_ROOT/.ralph/boundaries.md" << 'EOF'
# Boundaries

List files and directories that Ralph should NEVER modify.

## Examples

- `*.lock` files (package-lock.json, yarn.lock, etc.)
- `node_modules/`
- `.env` and other secrets
- Third-party code in `vendor/`
EOF
  echo "  - .ralph/boundaries.md (created)"
else
  echo "  - .ralph/boundaries.md (exists, skipped)"
fi

if [ ! -f "$PROJECT_ROOT/.ralph/tech-stack.md" ]; then
  cat > "$PROJECT_ROOT/.ralph/tech-stack.md" << 'EOF'
# Tech Stack

Describe your project's technology stack. This helps Ralph understand the context.

## Examples

**Language:** TypeScript
**Framework:** Next.js 14
**Database:** PostgreSQL with Prisma ORM
**Testing:** Jest + React Testing Library
**Styling:** Tailwind CSS
EOF
  echo "  - .ralph/tech-stack.md (created)"
else
  echo "  - .ralph/tech-stack.md (exists, skipped)"
fi

# Create progress.txt if it doesn't exist
if [ ! -f "$PROJECT_ROOT/scripts/ralph/progress.txt" ]; then
  cat > "$PROJECT_ROOT/scripts/ralph/progress.txt" << 'EOF'
# Ralph Progress Log

## Codebase Patterns

(Patterns discovered during implementation will be added here)

---

# Task History

(Completed tasks will be logged below)
EOF
  echo "  - scripts/ralph/progress.txt (created)"
fi

# Add to .gitignore
echo -e "${BLUE}Updating .gitignore...${NC}"
GITIGNORE="$PROJECT_ROOT/.gitignore"
IGNORE_ENTRIES=(
  "scripts/ralph/context.json"
  "scripts/ralph/.current_task.md"
  "scripts/ralph/.worker.lock"
)

for entry in "${IGNORE_ENTRIES[@]}"; do
  if ! grep -qxF "$entry" "$GITIGNORE" 2>/dev/null; then
    echo "$entry" >> "$GITIGNORE"
    echo "  - Added: $entry"
  fi
done

echo ""
echo -e "${GREEN}========================================"
echo -e "Installation Complete!"
echo -e "========================================${NC}"
echo ""
echo "Installed to: $PROJECT_ROOT/scripts/ralph/"
echo ""
echo -e "${YELLOW}Next steps:${NC}"
echo ""
echo "1. Edit configuration files in .ralph/:"
echo "   - config.yaml    - Project settings"
echo "   - principles.md  - Development principles"
echo "   - patterns.md    - Code patterns"
echo "   - boundaries.md  - Files to never touch"
echo "   - tech-stack.md  - Technology description"
echo ""
echo "2. Or run AI-assisted setup:"
echo "   ./scripts/ralph/ralph-init.sh --ai"
echo ""
echo "3. Create a plan file and run Ralph:"
echo "   ./scripts/ralph/ralph.sh docs/my-plan.md"
echo ""

# Run AI init if requested
if [ "$AI_INIT" = true ]; then
  echo -e "${BLUE}Running AI-assisted configuration...${NC}"
  echo ""
  "$PROJECT_ROOT/scripts/ralph/ralph-init.sh" --ai --force
fi
