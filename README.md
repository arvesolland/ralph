# Ralph

An implementation of the [Ralph Wiggum technique](https://ralph-wiggum.ai) for autonomous AI development.

## Why It Works

**Fresh context per iteration.** Like malloc'ing a new array instead of appending - each Claude Code invocation gets a clean context window, avoiding the pollution and degradation that happens when LLMs accumulate conversation history. Progress persists in files and git, not in context.

**External memory architecture.** Agents don't carry state - they read it:
- **Specs** (`specs/`) - Durable knowledge base. Entry point for understanding what exists and why.
- **Plans** (`plans/`) - Volatile execution state. What to do next, with dependencies and checkboxes.
- **Progress** (`progress.txt`) - Institutional memory. Gotchas and learnings that compound over time.

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

- **Structured Plans** - Tasks with dependencies, status tracking, and acceptance criteria
- **Plan Review** - AI-powered optimization catches overengineering before implementation
- **Task Queue** - File-based queue for processing multiple plans
- **Auto PR Creation** - Create pull requests via Claude Code on completion
- **AI-Assisted Setup** - Let Claude analyze your codebase and generate config
- **Config-Driven** - Customize prompts via config files, not code
- **Progress Tracking** - Learnings accumulate across iterations

## Installation

### Quick Install

```bash
curl -fsSL https://raw.githubusercontent.com/arvesolland/ralph/main/install.sh | bash
```

### With AI-Assisted Configuration

```bash
curl -fsSL https://raw.githubusercontent.com/arvesolland/ralph/main/install.sh | bash -s -- --ai
```

### From Local Clone

```bash
git clone https://github.com/arvesolland/ralph.git
cd your-project
../ralph/install.sh --local
```

### Shell Aliases (Optional)

After installation, add Ralph aliases to your shell:

```bash
./scripts/ralph/install-aliases.sh
```

This adds:
- `ralph` - Run implementation loop
- `ralph-worker` - Queue management
- `ralph-status` - Show queue status
- `ralph-loop` - Process all plans in queue
- `ralph-loop-pr` - Process all plans and create PRs

## Quick Start

### 1. Initialize Configuration

```bash
# Auto-detect project settings
./scripts/ralph/ralph-init.sh --detect

# Or use AI to analyze your codebase (recommended)
./scripts/ralph/ralph-init.sh --ai
```

### 2. Create a Plan File

Plans follow a structured format with tasks, dependencies, and acceptance criteria:

```markdown
# Plan: User Authentication

## Context
Add JWT-based authentication to the API. Must be backward compatible with existing sessions.

---

## Rules
1. **Pick task:** First task (by number) where status ≠ `complete` and all `Requires` are `complete`
2. **Subtasks are sequential.** Complete 1 before 2.
3. **Task complete when:** All "Done when" + all subtasks checked → set Status: `complete`
4. **Update file after each checkbox.**
5. **New work found?** Add to Discovered section, continue current task.

---

## Tasks

### T1: Create Token Service
> Core JWT generation and validation

**Requires:** —
**Status:** open

**Done when:**
- [ ] TokenService with generate() and validate() methods
- [ ] Unit tests pass

**Subtasks:**
1. [ ] Implement generate() with standard claims
2. [ ] Implement validate() with expiry handling
3. [ ] Write unit tests

---

### T2: Update Login Endpoint
> Modify /auth/login to return JWT

**Requires:** T1
**Status:** blocked

**Done when:**
- [ ] /auth/login returns { accessToken, refreshToken }

**Subtasks:**
1. [ ] Modify LoginController response
2. [ ] Update API documentation

---

## Discovered
<!-- New work found during implementation goes here -->
```

### 3. Run Ralph

```bash
# Single plan
./scripts/ralph/ralph.sh path/to/plan.md

# With plan review first (recommended)
./scripts/ralph/ralph.sh path/to/plan.md --review-plan

# With PR creation on completion
./scripts/ralph/ralph.sh path/to/plan.md --create-pr
```

Ralph will:
1. Read the plan file
2. Find the first task with met dependencies
3. Find the first unchecked subtask
4. Implement it
5. Run validation (lint, tests)
6. Commit changes
7. Update the plan file
8. Repeat until all tasks are complete
9. Output `<promise>COMPLETE</promise>`

## Usage

### Main Implementation Loop

```bash
./scripts/ralph/ralph.sh <plan-file> [options]

Options:
  --review-plan, -r      Run plan reviewer first (catches overengineering)
  --review-passes N      Number of review passes (default: 2)
  --max, -m N            Max iterations (default: 30)
  --create-pr, --pr      Create PR via Claude Code after completion
  --version, -v          Show version
  --help, -h             Show help
```

### Task Queue (Worker)

For processing multiple plans:

```bash
# Add plan to queue
./scripts/ralph/ralph-worker.sh --add path/to/plan.md

# Check queue status
./scripts/ralph/ralph-worker.sh --status

# Process current/next plan
./scripts/ralph/ralph-worker.sh

# Process all plans until queue empty
./scripts/ralph/ralph-worker.sh --loop

# Process all plans with PR creation
./scripts/ralph/ralph-worker.sh --loop --create-pr
```

Queue folder structure:
```
plans/
├── pending/      # Plans waiting to be processed
├── current/      # Currently active plan (0-1 files)
└── complete/     # Finished plans with logs
```

## Plan File Format

### Required Sections

| Section | Description |
|---------|-------------|
| `## Context` | Background, constraints, goals |
| `## Rules` | Task selection rules (embedded in plan) |
| `## Tasks` | Task definitions with T1, T2 numbering |
| `## Discovered` | New work found during implementation |

### Task Fields

| Field | Description | Values |
|-------|-------------|--------|
| `**Requires:**` | Dependencies | `—` (none), `T1`, `T1, T2` |
| `**Status:**` | Current state | `open`, `in_progress`, `blocked`, `complete` |
| `**Done when:**` | Acceptance criteria | Checkboxes |
| `**Subtasks:**` | Implementation steps | Numbered checkboxes |

### Task Selection Logic

```
Find first T[n] where:
  - Status ≠ complete
  - Every task in "Requires" has Status = complete

Within that task, find first unchecked subtask.
```

## Configuration

Configuration lives in `.ralph/`, with specs and plans at repo root:

```
# Repo root
specs/               # Feature specifications (see ralph-spec skill)
├── INDEX.md         # Lookup table for all features
└── feature/
    └── SPEC.md

plans/               # Task execution (see ralph-plan skill)
├── pending/         # Plans waiting to be processed
├── current/         # Currently active plan (0-1 files)
└── complete/        # Finished plans with logs

.ralph/              # Ralph configuration
├── config.yaml      # Project settings
├── principles.md    # Development principles
├── patterns.md      # Code patterns to follow
├── boundaries.md    # Files to never modify
└── tech-stack.md    # Technology description

.claude/skills/      # Claude Code skills
├── ralph-spec/      # Working with feature specs
├── ralph-plan/      # Working with task plans
└── ralph-spec-to-plan/  # Generating plans from specs
```

### config.yaml

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
```

### principles.md

Development principles injected into all prompts:

```markdown
# Project Principles

- Keep functions small and focused
- Write tests for all new features
- Never commit secrets
```

### patterns.md

Code patterns for your project:

```markdown
# Code Patterns

## Error Handling
Use the custom AppError class:
```typescript
throw new AppError('Not found', 404);
```
```

### boundaries.md

Files Ralph should never modify:

```markdown
# Boundaries

- `*.lock` files
- `node_modules/`
- `.env*` files
```

## How It Works

### Prompt Injection

Base prompts contain placeholders like `{{PRINCIPLES}}` that get replaced with your config files:

```
prompts/base/prompt.md + .ralph/principles.md → Final prompt
```

### Context Files

Ralph uses `context.json` to pass state between iterations:

```json
{
  "planFile": "docs/plan.md",
  "planPath": "/full/path/to/plan.md",
  "iteration": 3,
  "maxIterations": 30
}
```

### Completion Detection

Claude signals completion with `<promise>COMPLETE</promise>` when all tasks are complete.

When running via the worker queue, this triggers:
1. Move plan to `completed/` folder
2. Create PR (if `--create-pr` flag set)
3. Activate next pending plan

### Progress Tracking

`scripts/ralph/progress.txt` accumulates learnings:

```markdown
## Codebase Patterns
- Use useQuery hook for data fetching

---
## 2024-01-15 - T1.2: Implement validation
- **Implemented:** Input validation for registration
- **Files changed:** src/validators/user.ts
- **Learnings:** Use zod for schema validation
```

## Updating

Update Ralph scripts while preserving your config:

```bash
./scripts/ralph/ralph-update.sh
```

Or re-run the installer:

```bash
curl -fsSL https://raw.githubusercontent.com/arvesolland/ralph/main/install.sh | bash
```

## Requirements

- **Claude Code CLI** - `claude` command must be available
- **Git** - For version control
- **GitHub CLI** (optional) - For PR creation (`gh`)

## Project Structure

```
ralph/
├── VERSION                 # Semantic version
├── install.sh              # Installer
├── ralph.sh                # Main implementation loop
├── ralph-worker.sh         # Queue management
├── ralph-init.sh           # Project initialization
├── ralph-update.sh         # Update scripts
├── aliases.sh              # Shell aliases
├── install-aliases.sh      # Alias installer
├── lib/
│   └── config.sh           # Shared functions
└── prompts/
    └── base/
        ├── prompt.md              # Implementation prompt
        ├── plan_reviewer_prompt.md # Plan review prompt
        └── plan-spec.md           # Plan format specification
```

## Versioning

Ralph uses semantic versioning. Check version with:

```bash
./scripts/ralph/ralph.sh --version
```

## License

MIT
