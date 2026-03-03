# CLAUDE.md

This file provides guidance to Claude Code when working on the Ralph repository.

## Overview

Ralph is an autonomous AI development loop orchestration system implementing the "Ralph Wiggum technique" - fresh context per iteration with progress persisted in Board and git.

Ralph is written in Go. The codebase lives in `cmd/` and `internal/` directories following standard Go project layout. Task management is handled via Board (task management service), accessed through the `board` binary.

## Commands

```bash
# Build the Go binary
make build              # Production build with version info
make build-dev          # Fast development build

# Run tests
make test               # Run all unit tests (verbose)
make test-short         # Run tests without integration tests
make test-race          # Run tests with race detector
make test-coverage      # Run tests with coverage report
go test ./... -v        # Verbose test output

# Run ralph commands
./ralph init --detect          # Initialize project with auto-detection
./ralph status                 # Show Board project status
./ralph run --plan <plan-id>   # Run iteration loop on a plan
./ralph worker                 # Process Board queue (continuous)
./ralph worker --once          # Process one plan and exit
./ralph cleanup                # Remove orphaned worktrees
./ralph version                # Show version info

# Linting
make lint               # Run golangci-lint

# Release (requires goreleaser)
make release-snapshot   # Test release build
make release-dry-run    # Dry run release
```

## Architecture

### Project Structure

```
cmd/ralph/              # Main entry point
internal/
├── cli/                # Cobra commands (init, run, worker, status, cleanup, version)
├── config/             # Config loading, YAML parsing, project detection
├── board/              # Board client (shells out to board binary)
├── runner/             # Claude execution, streaming, retry logic, iteration loop
├── git/                # Git operations (commit, branch, worktree, status)
├── worktree/           # Worktree management, dependency auto-detection, hooks
├── worker/             # Queue processor orchestration, completion modes (PR/merge/branch)
├── notify/             # Slack notifications (webhook, bot API, Socket Mode, threads)
├── prompt/             # Prompt template building with embedded defaults
└── log/                # Structured logging with color support
test/
└── integration/        # End-to-end integration tests (requires Claude CLI)
    └── fakeboard/      # Fake board binary for test isolation
```

Key packages:
- `internal/runner/loop.go` - Main iteration loop (prompt -> Claude -> verify Board stats -> commit)
- `internal/worker/worker.go` - Queue processor (poll Board -> activate -> iterate -> complete)
- `internal/board/client.go` - Board client wrapping board commands
- `internal/worktree/manager.go` - Worktree creation, cleanup, dependency installation

### Board Integration

Ralph uses Board as its external task management backend. The `internal/board/` package wraps the `board` binary:

- **Plan lifecycle:** ready -> active -> complete (or blocked)
- **Task lifecycle:** todo -> claimed -> doing -> done (or blocked/skipped)
- **Agent context:** Single-call bootstrapping via `board plan context <id>`
- **Progress/Feedback:** Append-only logs for inter-iteration memory

The Board client (`internal/board/client.go`) shells out to `board` with `--api-url` and `--api-token` flags. Configuration is in `.ralph/config.yaml` under the `board:` section.

### Worktree-Based Isolation

Each plan executes in an isolated git worktree, preventing branch-switching conflicts:

```
repo/                          # Main worktree (always on base branch)
├── .ralph/
│   ├── config.yaml           # Project configuration
│   ├── prompts/              # Customizable agent prompts
│   └── worktrees/            # Execution worktrees (gitignored)
│       └── feat-my-feature/  # One per active plan
└── specs/                    # Feature specifications
```

**Completion Modes:**
- `pr` (default): Push branch, create PR via `gh` CLI (or Claude-generated PR), clean up worktree
- `merge`: Merge directly to base branch with `--no-ff`, push, delete feature branch + worktree
- `branch`: Push branch to origin only, no PR or merge

**Worktree Initialization:**

When a worktree is created, Ralph automatically initializes it:

1. **Copy .env files**: Copies `.env` (and others via config) from main worktree
2. **Custom hook**: Runs `.ralph/hooks/worktree-init` if executable
3. **Config commands**: Runs `worktree.init_commands` from config.yaml
4. **Auto-detection**: Installs dependencies based on lockfiles:
   - Node.js: `npm ci`, `yarn install`, `pnpm install`, `bun install`
   - PHP: `composer install`
   - Python: `pip install -r requirements.txt`, `poetry install`
   - Ruby: `bundle install`
   - Go: `go mod download`
   - Rust: `cargo fetch`

### Prompt System

Default prompts are embedded in the binary via `//go:embed` in `internal/prompt/templates.go`:

```
internal/prompt/prompts/
├── prompt.md                  # Main agent instructions (used each iteration)
└── pr_creation_prompt.md      # PR description generation prompt
```

Prompts use `{{PLACEHOLDER}}` syntax replaced by `prompt.Builder`:
- `{{PROJECT_NAME}}`, `{{PROJECT_DESCRIPTION}}` - from config.yaml
- `{{TEST_COMMAND}}`, `{{LINT_COMMAND}}`, `{{BUILD_COMMAND}}`, `{{DEV_COMMAND}}` - from config.yaml
- `{{ITERATION}}`, `{{MAX_ITERATIONS}}` - iteration state
- `{{FEATURE_BRANCH}}`, `{{BASE_BRANCH}}` - git branch info
- `{{PLAN_ID}}`, `{{BOARD_CONTEXT}}` - Board plan data injected into prompt

Custom prompts can be placed in `.ralph/prompts/` to override embedded defaults.

### State Management

Each iteration gets fresh context via `context.json` (stored at `.ralph/context.json` in the worktree):

```json
{
  "planId": 42,
  "featureBranch": "feat/my-feature",
  "baseBranch": "main",
  "iteration": 1,
  "maxIterations": 30
}
```

Progress persists externally:
- **Board tasks** - Task status, acceptance criteria, progress entries, feedback
- **Git commits** - Code changes committed after each iteration
- **context.json** - Iteration counter (only state in the worktree)

### Completion Detection

1. Agent outputs `<promise>COMPLETE</promise>` when it believes all tasks are done
2. Ralph checks Board stats: total tasks vs (done + skipped)
3. If Board confirms all tasks complete, the plan is verified done
4. If Board shows tasks remain, it's a false completion:
   - Ralph adds feedback to Board explaining which tasks are still incomplete
   - Consecutive false completions are tracked (counter resets on non-completion iterations)
   - After 5 consecutive false completions, Ralph halts with an error
5. Completion triggers the configured workflow (PR creation, merge, or branch push)

### Human Input / Blockers

When the agent encounters a task requiring human action, it signals a blocker:

```
<blocker>
Description of what is needed.
Action: Steps the human should take.
Resume: What happens once resolved.
</blocker>
```

**How it works:**
1. Agent outputs `<blocker>` marker when stuck on human-required task
2. Ralph detects the marker and sends Slack notification (if configured)
3. Human provides input via Board feedback or Slack thread reply
4. Agent reads feedback next iteration and continues

### Slack Notifications

Ralph supports Slack notifications via webhook or Bot API. Configuration in `.ralph/config.yaml`:

```yaml
slack:
  webhook_url: "https://hooks.slack.com/services/..."  # Simple notifications
  bot_token: "xoxb-..."    # For thread-based notifications
  app_token: "xapp-..."    # For Socket Mode (reply tracking)
  channel: "C0123456789"   # Channel ID (required for bot features)
  notify_start: true
  notify_complete: true
  notify_error: true
  notify_blocker: true
  notify_iteration: false  # Per-iteration updates (verbose)
```

The Go implementation in `internal/notify/` supports:
- Webhook notifications (simple, no dependencies)
- Bot API with thread tracking per plan
- Socket Mode for bidirectional communication
- Progress bar updates via message editing

### Skills (.claude/skills/)

```
ralph-spec/          # Feature specification management (durable documents)
ralph-plan/          # Task plan management (volatile execution state)
ralph-spec-to-plan/  # Generate plans from specs
```

## Key Files

| File | Purpose |
|------|---------|
| `cmd/ralph/main.go` | Entry point |
| `internal/cli/root.go` | Cobra root command and global flags |
| `internal/cli/run.go` | `ralph run` command |
| `internal/cli/worker.go` | `ralph worker` command |
| `internal/cli/init.go` | `ralph init` command |
| `internal/cli/status.go` | `ralph status` command |
| `internal/cli/cleanup.go` | `ralph cleanup` command |
| `internal/cli/version.go` | `ralph version` command |
| `internal/config/config.go` | Config struct, YAML loading, validation |
| `internal/config/defaults.go` | Default configuration values |
| `internal/config/detect.go` | Project type auto-detection |
| `internal/board/interface.go` | Board interface definition |
| `internal/board/client.go` | Board client (shells out to board) |
| `internal/board/types.go` | Board data types (Plan, Task, Criterion, etc.) |
| `internal/board/mock.go` | Mock Board client for testing |
| `internal/runner/loop.go` | Main iteration loop |
| `internal/runner/runner.go` | Claude CLI execution with streaming |
| `internal/runner/command.go` | Claude CLI argument builder |
| `internal/runner/context.go` | Iteration context (context.json) |
| `internal/runner/stream.go` | JSON stream parser for Claude output |
| `internal/runner/retry.go` | Retry logic with exponential backoff |
| `internal/runner/blocker.go` | Blocker extraction from Claude output |
| `internal/git/git.go` | Git CLI wrapper (status, commit, branch, worktree) |
| `internal/worktree/manager.go` | Worktree lifecycle management |
| `internal/worktree/deps.go` | Dependency auto-detection and installation |
| `internal/worktree/hooks.go` | Worktree initialization hooks |
| `internal/worker/worker.go` | Queue processor (Board polling, plan lifecycle) |
| `internal/worker/completion.go` | Completion modes (PR, merge, branch) |
| `internal/prompt/templates.go` | Embedded prompt templates (go:embed) |
| `internal/prompt/builder.go` | Prompt builder with placeholder substitution |
| `internal/notify/slack.go` | Slack Bot API notifications |
| `internal/notify/webhook.go` | Slack webhook notifications |
| `internal/notify/bot.go` | Slack Bot with Socket Mode |
| `internal/notify/threads.go` | Thread tracking for Slack notifications |
| `internal/log/log.go` | Structured logging with color |
| `.goreleaser.yaml` | Release configuration |
| `Makefile` | Build targets |
| `test/integration/integration_test.go` | End-to-end integration tests |
| `test/integration/fakeboard/` | Fake board binary for test isolation |

## Testing

```bash
# Run all unit tests
make test

# Run tests with verbose output
go test ./... -v

# Run specific package tests
go test ./internal/runner/... -v

# Run tests with race detector
make test-race

# Run short tests (skip integration)
make test-short

# Test coverage
make test-coverage

# Run integration tests (requires Claude CLI + built ralph binary)
make test-integration
```

### Unit Tests

Unit test fixtures are in `internal/*/testdata/` directories. Tests use mock implementations (e.g., `internal/board/mock.go`) for external dependencies.

### Integration Tests

Integration tests are in `test/integration/` and require:
- Built ralph binary (`make build`)
- Claude CLI available in PATH

They use a fake `board` binary (`test/integration/fakeboard/`) that stores state in a JSON file, providing full end-to-end testing without a real Board server.

Test cases:
- `TestSingleTask` - Basic single task completion with `ralph run --plan`
- `TestDependencies` - Task dependency ordering
- `TestProgressTracking` - Board progress entries created during execution
- `TestOneTaskPerIteration` - Verifies agent completes one task per iteration (separate commits)
- `TestWorkerQueue` - Worker queue processing with `ralph worker --once --merge`
- `TestDirtyState` - Dirty main worktree handling (worktree isolation)
- `TestWorktreeCleanup` - `ralph cleanup` command
- `TestCorePrinciples` - Comprehensive multi-task dependency chain with worker
- `TestSlackNotifications` - Slack Bot API notifications with mock server
- `TestBoardContextFailure` - Error handling with invalid Board configuration

Each test creates an isolated temp workspace with git repo, local bare origin, and seeded Board state.

## Development Patterns

### Adding New Commands

1. Create a new file in `internal/cli/` (e.g., `mycommand.go`)
2. Define a cobra command with `&cobra.Command{}`
3. Register it in `init()` with `rootCmd.AddCommand(myCmd)`
4. Add tests in `mycommand_test.go`

### Adding New Prompts

1. Add the prompt file to `internal/prompt/prompts/`
2. The `//go:embed prompts/*.md` directive in `templates.go` automatically includes it
3. Use it via `promptBuilder.Build("my_prompt.md", overrides)`

### Modifying Claude Execution

The runner package (`internal/runner/`) handles Claude CLI execution:
- `command.go` - Builds CLI arguments (model, output format, permissions)
- `runner.go` - Executes Claude with streaming output and process lifecycle
- `stream.go` - Parses JSON stream from Claude CLI (`stream-json` format)
- `retry.go` - Retry logic with exponential backoff and jitter
- `blocker.go` - Extracts blocker information from `<blocker>` tags
- `loop.go` - Main iteration loop with Board completion verification
- `context.go` - Iteration context management (context.json)

### Branch Management

Plans automatically get feature branches via worktree isolation:
- Branch name comes from Board plan's `feature_branch` field
- Each plan runs in its own worktree at `.ralph/worktrees/<branch-name>/`
- Main worktree stays on base branch (no stash/checkout needed)
- Agent runs inside the worktree and is told branch name via context.json
- On completion: PR created (default), direct merge, or branch push only

### Error Handling

Ralph uses standard Go error handling patterns:
- Sentinel errors for known conditions (e.g., `worker.ErrQueueEmpty`, `git.ErrMergeConflict`)
- Error wrapping with `fmt.Errorf("context: %w", err)` for error chains
- Non-retryable errors wrapped with `runner.WrapNonRetryable()` to skip retry logic
- Retryable error detection via error message patterns (rate limits, network errors, server 5xx)
- Graceful shutdown via context cancellation on SIGINT/SIGTERM
- Non-fatal errors (e.g., notification failures) are logged but don't stop execution

## Releasing

After implementing a feature or fix, always commit, tag, release, and reinstall:

```bash
# 1. Commit changes
git add <files>
git commit -m "Description of change"

# 2. Tag, push, and release
git push origin main
git tag -a v2.X.Y -m "Release v2.X.Y"
git push origin v2.X.Y

# 3. Reinstall locally
make install    # Installs to GOPATH/bin with version info from git tag

# 4. Verify
ralph version
```

Use patch version bumps (v2.3.1 -> v2.3.2) for fixes, minor bumps (v2.3.x -> v2.4.0) for features.

GoReleaser will automatically build binaries for all platforms and create a GitHub release. Release configuration is in `.goreleaser.yaml`. CI/CD workflows are in `.github/workflows/`.

```bash
# Test release build locally (optional)
make release-snapshot

# Dry run (build but don't publish, optional)
make release-dry-run
```

## Prod Server

Ralph runs on the same DigitalOcean droplet as Foreman and Spark, managed by Laravel Forge.

- **Server:** `forge@170.64.244.137`
- **Binary:** `~/.local/bin/ralph` (in PATH via `.bashrc`)
- **Update after release:**
  ```bash
  ssh forge@170.64.244.137 "curl -sL https://github.com/arvesolland/ralph/releases/download/v2.X.Y/ralph_2.X.Y_linux_amd64.tar.gz | tar xz ralph && mv ralph ~/.local/bin/ralph && ralph version"
  ```

## Gotchas

- **Board is required**: Ralph needs `board` in PATH and Board configuration in `.ralph/config.yaml`. Run `ralph init` to set up.
- **Feature branches come from Board**: The `feature_branch` field on the Board plan determines the branch name. Ralph creates the worktree on that branch.
- **Completion marker**: Agent may mention `<promise>COMPLETE</promise>` without meaning completion -- Board stats verification catches this.
- **False completion circuit breaker**: After 5 consecutive false completions (agent claims done but Board disagrees), Ralph halts.
- **Worktree cleanup**: If execution is interrupted, orphaned worktrees may remain. Run `ralph cleanup`.
- **Build artifacts**: Binary is named `ralph` (no extension on Unix, `.exe` on Windows). Add `ralph` to `.gitignore`.
- **Embedded prompts**: Default prompts are embedded via `//go:embed` in `internal/prompt/templates.go`.
- **Claude CLI flags**: When using `--output-format=stream-json`, the `--print` and `--verbose` flags are required.
- **Worker max iterations**: Worker defaults to 200 max iterations (vs 30 for `ralph run`).
- **Push after iteration**: Use `--push` flag to push after each iteration (prevents work loss on spot instances).
