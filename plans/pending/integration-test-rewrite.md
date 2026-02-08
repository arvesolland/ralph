# Integration Test Suite Rewrite for ATM-Based Ralph

## Context

Ralph was rewritten from filesystem-based plan/task management to ATM API-based management. The integration tests at `test/integration/integration_test.go` (1783 lines) are tightly coupled to the old model (pending/current/complete directories, plan.md checkboxes, state.yaml). The ATM client (`internal/atm/client.go`) is a concrete struct with no interface, making it impossible to mock for unit tests. Both the Go orchestrator and the Claude agent shell out to `atm-cli`, so integration tests need a fake binary that serves both.

### Key Technical Details (from code review)

- `*atm.Client` has 17 public methods, all shell out via private `exec()` method
- All JSON responses are wrapped in `{"data": ...}` envelope (except `plan context --format text` which returns raw text)
- CLI grammar: `atm-cli [--api-url URL] [--api-token TOKEN] <group> <action> [positional-args] [--flags]`
- `LoopConfig.ATM` and `WorkerConfig.ATM` are `*atm.Client` (concrete pointer, not interface)
- 9 ATM call sites across loop.go (4) and worker.go (5), zero are tested
- Existing mock patterns: `MockRunner`, `MockNotifier`, `MockATMRunner` - all use hook-based function fields
- `cli/run.go` and `cli/status.go` also call ATM methods directly via `cfg.ATMClient()`
- `Config.ATMClient()` returns `*atm.Client` — should return `atm.ATM` for consistency
- Prompt uses `{{ATM_CONTEXT}}` and `{{PLAN_ID}}` placeholders, populated via `buildPrompt()` overrides
- `context.json` has `PlanID int` (not plan file path)

## Plan

### Phase 1: ATM Interface + Mock (~5 files)

**Create `internal/atm/interface.go`** — extract interface from all 17 public methods on `*Client`:
```go
type ATM interface {
    ProjectContext(slug string) (*AgentContext, error)
    PlanContext(planID int) (*AgentContext, error)
    PlanContextText(planID int) (string, error)
    ListPlans(projectSlug, status string) ([]Plan, error)
    GetPlan(id int) (*Plan, error)
    UpdatePlanStatus(id int, status string) (*Plan, error)
    ListTasks(planID int, opts *TaskListOpts) ([]Task, error)
    GetTask(id int) (*Task, error)
    ClaimTask(id int, assignee string) (*Task, error)
    StartTask(id int) (*Task, error)
    CompleteTask(id int) (*Task, error)
    BlockTask(id int, reason string) (*Task, error)
    SkipTask(id int, reason string) (*Task, error)
    AddProgress(planID int, author, body string) (*Progress, error)
    AddFeedback(planID int, author, body string) (*Feedback, error)
    CheckCriterion(id int) (*Criterion, error)
    UncheckCriterion(id int) (*Criterion, error)
}
```

Add `var _ ATM = (*Client)(nil)` compile check to `client.go`.

**Create `internal/atm/mock.go`** — function-hook based mock with call tracking:
- `NewMockATM()` with sensible defaults (empty lists, success, zero-value returns)
- Each method delegates to `XxxFunc` hook if set, else returns default
- `Calls []MockCall` for assertion (tracks method name + args)
- Follow existing patterns from `MockRunner` and `MockNotifier`

**Update consumers** to accept interface instead of `*Client`:
- `internal/runner/loop.go`: `LoopConfig.ATM` and `IterationLoop.atm` → `atm.ATM`
- `internal/worker/worker.go`: `WorkerConfig.ATM` and `Worker.atm` → `atm.ATM`
- `internal/config/config.go`: `ATMClient()` return type → `atm.ATM` (backwards compatible since `*Client` satisfies `ATM`)
- `internal/cli/run.go`: No change needed (uses `cfg.ATMClient()` which returns interface)
- `internal/cli/status.go`: No change needed (same reason)

### Phase 2: Unit Test Improvements (~2 files)

**`internal/runner/loop_test.go`** — add tests using MockATM:
- `TestIterationLoop_ATMContextFailure_HardError` — `PlanContextText` fails → iteration returns error (fatal call site at loop.go:246)
- `TestIterationLoop_ATMCompletionCheck_AllDone` — stats show `Done+Skipped == TotalTasks` → loop ends, `result.Completed == true`
- `TestIterationLoop_ATMCompletionCheck_NotDone` — stats show incomplete → false completion, `AddFeedback` called with descriptive message
- `TestIterationLoop_FalseCompletionCircuitBreaker` — 5 consecutive false completions → `result.Error` contains halt message
- `TestIterationLoop_ATMCompletionCheck_Unreachable` — `PlanContext` fails all 3 retries → fail-closed (return false, don't trust marker)
- `TestIterationLoop_ATMProgressTracking` — `AddProgress` called with iteration info, non-fatal on error

**`internal/worker/worker_test.go`** — add tests using MockATM:
- `TestWorker_RunOnce_ActivatesPlan` — `ProjectContext` returns no active plan, `ListPlans` returns one ready plan → `UpdatePlanStatus(id, "active")` called, plan processed
- `TestWorker_RunOnce_ResumesActivePlan` — `ProjectContext` returns active plan → resumes without re-activating
- `TestWorker_RunOnce_EmptyQueue` — `ProjectContext` returns nothing, `ListPlans` returns empty → `ErrQueueEmpty`
- `TestWorker_RunOnce_CompletionFlow` — verifies plan transitions: ready → active → complete via `UpdatePlanStatus`

### Phase 3: Fake `atm-cli` Binary (~3 new files)

**`test/integration/fakeatm/main.go`** — CLI entry point:
- Parses global flags `--api-url` and `--api-token` (accept and ignore)
- Routes to handler based on `<group> <action>` (e.g., `plan context`, `task start`)
- State file path from `FAKEATM_STATE_PATH` env var (required)
- Exits non-zero with stderr message on errors
- All JSON output uses `{"data": <payload>}` envelope

**`test/integration/fakeatm/state.go`** — JSON state file management:
- Stores projects, plans, tasks, criteria, progress entries, feedback entries
- File-locked reads/writes (both Go orchestrator and Claude agent hit it concurrently via `exec.Command`)
- Auto-incrementing IDs per entity type
- Helper functions: `SeedProject`, `SeedPlan`, `SeedTask`, `SeedCriterion`

**`test/integration/fakeatm/handlers.go`** — command handlers:
- `project context <slug>` — returns `AgentContext` with project, active plan (if any), stats, tasks
- `plan context <id>` — returns `AgentContext` for specific plan
- `plan context <id> --format text` — returns structured text (formatted like real ATM output)
- `plan list <slug> [--status X]` — filter plans by status
- `plan show <id>` — returns single plan
- `plan status <id> --status <status>` — transition plan status
- `task start/complete/skip/block <id> [--reason X]` — state transitions with validation
- `task list <planID> [--status X] [--available]` — list/filter tasks
- `task show <id>` / `task claim <id> --assignee X` — simple lookups/updates
- `criteria check/uncheck <id>` — toggle checked state
- `progress add <planID> --author X --body X` — append progress entry
- `feedback add <planID> --author X --body X` — append feedback entry

Key: `--reason` is optional for `task skip`. `--format text` output must match what `PlanContextText` expects (used in prompt's `{{ATM_CONTEXT}}` placeholder).

### Phase 4: Integration Test Rewrite (~1 file, heavily modified)

**`test/integration/integration_test.go`** changes:

**TestMain** — build fake `atm-cli` binary once via `go build -o fakeatm ./test/integration/fakeatm/`

**Workspace helper updates:**
- `setupWorkspace()` writes ATM config to `.ralph/config.yaml`:
  - `atm.bin_path`: path to fake atm-cli binary
  - `atm.project_slug`: test project slug
  - `atm.api_url` / `atm.api_token`: dummy values
- New `SeedATMState(slug, planTitle, tasks)` — writes initial state to fake ATM state file
- New `ReadATMState()` — reads final state for assertions
- `RunRalph()` sets `FAKEATM_STATE_PATH` env var pointing to state file
- `CreatePlanBundle()` still works for creating the local plan file (agent reads this), but task tracking happens via ATM

**Tests to DELETE** (obsolete filesystem/state.yaml concepts):
- `TestReset` — `ralph reset` is a filesystem queue concept, doesn't exist in ATM model
- `TestPlanBundleCreate` — `ralph plan create` scaffolds filesystem bundles, not ATM plans
- `TestStateReview_StandardFormat` — `ralph plan review` populates state.yaml, not ATM
- `TestStateReview_NonStandardFormat` — same
- `standardFormatPlan` constant
- `stateYAMLFile`, `stateTask`, `stateCriterion` types
- `readStateYAML` helper
- `containsString` helper (move to simpler utility if still needed)

**Tests to ADAPT** (same concept, ATM backend):
- `TestSingleTask` — `ralph run --plan <id>` + seed plan/task via fake ATM + assert task marked complete in ATM state
- `TestDependencies` — seed 2 tasks with dependency in ATM + verify execution order via ATM task status transitions
- `TestProgressTracking` — verify ATM progress entries added during iteration (read from state file)
- `TestOneTaskPerIteration` — verify separate commits + ATM task transitions (each task start→complete in separate iterations)
- `TestWorktreeCleanup` — minimal change (not ATM-dependent, tests git worktree cleanup)
- `TestSlackNotifications` — seed plan via ATM state, keep mock Slack server, verify notifications fire

**Tests to REWRITE** (old queue model → new ATM worker flow):
- `TestWorkerQueue` — `ralph worker --once` picks up ready ATM plan, processes in worktree, plan status transitions to complete
- `TestDirtyState` — dirty main worktree + ATM worker still succeeds (worktree isolation)
- `TestCorePrinciples` — multi-task with deps via ATM, verify all principles (one-task-per-iteration, commits, task completion)

**Tests to ADD** (new ATM-specific behaviors):
- `TestFalseCompletionCircuitBreaker` — agent claims COMPLETE but ATM tasks remain → ralph halts after 5 false completions
- `TestATMContextFailure` — fake ATM returns errors → ralph fails cleanly with descriptive error

## Files Modified

| File | Action |
|------|--------|
| `internal/atm/interface.go` | NEW — ATM interface |
| `internal/atm/mock.go` | NEW — MockATM for unit tests |
| `internal/atm/client.go` | ADD interface compile check |
| `internal/runner/loop.go` | CHANGE `*atm.Client` → `atm.ATM` |
| `internal/runner/loop_test.go` | ADD 6 ATM mock tests |
| `internal/worker/worker.go` | CHANGE `*atm.Client` → `atm.ATM` |
| `internal/worker/worker_test.go` | ADD 4 ATM mock tests |
| `internal/config/config.go` | CHANGE `ATMClient()` return type → `atm.ATM` |
| `test/integration/fakeatm/main.go` | NEW — fake CLI entry point |
| `test/integration/fakeatm/state.go` | NEW — state management |
| `test/integration/fakeatm/handlers.go` | NEW — command handlers |
| `test/integration/integration_test.go` | REWRITE — all tests + helpers |
| `test/integration/testdata/nonstandard_plan.md` | DELETE |

## Verification

1. `go build ./...` — compiles
2. `go vet ./...` — clean
3. `go test ./internal/atm/...` — interface satisfied (compile check)
4. `go test ./internal/runner/...` — all loop tests pass (existing 7 + new 6 ATM tests)
5. `go test ./internal/worker/...` — all worker tests pass (existing 12 + new 4)
6. `go build ./test/integration/fakeatm/` — fake binary builds
7. Manual: `FAKEATM_STATE_PATH=/tmp/test.json ./fakeatm plan context 1 --format text` — produces expected output
8. `go test -tags integration ./test/integration/...` — all integration tests pass (requires Claude CLI)
