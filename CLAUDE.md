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

# Available tests: single-task, dependencies, progress, loose-format, worker-queue

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

Plans automatically get feature branches:
- Branch name: `feat/<plan-name>` (derived from plan filename)
- Created/checked out by bash before Claude runs
- Agent is told branch name via context.json

### Error Handling

- `set -e` in all scripts
- `log_error`, `log_warn`, `log_success` for colored output
- Exit codes: 0 = success, 1 = max iterations or error

## Gotchas

- **stdout pollution**: Worker functions that return values must redirect output to stderr (`>&2`)
- **Plan validation removed**: Plans can be any markdown format; Claude handles parsing
- **Feature branches**: Created by bash, not Claude - prompt just tells agent the branch name
- **Test workspace**: Must use `git init -b main` since config expects "main" branch
- **Completion marker**: Agent may mention `<promise>COMPLETE</promise>` without meaning completion - haiku verification catches this
