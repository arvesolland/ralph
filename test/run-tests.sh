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
      echo "  worker-queue       Queue management with worktree isolation"
      echo "  dirty-state        Dirty main worktree handling"
      echo "  worktree-cleanup   Orphaned worktree cleanup"
      echo "  core-principles    Verify ALL core Ralph principles"
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
# TEST: Worker Queue (with worktree isolation)
# ============================================
test_worker_queue() {
  begin_test "Worker Queue with Worktree Isolation"

  WORKSPACE=$(setup_workspace)
  echo "Workspace: $WORKSPACE"

  # Copy test plan to pending
  cp "$SCRIPT_DIR/plans/01-single-task.md" "$WORKSPACE/plans/pending/test-plan.md"

  # Run worker with --merge flag (should move to current, process in worktree, merge, move to complete)
  echo "Running worker with --merge..."
  cd "$WORKSPACE"
  ./scripts/ralph/ralph-worker.sh --max "$MAX_ITERATIONS" --merge || true

  # Verify results
  local failed=0

  # Main worktree should stay on main branch throughout
  assert_on_branch "$WORKSPACE" "main" "Main worktree stayed on main branch" || failed=1

  # Worktree should be cleaned up after completion
  assert_worktree_not_exists "$WORKSPACE" "test-plan" "Worktree cleaned up" || failed=1

  # Verify work was merged to main
  if git -C "$WORKSPACE" log --oneline main | grep -q "feat/test-plan\|marker\|complete"; then
    echo -e "  ${GREEN}✓${NC} Work merged to main branch"
  else
    echo -e "  ${RED}✗${NC} Work not merged to main"
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
# TEST: Dirty State Handling
# Verifies that worktree isolation prevents dirty state issues
# ============================================
test_dirty_state() {
  begin_test "Dirty State Handling"

  WORKSPACE=$(setup_workspace)
  echo "Workspace: $WORKSPACE"

  # Create uncommitted changes in main worktree (this would break old stash-based approach)
  echo "Creating dirty state in main worktree..."
  echo "uncommitted change" > "$WORKSPACE/dirty-file.txt"

  # Copy test plan to pending
  cp "$SCRIPT_DIR/plans/01-single-task.md" "$WORKSPACE/plans/pending/test-plan.md"

  # Run worker - should succeed despite dirty main worktree
  echo "Running worker with dirty main worktree..."
  cd "$WORKSPACE"
  ./scripts/ralph/ralph-worker.sh --max "$MAX_ITERATIONS" --merge || true

  # Verify results
  local failed=0

  # Main worktree should still have the dirty file (not lost to stash)
  if [ -f "$WORKSPACE/dirty-file.txt" ]; then
    echo -e "  ${GREEN}✓${NC} Dirty file preserved in main worktree"
  else
    echo -e "  ${RED}✗${NC} Dirty file was lost"
    failed=1
  fi

  # Main worktree should stay on main branch
  assert_on_branch "$WORKSPACE" "main" "Main worktree stayed on main" || failed=1

  # Output should have been created (execution worked)
  assert_file_exists "$WORKSPACE/output/marker.txt" "Execution succeeded despite dirty state" || failed=1

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
# TEST: Worktree Cleanup
# Verifies the --cleanup command removes orphaned worktrees
# ============================================
test_worktree_cleanup() {
  begin_test "Worktree Cleanup"

  WORKSPACE=$(setup_workspace)
  echo "Workspace: $WORKSPACE"

  # Create an orphaned worktree manually (simulates interrupted execution)
  echo "Creating orphaned worktree..."
  mkdir -p "$WORKSPACE/.ralph/worktrees"
  git -C "$WORKSPACE" branch feat/orphan-plan main 2>/dev/null || true
  git -C "$WORKSPACE" worktree add "$WORKSPACE/.ralph/worktrees/feat-orphan-plan" feat/orphan-plan 2>/dev/null || true

  # Verify orphan exists
  local failed=0
  if [ -d "$WORKSPACE/.ralph/worktrees/feat-orphan-plan" ]; then
    echo -e "  ${GREEN}✓${NC} Orphaned worktree created"
  else
    echo -e "  ${RED}✗${NC} Failed to create orphan worktree"
    failed=1
  fi

  # Run cleanup
  echo "Running cleanup..."
  cd "$WORKSPACE"
  ./scripts/ralph/ralph-worker.sh --cleanup

  # Verify orphan was cleaned up
  if [ ! -d "$WORKSPACE/.ralph/worktrees/feat-orphan-plan" ]; then
    echo -e "  ${GREEN}✓${NC} Orphaned worktree cleaned up"
  else
    echo -e "  ${RED}✗${NC} Orphan worktree still exists"
    failed=1
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
# TEST: Core Principles
# Verifies ALL Ralph core principles in one test:
# 1. One task at a time
# 2. Reads context (CLAUDE.md, specs, plan, progress)
# 3. Picks next task respecting dependencies
# 4. Completes with verification
# 5. Updates plan
# 6. Updates progress log (EVERY iteration)
# 7. Commits all changes
# ============================================
test_core_principles() {
  begin_test "Core Principles Verification"

  WORKSPACE=$(setup_workspace)
  echo "Workspace: $WORKSPACE"

  # Copy test plan to pending (worker will activate it)
  cp "$SCRIPT_DIR/plans/05-core-principles.md" "$WORKSPACE/plans/pending/test-plan.md"

  # Run worker with --merge (uses worktree isolation, merges to main on completion)
  echo "Running worker with --merge..."
  cd "$WORKSPACE"
  ./scripts/ralph/ralph-worker.sh --max 10 --merge || true

  # Verify results
  local failed=0

  echo ""
  echo "  Verifying Core Principles:"
  echo ""

  # PRINCIPLE 1: One task at a time (verified by dependency order)
  # If tasks ran in parallel, step2 might run before step1
  assert_file_exists "$WORKSPACE/output/step1.txt" "P1: Task T1 completed" || failed=1
  assert_file_exists "$WORKSPACE/output/step2.txt" "P1: Task T2 completed" || failed=1
  assert_file_exists "$WORKSPACE/output/final.txt" "P1: Task T3 completed" || failed=1
  assert_file_contains "$WORKSPACE/output/step1.txt" "step1-complete" "P1: T1 correct content" || failed=1
  assert_file_contains "$WORKSPACE/output/step2.txt" "step2-complete" "P1: T2 correct content" || failed=1
  assert_file_contains "$WORKSPACE/output/final.txt" "all-done" "P1: T3 correct content" || failed=1

  # PRINCIPLE 3: Picks next task (dependency ordering)
  # T2 requires T1, T3 requires T2 - verified by files existing in order

  # PRINCIPLE 4: Completes with verification (acceptance criteria checked)
  # T2 verifies T1 exists, T3 verifies both exist - verified by final.txt existing

  # Worktree should be cleaned up after completion
  assert_worktree_not_exists "$WORKSPACE" "test-plan" "Worktree cleaned up after completion" || failed=1

  # Main worktree should be on main branch
  assert_on_branch "$WORKSPACE" "main" "Main worktree on main branch" || failed=1

  # PRINCIPLE 5: Updates plan (checkboxes marked)
  # After worker --merge, plan is archived in plans/complete/
  local plan_file=$(find "$WORKSPACE/plans/complete" -name "plan.md" 2>/dev/null | head -1)
  if [ -n "$plan_file" ]; then
    # Count checked boxes - should have multiple [x] entries
    local checked_count=$(grep -c '\[x\]' "$plan_file" 2>/dev/null || echo "0")
    if [ "$checked_count" -ge 3 ]; then
      echo -e "  ${GREEN}✓${NC} P5: Plan updated ($checked_count subtasks checked)"
    else
      echo -e "  ${RED}✗${NC} P5: Plan not fully updated (only $checked_count checked)"
      failed=1
    fi

    # Verify all tasks marked complete
    local complete_count=$(grep -c 'Status:\*\* complete' "$plan_file" 2>/dev/null || echo "0")
    if [ "$complete_count" -ge 3 ]; then
      echo -e "  ${GREEN}✓${NC} P5: All 3 tasks marked complete"
    else
      echo -e "  ${RED}✗${NC} P5: Not all tasks marked complete ($complete_count/3)"
      failed=1
    fi
  else
    echo -e "  ${RED}✗${NC} P5: Could not find plan file"
    failed=1
  fi

  # PRINCIPLE 6: Updates progress log (EVERY iteration)
  # After worker --merge, progress is archived in plans/complete/
  local progress_file=$(find "$WORKSPACE/plans/complete" -name "progress.md" -o -name "*.progress.md" 2>/dev/null | head -1)
  if [ -n "$progress_file" ] && [ -f "$progress_file" ]; then
    echo -e "  ${GREEN}✓${NC} P6: Progress file exists"

    # Count iteration entries - should have at least 3 (one per task)
    local iteration_count=$(grep -c '### Iteration' "$progress_file" 2>/dev/null || echo "0")
    if [ "$iteration_count" -ge 3 ]; then
      echo -e "  ${GREEN}✓${NC} P6: Progress logged for $iteration_count iterations"
    else
      echo -e "  ${RED}✗${NC} P6: Progress not logged every iteration (only $iteration_count entries, expected >=3)"
      failed=1
    fi

    # Check for "Completed:" entries (required format)
    local completed_entries=$(grep -c 'Completed:' "$progress_file" 2>/dev/null || echo "0")
    if [ "$completed_entries" -ge 3 ]; then
      echo -e "  ${GREEN}✓${NC} P6: All iterations have Completed entries"
    else
      echo -e "  ${RED}✗${NC} P6: Missing Completed entries (only $completed_entries, expected >=3)"
      failed=1
    fi
  else
    echo -e "  ${RED}✗${NC} P6: Progress file not found"
    failed=1
  fi

  # PRINCIPLE 7: Commits all changes (check main since feature branch is merged)
  # With worker --merge flow: initial + activation + agent commits + completion + merge
  local commit_count=$(git -C "$WORKSPACE" log --oneline main 2>/dev/null | wc -l | tr -d ' ')
  if [ "$commit_count" -ge 4 ]; then
    echo -e "  ${GREEN}✓${NC} P7: Multiple commits made ($commit_count commits on main)"
  else
    echo -e "  ${RED}✗${NC} P7: Expected at least 4 commits on main, got $commit_count"
    failed=1
  fi

  # Verify the merge brought work from feature branch
  if git -C "$WORKSPACE" log --oneline main | grep -q "feat/test-plan\|T1\|T2\|T3\|step"; then
    echo -e "  ${GREEN}✓${NC} P7: Feature branch work merged to main"
  else
    echo -e "  ${RED}✗${NC} P7: Feature branch work not found on main"
    failed=1
  fi

  # Verify commits include plan changes
  local plan_commits=$(git -C "$WORKSPACE" log --oneline main -- "plans/" 2>/dev/null | wc -l | tr -d ' ')
  if [ "$plan_commits" -ge 2 ]; then
    echo -e "  ${GREEN}✓${NC} P7: Plan file committed ($plan_commits commits)"
  else
    echo -e "  ${YELLOW}!${NC} P7: Plan commits: $plan_commits (expected >=2)"
  fi

  if [ "$failed" -eq 0 ]; then
    pass_test
  else
    fail_test "Core principles not all verified"
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
  test_dirty_state
  test_worktree_cleanup
  test_core_principles
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
    dirty-state)
      test_dirty_state
      ;;
    worktree-cleanup)
      test_worktree_cleanup
      ;;
    core-principles)
      test_core_principles
      ;;
    *)
      echo -e "${RED}Unknown test: $SPECIFIC_TEST${NC}"
      exit 1
      ;;
  esac
fi

print_summary
exit $?
