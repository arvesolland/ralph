# Ralph

[![Release](https://img.shields.io/github/v/release/arvesolland/ralph)](https://github.com/arvesolland/ralph/releases)
[![License](https://img.shields.io/github/license/arvesolland/ralph)](LICENSE)

An implementation of the Ralph Wiggum technique for autonomous AI development.

## Why It Works

**Fresh context per iteration.** Like malloc'ing a new array instead of appending - each Claude Code invocation gets a clean context window, avoiding the pollution and degradation that happens when LLMs accumulate conversation history. Progress persists in files and git, not in context.

**External memory architecture.** Agents don't carry state - they read it:
- **Specs** (`specs/`) - Durable knowledge base. Entry point for understanding what exists and why.
- **Plans** (`plans/`) - Volatile execution state. What to do next, with dependencies and checkboxes.
- **Progress** (`<plan>.progress.md`) - Per-plan institutional memory. Gotchas that future iterations read to avoid mistakes.

**Collective learning.** Each agent writes learnings back before exiting. Future agents read these gotchas and don't repeat mistakes. The system gets smarter with every iteration.

```
Fresh context window (clean slate)
    → Reads specs (understands system)
    → Reads progress (learns from past gotchas)
    → Reads plan (knows exactly what to do)
    → Executes ONE subtask
    → Writes learnings back
    → Commits & exits
    → Next agent picks up smarter
```

## Features

- **Single Binary** - Cross-platform Go binary with no dependencies
- **Structured Plans** - Tasks with dependencies, status tracking, and acceptance criteria
- **Worktree Isolation** - Each plan runs in its own git worktree (no branch switching conflicts)
- **Task Queue** - File-based queue for processing multiple plans
- **Auto PR Creation** - Create pull requests via `gh` CLI on completion
- **Slack Notifications** - Real-time updates via webhooks or Bot API
- **Progress Tracking** - Per-plan learnings that future iterations read
- **Config-Driven** - Customize prompts via config files, not code

## Installation

### Homebrew (macOS/Linux)

```bash
brew install arvesolland/tap/ralph
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
- `.ralph/config.yaml` - Project configuration
- `plans/` - Queue directories (pending, current, complete)
- `specs/INDEX.md` - Feature specification index

### 2. Create a Plan

Create a plan file at `plans/pending/my-feature.md`:

```markdown
# Plan: My Feature

**Status:** pending

## Context
Brief description of what this plan implements.

---

## Tasks

### T1: First Task
> Description of the task

**Requires:** —
**Status:** open

**Done when:**
- [ ] Acceptance criterion 1
- [ ] Acceptance criterion 2

**Subtasks:**
1. [ ] First implementation step
2. [ ] Second implementation step

---

### T2: Second Task
> Description of the second task

**Requires:** T1
**Status:** open

**Done when:**
- [ ] Task 2 acceptance criterion

**Subtasks:**
1. [ ] Implementation step

---

## Discovered
<!-- New work found during implementation -->
```

### 3. Run Ralph

```bash
# Run a single plan
ralph run plans/pending/my-feature.md

# Or use the worker to process the queue
ralph worker --once
```

Ralph will:
1. Create a worktree for the plan's feature branch
2. Find the first task with met dependencies
3. Execute Claude Code with the implementation prompt
4. Verify completion and commit changes
5. Repeat until all tasks are complete
6. Create a PR (if configured)

## Commands

### `ralph init`

Initialize Ralph configuration in a project.

```bash
ralph init [flags]

Flags:
  --detect    Auto-detect project type and commands
```

### `ralph run`

Run the implementation loop on a specific plan.

```bash
ralph run <plan-file> [flags]

Flags:
  --max int       Max iterations (default 30)
  --review        Run plan review before execution
```

### `ralph worker`

Process plans from the queue.

```bash
ralph worker [flags]

Flags:
  --once              Process one plan and exit
  --pr                Create PR on completion (default)
  --merge             Merge to base branch on completion
  --interval duration Poll interval when queue empty (default 30s)
  --max int           Max iterations per plan (default 30)
```

### `ralph status`

Display queue status and current plan information.

```bash
ralph status
```

### `ralph reset`

Move the current plan back to pending.

```bash
ralph reset [flags]

Flags:
  --force, -f       Skip confirmation prompt
  --keep-worktree   Don't remove the worktree
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
  mode: "pr"  # or "merge"

worktree:
  copy_env_files: ".env, .env.local"
  init_commands: ""  # Custom init (skips auto-detection if set)

slack:
  webhook_url: "https://hooks.slack.com/services/..."
  bot_token: "xoxb-..."  # Optional: for thread replies
  app_token: "xapp-..."  # Optional: for Socket Mode
  channel: "C0123456789"  # Required for bot features
  notify_start: true
  notify_complete: true
  notify_error: true
  notify_blocker: true
  notify_iteration: false
```

### Prompt Customization

Override default prompts by creating files in `.ralph/`:

| File | Purpose |
|------|---------|
| `principles.md` | Development principles injected into prompts |
| `patterns.md` | Code patterns for your project |
| `boundaries.md` | Files Ralph should never modify |
| `tech-stack.md` | Technology stack description |

### Directory Structure

```
your-project/
├── .ralph/
│   ├── config.yaml       # Project configuration
│   ├── principles.md     # Development principles
│   ├── patterns.md       # Code patterns
│   ├── boundaries.md     # Protected files
│   ├── tech-stack.md     # Technology description
│   └── worktrees/        # Git worktrees (gitignored)
├── plans/
│   ├── pending/          # Plans waiting to be processed
│   ├── current/          # Currently active plan (0-1)
│   └── complete/         # Finished plans
└── specs/
    └── INDEX.md          # Feature specification index
```

## Queue Workflow

1. **Pending** - Plans waiting to be processed
2. **Current** - One plan being actively worked on
3. **Complete** - Finished plans (archived)

```bash
# Add a plan to the queue
mv my-plan.md plans/pending/

# Process the queue
ralph worker

# Check status
ralph status

# Reset current plan (start over)
ralph reset
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

## Migration from Bash Version

If you were using the bash scripts (`ralph.sh`, `ralph-worker.sh`):

1. **Install the Go binary** (see Installation above)
2. **Update paths** - Commands are now just `ralph` instead of `./scripts/ralph/ralph.sh`
3. **Config unchanged** - `.ralph/config.yaml` format is identical
4. **Plans unchanged** - Plan file format is identical
5. **Remove old scripts** - Delete `scripts/ralph/` directory

### Command Mapping

| Bash | Go |
|------|----|
| `./scripts/ralph/ralph.sh plan.md` | `ralph run plan.md` |
| `./scripts/ralph/ralph-worker.sh` | `ralph worker` |
| `./scripts/ralph/ralph-worker.sh --status` | `ralph status` |
| `./scripts/ralph/ralph-worker.sh --reset` | `ralph reset` |
| `./scripts/ralph/ralph-worker.sh --cleanup` | `ralph cleanup` |
| `./scripts/ralph/ralph-init.sh --detect` | `ralph init --detect` |

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

### "branch is already checked out"

Another worktree has the branch checked out. Either:
- Wait for the other worker to finish
- Run `ralph cleanup` to remove orphaned worktrees
- Manually remove the conflicting worktree

### Worktree has uncommitted changes

Ralph won't clean up worktrees with uncommitted changes for safety. Either:
- Commit or discard the changes manually
- The changes will be preserved for review

### Plan verification keeps failing

If Claude outputs `<promise>COMPLETE</promise>` but verification fails:
1. Check `<plan>.feedback.md` for the reason
2. The next iteration will read this and address it
3. If it keeps failing, the plan may have unclear acceptance criteria

## Requirements

- **Claude Code CLI** - `claude` command must be available
- **Git** - For version control
- **GitHub CLI** (optional) - For PR creation (`gh`)

## License

MIT
