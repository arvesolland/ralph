# Feature: Plan & Queue

**ID:** F1.2
**Status:** planned
**Requires:** F1.1

## Summary

Plan file parsing, task extraction, and queue management (pending → current → complete lifecycle). Plans are markdown files with checkboxes representing tasks. The queue is file-based for git compatibility.

## Goals

- Parse markdown plans, extract tasks with checkbox state
- Update task checkboxes in-place without corrupting markdown
- Manage queue directories (pending/, current/, complete/)
- Extract plan metadata (status, dependencies)
- Handle progress files (<plan>.progress.md)
- Handle feedback files (<plan>.feedback.md)
- Support flexible plan formats (not strict schema)

## Non-Goals

- Validate plan content beyond basic parsing
- Enforce specific plan structure
- Database-backed queue

## Design

### Plan Structure

```go
type Plan struct {
    Path        string      // Path to plan file
    Name        string      // Derived from filename
    Content     string      // Raw markdown content
    Tasks       []Task      // Extracted tasks
    Status      string      // From **Status:** header
    Branch      string      // Derived feature branch name
    Iteration   int         // Current iteration
    ProgressFile string     // Path to .progress.md
    FeedbackFile string     // Path to .feedback.md
}

type Task struct {
    Line        int         // Line number in file
    Text        string      // Task text (without checkbox)
    Complete    bool        // Checkbox state
    Requires    []string    // Task dependencies
    Subtasks    []Task      // Nested subtasks
}
```

### Queue Operations

```go
type Queue struct {
    BaseDir string // Usually "plans/" or ".ralph/plans/"
}

func (q *Queue) Pending() ([]*Plan, error)           // List pending plans
func (q *Queue) Current() (*Plan, error)             // Get current plan (nil if none)
func (q *Queue) Activate(plan *Plan) error           // Move pending → current
func (q *Queue) Complete(plan *Plan) error           // Move current → complete
func (q *Queue) Reset(plan *Plan) error              // Move current → pending
func (q *Queue) Status() (*QueueStatus, error)       // Summary of all queues
```

### Task Extraction

Parse markdown looking for:
- `- [ ] Task text` → incomplete task
- `- [x] Task text` → complete task
- Indented items are subtasks
- `requires: task-1` in task text extracts dependencies

### Progress File Format

```markdown
# Progress: plan-name

## Iteration 1 (2026-01-31 10:30)
- Completed task X
- Learned: gotcha about Y

## Iteration 2 (2026-01-31 10:35)
- Working on task Z
```

### Key Files

| File | Purpose |
|------|---------|
| `internal/plan/plan.go` | Plan struct and parsing |
| `internal/plan/task.go` | Task extraction from markdown |
| `internal/plan/queue.go` | Queue directory operations |
| `internal/plan/progress.go` | Progress file read/write |
| `internal/plan/feedback.go` | Feedback file handling |

## Gotchas

- Plan files may have varying formats - be flexible in parsing
- Checkbox update must preserve surrounding markdown exactly
- Progress file append must handle concurrent access gracefully
- Plan name derived from filename, not content
- Branch name: `feat/<plan-name>` with sanitization (spaces → hyphens, etc.)

---

## Changelog

- 2026-01-31: Initial spec
