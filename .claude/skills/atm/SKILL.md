# Skill: ATM (Agent Task Manager)

Interact with ATM plans, tasks, feedback, and progress via `atm-cli`.

## When to Use

Use `/atm` when you want to:
- Check project or plan status
- View what the agent sees (agent context)
- Give feedback to steer the next Ralph iteration
- Manage tasks (block, skip, complete, create)
- Review progress entries and feedback history
- Manage plans (create, activate, complete, block)

## Prerequisites

- `atm-cli` must be on PATH
- `atm-cli` manages its own auth via `~/.config/atm/config.json` (set up separately)
- Project slug is needed for project-level commands — read from `.ralph/config.yaml` (`atm.project_slug`) if configured, otherwise ask the user or infer from the project name

## Usage

The user invokes `/atm <subcommand> [args]`. Parse the subcommand and arguments, then run the appropriate `atm-cli` command(s). If no subcommand is given, show a help summary.

### Subcommands

#### `/atm status`
Show the current project status: active plan, task stats, available tasks, recent feedback.

```bash
atm-cli project context <project-slug> --format text
```

Read the project slug from `.ralph/config.yaml` (`atm.project_slug`), falling back to `project.name`. Display the output clearly formatted.

#### `/atm context [plan-id]`
Show the full agent context for a plan (what Ralph's agent sees each iteration).

```bash
# With plan ID
atm-cli plan context <plan-id> --format text

# Without plan ID - get active plan from project context first
atm-cli project context <project-slug> --format text
```

If no plan ID provided, read project slug from config and show the active plan's context.

#### `/atm plans [status]`
List plans for the project, optionally filtered by status.

```bash
# All plans
atm-cli plan list <project-slug> --pretty

# Filtered by status (draft, ready, active, complete, blocked)
atm-cli plan list <project-slug> --status <status> --pretty
```

#### `/atm plan <plan-id>`
Show details for a specific plan.

```bash
atm-cli plan show <plan-id> --pretty
```

#### `/atm tasks <plan-id> [--status <status>] [--available]`
List tasks for a plan.

```bash
# All tasks
atm-cli task list <plan-id> --pretty

# Available tasks only (unblocked, todo)
atm-cli task list <plan-id> --available --pretty

# By status (todo, claimed, doing, done, blocked, skipped)
atm-cli task list <plan-id> --status <status> --pretty
```

#### `/atm task <task-id>`
Show a specific task with its acceptance criteria.

```bash
atm-cli task show <task-id> --format full --pretty
```

#### `/atm feedback <plan-id> <message>`
Add feedback to a plan. This is the primary way to steer Ralph's agent on the next iteration. The agent reads the most recent feedback entry first.

```bash
atm-cli feedback add <plan-id> --author human --body "<message>"
```

Tips for effective feedback:
- Be specific: "Task 5 is wrong - the API endpoint should be POST /users not PUT /users"
- Reference task IDs when relevant
- Say "skip task X" if a task should be skipped
- Say "redo task X" if previous work needs fixing
- The most recent feedback entry has highest priority for the agent

#### `/atm progress <plan-id> [message]`
View or add progress entries.

```bash
# View recent progress
atm-cli progress list <plan-id> --pretty

# Add a progress note
atm-cli progress add <plan-id> --author human --body "<message>"
```

#### `/atm feedback-list <plan-id>`
View feedback history for a plan.

```bash
atm-cli feedback list <plan-id> --pretty
```

#### `/atm complete <task-id>`
Manually mark a task as done.

```bash
atm-cli task complete <task-id>
```

#### `/atm block <task-id> <reason>`
Mark a task as blocked with a reason.

```bash
atm-cli task block <task-id> --reason "<reason>"
```

#### `/atm skip <task-id> [reason]`
Skip a task, optionally with a reason.

```bash
atm-cli task skip <task-id> --reason "<reason>"
```

#### `/atm create-task <plan-id> <title> [--description <desc>]`
Add a new task to a plan.

```bash
atm-cli task create <plan-id> --title "<title>" --description "<description>"
```

#### `/atm plan-status <plan-id> <status>`
Transition a plan's status (draft, ready, active, complete, blocked).

```bash
atm-cli plan status <plan-id> --status <status>
```

#### `/atm check <criteria-id>`
Mark an acceptance criterion as satisfied.

```bash
atm-cli criteria check <criteria-id>
```

#### `/atm uncheck <criteria-id>`
Mark an acceptance criterion as not satisfied.

```bash
atm-cli criteria uncheck <criteria-id>
```

### Default: `/atm` (no args)

Show this help summary:

```
ATM Skill - Agent Task Manager commands

  /atm status                    Project status (active plan, task stats)
  /atm context [plan-id]         Agent context view (what Ralph sees)
  /atm plans [status]            List plans (optional status filter)
  /atm plan <id>                 Show plan details
  /atm tasks <plan-id>           List tasks (--available, --status)
  /atm task <id>                 Show task with criteria
  /atm feedback <plan-id> <msg>  Send feedback to agent
  /atm feedback-list <plan-id>   View feedback history
  /atm progress <plan-id> [msg]  View or add progress
  /atm complete <task-id>        Mark task done
  /atm block <task-id> <reason>  Block a task
  /atm skip <task-id> [reason]   Skip a task
  /atm create-task <plan-id> ... Add task to plan
  /atm plan-status <id> <status> Change plan status
  /atm check <criteria-id>       Check acceptance criterion
  /atm uncheck <criteria-id>     Uncheck acceptance criterion
```

## Config

`atm-cli` handles its own authentication via `~/.config/atm/config.json`. No auth flags need to be passed — just call `atm-cli` directly.

The only config this skill reads from `.ralph/config.yaml` is the project slug:

```yaml
atm:
  project_slug: "ralph"    # Used for project-level commands (status, plans, context)
```

If `atm.project_slug` is not set in the config, ask the user for it or try using the `project.name` field as a fallback.

## Behavior

- Read `.ralph/config.yaml` to get the project slug for project-level commands.
- Do NOT pass `--api-url` or `--api-token` flags — `atm-cli` handles auth from its own config.
- Use `--pretty` for human-readable JSON output.
- Use `--format text` for context commands (optimized for readability).
- When showing task lists, summarize the output in a clear table format.
- When adding feedback, confirm what was sent and remind the user that Ralph will see it on the next iteration.
