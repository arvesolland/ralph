# Ralph

Autonomous AI development loops for implementing tasks from plan files.

Ralph orchestrates Claude Code to implement tasks one at a time, with proper commits, validation, and progress tracking.

## Features

- **Plan-based implementation** - Work through tasks in a markdown plan file
- **Plan reviewer** - Optimize plans before implementation (catches overengineering)
- **AI-assisted setup** - Let Claude analyze your codebase and generate config
- **Config-driven prompts** - Customize via config files, not code
- **Progress tracking** - Learnings accumulate across iterations

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

## Quick Start

### 1. Initialize Configuration

```bash
# Auto-detect project settings
./scripts/ralph/ralph-init.sh --detect

# Or use AI to analyze your codebase (recommended)
./scripts/ralph/ralph-init.sh --ai
```

### 2. Review Configuration

Configuration lives in `.ralph/`:

```
.ralph/
├── config.yaml      # Project settings (name, commands)
├── principles.md    # Development principles
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

## Usage

### Basic Implementation

```bash
./scripts/ralph/ralph.sh path/to/plan.md
```

### With Plan Review First

Review optimizes the plan before implementation (catches overengineering):

```bash
./scripts/ralph/ralph.sh plan.md --review-plan
```

### Options

```bash
./scripts/ralph/ralph.sh plan.md [options]

Options:
  --review-plan, -r      Run plan reviewer first
  --review-passes N      Number of review passes (default: 2)
  --max, -m N            Max iterations (default: 30)
  --help, -h             Show help
```

## Configuration

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
  dev: "npm run dev"
```

### principles.md

Development principles injected into all prompts:

```markdown
# Project Principles

- Keep functions small and focused (< 20 lines)
- Write tests for all new features
- Never commit secrets
```

### patterns.md

Code patterns specific to your project:

```markdown
# Code Patterns

## Error Handling
Use the custom AppError class:
\`\`\`typescript
throw new AppError('Not found', 404);
\`\`\`
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
  "iteration": 3,
  "maxIterations": 30
}
```

### Completion Markers

Claude signals completion with:
- `<promise>COMPLETE</promise>` - All tasks done

### Progress Tracking

`scripts/ralph/progress.txt` accumulates learnings:

```markdown
## Codebase Patterns
- Use useQuery hook for data fetching

---
## 2024-01-15 - Task 1
- **Implemented:** User model
- **Learnings:** Schema in schema.prisma
```

## Requirements

- **Claude Code CLI** - `claude` command must be available
- **Git** - For version control

## Project Structure

```
ralph/
├── install.sh              # Installer
├── ralph-init.sh           # Project initialization
├── ralph.sh                # Main implementation loop
├── lib/
│   └── config.sh           # Shared config library
└── prompts/
    └── base/
        ├── prompt.md           # Implementation prompt
        └── plan_reviewer_prompt.md
```

## Updating

Re-run the installer to update scripts (config is preserved):

```bash
curl -fsSL https://raw.githubusercontent.com/arvesolland/ralph/main/install.sh | bash
```

## License

MIT
