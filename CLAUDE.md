# CLAUDE.md

This file provides guidance to Claude Code when working on the Ralph repository.

## Overview

Ralph is an autonomous AI development loop orchestration system implementing the "Ralph Wiggum technique" - fresh context per iteration with progress persisted in files and git.

Ralph is written in Go. The codebase lives in `cmd/` and `internal/` directories following standard Go project layout.

## Commands

```bash
# Build the Go binary
make build              # Production build with version info
make build-dev          # Fast development build

# Run tests
make test               # Run all unit tests
make test-short         # Run tests without integration tests
make test-race          # Run tests with race detector
go test ./... -v        # Verbose test output

# Run ralph commands
./ralph init --detect   # Initialize project with auto-detection
./ralph status          # Show queue status
./ralph run plan.md     # Run implementation loop on a plan
./ralph worker          # Process queue (continuous)
./ralph worker --once   # Process one plan and exit
./ralph reset           # Move current plan back to pending
./ralph cleanup         # Remove orphaned worktrees
./ralph version         # Show version info

# Release (requires goreleaser)
make release-snapshot   # Test release build
make release-dry-run    # Dry run release
```

## Architecture

### Project Structure

```
cmd/ralph/              # Main entry point
internal/
├── cli/                # Cobra commands (init, run, worker, status, reset, cleanup, version)
├── config/             # Config loading, YAML parsing, project detection
├── plan/               # Plan parsing, task extraction, queue management
├── runner/             # Claude execution, streaming, retry logic, verification
├── git/                # Git operations (commit, branch, worktree)
├── worktree/           # Worktree management, file sync, hooks
├── notify/             # Slack notifications (webhook, bot API, Socket Mode)
├── prompt/             # Prompt template building with embedded defaults
└── log/                # Structured logging with color support
```

Key packages:
- `internal/runner/loop.go` - Main iteration loop (prompt → Claude → verify → commit)
- `internal/worker/worker.go` - Queue processor (pending → current → execute → complete)
- `internal/worktree/manager.go` - Worktree creation, cleanup, file sync
- `internal/notify/slack.go` - Slack Bot API with thread tracking

### Worktree-Based Isolation

Each plan executes in an isolated git worktree, preventing branch-switching conflicts:

```
repo/                          # Main worktree (always on base branch)
├── plans/
│   ├── pending/              # Queue of plans to run
│   ├── current/              # Currently active plan
│   └── complete/             # Archived plans
├── .ralph/
│   └── worktrees/            # Execution worktrees (gitignored)
│       └── feat-my-plan/     # One per active plan
```

**Concurrency Protection (Three-Layer Lock):**
1. **File location lock**: Plan in `current/` = claimed (can't move same file twice)
2. **Git worktree lock**: Branch checked out = locked (`fatal: '<branch>' is already checked out`)
3. **Directory lock**: Worktree exists = execution in progress

**Completion Modes:**
- `--pr` (default): Push branch, create PR via `gh`, archive plan, clean up worktree
- `--merge`: Merge directly to base branch, archive, delete branch + worktree
- Config: `completion.mode: pr|merge` in `.ralph/config.yaml`

**Commands:**
```bash
ralph cleanup     # Remove orphaned worktrees
ralph status      # Show queue and worktree status
ralph reset       # Reset current plan to pending (start over)
```

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

Configure in `.ralph/config.yaml`:
```yaml
worktree:
  # Files to copy from main worktree (default: .env)
  copy_env_files: ".env, .env.local"

  # Custom init commands (skips auto-detection)
  init_commands: "npm ci && cp ../.env.example .env"
```

Or create `.ralph/hooks/worktree-init` (must be executable):
```bash
#!/bin/bash
# Custom worktree initialization
# $PWD = worktree path, $MAIN_WORKTREE = main repo path
cp "$MAIN_WORKTREE/.env" .env
npm ci
php artisan key:generate
```

### Prompt System

Default prompts are embedded in the binary via `//go:embed` in `internal/prompt/templates.go`:

```
internal/prompt/prompts/
├── prompt.md                  # Worker agent instructions (main implementation)
├── worker_prompt.md           # Alternative worker prompt
├── plan_reviewer_prompt.md    # Plan optimization before execution
└── plan-spec.md               # Plan format specification
```

Prompts use `{{PLACEHOLDER}}` syntax replaced by `prompt.Builder`:
- `{{PROJECT_NAME}}`, `{{PROJECT_DESCRIPTION}}` - from config.yaml
- `{{PRINCIPLES}}`, `{{PATTERNS}}`, `{{BOUNDARIES}}`, `{{TECH_STACK}}` - from .ralph/*.md files
- `{{TEST_COMMAND}}`, `{{LINT_COMMAND}}` - from config.yaml commands

Custom prompts can be placed in `.ralph/prompts/` to override embedded defaults.

### State Management

Each iteration gets fresh context via `context.json`:
```json
{
  "planFile": "path/to/plan.md",
  "featureBranch": "feat/plan-name",
  "baseBranch": "main",
  "iteration": 1,
  "maxIterations": 30
}
```

Progress persists in:
- Plan file (checkbox updates, status changes)
- `<plan>.progress.md` (gotchas/learnings)
- Git commits

### Completion Detection

1. Agent outputs `<promise>COMPLETE</promise>` when all tasks done
2. Haiku verification confirms plan is actually complete (prevents false positives)
3. If verification fails, detailed reason is written to `<plan>.feedback.md` so agent can address it
4. If plan is in `plans/current/`, triggers completion workflow (archive + optional PR)

### Slack Notifications (Optional)

Configure in `.ralph/config.yaml` to receive Slack notifications:

```yaml
slack:
  webhook_url: "https://hooks.slack.com/services/..."
  notify_start: true      # plan start (default: true)
  notify_complete: true   # plan completion (default: true)
  notify_iteration: false # each iteration (default: false)
  notify_error: true      # errors/max iterations (default: true)
  notify_blocker: true    # when human input needed (default: true)
```

Notifications are sent async and silently skip if `webhook_url` is not set.

### Human Input / Blockers

When the agent encounters a task requiring human action (e.g., making a GitHub package public, approving a deployment), it signals a blocker:

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
3. Human provides input via `<plan>.feedback.md` file
4. Agent reads feedback file next iteration and continues

**Feedback file format** (`plans/current/<plan>.feedback.md`):
```markdown
# Feedback: plan-name

## Pending
- [2024-01-30 14:32] Package is now public, you can verify the pull

## Processed
<!-- Agent moves items here after reading -->
```

**Files involved:**
- `<plan>.feedback.md` - Human writes here, agent reads and acts
- `<plan>.blockers` - Tracks notified blockers (avoids Slack spam)
- `.ralph/slack_threads.json` - Maps Slack threads to plans (for reply tracking)

Both feedback and blocker files are synced between queue directory and worktree.

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
```

The Go implementation in `internal/notify/` supports:
- Webhook notifications (simple, no dependencies)
- Bot API with thread tracking per plan
- Socket Mode for bidirectional communication

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
| `internal/runner/loop.go` | Main iteration loop |
| `internal/runner/runner.go` | Claude CLI execution with streaming |
| `internal/runner/verify.go` | Plan completion verification via Haiku |
| `internal/worker/worker.go` | Queue processor |
| `internal/config/config.go` | Config struct and YAML loading |
| `internal/config/detect.go` | Project type auto-detection |
| `internal/plan/plan.go` | Plan parsing and task extraction |
| `internal/plan/queue.go` | Plan queue management (pending/current/complete) |
| `internal/git/git.go` | Git CLI wrapper |
| `internal/worktree/manager.go` | Worktree lifecycle management |
| `internal/worktree/sync.go` | File sync between worktrees |
| `internal/prompt/templates.go` | Embedded prompt templates |
| `internal/notify/slack.go` | Slack Bot API notifications |
| `internal/notify/webhook.go` | Slack webhook notifications |
| `internal/log/log.go` | Structured logging with color |
| `.goreleaser.yaml` | Release configuration |
| `Makefile` | Build targets |

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

# Run integration tests (requires claude CLI)
make test-integration
```

Test fixtures are in `internal/*/testdata/` directories. Integration test plans are in `internal/integration/testdata/plans/`.

## Development Patterns

### Adding New Commands

1. Create a new file in `internal/cli/` (e.g., `mycommand.go`)
2. Define a cobra command with `&cobra.Command{}`
3. Register it in `init()` with `rootCmd.AddCommand(myCmd)`
4. Add tests in `mycommand_test.go`

### Adding New Prompts

1. Add the prompt file to `internal/prompt/prompts/`
2. Update `internal/prompt/templates.go` to embed it with `//go:embed`
3. Add a method to `Builder` to build the new prompt type

### Modifying Claude Execution

The runner package (`internal/runner/`) handles Claude CLI execution:
- `command.go` - Builds CLI arguments
- `runner.go` - Executes Claude with streaming output
- `stream.go` - Parses JSON stream from Claude
- `retry.go` - Retry logic for transient failures
- `verify.go` - Plan completion verification via Haiku

### Branch Management

Plans automatically get feature branches via worktree isolation:
- Branch name: `feat/<plan-name>` (derived from plan filename)
- Each plan runs in its own worktree at `.ralph/worktrees/feat-<plan>/`
- Main worktree stays on base branch (no stash/checkout needed)
- Agent runs inside the worktree and is told branch name via context.json
- On completion: PR created (default) or direct merge (`--merge` flag)

### Error Handling

- `set -e` in all scripts
- `log_error`, `log_warn`, `log_success` for colored output
- Exit codes: 0 = success, 1 = max iterations or error

## Releasing

```bash
# Test release build locally
make release-snapshot

# Dry run (build but don't publish)
make release-dry-run

# Create a release (requires git tag)
git tag -a v1.0.0 -m "Release v1.0.0"
git push origin v1.0.0

# GoReleaser will automatically:
# - Build binaries for all platforms (linux, darwin, windows × amd64, arm64)
# - Create GitHub release with binaries
# - Update Homebrew formula (if configured)
```

Release configuration is in `.goreleaser.yaml`. CI/CD workflows are in `.github/workflows/`.

## Gotchas

- **Plan validation removed**: Plans can be any markdown format; Claude handles parsing
- **Feature branches**: Created via worktree by Ralph, not Claude - prompt just tells agent the branch name
- **Completion marker**: Agent may mention `<promise>COMPLETE</promise>` without meaning completion - Haiku verification catches this
- **Verification failures**: When Haiku says plan is incomplete, detailed explanation is written to feedback file for agent to address
- **Worktree cleanup**: If execution is interrupted, orphaned worktrees may remain. Run `ralph cleanup`
- **Plan file sync**: Plan file is copied into worktree; changes are synced back to `current/` after each iteration
- **Build artifacts**: Binary is named `ralph` (no extension on Unix, `.exe` on Windows). Add `ralph` to `.gitignore`
- **Test fixtures**: Located in `internal/*/testdata/` - some tests create temp directories that may need cleanup on failure
- **Embedded prompts**: Default prompts are embedded via `//go:embed` in `internal/prompt/templates.go`
- **Claude CLI flags**: When using `--output-format=stream-json`, the `--print` and `--verbose` flags are required
