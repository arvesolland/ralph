#!/bin/bash
set -e

# Ralph Update
# Updates Ralph scripts while preserving project configuration
#
# Usage:
#   ./scripts/ralph/ralph-update.sh          # Update scripts only
#   ./scripts/ralph/ralph-update.sh --ai     # Also fill in stub config files with AI
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
#   - .claude/skills/ralph-*/ (Claude Code skills)

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
CONFIG_DIR="$PROJECT_ROOT/.ralph"

RALPH_REPO="${RALPH_REPO:-https://raw.githubusercontent.com/arvesolland/ralph/main}"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

# Parse arguments
AI_MODE=false

while [[ $# -gt 0 ]]; do
  case $1 in
    --ai)
      AI_MODE=true
      shift
      ;;
    --help|-h)
      echo "Ralph Update - Update Ralph scripts and optionally regenerate stub configs"
      echo ""
      echo "Usage:"
      echo "  ./ralph-update.sh [options]"
      echo ""
      echo "Options:"
      echo "  --ai     Use AI to fill in stub/placeholder config files"
      echo "  --help   Show this help message"
      echo ""
      echo "The --ai flag will only regenerate files that are stubs (contain TODO)"
      echo "or are missing. Files with real content are preserved."
      exit 0
      ;;
    *)
      echo "Unknown option: $1"
      exit 1
      ;;
  esac
done

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
  "ralph-reverse.sh"
  "ralph-cron.sh"
  "ralph-discover.sh"
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
LIB_FILES=(
  "lib/config.sh"
  "lib/worktree.sh"
)

for lib_file in "${LIB_FILES[@]}"; do
  echo -n "  - $lib_file "
  if download_file "$lib_file" "$SCRIPT_DIR/$lib_file"; then
    echo -e "${GREEN}✓${NC}"
  fi
done

# Update prompts
echo -e "${BLUE}Updating base prompts...${NC}"
mkdir -p "$SCRIPT_DIR/prompts/base"
PROMPTS=(
  "prompts/base/prompt.md"
  "prompts/base/worker_prompt.md"
  "prompts/base/plan_reviewer_prompt.md"
  "prompts/base/plan-spec.md"
  "prompts/base/discover_prompt.md"
  "prompts/base/reverse_discover_prompt.md"
  "prompts/base/reverse_generate_prompt.md"
  "prompts/base/reverse_spec_prompt.md"
)

for prompt in "${PROMPTS[@]}"; do
  echo -n "  - $prompt "
  if download_file "$prompt" "$SCRIPT_DIR/$prompt"; then
    echo -e "${GREEN}✓${NC}"
  fi
done

# Update Claude Code skills
echo -e "${BLUE}Updating Claude Code skills...${NC}"
mkdir -p "$PROJECT_ROOT/.claude/skills/ralph-spec"
mkdir -p "$PROJECT_ROOT/.claude/skills/ralph-plan"
mkdir -p "$PROJECT_ROOT/.claude/skills/ralph-spec-to-plan"

SKILLS=(
  ".claude/skills/ralph-spec/SKILL.md"
  ".claude/skills/ralph-plan/SKILL.md"
  ".claude/skills/ralph-spec-to-plan/SKILL.md"
)

for skill in "${SKILLS[@]}"; do
  echo -n "  - $skill "
  if download_file "$skill" "$PROJECT_ROOT/$skill"; then
    echo -e "${GREEN}✓${NC}"
  fi
done

# Update Slack bot
echo -e "${BLUE}Updating Slack bot...${NC}"
mkdir -p "$SCRIPT_DIR/slack-bot"
SLACK_BOT_FILES=(
  "slack-bot/ralph_slack_bot.py"
  "slack-bot/requirements.txt"
  "slack-bot/README.md"
)

for bot_file in "${SLACK_BOT_FILES[@]}"; do
  echo -n "  - $bot_file "
  if download_file "$bot_file" "$SCRIPT_DIR/$bot_file"; then
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
echo "  - scripts/ralph/slack-bot/.env"

# Check for new config sections to add
echo -e "${BLUE}Checking for new config options...${NC}"

# Check if slack section exists in config.yaml
if [ -f "$CONFIG_DIR/config.yaml" ]; then
  if ! grep -q "^slack:" "$CONFIG_DIR/config.yaml" && ! grep -q "^# slack:" "$CONFIG_DIR/config.yaml"; then
    echo "  - Adding Slack config template to config.yaml"
    cat >> "$CONFIG_DIR/config.yaml" << 'EOF'

# Slack notifications (optional)
# slack:
#   channel: "C0123456789"    # Channel ID (required for notifications)
#   global_bot: true          # Use global bot at ~/.ralph/ (recommended)
#   notify_start: true
#   notify_complete: true
#   notify_blocker: true
#   notify_error: true
# Credentials: Set SLACK_BOT_TOKEN and SLACK_APP_TOKEN in ~/.ralph/slack.env
EOF
  fi

  if ! grep -q "^worktree:" "$CONFIG_DIR/config.yaml" && ! grep -q "^# worktree:" "$CONFIG_DIR/config.yaml"; then
    echo "  - Adding worktree config template to config.yaml"
    cat >> "$CONFIG_DIR/config.yaml" << 'EOF'

# Worktree initialization (runs when creating plan worktrees)
# worktree:
#   copy_env_files: ".env, .env.local"  # Files to copy (default: .env)
#   init_commands: "npm ci"              # Custom commands (skips auto-detection)
# Or create .ralph/hooks/worktree-init executable script for full control
EOF
  fi
fi

# AI mode: fill in stub config files
if [ "$AI_MODE" = true ]; then
  echo ""
  echo -e "${BLUE}Checking config files for stubs...${NC}"

  # Check which files need AI generation
  NEEDS_AI=()

  is_stub_file() {
    local file="$1"
    if [ ! -f "$file" ]; then
      return 0  # Missing = needs AI
    fi
    # Check if file contains TODO or is very short (< 200 chars excluding whitespace)
    if grep -q "TODO:" "$file" 2>/dev/null; then
      return 0  # Contains TODO = stub
    fi
    local content_size=$(cat "$file" | tr -d '[:space:]' | wc -c)
    if [ "$content_size" -lt 100 ]; then
      return 0  # Very short = likely stub
    fi
    return 1  # Has real content
  }

  for file in "principles.md" "patterns.md" "boundaries.md" "tech-stack.md"; do
    if is_stub_file "$CONFIG_DIR/$file"; then
      NEEDS_AI+=("$file")
      echo -e "  ${YELLOW}$file - stub/missing, will regenerate${NC}"
    else
      echo -e "  ${GREEN}$file - has content, keeping${NC}"
    fi
  done

  if [ ${#NEEDS_AI[@]} -eq 0 ]; then
    echo ""
    echo -e "${GREEN}All config files have content. Nothing to regenerate.${NC}"
  else
    echo ""
    echo -e "${BLUE}Running AI to generate ${#NEEDS_AI[@]} file(s)...${NC}"

    # Check for Claude
    if ! command -v claude &> /dev/null; then
      echo -e "${RED}Error: Claude Code CLI not found${NC}"
      echo "Install from: https://github.com/anthropics/claude-code"
      exit 1
    fi

    # Build file list for prompt
    FILES_TO_GENERATE=$(printf ", %s" "${NEEDS_AI[@]}")
    FILES_TO_GENERATE=${FILES_TO_GENERATE:2}  # Remove leading ", "

    # Create AI prompt
    AI_PROMPT=$(cat << PROMPT_EOF
You are analyzing a codebase to configure Ralph, an AI development agent.

## Your Task

Analyze this codebase and generate ONLY these specific files: $FILES_TO_GENERATE

Other config files already exist and have content - do NOT generate them.

## What to Analyze

1. Look for existing documentation (README.md, CLAUDE.md, CONTRIBUTING.md, etc.)
2. Examine package.json, composer.json, pyproject.toml, etc. for dependencies
3. Look at the code structure and patterns used
4. Check for existing tests and their patterns
5. Identify any security-sensitive files or patterns

## Output Format

Generate ONLY the files listed above. Output raw content directly after each marker - DO NOT wrap in markdown code blocks.

PROMPT_EOF
)

    # Add file-specific instructions
    for file in "${NEEDS_AI[@]}"; do
      case $file in
        principles.md)
          AI_PROMPT+=$'\n\n---FILE: .ralph/principles.md---\n(Development principles specific to this codebase - raw markdown)'
          ;;
        patterns.md)
          AI_PROMPT+=$'\n\n---FILE: .ralph/patterns.md---\n(Code patterns with examples from this codebase - raw markdown)'
          ;;
        boundaries.md)
          AI_PROMPT+=$'\n\n---FILE: .ralph/boundaries.md---\n(Files/directories to never modify - raw markdown list)'
          ;;
        tech-stack.md)
          AI_PROMPT+=$'\n\n---FILE: .ralph/tech-stack.md---\n(Technology stack description - raw markdown)'
          ;;
      esac
    done

    AI_PROMPT+=$'\n\n## Guidelines\n\n- Be SPECIFIC to this codebase, not generic advice\n- Include actual patterns you see in the code\n- Reference real files and directories\n- Keep each file focused and concise\n- Output raw file content directly - NO ```markdown wrappers\n\nStart your analysis now.'

    # Run Claude
    OUTPUT=$(echo "$AI_PROMPT" | claude -p --dangerously-skip-permissions 2>&1 | tee /dev/stderr)

    # Parse and write files
    echo ""
    echo -e "${BLUE}Saving generated files...${NC}"

    extract_file() {
      local marker="$1"
      local output="$2"
      echo "$output" | awk -v marker="$marker" '
        $0 ~ marker { found=1; next }
        found && /^---FILE:/ { exit }
        found { print }
      ' | sed '/^```\(markdown\|yaml\|yml\)\?$/d' | sed '/^```$/d'
    }

    for file in "${NEEDS_AI[@]}"; do
      content=$(extract_file "---FILE: .ralph/$file---" "$OUTPUT")
      if [ -n "$content" ] && [ ${#content} -gt 10 ]; then
        echo "$content" > "$CONFIG_DIR/$file"
        echo -e "  ${GREEN}$file regenerated${NC}"
      else
        echo -e "  ${YELLOW}$file - no content generated, keeping existing${NC}"
      fi
    done
  fi
fi

echo ""
echo -e "${GREEN}Update complete!${NC}"
