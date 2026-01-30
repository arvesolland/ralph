# CLAUDE.md

This file provides guidance to Claude Code when working on the Ralph repository.

## Overview

Ralph is an autonomous AI development loop orchestration system implementing the "Ralph Wiggum technique" - fresh context per iteration with progress persisted in files and git.

## Commands

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

### Core Components

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
3. If plan is in `plans/current/`, triggers completion workflow (archive + optional PR)

### Slack Notifications (Optional)

Configure in `.ralph/config.yaml` to receive Slack notifications:

```yaml
slack:
  webhook_url: "https://hooks.slack.com/services/..."
  notify_start: true      # plan start (default: true)
  notify_complete: true   # plan completion (default: true)
  notify_iteration: false # each iteration (default: false)
  notify_error: true      # errors/max iterations (default: true)
```

Notifications are sent async and silently skip if `webhook_url` is not set.

### Skills (.claude/skills/)

```
ralph-spec/          # Feature specification management (durable documents)
ralph-plan/          # Task plan management (volatile execution state)
ralph-spec-to-plan/  # Generate plans from specs
```

## Key Files

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

- **stdout pollution**: Worker functions that return values must redirect output to stderr (`>&2`)
- **Plan validation removed**: Plans can be any markdown format; Claude handles parsing
- **Feature branches**: Created via worktree by bash, not Claude - prompt just tells agent the branch name
- **Test workspace**: Must use `git init -b main` since config expects "main" branch
- **Completion marker**: Agent may mention `<promise>COMPLETE</promise>` without meaning completion - haiku verification catches this
- **Worktree cleanup**: If execution is interrupted, orphaned worktrees may remain. Run `ralph-worker.sh --cleanup` to remove them
- **Plan file sync**: Plan file is copied into worktree; changes are synced back to `current/` after each iteration
