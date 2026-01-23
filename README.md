# Ralph

Autonomous AI development loops for implementing tasks from plan files.

Ralph orchestrates Claude Code to implement tasks one at a time, with proper commits, validation, and progress tracking.

## Features

- **Implementation Loop** - Implement tasks from a plan file, one at a time
- **Worker Loop** - Process Beads epics, create PRs automatically
- **Discovery Loop** - Analyze codebase for improvements, create tasks
- **Plan Reviewer** - Optimize plans before implementation
- **AI-Assisted Setup** - Let Claude analyze your codebase and generate config
- **Config-Driven** - Customize prompts via config files, not code

## Installation

### Quick Install

```bash
# From your project root
curl -fsSL https://raw.githubusercontent.com/USER/ralph/main/install.sh | bash
```

### With AI-Assisted Configuration

```bash
curl -fsSL https://raw.githubusercontent.com/USER/ralph/main/install.sh | bash -s -- --ai
```

### From Local Clone

```bash
git clone https://github.com/USER/ralph.git
cd your-project
../ralph/install.sh --local
```

## Quick Start

### 1. Initialize Configuration

```bash
# Auto-detect project settings
./scripts/ralph/ralph-init.sh --detect

# Or use AI to analyze your codebase
./scripts/ralph/ralph-init.sh --ai
```

### 2. Edit Configuration

Configuration lives in `.ralph/`:

```
.ralph/
├── config.yaml      # Project settings (name, commands, etc.)
├── principles.md    # Development principles (injected into prompts)
├── patterns.md      # Code patterns to follow
├── boundaries.md    # Files to never modify
└── tech-stack.md    # Technology description
```

### 3. Create a Plan File

```markdown
# Feature: User Authentication

## Tasks

- [ ] Create User model with email/password fields
- [ ] Add registration endpoint
- [ ] Add login endpoint with JWT tokens
- [ ] Add password reset flow
- [ ] Write tests for all endpoints
```

### 4. Run Ralph

```bash
./scripts/ralph/ralph.sh docs/auth-plan.md
```

Ralph will:
1. Read the plan file
2. Find the next incomplete task
3. Implement it
4. Run validation (tests, lint)
5. Commit the changes
6. Mark the task complete
7. Repeat until done

## Commands

### Implementation Loop

```bash
# Basic usage
./scripts/ralph/ralph.sh path/to/plan.md

# With plan review first
./scripts/ralph/ralph.sh plan.md --review-plan

# Custom iteration limit
./scripts/ralph/ralph.sh plan.md --max 50
```

### Worker Loop (Beads Integration)

Requires [Beads](https://github.com/beads-project/beads) for task management.

```bash
# Process one ready epic
./scripts/ralph/ralph-worker.sh

# Process up to 3 epics
./scripts/ralph/ralph-worker.sh --max 3

# Process specific epic
./scripts/ralph/ralph-worker.sh --epic bd-a1b2

# Preview without implementing
./scripts/ralph/ralph-worker.sh --dry-run
```

### Discovery Loop

```bash
# Analyze codebase, create Beads tasks
./scripts/ralph/ralph-discover.sh

# Preview findings without creating tasks
./scripts/ralph/ralph-discover.sh --dry-run

# Focus on specific category
./scripts/ralph/ralph-discover.sh --category tests
```

## Configuration

### config.yaml

```yaml
project:
  name: "My Project"
  description: "A web application for..."

git:
  base_branch: "main"

commands:
  test: "npm test"
  lint: "npm run lint"
  build: "npm run build"
  dev: "npm run dev"
```

### principles.md

Development principles injected into all prompts:

```markdown
# Project Principles

- Keep functions small and focused (< 20 lines)
- Write tests for all new features
- Never commit secrets or credentials
- Use TypeScript strict mode
```

### patterns.md

Code patterns specific to your project:

```markdown
# Code Patterns

## Error Handling
Always use the custom `AppError` class:
\`\`\`typescript
throw new AppError('User not found', 404);
\`\`\`

## API Responses
Use the `sendResponse` helper:
\`\`\`typescript
return sendResponse(res, 200, { user });
\`\`\`
```

### boundaries.md

Files Ralph should never modify:

```markdown
# Boundaries

- `*.lock` files
- `node_modules/`
- `.env*` files
- `migrations/` (modify via migration commands only)
```

## How It Works

### Prompt Injection

Base prompts contain placeholders like `{{PRINCIPLES}}` that get replaced with your config files at runtime:

```
prompts/base/prompt.md + .ralph/principles.md → Final prompt
```

This means you customize Ralph by editing markdown files, not code.

### Context Files

Ralph uses `context.json` to pass state between iterations:

```json
{
  "planFile": "docs/plan.md",
  "iteration": 3,
  "maxIterations": 30
}
```

Claude reads this file to understand its current task.

### Completion Markers

Claude signals completion with markers:

- `<promise>COMPLETE</promise>` - All tasks done
- `<promise>TASK_COMPLETE</promise>` - Current task done (worker mode)
- `<promise>TASK_FAILED</promise>` - Task cannot be completed

### Progress Tracking

`scripts/ralph/progress.txt` accumulates learnings across iterations:

```markdown
## Codebase Patterns
- Use `useQuery` hook for data fetching
- Error boundaries are in `components/ErrorBoundary.tsx`

---
## 2024-01-15 - Task 1
- **Implemented:** User model
- **Files:** src/models/User.ts
- **Learnings:** Prisma schema in schema.prisma
```

## CI/CD Integration

### Cron-Based Worker

Run the worker every 15 minutes:

```bash
*/15 * * * * cd /path/to/project && ./scripts/ralph/ralph-worker.sh >> /var/log/ralph.log 2>&1
```

### Daily Discovery

Run discovery daily:

```bash
0 6 * * * cd /path/to/project && ./scripts/ralph/ralph-discover.sh >> /var/log/ralph-discover.log 2>&1
```

## Requirements

- **Claude Code CLI** - `claude` command must be available
- **Git** - For version control operations
- **Beads** (optional) - For worker/discovery loops
- **GitHub CLI** (optional) - For PR creation

## Project Structure

```
ralph/
├── install.sh              # Curl-able installer
├── ralph-init.sh           # Project initialization
├── ralph.sh                # Main implementation loop
├── ralph-worker.sh         # Beads epic processor
├── ralph-discover.sh       # Codebase analyzer
├── lib/
│   └── config.sh           # Shared configuration library
└── prompts/
    └── base/
        ├── prompt.md           # Implementation prompt
        ├── worker_prompt.md    # Worker prompt
        ├── discover_prompt.md  # Discovery prompt
        └── plan_reviewer_prompt.md  # Plan review prompt
```

## Updating

Re-run the installer to update scripts while preserving your config:

```bash
curl -fsSL https://raw.githubusercontent.com/USER/ralph/main/install.sh | bash
```

Your `.ralph/` configuration files are never overwritten.

## License

MIT
