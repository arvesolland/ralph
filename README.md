# Ralph

[![Release](https://img.shields.io/github/v/release/arvesolland/ralph)](https://github.com/arvesolland/ralph/releases)
[![License](https://img.shields.io/github/license/arvesolland/ralph)](LICENSE)

An implementation of the Ralph Wiggum technique for autonomous AI development.

## Why It Works

**Fresh context per iteration.** Like malloc'ing a new array instead of appending - each Claude Code invocation gets a clean context window, avoiding the pollution and degradation that happens when LLMs accumulate conversation history. Progress persists in ATM (Agent Task Manager) and git, not in context.

**External memory architecture.** Agents don't carry state - they read it:
- **ATM Plans** - Structured tasks with dependencies, acceptance criteria, and status tracked via the ATM API.
- **ATM Progress/Feedback** - Append-only logs for institutional memory and human-in-the-loop communication.
- **Git Commits** - Code changes persist between iterations. Each iteration commits its work.

**Collective learning.** Each agent writes progress back to ATM before exiting. Future agents read this context and don't repeat mistakes. The system gets smarter with every iteration.

```
Fresh context window (clean slate)
    -> Fetches plan context from ATM (tasks, progress, feedback)
    -> Knows exactly what to do (structured task with acceptance criteria)
    -> Executes ONE task
    -> Updates task status in ATM
    -> Commits & exits
    -> Next agent picks up smarter
```

## Features

- **Single Binary** - Cross-platform Go binary with no dependencies
- **ATM Integration** - Polls ATM for ready plans, tracks tasks, progress, and feedback via API
- **Structured Plans** - Tasks with dependencies, status tracking, and acceptance criteria
- **Worktree Isolation** - Each plan runs in its own git worktree (no branch switching conflicts)
- **Auto PR Creation** - Create pull requests via `gh` CLI on completion
- **Slack Notifications** - Real-time updates via webhooks or Bot API
- **False Completion Guard** - ATM stats verification prevents premature completion claims
- **Config-Driven** - Customize prompts, commands, and behavior via config files

## Installation

### Homebrew (macOS/Linux)

```bash
brew install --cask arvesolland/tap/ralph
```

### Binary Download

Download the latest release for your platform from [GitHub Releases](https://github.com/arvesolland/ralph/releases).

```bash
# macOS (Apple Silicon)
curl -LO https://github.com/arvesolland/ralph/releases/latest/download/ralph_darwin_arm64.tar.gz
tar xzf ralph_darwin_arm64.tar.gz
sudo mv ralph /usr/local/bin/

# macOS (Intel)
curl -LO https://github.com/arvesolland/ralph/releases/latest/download/ralph_darwin_amd64.tar.gz
tar xzf ralph_darwin_amd64.tar.gz
sudo mv ralph /usr/local/bin/

# Linux (x64)
curl -LO https://github.com/arvesolland/ralph/releases/latest/download/ralph_linux_amd64.tar.gz
tar xzf ralph_linux_amd64.tar.gz
sudo mv ralph /usr/local/bin/
```

### Build from Source

```bash
git clone https://github.com/arvesolland/ralph.git
cd ralph
make build
sudo mv ralph /usr/local/bin/
```

## Quick Start

### 1. Initialize a Project

```bash
cd your-project
ralph init --detect
```

This creates:
- `.ralph/config.yaml` - Project configuration (including ATM settings)
- `.ralph/prompts/` - Customizable agent prompts
- `specs/INDEX.md` - Feature specification index

During init, you'll be prompted for ATM configuration (project slug, API URL, API token).

### 2. Create a Plan in ATM

Create plans in the ATM task manager via the `atm-cli` or ATM web UI:

```bash
atm-cli plan create <project-slug> --title "My Feature"
atm-cli task create <plan-id> --title "First task" --description "..."
atm-cli plan status <plan-id> --status ready
```

### 3. Run Ralph

```bash
# Run a single plan by ID
ralph run --plan 42

# Or use the worker to process the ATM queue
ralph worker --once
```

Ralph will:
1. Fetch plan details and context from ATM
2. Create a worktree for the plan's feature branch
3. Build a prompt with ATM context (tasks, progress, feedback)
4. Execute Claude Code to work on the next available task
5. Verify completion via ATM task stats
6. Commit changes after each iteration
7. Repeat until all tasks are complete
8. Create a PR (default), merge, or push branch only

## Commands

### `ralph init`

Initialize Ralph configuration in a project.

```bash
ralph init [flags]

Flags:
  --detect    Auto-detect project type and commands
```

### `ralph run`

Run the iteration loop on a specific ATM plan.

```bash
ralph run --plan <plan-id> [flags]

Flags:
  --plan int              ATM plan ID (required)
  --max int               Max iterations (default 30)
  --completion-mode str   Completion mode: pr, merge, or branch (default from config)
  --push                  Push to remote after each iteration
```

Example:
```bash
ralph run --plan 42
ralph run --plan 42 --max 100
ralph run --plan 42 --completion-mode merge
```

### `ralph worker`

Process plans from the ATM queue.

```bash
ralph worker [flags]

Flags:
  --once                  Process one plan and exit
  --pr                    Create PR on completion (default)
  --merge                 Merge to base branch on completion
  --branch                Push to branch only, no PR or merge
  --interval duration     Poll interval when queue empty (default 30s)
  --max int               Max iterations per plan (default 200)
  --sync                  Pull from remote before each queue check
  --sync-interval dur     Minimum time between syncs (e.g., 60s)
  --push                  Push to remote after each iteration
```

### `ralph status`

Display project status from ATM (active plan, task stats, available tasks, recent progress).

```bash
ralph status
```

### `ralph cleanup`

Remove orphaned worktrees.

```bash
ralph cleanup [flags]

Flags:
  --dry-run    Show what would be removed without removing
```

### `ralph version`

Show version information.

```bash
ralph version
```

### Global Flags

```bash
  -c, --config string   Config file path (default ".ralph/config.yaml")
  -v, --verbose         Enable verbose output (debug level)
  -q, --quiet           Suppress informational output
      --no-color        Disable color output
```

## Configuration

### config.yaml

Main configuration file at `.ralph/config.yaml`:

```yaml
project:
  name: "My Project"
  description: "A web application"

git:
  base_branch: "main"

commands:
  test: "npm test"
  lint: "npm run lint"
  build: "npm run build"

completion:
  mode: "pr"  # or "merge" or "branch"

worktree:
  copy_env_files: ".env, .env.local"
  init_commands: ""  # Custom init (skips auto-detection if set)

atm:
  project_slug: "my-project"
  api_url: "https://atm.example.com/api"
  api_token: "your-api-token"
  bin_path: "atm-cli"  # Path to atm-cli binary (default: "atm-cli")

worker:
  sync: false           # Pull from remote before each queue check
  sync_interval: "60s"  # Minimum time between syncs

slack:
  webhook_url: "https://hooks.slack.com/services/..."
  bot_token: "xoxb-..."  # Optional: for thread-based notifications
  app_token: "xapp-..."  # Optional: for Socket Mode
  channel: "C0123456789"  # Required for bot features
  notify_start: true
  notify_complete: true
  notify_error: true
  notify_blocker: true
  notify_iteration: false
```

### Prompt Customization

Default prompts are embedded in the binary. Override them by placing files in `.ralph/prompts/`:

| File | Purpose |
|------|---------|
| `prompt.md` | Main agent instructions (injected each iteration) |
| `pr_creation_prompt.md` | PR description generation prompt |

Prompts use `{{PLACEHOLDER}}` syntax for dynamic values:
- `{{PROJECT_NAME}}`, `{{PROJECT_DESCRIPTION}}` - from config.yaml
- `{{TEST_COMMAND}}`, `{{LINT_COMMAND}}`, `{{BUILD_COMMAND}}` - from config.yaml
- `{{ITERATION}}`, `{{MAX_ITERATIONS}}` - current iteration state
- `{{FEATURE_BRANCH}}`, `{{BASE_BRANCH}}` - git branch info
- `{{PLAN_ID}}`, `{{ATM_CONTEXT}}` - ATM plan data

### Directory Structure

```
your-project/
├── .ralph/
│   ├── config.yaml       # Project configuration
│   ├── prompts/          # Customizable agent prompts
│   │   └── prompt.md     # Main agent instructions
│   └── worktrees/        # Git worktrees (gitignored)
└── specs/
    └── INDEX.md          # Feature specification index
```

## ATM Workflow

Ralph uses the ATM (Agent Task Manager) service for plan and task management:

1. **Ready** - Plans in ATM with status "ready" are available for processing
2. **Active** - Ralph activates a plan and begins the iteration loop
3. **Complete** - All tasks done in ATM, plan marked complete, PR/merge/branch created
4. **Blocked** - Plan marked blocked if max iterations reached or errors occur

```bash
# Create a plan in ATM
atm-cli plan create my-project --title "Add user auth"

# Add tasks with acceptance criteria
atm-cli task create <plan-id> --title "Create login endpoint"

# Mark plan as ready for Ralph
atm-cli plan status <plan-id> --status ready

# Ralph picks it up automatically
ralph worker

# Check status
ralph status
```

## Worktree Isolation

Each plan runs in its own git worktree:
- No branch switching in main worktree
- Parallel plan execution (different workers)
- Clean separation of work

```
.ralph/worktrees/
└── feat-my-plan/    # Worktree for plan "my-plan"
```

Worktrees are automatically:
- Created when a plan is activated
- Initialized with dependencies (`npm ci`, `go mod download`, etc.)
- Removed when the plan completes

## Slack Integration

### Webhook Notifications

Basic notifications via incoming webhook:

```yaml
slack:
  webhook_url: "https://hooks.slack.com/services/..."
```

### Bot API with Thread Replies

Full-featured notifications with thread tracking:

```yaml
slack:
  bot_token: "xoxb-..."
  app_token: "xapp-..."
  channel: "C0123456789"
```

This enables:
- Thread-based notifications per plan
- Reply tracking (human replies become feedback)
- Blocker deduplication

## Development

### Building from Source

```bash
git clone https://github.com/arvesolland/ralph.git
cd ralph

make build-dev     # Fast development build
make build         # Production build with version info
make test          # Run all tests
make test-race     # Run tests with race detector
make lint          # Run golangci-lint
```

### Project Structure

```
cmd/ralph/              # Main entry point
internal/
├── cli/                # Cobra commands (init, run, worker, status, cleanup, version)
├── config/             # Config loading, YAML parsing, project detection
├── atm/                # ATM client (shells out to atm-cli)
├── runner/             # Claude execution, streaming, retry, iteration loop
├── git/                # Git operations (commit, branch, status, worktree)
├── worktree/           # Worktree management, dependency auto-detection, hooks
├── worker/             # Queue processor orchestration, completion modes
├── notify/             # Slack notifications (webhook, bot, threads)
├── prompt/             # Prompt template building with embedded defaults
└── log/                # Structured logging with color support
```

### Key Packages

- **atm** - ATM client wrapping the `atm-cli` binary for plan/task management
- **runner** - Core Claude CLI execution with JSON streaming, retry logic, and ATM-based completion verification
- **worker** - Queue processor that polls ATM for ready plans, runs iteration loops, handles PR/merge/branch completion
- **worktree** - Git worktree lifecycle management with dependency auto-detection

See [CLAUDE.md](CLAUDE.md) for detailed development guidance.

## Troubleshooting

### "claude: command not found"

Ensure Claude Code CLI is installed and in your PATH:
```bash
which claude
```

### "gh: command not found" (PR creation fails)

Install GitHub CLI for PR creation:
```bash
brew install gh
gh auth login
```

Or use merge mode instead:
```bash
ralph worker --merge
```

### "atm.project_slug not configured"

Run `ralph init` to set up ATM configuration, or manually add to `.ralph/config.yaml`:
```yaml
atm:
  project_slug: "my-project"
  api_url: "https://atm.example.com/api"
  api_token: "your-token"
```

### "branch is already checked out"

Another worktree has the branch checked out. Either:
- Wait for the other worker to finish
- Run `ralph cleanup` to remove orphaned worktrees
- Manually remove the conflicting worktree

### Worktree has uncommitted changes

Ralph won't clean up worktrees with uncommitted changes for safety. Either:
- Commit or discard the changes manually
- The changes will be preserved for review

### False completion claims

If the agent outputs `<promise>COMPLETE</promise>` but ATM stats show tasks remain:
1. Ralph automatically adds feedback to ATM explaining which tasks are incomplete
2. The next iteration reads this feedback and addresses it
3. After 5 consecutive false completions, Ralph halts to prevent infinite loops

## Requirements

- **Claude Code CLI** - `claude` command must be available in PATH
- **ATM CLI** - `atm-cli` command for task management (configurable via `atm.bin_path`)
- **Git** - For version control and worktree management
- **GitHub CLI** (optional) - For PR creation (`gh`)

## License

MIT
