# Feature: Claude Runner

**ID:** F1.3
**Status:** planned
**Requires:** F1.1, F1.2

## Summary

Execute Claude CLI with retry logic, timeout handling, completion detection, and verification. This is the core execution engine that handles the known Claude CLI hanging issue and implements the iteration loop.

## Goals

- Execute `claude` CLI with configurable options
- Retry on transient failures with exponential backoff
- Detect and handle hanging processes (timeout + kill)
- Parse streaming JSON output for real-time display
- Detect completion marker (`<promise>COMPLETE</promise>`)
- Detect blocker marker (`<blocker>...</blocker>`)
- Verify completion with Haiku model
- Track token usage and timing

## Non-Goals

- Direct API calls (always use CLI)
- Custom model selection beyond what CLI supports
- Caching responses

## Design

### Runner Interface

```go
type Runner interface {
    Run(ctx context.Context, prompt string, opts Options) (*Result, error)
}

type Options struct {
    Model           string        // e.g., "sonnet"
    SystemPrompt    string        // System prompt content
    AllowedTools    []string      // Tools to allow
    MaxTokens       int           // Max response tokens
    Timeout         time.Duration // Per-attempt timeout
    MaxRetries      int           // Retry count
    RetryDelay      time.Duration // Initial retry delay
    WorkDir         string        // Working directory for claude
}

type Result struct {
    Output      string        // Full response text
    IsComplete  bool          // Found completion marker
    Blocker     *Blocker      // Extracted blocker, if any
    TokensUsed  int           // Tokens consumed
    Duration    time.Duration // Execution time
    Attempts    int           // Number of attempts made
}

type Blocker struct {
    Description string
    Action      string
    Resume      string
    Hash        string // MD5 for deduplication
}
```

### Execution Flow

```
1. Build claude command with options
2. Start process with context for cancellation
3. Read stdout line-by-line (streaming JSON)
4. Display content in real-time
5. Monitor for timeout (kill if exceeded)
6. Parse final output for markers
7. On failure: check if retryable, backoff, retry
8. Return result or error
```

### Retry Logic

Retryable conditions:
- Context deadline exceeded (timeout)
- Exit code indicating transient failure
- Rate limit errors
- Connection errors

Non-retryable:
- Invalid arguments
- Authentication failure
- Explicit abort

Backoff: exponential with jitter, capped at 60 seconds

### Verification

After completion marker detected:
1. Build verification prompt with plan state
2. Run Haiku model (fast, cheap)
3. Parse yes/no response
4. If "no", extract reason and write to feedback file
5. Continue iteration loop

### Key Files

| File | Purpose |
|------|---------|
| `internal/runner/runner.go` | Runner interface and implementation |
| `internal/runner/command.go` | Claude CLI command building |
| `internal/runner/stream.go` | Streaming JSON parsing |
| `internal/runner/retry.go` | Retry logic with backoff |
| `internal/runner/verify.go` | Completion verification |
| `internal/runner/blocker.go` | Blocker extraction |

## Gotchas

- Claude CLI can hang indefinitely - MUST implement timeout with process kill
- Streaming output is JSON lines, not plain text
- Context cancellation must kill child process, not just stop reading
- Verification uses different model (Haiku) than main execution
- Blocker hash uses first 8 chars of MD5 for deduplication
- Must handle partial output if process times out

---

## Changelog

- 2026-01-31: Initial spec
