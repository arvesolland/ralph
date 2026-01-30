# Feature: Go Rewrite

**ID:** F1
**Status:** planned
**Requires:** —

## Summary

Complete rewrite of Ralph from bash/shell scripts (~5,400 lines) to Go. This addresses fundamental limitations of shell scripting: fragile YAML parsing, cross-platform incompatibilities (macOS vs Linux sed), inability to unit test, and the need for a separate Python process for Slack bot functionality. The Go version will be a single, cross-platform binary with comprehensive test coverage.

## Goals

- Single binary distribution (no runtime dependencies beyond git and claude CLI)
- Comprehensive unit test coverage for all business logic
- Same config.yaml format and file structure (drop-in replacement)
- Absorb Python Slack bot into Go binary
- Cross-platform without sed/bash compatibility hacks
- Structured logging with configurable verbosity
- Faster feedback loop through proper testing

## Non-Goals

- Changing the plan file format (keep markdown with checkboxes)
- Changing the prompt template format (keep `{{PLACEHOLDER}}` syntax)
- Changing directory structure (keep plans/pending, plans/current, etc.)
- GUI or web interface
- Replacing git or claude CLI (still shell out to these)
- Multi-language support (English only)

## Design

### Overview

The Go implementation follows the same architectural principles as the bash version:

1. **External Memory Pattern**: Fresh Claude context per iteration, state persisted in files and git
2. **Worktree Isolation**: Each plan runs in dedicated git worktree
3. **Three-Layer Locking**: File location + git worktree + directory locks for concurrency
4. **Queue-Based Processing**: FIFO queue with pending → current → complete lifecycle

The key difference is implementation: proper data structures, interfaces for testability, and structured error handling.

### Architecture

```
cmd/ralph/main.go           # Single entry point
internal/
├── cli/                    # Cobra commands (run, worker, init, status)
├── config/                 # YAML config + project detection
├── plan/                   # Plan parsing, queue, progress files
├── runner/                 # Claude execution, retry, verification
├── worktree/               # Git worktree operations, dep install
├── git/                    # Git CLI wrapper
├── prompt/                 # Template loading, placeholder injection
└── notify/                 # Slack webhook + Bot API + Socket Mode
```

### Key Interfaces

```go
// Runner executes Claude CLI - mockable for testing
type Runner interface {
    Run(ctx context.Context, prompt string, opts Options) (*Result, error)
}

// Git wraps git operations - mockable for testing
type Git interface {
    Status() (*Status, error)
    Commit(msg string, files ...string) error
    CreateWorktree(path, branch string) error
    RemoveWorktree(path string) error
}

// Notifier sends notifications - mockable for testing
type Notifier interface {
    Start(plan *Plan) error
    Complete(plan *Plan, prURL string) error
    Blocker(plan *Plan, blocker *Blocker) error
}
```

### CLI Commands

```
ralph run <plan>              # Run iteration loop on a plan
ralph worker                  # Process queue continuously
ralph worker --once           # Process one plan and exit
ralph status                  # Show queue and worktree status
ralph cleanup                 # Remove orphaned worktrees
ralph reset                   # Reset current plan to pending
ralph init                    # Interactive project setup
ralph init --detect           # Auto-detect and configure
ralph version                 # Show version info
```

### Data Model

**Config** (same YAML format as bash):
- project.name, project.description
- git.base_branch
- commands.test, commands.lint, commands.build
- slack.webhook_url, slack.channel, slack.notify_*
- worktree.copy_env_files, worktree.init_commands
- completion.mode (pr|merge)

**Plan** (same markdown format):
- Parsed from markdown, tasks extracted via regex
- Checkbox state tracked: `- [ ]` vs `- [x]`
- Status header: `**Status:** in_progress`
- Dependencies: `requires: task-1, task-2`

**Context** (same JSON format):
- planFile, featureBranch, baseBranch
- iteration, maxIterations

### External Dependencies

- **git**: Required for all operations
- **claude**: Claude CLI for AI execution
- **gh**: Optional, for PR creation (falls back to manual instructions)

### Error Handling Strategy

- All errors wrapped with context (`fmt.Errorf("loading config: %w", err)`)
- Retryable errors (timeouts, rate limits) handled with exponential backoff
- Non-retryable errors fail fast with clear messages
- Slack notification failures are logged but don't block execution

## Sub-Features

| ID | Sub-Feature | Status | Path |
|----|-------------|--------|------|
| F1.1 | Config & Prompt | planned | [config/](config/SPEC.md) |
| F1.2 | Plan & Queue | planned | [plan/](plan/SPEC.md) |
| F1.3 | Claude Runner | planned | [runner/](runner/SPEC.md) |
| F1.4 | Git & Worktree | planned | [worktree/](worktree/SPEC.md) |
| F1.5 | Slack Integration | planned | [slack/](slack/SPEC.md) |
| F1.6 | CLI & Release | planned | [cli/](cli/SPEC.md) |

## Gotchas

- **Claude CLI hanging**: The bash version has extensive timeout/retry logic for a known Claude CLI issue (GitHub #19060). Must replicate this in Go with context cancellation.
- **Stream JSON output**: Claude CLI outputs JSON per line when streaming. Need to parse incrementally.
- **macOS vs Linux git**: Some git behaviors differ. Test on both platforms.
- **Worktree cleanup on interrupt**: If process is killed mid-execution, worktrees may be orphaned. Need cleanup command.
- **File sync bidirectionality**: Plan and progress files must sync FROM worktree after each iteration, not just TO worktree at start.

## Plan

**Plan:** [/plans/pending/go-rewrite.md](/plans/pending/go-rewrite.md)

## Open Questions

- [x] Keep same CLI names (ralph.sh → ralph)? → **Decision:** Yes, `ralph` binary with subcommands
- [x] Embed prompts in binary or keep as external files? → **Decision:** Keep external for customization, embed defaults as fallback
- [ ] Support both YAML and TOML config? → *Pending* (lean toward YAML-only for simplicity)
- [ ] Add `ralph doctor` command for troubleshooting? → *Pending*

## References

- [Original CLAUDE.md](/CLAUDE.md) - Current architecture documentation
- [Cobra CLI framework](https://github.com/spf13/cobra)
- [Viper config library](https://github.com/spf13/viper)
- [GoReleaser](https://goreleaser.com/) - Cross-platform releases

---

## Changelog

- 2026-01-31: Initial spec created
