# Plan: Mock Support for Integration Tests

**Created:** 2026-02-02
**Status:** complete

## Context

Integration tests currently require real Claude CLI, making them:
- Slow (~1-2 minutes per test)
- Expensive (API costs)
- Dependent on network/service availability
- Non-deterministic (Claude responses vary)

This plan implements fixture-based mock support for offline, deterministic testing.

### Architecture

```
test/
├── fixtures/
│   ├── scenarios/           # Scenario definitions
│   │   └── single-task.json # Maps iterations to fixture files
│   └── recordings/          # Recorded Claude CLI outputs
│       └── single-task/
│           ├── iteration-1.jsonl
│           └── verification.txt
├── mock/
│   └── claude/
│       └── main.go          # Mock claude binary
└── integration/
    └── integration_test.go  # Updated to support mock mode
```

### Gotchas

- Mock binary must handle both `--output-format stream-json` and `--output-format text`
- Fixtures need realistic timing (sleep between chunks) to test streaming behavior
- Scenario selection must be deterministic based on test name/iteration

---

## Rules

1. **Pick task:** First task where status is not `complete` and all `Requires` are `complete`
2. **Subtasks are sequential.** Complete 1 before 2.
3. **Task complete when:** All "Done when" checked and verified, then set Status: `complete`
4. **Update file after each checkbox.**
5. **New work found?** Add to Discovered section, continue current task.

---

## Tasks

### T1: Create fixture recording infrastructure
> Set up directory structure and recording wrapper for capturing Claude CLI output

**Requires:** —
**Status:** complete

**Done when:**
- [x] `test/fixtures/` directory structure exists
- [x] Recording wrapper can capture Claude CLI output
- [x] Output is saved with scenario/iteration naming

**Subtasks:**
1. [x] Create `test/fixtures/scenarios/` and `test/fixtures/recordings/` directories
2. [x] Create `internal/testutil/recorder.go` - wraps Claude CLI and saves output
3. [x] Add `--record` flag to integration tests to enable recording mode

---

### T2: Build mock Claude binary
> Create a mock binary that replays recorded fixtures

**Requires:** T1
**Status:** complete

**Done when:**
- [x] `test/mock/claude/main.go` exists
- [x] Binary reads scenario from environment variable
- [x] Returns fixture content matching current iteration
- [x] Supports both stream-json and text output formats

**Subtasks:**
1. [x] Create `test/mock/claude/main.go`
2. [x] Parse `RALPH_MOCK_SCENARIO` and `RALPH_MOCK_ITERATION` env vars
3. [x] Load and stream fixture file content with realistic timing
4. [x] Handle `--output-format` flag to return appropriate format
5. [x] Build binary to `test/mock/claude/claude`

---

### T3: Create scenario configuration format
> Define JSON schema for mapping iterations to fixture files

**Requires:** T1
**Status:** complete

**Done when:**
- [x] JSON schema defined for scenario files
- [x] Scenarios map iteration numbers to fixture files
- [x] Support for verification fixtures (separate from iteration fixtures)

**Subtasks:**
1. [x] Design scenario JSON schema
2. [x] Create `test/fixtures/scenarios/single-task.json` as example
3. [x] Document scenario format in test/fixtures/README.md

---

### T4: Update integration tests for mock mode
> Modify tests to support running with mock Claude binary

**Requires:** T2, T3
**Status:** complete

**Done when:**
- [x] Tests detect `RALPH_MOCK_MODE=1` environment variable
- [x] Mock claude binary injected via PATH override
- [x] Tests pass using fixtures instead of real Claude CLI (verified after T5)

**Subtasks:**
1. [x] Add `setupMockEnvironment()` helper to integration tests
2. [x] Override PATH to include mock binary directory
3. [x] Set scenario environment variables per test
4. [x] Skip real Claude CLI check when in mock mode
5. [x] Add recording support to runner (`internal/runner/runner.go`) for RALPH_RECORD mode

---

### T5: Record initial fixture set
> Capture fixtures from real Claude CLI runs for core tests

**Requires:** T4
**Status:** complete

**Done when:**
- [x] Fixtures recorded for `TestSingleTask`
- [x] Fixtures recorded for `TestDependencies`
- [x] Fixtures recorded for `TestWorkerQueue`
- [x] All recorded tests pass in mock mode

**Subtasks:**
1. [x] Run TestSingleTask with `--record` to capture fixtures
2. [x] Run TestDependencies with `--record`
3. [x] Run TestWorkerQueue with `--record`
4. [x] Verify tests pass with `RALPH_MOCK_MODE=1`

---

### T6: Add CI configuration for mock tests
> Configure CI to run mock tests by default

**Requires:** T5
**Status:** complete

**Done when:**
- [x] Mock tests run in CI without Claude CLI
- [x] Real integration tests available as separate workflow
- [x] Documentation updated

**Subtasks:**
1. [x] Update `.github/workflows/test.yml` to run mock tests
2. [x] Create separate workflow for real integration tests (manual trigger)
3. [x] Update CLAUDE.md with mock test instructions

---

## Discovered

- **Recording integration in runner** (completed during T4/T5): The original plan assumed testutil/recorder.go would be used externally, but the runner itself needed modification to capture Claude CLI output. Added recording support directly in `internal/runner/runner.go` with `RALPH_RECORD=1` env var support.

---

## Completed

<!-- Completion dates will be added here -->
