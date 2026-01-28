#!/bin/bash
# Ralph Configuration Library
# Shared functions for loading config and building prompts

# Get Ralph version
get_ralph_version() {
  local script_dir="$1"
  local version_file="$script_dir/VERSION"

  if [ -f "$version_file" ]; then
    cat "$version_file" | tr -d '\n'
  else
    echo "unknown"
  fi
}

# Find project root (git root or current dir)
find_project_root() {
  git rev-parse --show-toplevel 2>/dev/null || pwd
}

# Find Ralph scripts directory
find_ralph_dir() {
  local project_root="$1"

  # Check common locations
  if [ -d "$project_root/scripts/ralph" ]; then
    echo "$project_root/scripts/ralph"
  elif [ -d "$project_root/.ralph/scripts" ]; then
    echo "$project_root/.ralph/scripts"
  else
    echo ""
  fi
}

# Find Ralph config directory
find_config_dir() {
  local project_root="$1"

  if [ -d "$project_root/.ralph" ]; then
    echo "$project_root/.ralph"
  else
    echo ""
  fi
}

# Load a config value from YAML (simple parser, no yq dependency)
# Usage: config_get "project.name" "/path/to/config.yaml"
config_get() {
  local key="$1"
  local config_file="$2"

  if [ ! -f "$config_file" ]; then
    echo ""
    return
  fi

  # Simple YAML parsing for top-level and nested keys
  # Handles: key: value and parent.child: value
  local IFS='.'
  read -ra parts <<< "$key"

  if [ ${#parts[@]} -eq 1 ]; then
    # Top-level key
    grep "^${parts[0]}:" "$config_file" 2>/dev/null | sed 's/^[^:]*: *//' | sed 's/^"//' | sed 's/"$//'
  else
    # Nested key (e.g., project.name)
    # Find the parent section, then the child key
    awk -v parent="${parts[0]}" -v child="${parts[1]}" '
      /^[a-z]/ { section = $0; gsub(/:.*/, "", section) }
      section == parent && /^  / {
        key = $0
        gsub(/^  /, "", key)
        gsub(/:.*/, "", key)
        if (key == child) {
          val = $0
          gsub(/^[^:]*: */, "", val)
          gsub(/^"/, "", val)
          gsub(/"$/, "", val)
          print val
          exit
        }
      }
    ' "$config_file"
  fi
}

# Load file contents or return empty string
load_file_or_empty() {
  local file_path="$1"
  if [ -f "$file_path" ]; then
    cat "$file_path"
  else
    echo ""
  fi
}

# Build prompt by replacing placeholders with config values
# Usage: build_prompt "/path/to/base/prompt.md" "/path/to/.ralph"
build_prompt() {
  local base_prompt="$1"
  local config_dir="$2"
  local config_file="$config_dir/config.yaml"

  if [ ! -f "$base_prompt" ]; then
    echo "Error: Base prompt not found: $base_prompt" >&2
    return 1
  fi

  # Load override files
  local principles=$(load_file_or_empty "$config_dir/principles.md")
  local patterns=$(load_file_or_empty "$config_dir/patterns.md")
  local boundaries=$(load_file_or_empty "$config_dir/boundaries.md")
  local tech_stack=$(load_file_or_empty "$config_dir/tech-stack.md")

  # Load config values
  local project_name=$(config_get "project.name" "$config_file")
  local project_description=$(config_get "project.description" "$config_file")
  local test_command=$(config_get "commands.test" "$config_file")
  local lint_command=$(config_get "commands.lint" "$config_file")
  local build_command=$(config_get "commands.build" "$config_file")
  local dev_command=$(config_get "commands.dev" "$config_file")

  # Default values
  project_name=${project_name:-"Project"}
  test_command=${test_command:-"npm test"}
  lint_command=${lint_command:-"npm run lint"}

  # Read base prompt and replace placeholders
  local prompt_content
  prompt_content=$(cat "$base_prompt")

  # Replace placeholders
  prompt_content="${prompt_content//\{\{PROJECT_NAME\}\}/$project_name}"
  prompt_content="${prompt_content//\{\{PROJECT_DESCRIPTION\}\}/$project_description}"
  prompt_content="${prompt_content//\{\{PRINCIPLES\}\}/$principles}"
  prompt_content="${prompt_content//\{\{PATTERNS\}\}/$patterns}"
  prompt_content="${prompt_content//\{\{BOUNDARIES\}\}/$boundaries}"
  prompt_content="${prompt_content//\{\{TECH_STACK\}\}/$tech_stack}"
  prompt_content="${prompt_content//\{\{TEST_COMMAND\}\}/$test_command}"
  prompt_content="${prompt_content//\{\{LINT_COMMAND\}\}/$lint_command}"
  prompt_content="${prompt_content//\{\{BUILD_COMMAND\}\}/$build_command}"
  prompt_content="${prompt_content//\{\{DEV_COMMAND\}\}/$dev_command}"

  echo "$prompt_content"
}

# Colors for output
setup_colors() {
  if [ -t 1 ]; then
    RED='\033[0;31m'
    GREEN='\033[0;32m'
    YELLOW='\033[1;33m'
    BLUE='\033[0;34m'
    NC='\033[0m'
  else
    RED=''
    GREEN=''
    YELLOW=''
    BLUE=''
    NC=''
  fi
}

# Logging with timestamp
log() {
  echo "[$(date -u +%Y-%m-%dT%H:%M:%SZ)] $1"
}

log_success() {
  setup_colors
  echo -e "${GREEN}$1${NC}"
}

log_error() {
  setup_colors
  echo -e "${RED}$1${NC}" >&2
}

log_warn() {
  setup_colors
  echo -e "${YELLOW}$1${NC}"
}

log_info() {
  setup_colors
  echo -e "${BLUE}$1${NC}"
}

# Run claude with retry logic for transient errors
# Usage: echo "$PROMPT" | run_claude_with_retry [claude_args...]
# Returns: Claude output on success, exits with error after max retries
run_claude_with_retry() {
  local max_retries=${RALPH_MAX_RETRIES:-5}
  local base_delay=${RALPH_RETRY_DELAY:-5}
  local attempt=1
  local prompt
  local output
  local exit_code
  local temp_file

  # Read prompt from stdin
  prompt=$(cat)

  # Create temp file for output (to capture both stdout and check for errors)
  temp_file=$(mktemp)
  trap "rm -f '$temp_file'" RETURN

  while [ $attempt -le $max_retries ]; do
    # Run claude and capture output + exit code
    set +e
    output=$(echo "$prompt" | claude "$@" 2>&1 | tee /dev/stderr)
    exit_code=$?
    set -e

    # Check for known transient errors
    if echo "$output" | grep -q "No messages returned" || \
       echo "$output" | grep -q "promise rejected" || \
       echo "$output" | grep -q "ECONNRESET" || \
       echo "$output" | grep -q "ETIMEDOUT" || \
       echo "$output" | grep -q "rate limit" || \
       [ $exit_code -ne 0 ]; then

      if [ $attempt -lt $max_retries ]; then
        local delay=$((base_delay * attempt))
        log_warn "Claude CLI error (attempt $attempt/$max_retries). Retrying in ${delay}s..."
        sleep $delay
        attempt=$((attempt + 1))
        continue
      else
        log_error "Claude CLI failed after $max_retries attempts"
        echo "$output"
        return 1
      fi
    fi

    # Success
    echo "$output"
    return 0
  done
}

# Run claude with retry (simple version for verification calls)
# Usage: run_claude_simple_with_retry "prompt" [claude_args...]
run_claude_simple_with_retry() {
  local prompt="$1"
  shift
  local max_retries=${RALPH_MAX_RETRIES:-5}
  local base_delay=${RALPH_RETRY_DELAY:-5}
  local attempt=1
  local output
  local exit_code

  while [ $attempt -le $max_retries ]; do
    set +e
    output=$(echo "$prompt" | claude "$@" 2>/dev/null)
    exit_code=$?
    set -e

    # Check for errors (empty output or non-zero exit)
    if [ -z "$output" ] || [ $exit_code -ne 0 ]; then
      if [ $attempt -lt $max_retries ]; then
        local delay=$((base_delay * attempt))
        log_warn "Claude verification error (attempt $attempt/$max_retries). Retrying in ${delay}s..." >&2
        sleep $delay
        attempt=$((attempt + 1))
        continue
      else
        log_error "Claude verification failed after $max_retries attempts" >&2
        return 1
      fi
    fi

    echo "$output"
    return 0
  done
}

# Check required dependencies
check_dependencies() {
  local missing=()

  if ! command -v claude &> /dev/null; then
    missing+=("claude (Claude Code CLI)")
  fi

  if ! command -v git &> /dev/null; then
    missing+=("git")
  fi

  if [ ${#missing[@]} -gt 0 ]; then
    log_error "Missing required dependencies:"
    for dep in "${missing[@]}"; do
      echo "  - $dep"
    done
    return 1
  fi

  return 0
}

# Check optional dependencies and warn
check_optional_dependencies() {
  local warnings=()

  if ! command -v gh &> /dev/null; then
    warnings+=("gh (GitHub CLI) - useful for PR creation")
  fi

  if [ ${#warnings[@]} -gt 0 ]; then
    log_warn "Optional dependencies not found:"
    for warn in "${warnings[@]}"; do
      echo "  - $warn"
    done
  fi
}
