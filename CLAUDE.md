# CLAUDE.md

This file provides guidance to Claude Code when working on the Ralph repository.

## Overview

Ralph is an autonomous AI development loop orchestration system implementing the "Ralph Wiggum technique" - fresh context per iteration with progress persisted in files and git.

## Versions

Ralph has two implementations:
- **Go version** (recommended) - Single binary at `cmd/ralph/`
- **Bash version** (legacy) - Shell scripts in `scripts/ralph/`

Both use the same config format (`.ralph/config.yaml`) and plan format.

## Commands (Go Version)

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

## Commands (Bash Version - Legacy)

```bash
# Run integration tests (real Claude execution, no mocking)
./test/run-tests.sh

# Run specific test
./test/run-tests.sh --test single-task

# Available tests: single-task, dependencies, progress, loose-format, worker-queue, dirty-state, worktree-cleanup, core-principles

# Check script versions
./ralph.sh --version
./ralph-worker.sh --version

# Test install in a temp directory
cd $(mktemp -d) && git init && /path/to/ralph/install.sh --local
```

## Architecture

### Go Version Structure

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

### Bash Version Components (Legacy)

```
ralph.sh            # Main implementation loop - iterates until plan complete
ralph-worker.sh     # Queue management - pending → current → complete
ralph-init.sh       # Project initialization with --detect or --ai modes
lib/config.sh       # Shared functions: config loading, prompt building, logging
lib/worktree.sh     # Worktree isolation for plan execution
```

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
ralph-worker.sh --cleanup     # Remove orphaned worktrees
ralph-worker.sh --status      # Show queue and worktree status
ralph-worker.sh --reset       # Reset current plan to pending (start over)
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

```
prompts/base/
├── prompt.md                  # Worker agent instructions (main implementation)
├── plan_reviewer_prompt.md    # Plan optimization before execution
└── plan-spec.md               # Plan format specification
```

Prompts use `{{PLACEHOLDER}}` syntax replaced by `build_prompt()` in lib/config.sh:
- `{{PROJECT_NAME}}`, `{{PROJECT_DESCRIPTION}}` - from config.yaml
- `{{PRINCIPLES}}`, `{{PATTERNS}}`, `{{BOUNDARIES}}`, `{{TECH_STACK}}` - from .ralph/*.md files
- `{{TEST_COMMAND}}`, `{{LINT_COMMAND}}` - from config.yaml commands

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

### Slack Bot (Reply Tracking)

The Slack bot handles thread replies and writes them to feedback files. It auto-starts when `ralph-worker.sh` runs if configured.

**Setup:**
1. Install: `pip install -r slack-bot/requirements.txt`
2. Create `~/.ralph/slack.env` with your tokens (for global bot):
   ```
   SLACK_BOT_TOKEN=xoxb-...
   SLACK_APP_TOKEN=xapp-...
   ```
3. Or export them as environment variables

**Configuration in `.ralph/config.yaml`:**
```yaml
slack:
  webhook_url: "https://hooks.slack.com/services/..."  # Fallback
  channel: "C0123456789"    # Channel ID (required for reply tracking)
  global_bot: true          # Use single bot for all repos (recommended)
  notify_blocker: true
```

**Modes:**
- `global_bot: true` - One bot per machine at `~/.ralph/`, handles multiple repos
- `global_bot: false` - One bot per repo at `.ralph/` (default)

**Auto-start:** When `ralph-worker.sh` runs, it automatically starts the bot if:
- `SLACK_BOT_TOKEN` and `SLACK_APP_TOKEN` are set
- `slack.channel` is configured
- Bot isn't already running

**Manual start:** `python slack-bot/ralph_slack_bot.py --global`

See `slack-bot/README.md` for full setup instructions.

### Skills (.claude/skills/)

```
ralph-spec/          # Feature specification management (durable documents)
ralph-plan/          # Task plan management (volatile execution state)
ralph-spec-to-plan/  # Generate plans from specs
```

## Key Files

### Go Version

| File | Purpose |
|------|---------|
| `cmd/ralph/main.go` | Entry point |
| `internal/cli/root.go` | Cobra root command and global flags |
| `internal/cli/run.go` | `ralph run` command |
| `internal/cli/worker.go` | `ralph worker` command |
| `internal/runner/loop.go` | Main iteration loop |
| `internal/worker/worker.go` | Queue processor |
| `internal/config/config.go` | Config struct and YAML loading |
| `internal/plan/plan.go` | Plan parsing and task extraction |
| `internal/git/git.go` | Git CLI wrapper |
| `internal/worktree/manager.go` | Worktree lifecycle management |
| `internal/prompt/templates.go` | Embedded prompt templates |
| `.goreleaser.yaml` | Release configuration |
| `Makefile` | Build targets |

### Bash Version (Legacy)

| File | Purpose |
|------|---------|
| `ralph.sh` | Main entry point - runs implementation loop |
| `ralph-worker.sh` | Queue management and plan lifecycle |
| `lib/config.sh` | All shared functions (`build_prompt`, `config_get`, logging) |
| `lib/worktree.sh` | Worktree creation, cleanup, and locking helpers |
| `prompts/base/prompt.md` | Agent instructions for implementation |
| `test/run-tests.sh` | Integration test suite |
| `test/lib/helpers.sh` | Test utilities and assertions |
| `install.sh` | Installer script |

## Testing

### Go Version

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
```

Test fixtures are in `internal/*/testdata/` directories.

### Bash Version (Legacy)

Tests run real Claude against test plans in isolated git workspaces:

```bash
# Full suite
./test/run-tests.sh

# Keep workspace for debugging
./test/run-tests.sh --test single-task --keep

# Workspace at /var/folders/.../test-workspace
```

Test plans in `test/plans/`:
- `01-single-task.md` - Basic task completion
- `02-dependencies.md` - Task dependency ordering
- `03-progress-tracking.md` - Progress file creation
- `04-loose-format.md` - Non-strict plan format support

## Development Patterns

### Adding New Prompts

1. Create prompt in `prompts/base/`
2. Use `{{PLACEHOLDER}}` for config injection
3. Call `build_prompt "path/to/prompt.md" "$CONFIG_DIR"` in scripts

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

### Go Version

```bash
# Test release build locally
make release-snapshot

# Create a release (requires git tag)
git tag -a v1.0.0 -m "Release v1.0.0"
git push origin v1.0.0

# GoReleaser will automatically:
# - Build binaries for all platforms (linux, darwin, windows × amd64, arm64)
# - Create GitHub release with binaries
# - Update Homebrew formula (if configured)
```

Release configuration is in `.goreleaser.yaml`.

### Bash Version (Legacy)

```bash
# Install hooks (one time)
./hooks/install-hooks.sh

# Commits auto-update CHANGELOG.md based on conventional commit prefix:
# feat: → Added, fix: → Fixed, refactor: → Changed, chore: → skipped

# Create release (moves [Unreleased] to version, bumps VERSION, tags)
./ralph-release.sh          # Auto-detect: Breaking→major, Added→minor, else→patch
./ralph-release.sh minor    # Force minor bump

# Push release
git push && git push --tags
```

## Gotchas

### Both Versions

- **Plan validation removed**: Plans can be any markdown format; Claude handles parsing
- **Feature branches**: Created via worktree by Ralph, not Claude - prompt just tells agent the branch name
- **Completion marker**: Agent may mention `<promise>COMPLETE</promise>` without meaning completion - Haiku verification catches this
- **Verification failures**: When Haiku says plan is incomplete, detailed explanation is written to feedback file for agent to address
- **Worktree cleanup**: If execution is interrupted, orphaned worktrees may remain. Run `ralph cleanup` (Go) or `ralph-worker.sh --cleanup` (bash)
- **Plan file sync**: Plan file is copied into worktree; changes are synced back to `current/` after each iteration

### Go Version Specific

- **Build artifacts**: Binary is named `ralph` (no extension on Unix, `.exe` on Windows). Add `ralph` to `.gitignore`
- **Test fixtures**: Located in `internal/*/testdata/` - some tests create temp directories that may need cleanup on failure
- **Mock scripts**: Integration tests use mock scripts in `internal/runner/testdata/mock-*.sh` - must be executable
- **Embedded prompts**: Default prompts are embedded via `//go:embed` in `internal/prompt/templates.go`

### Bash Version Specific (Legacy)

- **stdout pollution**: Worker functions that return values must redirect output to stderr (`>&2`)
- **Test workspace**: Must use `git init -b main` since config expects "main" branch

## Migration from Bash to Go

The Go version is a drop-in replacement for the bash scripts. Both share:
- Same config format (`.ralph/config.yaml`)
- Same plan format (markdown with checkboxes)
- Same directory structure (`plans/pending|current|complete/`)
- Same completion modes (`--pr`, `--merge`)

Command mapping:

| Bash | Go |
|------|----|
| `./ralph.sh plan.md` | `ralph run plan.md` |
| `./ralph-worker.sh` | `ralph worker` |
| `./ralph-worker.sh --status` | `ralph status` |
| `./ralph-worker.sh --reset` | `ralph reset` |
| `./ralph-worker.sh --cleanup` | `ralph cleanup` |
| `./ralph-init.sh --detect` | `ralph init --detect` |

**Note:** The bash scripts will be deprecated in a future release. New development should use the Go version.
