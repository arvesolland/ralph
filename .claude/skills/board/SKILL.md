# Skill: Board

Interact with Board plans, tasks, feedback, and progress via `board-cli`.

## When to Use

Use `/board` when you want to:
- Check project or plan status
- View what the agent sees (agent context)
- Give feedback to steer the next Ralph iteration
- Manage tasks (block, skip, complete, create)
- Review progress entries and feedback history
- Manage plans (create, activate, complete, block)

## Prerequisites

- `board-cli` must be on PATH
- `board-cli` manages its own auth via `~/.config/board/config.json` (set up separately)
- Project slug is needed for project-level commands — read from `.ralph/config.yaml` (`board.project_slug`) if configured, otherwise ask the user or infer from the project name

## Usage

The user invokes `/board <subcommand> [args]`. Parse the subcommand and arguments, then run the appropriate `board-cli` command(s). If no subcommand is given, show a help summary.

### Subcommands

#### `/board status`
Show the current project status: active plan, task stats, available tasks, recent feedback.

```bash
board-cli project context <project-slug> --format text
```

Read the project slug from `.ralph/config.yaml` (`board.project_slug`), falling back to `project.name`. Display the output clearly formatted.

#### `/board context [plan-id]`
Show the full agent context for a plan (what Ralph's agent sees each iteration).

```bash
# With plan ID
board-cli plan context <plan-id> --format text

# Without plan ID - get active plan from project context first
board-cli project context <project-slug> --format text
```

If no plan ID provided, read project slug from config and show the active plan's context.

#### `/board plans [status]`
List plans for the project, optionally filtered by status.

```bash
# All plans
board-cli plan list <project-slug> --pretty

# Filtered by status (draft, ready, active, complete, blocked)
board-cli plan list <project-slug> --status <status> --pretty
```

#### `/board plan <plan-id>`
Show details for a specific plan.

```bash
board-cli plan show <plan-id> --pretty
```

#### `/board tasks <plan-id> [--status <status>] [--available]`
List tasks for a plan.

```bash
# All tasks
board-cli task list <plan-id> --pretty

# Available tasks only (unblocked, todo)
board-cli task list <plan-id> --available --pretty

# By status (todo, claimed, doing, done, blocked, skipped)
board-cli task list <plan-id> --status <status> --pretty
```

#### `/board task <task-id>`
Show a specific task with its acceptance criteria.

```bash
board-cli task show <task-id> --format full --pretty
```

#### `/board feedback <plan-id> <message>`
Add feedback to a plan. This is the primary way to steer Ralph's agent on the next iteration. The agent reads the most recent feedback entry first.

```bash
board-cli feedback add <plan-id> --author human --body "<message>"
```

Tips for effective feedback:
- Be specific: "Task 5 is wrong - the API endpoint should be POST /users not PUT /users"
- Reference task IDs when relevant
- Say "skip task X" if a task should be skipped
- Say "redo task X" if previous work needs fixing
- The most recent feedback entry has highest priority for the agent

#### `/board progress <plan-id> [message]`
View or add progress entries.

```bash
# View recent progress
board-cli progress list <plan-id> --pretty

# Add a progress note
board-cli progress add <plan-id> --author human --body "<message>"
```

#### `/board feedback-list <plan-id>`
View feedback history for a plan.

```bash
board-cli feedback list <plan-id> --pretty
```

#### `/board complete <task-id>`
Manually mark a task as done.

```bash
board-cli task complete <task-id>
```

#### `/board block <task-id> <reason>`
Mark a task as blocked with a reason.

```bash
board-cli task block <task-id> --reason "<reason>"
```

#### `/board skip <task-id> [reason]`
Skip a task, optionally with a reason.

```bash
board-cli task skip <task-id> --reason "<reason>"
```

#### `/board create-task <plan-id> <title> [--description <desc>]`
Add a new task to a plan.

```bash
board-cli task create <plan-id> --title "<title>" --description "<description>"
```

#### `/board plan-status <plan-id> <status>`
Transition a plan's status (draft, ready, active, complete, blocked).

```bash
board-cli plan status <plan-id> --status <status>
```

#### `/board check <criteria-id>`
Mark an acceptance criterion as satisfied.

```bash
board-cli criteria check <criteria-id>
```

#### `/board uncheck <criteria-id>`
Mark an acceptance criterion as not satisfied.

```bash
board-cli criteria uncheck <criteria-id>
```

### Default: `/board` (no args)

Show this help summary:

```
Board Skill - Task management commands

  /board status                    Project status (active plan, task stats)
  /board context [plan-id]         Agent context view (what Ralph sees)
  /board plans [status]            List plans (optional status filter)
  /board plan <id>                 Show plan details
  /board tasks <plan-id>           List tasks (--available, --status)
  /board task <id>                 Show task with criteria
  /board feedback <plan-id> <msg>  Send feedback to agent
  /board feedback-list <plan-id>   View feedback history
  /board progress <plan-id> [msg]  View or add progress
  /board complete <task-id>        Mark task done
  /board block <task-id> <reason>  Block a task
  /board skip <task-id> [reason]   Skip a task
  /board create-task <plan-id> ... Add task to plan
  /board plan-status <id> <status> Change plan status
  /board check <criteria-id>       Check acceptance criterion
  /board uncheck <criteria-id>     Uncheck acceptance criterion
```

## Config

`board-cli` handles its own authentication via `~/.config/board/config.json`. No auth flags need to be passed — just call `board-cli` directly.

The only config this skill reads from `.ralph/config.yaml` is the project slug:

```yaml
board:
  project_slug: "ralph"    # Used for project-level commands (status, plans, context)
```

If `board.project_slug` is not set in the config, ask the user for it or try using the `project.name` field as a fallback.

## Behavior

- Read `.ralph/config.yaml` to get the project slug for project-level commands.
- Do NOT pass `--api-url` or `--api-token` flags — `board-cli` handles auth from its own config.
- Use `--pretty` for human-readable JSON output.
- Use `--format text` for context commands (optimized for readability).
- When showing task lists, summarize the output in a clear table format.
- When adding feedback, confirm what was sent and remind the user that Ralph will see it on the next iteration.
