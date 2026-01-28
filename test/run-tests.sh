#!/bin/bash
set -e

# Ralph Integration Tests
# Runs real Claude against test plans to verify the loop works

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
RALPH_DIR="$(dirname "$SCRIPT_DIR")"

# Load helpers
source "$SCRIPT_DIR/lib/helpers.sh"

# Export for helpers
export RALPH_DIR

# Parse arguments
RUN_ALL=true
SPECIFIC_TEST=""
KEEP_WORKSPACE=false
MAX_ITERATIONS=5

while [[ $# -gt 0 ]]; do
  case $1 in
    --test|-t)
      RUN_ALL=false
      SPECIFIC_TEST="$2"
      shift 2
      ;;
    --keep|-k)
      KEEP_WORKSPACE=true
      shift
      ;;
    --max|-m)
      MAX_ITERATIONS="$2"
      shift 2
      ;;
    --help|-h)
      echo "Ralph Integration Tests"
      echo ""
      echo "Usage:"
      echo "  ./run-tests.sh              Run all tests"
      echo "  ./run-tests.sh --test NAME  Run specific test"
      echo "  ./run-tests.sh --keep       Keep workspace after test (for debugging)"
      echo ""
      echo "Options:"
      echo "  --test, -t NAME    Run only the named test"
      echo "  --keep, -k         Don't delete workspace after test"
      echo "  --max, -m N        Max iterations per plan (default: 5)"
      echo ""
      echo "Available tests:"
      echo "  single-task        Basic single task completion"
      echo "  dependencies       Task dependency ordering"
      echo "  progress           Progress file creation"
      echo "  loose-format       Non-strict plan format"
      echo "  worker-queue       Queue management workflow"
      exit 0
      ;;
    *)
      echo "Unknown option: $1"
      exit 1
      ;;
  esac
done

echo ""
echo -e "${GREEN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo -e "${GREEN}RALPH INTEGRATION TESTS${NC}"
echo -e "${GREEN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo ""
echo "Ralph directory: $RALPH_DIR"
echo "Max iterations: $MAX_ITERATIONS"

# Verify claude is available
if ! command -v claude &> /dev/null; then
  echo -e "${RED}Error: Claude CLI not found${NC}"
  exit 1
fi

# ============================================
# TEST: Single Task
# ============================================
test_single_task() {
  begin_test "Single Task Completion"

  WORKSPACE=$(setup_workspace)
  echo "Workspace: $WORKSPACE"

  # Copy test plan
  cp "$SCRIPT_DIR/plans/01-single-task.md" "$WORKSPACE/plans/current/test-plan.md"

  # Run ralph
  echo "Running ralph..."
  cd "$WORKSPACE"
  ./scripts/ralph/ralph.sh plans/current/test-plan.md --max "$MAX_ITERATIONS" || true

  # Verify results
  local failed=0

  assert_file_exists "$WORKSPACE/output/marker.txt" "Marker file created" || failed=1
  assert_file_contains "$WORKSPACE/output/marker.txt" "ralph-test-complete" "Marker has correct content" || failed=1

  # Verify feature branch was created
  if git -C "$WORKSPACE" show-ref --verify --quiet "refs/heads/feat/test-plan"; then
    echo -e "  ${GREEN}✓${NC} Feature branch created: feat/test-plan"
  else
    echo -e "  ${RED}✗${NC} Feature branch not created"
    failed=1
  fi

  # Plan may be in current/ (incomplete) or complete/ (finished)
  # Note: worker renames to plan.md when archiving to complete/
  local plan_file=$(find "$WORKSPACE/plans" -name "plan.md" -path "*/complete/*" -o -name "test-plan.md" -path "*/current/*" 2>/dev/null | head -1)
  if [ -n "$plan_file" ]; then
    assert_file_contains "$plan_file" "\[x\]" "Subtask checked off" || failed=1
  else
    echo -e "  ${RED}✗${NC} Could not find plan file"
    failed=1
  fi

  if [ "$failed" -eq 0 ]; then
    pass_test
  else
    fail_test "Assertions failed"
  fi

  # Cleanup
  if [ "$KEEP_WORKSPACE" = false ]; then
    teardown_workspace
  else
    echo -e "${YELLOW}Workspace kept at: $WORKSPACE${NC}"
  fi
}

# ============================================
# TEST: Dependencies
# ============================================
test_dependencies() {
  begin_test "Task Dependencies"

  WORKSPACE=$(setup_workspace)
  echo "Workspace: $WORKSPACE"

  # Copy test plan
  cp "$SCRIPT_DIR/plans/02-dependencies.md" "$WORKSPACE/plans/current/test-plan.md"

  # Run ralph
  echo "Running ralph..."
  cd "$WORKSPACE"
  ./scripts/ralph/ralph.sh plans/current/test-plan.md --max "$MAX_ITERATIONS" || true

  # Verify results
  local failed=0

  assert_file_exists "$WORKSPACE/output/first.txt" "First file created" || failed=1
  assert_file_contains "$WORKSPACE/output/first.txt" "step-1-done" "First file correct" || failed=1
  assert_file_exists "$WORKSPACE/output/second.txt" "Second file created" || failed=1
  assert_file_contains "$WORKSPACE/output/second.txt" "step-2-done" "Second file correct" || failed=1

  # Verify both tasks completed (plan may be in complete/ folder)
  local plan_file=$(find "$WORKSPACE/plans" -name "*.md" -path "*/complete/*" -o -name "test-plan.md" -path "*/current/*" 2>/dev/null | head -1)
  if [ -n "$plan_file" ]; then
    assert_file_contains "$plan_file" "Status:\*\* complete" "Tasks marked complete" || failed=1
  fi

  if [ "$failed" -eq 0 ]; then
    pass_test
  else
    fail_test "Assertions failed"
  fi

  if [ "$KEEP_WORKSPACE" = false ]; then
    teardown_workspace
  else
    echo -e "${YELLOW}Workspace kept at: $WORKSPACE${NC}"
  fi
}

# ============================================
# TEST: Progress Tracking
# ============================================
test_progress() {
  begin_test "Progress File Tracking"

  WORKSPACE=$(setup_workspace)
  echo "Workspace: $WORKSPACE"

  # Copy test plan
  cp "$SCRIPT_DIR/plans/03-progress-tracking.md" "$WORKSPACE/plans/current/test-plan.md"

  # Run ralph
  echo "Running ralph..."
  cd "$WORKSPACE"
  ./scripts/ralph/ralph.sh plans/current/test-plan.md --max "$MAX_ITERATIONS" || true

  # Verify results
  local failed=0

  assert_file_exists "$WORKSPACE/output/encoded.txt" "Encoded file created" || failed=1

  # Progress file may be in current/ or archived in complete/
  local progress_file=$(find "$WORKSPACE/plans" -name "*.progress.md" -o -name "progress.md" 2>/dev/null | head -1)
  if [ -n "$progress_file" ]; then
    echo -e "  ${GREEN}✓${NC} Progress file created: $(basename "$progress_file")"
  else
    echo -e "  ${YELLOW}!${NC} Progress file not created (may be skipped if no notable learnings)"
  fi

  if [ "$failed" -eq 0 ]; then
    pass_test
  else
    fail_test "Assertions failed"
  fi

  if [ "$KEEP_WORKSPACE" = false ]; then
    teardown_workspace
  else
    echo -e "${YELLOW}Workspace kept at: $WORKSPACE${NC}"
  fi
}

# ============================================
# TEST: Loose Format
# ============================================
test_loose_format() {
  begin_test "Loose Format Plan"

  WORKSPACE=$(setup_workspace)
  echo "Workspace: $WORKSPACE"

  # Copy test plan
  cp "$SCRIPT_DIR/plans/04-loose-format.md" "$WORKSPACE/plans/current/test-plan.md"

  # Run ralph
  echo "Running ralph..."
  cd "$WORKSPACE"
  ./scripts/ralph/ralph.sh plans/current/test-plan.md --max "$MAX_ITERATIONS" || true

  # Verify results
  local failed=0

  assert_file_exists "$WORKSPACE/output/loose-test.txt" "First loose file created" || failed=1
  assert_file_contains "$WORKSPACE/output/loose-test.txt" "loose format works" "First file correct" || failed=1
  assert_file_exists "$WORKSPACE/output/loose-test-2.txt" "Second loose file created" || failed=1

  if [ "$failed" -eq 0 ]; then
    pass_test
  else
    fail_test "Assertions failed"
  fi

  if [ "$KEEP_WORKSPACE" = false ]; then
    teardown_workspace
  else
    echo -e "${YELLOW}Workspace kept at: $WORKSPACE${NC}"
  fi
}

# ============================================
# TEST: Worker Queue
# ============================================
test_worker_queue() {
  begin_test "Worker Queue Management"

  WORKSPACE=$(setup_workspace)
  echo "Workspace: $WORKSPACE"

  # Copy test plan to pending
  cp "$SCRIPT_DIR/plans/01-single-task.md" "$WORKSPACE/plans/pending/test-plan.md"

  # Run worker (should move to current, process, move to complete)
  echo "Running worker..."
  cd "$WORKSPACE"
  ./scripts/ralph/ralph-worker.sh --max "$MAX_ITERATIONS" || true

  # Verify results
  local failed=0

  # Verify feature branch was created by worker
  if git -C "$WORKSPACE" show-ref --verify --quiet "refs/heads/feat/test-plan"; then
    echo -e "  ${GREEN}✓${NC} Feature branch created by worker: feat/test-plan"
  else
    echo -e "  ${RED}✗${NC} Feature branch not created"
    failed=1
  fi

  # Plan should have moved through the queue
  assert_dir_exists "$WORKSPACE/plans/complete" "Complete directory exists" || failed=1

  # Check if any completed plan exists
  local completed_dirs=$(ls -d "$WORKSPACE/plans/complete"/*/ 2>/dev/null | wc -l)
  if [ "$completed_dirs" -gt 0 ]; then
    echo -e "  ${GREEN}✓${NC} Plan moved to complete folder"
  else
    echo -e "  ${RED}✗${NC} Plan not in complete folder"
    failed=1
  fi

  # Output file should exist
  assert_file_exists "$WORKSPACE/output/marker.txt" "Marker file created" || failed=1

  if [ "$failed" -eq 0 ]; then
    pass_test
  else
    fail_test "Assertions failed"
  fi

  if [ "$KEEP_WORKSPACE" = false ]; then
    teardown_workspace
  else
    echo -e "${YELLOW}Workspace kept at: $WORKSPACE${NC}"
  fi
}

# ============================================
# Run tests
# ============================================

if [ "$RUN_ALL" = true ]; then
  test_single_task
  test_dependencies
  test_progress
  test_loose_format
  test_worker_queue
else
  case "$SPECIFIC_TEST" in
    single-task)
      test_single_task
      ;;
    dependencies)
      test_dependencies
      ;;
    progress)
      test_progress
      ;;
    loose-format)
      test_loose_format
      ;;
    worker-queue)
      test_worker_queue
      ;;
    *)
      echo -e "${RED}Unknown test: $SPECIFIC_TEST${NC}"
      exit 1
      ;;
  esac
fi

print_summary
exit $?
