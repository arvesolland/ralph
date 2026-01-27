#!/bin/bash
# Test helper functions

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

# Test state
TESTS_RUN=0
TESTS_PASSED=0
TESTS_FAILED=0
CURRENT_TEST=""
WORKSPACE=""

# Create isolated test workspace (temp git repo)
setup_workspace() {
  WORKSPACE=$(mktemp -d)
  cd "$WORKSPACE"

  # Initialize git repo (required for ralph)
  git init -q
  git config user.email "test@test.com"
  git config user.name "Test"

  # Create minimal structure
  mkdir -p scripts/ralph/lib scripts/ralph/prompts/base .ralph plans/pending plans/current plans/complete

  # Copy ralph scripts
  cp "$RALPH_DIR"/*.sh scripts/ralph/ 2>/dev/null || true
  cp "$RALPH_DIR"/lib/*.sh scripts/ralph/lib/ 2>/dev/null || true
  cp "$RALPH_DIR"/prompts/base/*.md scripts/ralph/prompts/base/ 2>/dev/null || true
  chmod +x scripts/ralph/*.sh

  # Create minimal config
  cat > .ralph/config.yaml << 'EOF'
project:
  name: "Test Project"
  description: "Integration test workspace"

git:
  base_branch: "main"

commands:
  test: "echo 'no tests'"
  lint: "echo 'no lint'"
EOF

  cat > .ralph/principles.md << 'EOF'
# Test Principles
- Keep changes minimal
- Create only what's asked
EOF

  cat > .ralph/patterns.md << 'EOF'
# Test Patterns
No specific patterns.
EOF

  cat > .ralph/boundaries.md << 'EOF'
# Boundaries
- Do not modify scripts/ralph/
EOF

  cat > .ralph/tech-stack.md << 'EOF'
# Tech Stack
Plain text files for testing.
EOF

  # Initial commit
  git add -A
  git commit -q -m "Initial test workspace"

  echo "$WORKSPACE"
}

# Cleanup workspace
teardown_workspace() {
  if [ -n "$WORKSPACE" ] && [ -d "$WORKSPACE" ]; then
    rm -rf "$WORKSPACE"
  fi
  WORKSPACE=""
}

# Start a test
begin_test() {
  CURRENT_TEST="$1"
  TESTS_RUN=$((TESTS_RUN + 1))
  echo ""
  echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
  echo -e "${BLUE}TEST: $CURRENT_TEST${NC}"
  echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
}

# Assert file exists
assert_file_exists() {
  local file="$1"
  local msg="${2:-File should exist: $file}"

  if [ -f "$file" ]; then
    echo -e "  ${GREEN}✓${NC} $msg"
    return 0
  else
    echo -e "  ${RED}✗${NC} $msg"
    echo -e "    ${RED}File not found: $file${NC}"
    return 1
  fi
}

# Assert file contains string
assert_file_contains() {
  local file="$1"
  local pattern="$2"
  local msg="${3:-File should contain: $pattern}"

  if grep -q "$pattern" "$file" 2>/dev/null; then
    echo -e "  ${GREEN}✓${NC} $msg"
    return 0
  else
    echo -e "  ${RED}✗${NC} $msg"
    echo -e "    ${RED}Pattern not found in $file${NC}"
    return 1
  fi
}

# Assert file does NOT contain string
assert_file_not_contains() {
  local file="$1"
  local pattern="$2"
  local msg="${3:-File should not contain: $pattern}"

  if ! grep -q "$pattern" "$file" 2>/dev/null; then
    echo -e "  ${GREEN}✓${NC} $msg"
    return 0
  else
    echo -e "  ${RED}✗${NC} $msg"
    echo -e "    ${RED}Pattern found in $file${NC}"
    return 1
  fi
}

# Assert directory exists
assert_dir_exists() {
  local dir="$1"
  local msg="${2:-Directory should exist: $dir}"

  if [ -d "$dir" ]; then
    echo -e "  ${GREEN}✓${NC} $msg"
    return 0
  else
    echo -e "  ${RED}✗${NC} $msg"
    echo -e "    ${RED}Directory not found: $dir${NC}"
    return 1
  fi
}

# Assert command succeeds
assert_success() {
  local msg="$1"
  shift

  if "$@" >/dev/null 2>&1; then
    echo -e "  ${GREEN}✓${NC} $msg"
    return 0
  else
    echo -e "  ${RED}✗${NC} $msg"
    echo -e "    ${RED}Command failed: $*${NC}"
    return 1
  fi
}

# Mark test as passed
pass_test() {
  TESTS_PASSED=$((TESTS_PASSED + 1))
  echo -e "${GREEN}PASSED: $CURRENT_TEST${NC}"
}

# Mark test as failed
fail_test() {
  local reason="${1:-Assertion failed}"
  TESTS_FAILED=$((TESTS_FAILED + 1))
  echo -e "${RED}FAILED: $CURRENT_TEST${NC}"
  echo -e "${RED}  Reason: $reason${NC}"
}

# Print test summary
print_summary() {
  echo ""
  echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
  echo -e "${BLUE}TEST SUMMARY${NC}"
  echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
  echo ""
  echo "  Total:  $TESTS_RUN"
  echo -e "  ${GREEN}Passed: $TESTS_PASSED${NC}"
  echo -e "  ${RED}Failed: $TESTS_FAILED${NC}"
  echo ""

  if [ "$TESTS_FAILED" -eq 0 ]; then
    echo -e "${GREEN}All tests passed!${NC}"
    return 0
  else
    echo -e "${RED}Some tests failed.${NC}"
    return 1
  fi
}
