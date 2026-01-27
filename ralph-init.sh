#!/bin/bash
set -e

# Ralph Init
# Initialize or reconfigure Ralph for a project
#
# Usage:
#   ./ralph-init.sh           # Interactive setup with stubs
#   ./ralph-init.sh --ai      # AI-assisted configuration
#   ./ralph-init.sh --detect  # Auto-detect project settings

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Load shared config library if available
if [ -f "$SCRIPT_DIR/lib/config.sh" ]; then
  source "$SCRIPT_DIR/lib/config.sh"
else
  # Minimal color setup if lib not available
  RED='\033[0;31m'
  GREEN='\033[0;32m'
  YELLOW='\033[1;33m'
  BLUE='\033[0;34m'
  NC='\033[0m'
fi

# Find project root
find_project_root() {
  git rev-parse --show-toplevel 2>/dev/null || pwd
}

PROJECT_ROOT=$(find_project_root)
CONFIG_DIR="$PROJECT_ROOT/.ralph"

# Parse arguments
AI_MODE=false
DETECT_MODE=false
FORCE=false

while [[ $# -gt 0 ]]; do
  case $1 in
    --ai)
      AI_MODE=true
      shift
      ;;
    --detect)
      DETECT_MODE=true
      shift
      ;;
    --force|-f)
      FORCE=true
      shift
      ;;
    --help|-h)
      echo "Ralph Init - Configure Ralph for your project"
      echo ""
      echo "Usage:"
      echo "  ./ralph-init.sh [options]"
      echo ""
      echo "Options:"
      echo "  --ai       Use Claude to analyze codebase and generate config"
      echo "  --detect   Auto-detect project settings (no AI)"
      echo "  --force    Overwrite existing configuration"
      echo "  --help     Show this help message"
      echo ""
      echo "Examples:"
      echo "  ./ralph-init.sh --ai          # Full AI analysis"
      echo "  ./ralph-init.sh --detect      # Quick auto-detection"
      exit 0
      ;;
    *)
      echo "Unknown option: $1"
      exit 1
      ;;
  esac
done

echo -e "${GREEN}========================================"
echo -e "Ralph Init"
echo -e "========================================${NC}"
echo ""
echo "Project root: $PROJECT_ROOT"
echo ""

# Create config directory
mkdir -p "$CONFIG_DIR"

# Auto-detect project type
detect_project() {
  local project_name=""
  local language=""
  local framework=""
  local test_cmd=""
  local lint_cmd=""
  local build_cmd=""
  local dev_cmd=""

  # Detect project name from directory or package.json
  if [ -f "$PROJECT_ROOT/package.json" ]; then
    project_name=$(grep '"name"' "$PROJECT_ROOT/package.json" 2>/dev/null | head -1 | sed 's/.*"name".*"\([^"]*\)".*/\1/')
  fi
  project_name=${project_name:-$(basename "$PROJECT_ROOT")}

  # Detect by package manager / config files
  if [ -f "$PROJECT_ROOT/package.json" ]; then
    language="JavaScript/TypeScript"

    # Detect framework
    if grep -q '"next"' "$PROJECT_ROOT/package.json" 2>/dev/null; then
      framework="Next.js"
      dev_cmd="npm run dev"
    elif grep -q '"react"' "$PROJECT_ROOT/package.json" 2>/dev/null; then
      framework="React"
      dev_cmd="npm start"
    elif grep -q '"vue"' "$PROJECT_ROOT/package.json" 2>/dev/null; then
      framework="Vue.js"
      dev_cmd="npm run dev"
    elif grep -q '"express"' "$PROJECT_ROOT/package.json" 2>/dev/null; then
      framework="Express.js"
      dev_cmd="npm start"
    fi

    # Detect test/lint commands
    if grep -q '"test"' "$PROJECT_ROOT/package.json" 2>/dev/null; then
      test_cmd="npm test"
    fi
    if grep -q '"lint"' "$PROJECT_ROOT/package.json" 2>/dev/null; then
      lint_cmd="npm run lint"
    fi
    if grep -q '"build"' "$PROJECT_ROOT/package.json" 2>/dev/null; then
      build_cmd="npm run build"
    fi

    # TypeScript detection
    if [ -f "$PROJECT_ROOT/tsconfig.json" ]; then
      language="TypeScript"
    fi

  elif [ -f "$PROJECT_ROOT/composer.json" ]; then
    language="PHP"

    # Detect framework
    if grep -q '"laravel/framework"' "$PROJECT_ROOT/composer.json" 2>/dev/null; then
      framework="Laravel"
      dev_cmd="php artisan serve"
      test_cmd="php artisan test"
    elif grep -q '"symfony/framework-bundle"' "$PROJECT_ROOT/composer.json" 2>/dev/null; then
      framework="Symfony"
      test_cmd="./bin/phpunit"
    fi

    # Detect pint
    if [ -f "$PROJECT_ROOT/vendor/bin/pint" ]; then
      lint_cmd="./vendor/bin/pint"
    elif [ -f "$PROJECT_ROOT/vendor/bin/phpcs" ]; then
      lint_cmd="./vendor/bin/phpcs"
    fi

  elif [ -f "$PROJECT_ROOT/requirements.txt" ] || [ -f "$PROJECT_ROOT/pyproject.toml" ]; then
    language="Python"

    # Detect framework
    if grep -q 'django' "$PROJECT_ROOT/requirements.txt" 2>/dev/null; then
      framework="Django"
      dev_cmd="python manage.py runserver"
      test_cmd="python manage.py test"
    elif grep -q 'fastapi' "$PROJECT_ROOT/requirements.txt" 2>/dev/null; then
      framework="FastAPI"
      dev_cmd="uvicorn main:app --reload"
      test_cmd="pytest"
    elif grep -q 'flask' "$PROJECT_ROOT/requirements.txt" 2>/dev/null; then
      framework="Flask"
      test_cmd="pytest"
    else
      test_cmd="pytest"
    fi

    lint_cmd="ruff check ."

  elif [ -f "$PROJECT_ROOT/go.mod" ]; then
    language="Go"
    test_cmd="go test ./..."
    lint_cmd="golangci-lint run"
    build_cmd="go build"

  elif [ -f "$PROJECT_ROOT/Cargo.toml" ]; then
    language="Rust"
    test_cmd="cargo test"
    lint_cmd="cargo clippy"
    build_cmd="cargo build"
  fi

  # Output detected values
  echo "PROJECT_NAME=\"$project_name\""
  echo "LANGUAGE=\"$language\""
  echo "FRAMEWORK=\"$framework\""
  echo "TEST_CMD=\"$test_cmd\""
  echo "LINT_CMD=\"$lint_cmd\""
  echo "BUILD_CMD=\"$build_cmd\""
  echo "DEV_CMD=\"$dev_cmd\""
}

# Run detection
echo -e "${BLUE}Detecting project settings...${NC}"
eval "$(detect_project)"

echo "  Project: $PROJECT_NAME"
echo "  Language: ${LANGUAGE:-Unknown}"
echo "  Framework: ${FRAMEWORK:-None detected}"
echo ""

if [ "$AI_MODE" = true ]; then
  # AI-assisted configuration
  echo -e "${BLUE}Running AI-assisted configuration...${NC}"
  echo ""
  echo "Claude will analyze your codebase and generate:"
  echo "  - principles.md (development principles)"
  echo "  - patterns.md (code patterns to follow)"
  echo "  - boundaries.md (files to never modify)"
  echo "  - tech-stack.md (technology description)"
  echo "  - config.yaml (project settings)"
  echo ""

  # Check for Claude
  if ! command -v claude &> /dev/null; then
    echo -e "${RED}Error: Claude Code CLI not found${NC}"
    echo "Install from: https://github.com/anthropics/claude-code"
    exit 1
  fi

  # Create AI prompt
  AI_PROMPT=$(cat << 'PROMPT_EOF'
You are analyzing a codebase to configure Ralph, an AI development agent.

## Your Task

Analyze this codebase and generate configuration files. Output each file with clear markers.

## What to Analyze

1. Look for existing documentation (README.md, CLAUDE.md, CONTRIBUTING.md, etc.)
2. Examine package.json, composer.json, pyproject.toml, etc. for dependencies
3. Look at the code structure and patterns used
4. Check for existing tests and their patterns
5. Identify any security-sensitive files or patterns

## Output Format

Generate EXACTLY these files with the markers shown. Output raw content directly after each marker - DO NOT wrap in markdown code blocks.

---FILE: .ralph/config.yaml---
(YAML configuration - raw YAML, no code blocks)

---FILE: .ralph/principles.md---
(Markdown content - raw markdown, no code blocks)

---FILE: .ralph/patterns.md---
(Markdown with code examples - raw markdown, no outer code blocks)

---FILE: .ralph/boundaries.md---
(Markdown list - raw markdown, no code blocks)

---FILE: .ralph/tech-stack.md---
(Markdown content - raw markdown, no code blocks)

## Guidelines

- Be SPECIFIC to this codebase, not generic advice
- Include actual patterns you see in the code
- Reference real files and directories
- Keep each file focused and concise
- For boundaries, include lock files, vendor directories, env files, etc.
- Output raw file content directly - NO ```markdown or ```yaml wrappers

Start your analysis now.
PROMPT_EOF
)

  # Run Claude
  OUTPUT=$(echo "$AI_PROMPT" | claude -p --dangerously-skip-permissions 2>&1 | tee /dev/stderr)

  # Parse and write files
  echo ""
  echo -e "${BLUE}Saving configuration files...${NC}"

  # Extract each file from the output
  extract_file() {
    local marker="$1"
    local output="$2"

    # Extract content between marker and next marker (or end)
    # Also strip markdown code block wrappers if present
    echo "$output" | awk -v marker="$marker" '
      $0 ~ marker { found=1; next }
      found && /^---FILE:/ { exit }
      found { print }
    ' | sed '/^```\(markdown\|yaml\|yml\)\?$/d' | sed '/^```$/d'
  }

  # Save each file if content was generated
  for file in "config.yaml" "principles.md" "patterns.md" "boundaries.md" "tech-stack.md"; do
    content=$(extract_file "---FILE: .ralph/$file---" "$OUTPUT")
    if [ -n "$content" ] && [ ${#content} -gt 10 ]; then
      if [ -f "$CONFIG_DIR/$file" ] && [ "$FORCE" != true ]; then
        echo -e "  ${YELLOW}$file exists, skipping (use --force to overwrite)${NC}"
      else
        echo "$content" > "$CONFIG_DIR/$file"
        echo -e "  ${GREEN}$file saved${NC}"
      fi
    fi
  done

else
  # Non-AI mode: create config from detection
  echo -e "${BLUE}Creating configuration from detection...${NC}"

  # Write config.yaml
  if [ ! -f "$CONFIG_DIR/config.yaml" ] || [ "$FORCE" = true ]; then
    cat > "$CONFIG_DIR/config.yaml" << EOF
# Ralph Configuration
# Generated by ralph-init

project:
  name: "$PROJECT_NAME"
  description: "TODO: Add project description"

git:
  base_branch: "main"

commands:
  test: "${TEST_CMD:-npm test}"
  lint: "${LINT_CMD:-npm run lint}"
  build: "${BUILD_CMD:-npm run build}"
  dev: "${DEV_CMD:-npm run dev}"
EOF
    echo "  - config.yaml created"
  else
    echo "  - config.yaml exists, skipped"
  fi

  # Write tech-stack.md
  if [ ! -f "$CONFIG_DIR/tech-stack.md" ] || [ "$FORCE" = true ]; then
    cat > "$CONFIG_DIR/tech-stack.md" << EOF
# Tech Stack

**Language:** ${LANGUAGE:-Unknown}
**Framework:** ${FRAMEWORK:-None detected}

TODO: Add more details about your technology stack.
EOF
    echo "  - tech-stack.md created"
  else
    echo "  - tech-stack.md exists, skipped"
  fi

  # Create stub files if they don't exist
  for file in "principles.md" "patterns.md" "boundaries.md"; do
    if [ ! -f "$CONFIG_DIR/$file" ]; then
      case $file in
        principles.md)
          echo "# Project Principles" > "$CONFIG_DIR/$file"
          echo "" >> "$CONFIG_DIR/$file"
          echo "TODO: Add your development principles" >> "$CONFIG_DIR/$file"
          ;;
        patterns.md)
          echo "# Code Patterns" > "$CONFIG_DIR/$file"
          echo "" >> "$CONFIG_DIR/$file"
          echo "TODO: Document the patterns used in this codebase" >> "$CONFIG_DIR/$file"
          ;;
        boundaries.md)
          echo "# Boundaries" > "$CONFIG_DIR/$file"
          echo "" >> "$CONFIG_DIR/$file"
          echo "Files and directories Ralph should NEVER modify:" >> "$CONFIG_DIR/$file"
          echo "" >> "$CONFIG_DIR/$file"
          echo "- \`*.lock\` files" >> "$CONFIG_DIR/$file"
          echo "- \`node_modules/\`" >> "$CONFIG_DIR/$file"
          echo "- \`vendor/\`" >> "$CONFIG_DIR/$file"
          echo "- \`.env*\` files" >> "$CONFIG_DIR/$file"
          ;;
      esac
      echo "  - $file created (stub)"
    else
      echo "  - $file exists, skipped"
    fi
  done
fi

# Add Ralph section to CLAUDE.md
echo -e "${BLUE}Updating CLAUDE.md...${NC}"

RALPH_SECTION='## Ralph (AI Development Agent)

This project uses Ralph for autonomous feature implementation.

- **Specs** (`specs/`) describe WHAT to build and WHY. Use the `ralph-spec` skill to create/manage specs.
- **Plans** (`plans/`) describe HOW to build it with trackable tasks. Use the `ralph-plan` skill to manage plans.
- Generate plans from specs with the `ralph-spec-to-plan` skill.

Run plans with `./scripts/ralph/ralph.sh <plan-file>`.'

CLAUDE_MD="$PROJECT_ROOT/CLAUDE.md"

if [ -f "$CLAUDE_MD" ]; then
  # Check if Ralph section already exists
  if grep -q "## Ralph" "$CLAUDE_MD"; then
    echo "  - CLAUDE.md already has Ralph section, skipped"
  else
    # Append Ralph section
    echo "" >> "$CLAUDE_MD"
    echo "$RALPH_SECTION" >> "$CLAUDE_MD"
    echo -e "  ${GREEN}CLAUDE.md updated with Ralph section${NC}"
  fi
else
  # Create new CLAUDE.md with Ralph section
  echo "$RALPH_SECTION" > "$CLAUDE_MD"
  echo -e "  ${GREEN}CLAUDE.md created${NC}"
fi

echo ""
echo -e "${GREEN}========================================"
echo -e "Configuration Complete!"
echo -e "========================================${NC}"
echo ""
echo "Configuration files are in: $CONFIG_DIR/"
echo ""
echo -e "${YELLOW}Review and edit these files:${NC}"
echo "  - CLAUDE.md      - Claude Code instructions (Ralph section added)"
echo "  - config.yaml    - Project settings and commands"
echo "  - principles.md  - Development principles"
echo "  - patterns.md    - Code patterns to follow"
echo "  - boundaries.md  - Files to never touch"
echo "  - tech-stack.md  - Technology description"
echo ""
echo "Then run Ralph with a plan file:"
echo "  ./scripts/ralph/ralph.sh path/to/plan.md"
