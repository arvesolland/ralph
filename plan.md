# Plan: Go Rewrite

**Spec:** [/specs/go-rewrite/SPEC.md](/specs/go-rewrite/SPEC.md)
**Created:** 2026-01-31
**Status:** pending

## Context

Complete rewrite of Ralph from bash/shell scripts (~5,400 lines) to Go. The bash implementation has fundamental limitations: fragile YAML parsing (~100 lines that broke on inline comments), cross-platform incompatibilities (macOS vs Linux sed), inability to unit test, and a separate Python process for the Slack bot.

The Go version will be a single, cross-platform binary with comprehensive test coverage. It must be a **drop-in replacement** - same config.yaml format, same directory structure, same plan format.

### Gotchas (from spec)

- **Claude CLI hanging**: Known issue (GitHub #19060). Must implement timeout with context cancellation and process kill. Bash uses extensive retry logic with 5 retries, 5s base delay, exponential backoff capped at 60s.
- **Stream JSON output**: Claude CLI outputs JSON per line when streaming. Parse incrementally, display content in real-time.
- **Worktree cleanup on interrupt**: If process killed mid-execution, worktrees orphaned. Need `ralph cleanup` command.
- **File sync bidirectionality**: Plan/progress files sync TO worktree at start, FROM worktree after each iteration.
- **Branch name sanitization**: `feat/<plan-name>` with spaces→hyphens, special chars removed.
- **Three-layer locking**: File location (current/) + git worktree (branch checkout) + directory (worktree exists).
- **Verification uses Haiku**: Completion verification uses different (cheaper) model than main execution.
- **Blocker hash deduplication**: First 8 chars of MD5 to prevent Slack spam on retries.
- **Socket Mode reconnects**: Slack bot needs constant connection, handle disconnects gracefully.

---

## Rules

1. **Pick task:** First task where status ≠ `complete` and all `Requires` are `complete`
2. **Subtasks are sequential.** Complete 1 before 2.
3. **Task complete when:** All "Done when" checked → set Status: `complete`
4. **Update file after each checkbox.**
5. **New work found?** Add to Discovered section, continue current task.

---

## Phase 1: Project Foundation

### T1: Initialize Go module and project structure
> Establish the foundational project layout that all other tasks build upon.

**Requires:** —
**Status:** open

**Done when:**
- [ ] `go.mod` exists with module `github.com/arvesolland/ralph`
- [ ] Go version set to 1.22 or later in go.mod
- [ ] Directory structure created: `cmd/ralph/`, `internal/{cli,config,plan,runner,git,worktree,prompt,notify,log}/`
- [ ] `cmd/ralph/main.go` exists with minimal main function
- [ ] `go build ./cmd/ralph` succeeds and produces `ralph` binary
- [ ] `.gitignore` updated to ignore Go artifacts (`ralph`, `*.exe`, `dist/`)

**Subtasks:**
1. [ ] Run `go mod init github.com/arvesolland/ralph`
2. [ ] Create directory structure with placeholder `.go` files
3. [ ] Create minimal main.go that prints "ralph dev"
4. [ ] Verify build succeeds
5. [ ] Update .gitignore

---

### T2: Implement structured logging
> Logging foundation used by all other components. Must be in place before any other code.

**Requires:** T1
**Status:** open

**Done when:**
- [ ] `internal/log/logger.go` defines `Logger` interface with `Debug`, `Info`, `Warn`, `Error` methods
- [ ] Default implementation writes to stderr with timestamps
- [ ] `--verbose` enables Debug level, default is Info
- [ ] `--quiet` suppresses Info (only Warn and Error)
- [ ] Colors enabled when stdout is TTY: Debug=gray, Info=default, Warn=yellow, Error=red, Success=green
- [ ] `--no-color` flag disables colors
- [ ] Unit tests verify log level filtering and color codes

**Subtasks:**
1. [ ] Define Logger interface
2. [ ] Implement ConsoleLogger with level filtering
3. [ ] Add color support with TTY detection
4. [ ] Add Success level (custom, not standard log level)
5. [ ] Write unit tests for level filtering
6. [ ] Write unit tests for color output

---

### T3: Set up Cobra CLI framework with root command
> CLI skeleton that all subcommands attach to. Global flags defined here.

**Requires:** T2
**Status:** open

**Done when:**
- [ ] `internal/cli/root.go` defines root command with description
- [ ] Global flags registered: `--config/-c`, `--verbose/-v`, `--quiet/-q`, `--no-color`
- [ ] `cmd/ralph/main.go` calls `cli.Execute()`
- [ ] `ralph --help` displays usage with all global flags
- [ ] `ralph version` subcommand shows version, commit, build date (hardcoded "dev" for now)
- [ ] Unknown commands return helpful error message

**Subtasks:**
1. [ ] Add cobra dependency: `go get github.com/spf13/cobra`
2. [ ] Create root.go with root command
3. [ ] Register global flags with defaults
4. [ ] Create version.go with version subcommand
5. [ ] Wire up in main.go
6. [ ] Verify help output

---

## Phase 2: Configuration System

### T4: Implement Config struct and YAML loading
> Core configuration that all components depend on. Must handle all edge cases the bash version struggled with.

**Requires:** T3
**Status:** open

**Done when:**
- [ ] `internal/config/config.go` defines `Config` struct matching spec (Project, Git, Commands, Slack, Worktree, Completion)
- [ ] `Load(path string) (*Config, error)` reads and parses YAML file
- [ ] `LoadWithDefaults(path string) (*Config, error)` applies defaults for missing fields
- [ ] YAML inline comments handled correctly: `name: "value" # comment` → value is "value", not "value # comment"
- [ ] Missing config file returns config with all defaults (not error)
- [ ] Empty config file returns config with all defaults (not error)
- [ ] Nested access works: config loaded from `project:\n  name: "Test"` has `config.Project.Name == "Test"`
- [ ] Unit tests cover: valid config, empty file, missing file, inline comments, nested keys, all field types

**Subtasks:**
1. [ ] Add yaml dependency: `go get gopkg.in/yaml.v3`
2. [ ] Define all config structs with yaml tags
3. [ ] Implement Load() function
4. [ ] Implement defaults in separate defaults.go
5. [ ] Implement LoadWithDefaults() that merges
6. [ ] Write comprehensive unit tests

---

### T5: Implement project auto-detection
> Detect project type, language, framework, and commands from files present.

**Requires:** T4
**Status:** open

**Done when:**
- [ ] `internal/config/detect.go` defines `Detect(dir string) (*DetectedConfig, error)`
- [ ] Detects Node.js from package.json, extracts test/lint/build scripts
- [ ] Detects PHP from composer.json
- [ ] Detects Python from pyproject.toml or requirements.txt
- [ ] Detects Go from go.mod
- [ ] Detects Rust from Cargo.toml
- [ ] Detects Ruby from Gemfile
- [ ] Returns appropriate test/lint/build commands for each language
- [ ] Unit tests with fixture directories for each language

**Subtasks:**
1. [ ] Define DetectedConfig struct
2. [ ] Implement Node.js detection with package.json parsing
3. [ ] Implement PHP detection
4. [ ] Implement Python detection
5. [ ] Implement Go, Rust, Ruby detection
6. [ ] Create test fixtures in testdata/detect/
7. [ ] Write unit tests for each language

---

### T6: Implement prompt template builder
> Load prompt templates and substitute placeholders. Critical for Claude execution.

**Requires:** T4
**Status:** open

**Done when:**
- [ ] `internal/prompt/builder.go` defines `Builder` struct
- [ ] `Build(templatePath string, config *Config, overrides map[string]string) (string, error)` loads and processes template
- [ ] `{{PLACEHOLDER}}` syntax replaced with config values
- [ ] Supports all placeholders: PROJECT_NAME, PROJECT_DESCRIPTION, PRINCIPLES, PATTERNS, BOUNDARIES, TECH_STACK, TEST_COMMAND, LINT_COMMAND, BUILD_COMMAND
- [ ] Missing placeholder files (e.g., .ralph/principles.md) result in empty string substitution (not error)
- [ ] Unknown placeholders left as-is (for forward compatibility)
- [ ] `internal/prompt/templates.go` embeds default prompts from `prompts/base/` using `//go:embed`
- [ ] Falls back to embedded prompts if external file not found
- [ ] Unit tests verify all placeholder substitutions

**Subtasks:**
1. [ ] Define Builder struct with config reference
2. [ ] Implement placeholder detection regex
3. [ ] Implement substitution logic
4. [ ] Add embed directives for prompts/base/*.md
5. [ ] Implement fallback to embedded prompts
6. [ ] Load .ralph/*.md override files
7. [ ] Write unit tests with sample templates

---

### T7: Add `ralph init` command
> Initialize a new project with config file. Uses detection from T5.

**Requires:** T5, T6
**Status:** open

**Done when:**
- [ ] `ralph init` creates `.ralph/` directory if not exists
- [ ] `ralph init` creates `.ralph/config.yaml` with detected settings
- [ ] `ralph init --detect` runs auto-detection and populates config
- [ ] Existing config file prompts for confirmation before overwrite
- [ ] Creates `plans/pending/`, `plans/current/`, `plans/complete/` directories
- [ ] Creates `specs/` directory with INDEX.md
- [ ] Prints summary of what was created/detected
- [ ] Integration test verifies directory and file creation

**Subtasks:**
1. [ ] Create internal/cli/init.go
2. [ ] Implement directory creation logic
3. [ ] Implement config generation from detection
4. [ ] Add --detect flag
5. [ ] Add overwrite confirmation prompt
6. [ ] Create starter INDEX.md for specs
7. [ ] Write integration test

---

## Phase 3: Plan Management

### T8: Implement Plan struct and parsing
> Parse markdown plans into structured data. Foundation for queue management.

**Requires:** T4
**Status:** open

**Done when:**
- [ ] `internal/plan/plan.go` defines `Plan` struct with Path, Name, Content, Tasks, Status, Branch fields
- [ ] `Load(path string) (*Plan, error)` reads and parses plan file
- [ ] Name derived from filename: `go-rewrite.md` → `go-rewrite`
- [ ] Branch derived from name: `go-rewrite` → `feat/go-rewrite`
- [ ] Status extracted from `**Status:** value` in content
- [ ] Handles plans without explicit Status (defaults to "pending")
- [ ] Plan with special characters in name sanitizes branch: `my plan (v2)` → `feat/my-plan-v2`
- [ ] Unit tests cover: valid plan, missing status, special characters in name

**Subtasks:**
1. [ ] Define Plan struct
2. [ ] Implement Load() with file reading
3. [ ] Implement name derivation from path
4. [ ] Implement branch name sanitization
5. [ ] Implement status extraction regex
6. [ ] Write unit tests with fixtures

---

### T9: Implement task extraction from plans
> Extract checkbox tasks from markdown, track completion state.

**Requires:** T8
**Status:** complete

**Done when:**
- [x] `internal/plan/task.go` defines `Task` struct with Line, Text, Complete, Requires, Subtasks fields
- [x] `ExtractTasks(content string) []Task` parses markdown checkboxes
- [x] `- [ ] Task text` → Task{Complete: false, Text: "Task text"}
- [x] `- [x] Task text` → Task{Complete: true, Text: "Task text"}
- [x] Indented tasks become subtasks of previous non-indented task
- [x] `requires: T1, T2` in task text extracts dependencies
- [x] Line numbers tracked for in-place updates
- [x] Unit tests cover: simple tasks, nested subtasks, dependencies, mixed complete/incomplete

**Subtasks:**
1. [x] Define Task struct
2. [x] Implement checkbox regex matching
3. [x] Implement indentation-based subtask nesting
4. [x] Implement dependency extraction
5. [x] Track line numbers during parsing
6. [x] Write comprehensive unit tests

---

### T10: Implement checkbox update in plans
> Update task completion state without corrupting surrounding markdown.

**Requires:** T9
**Status:** open

**Done when:**
- [ ] `UpdateCheckbox(content string, lineNum int, complete bool) (string, error)` modifies specific checkbox
- [ ] `- [ ]` ↔ `- [x]` toggle preserves all other content on line
- [ ] Preserves exact whitespace and formatting around checkbox
- [ ] Returns error if line doesn't contain checkbox
- [ ] `Save(plan *Plan) error` writes updated content to file
- [ ] Atomic write (write to temp, rename) prevents corruption on crash
- [ ] Unit tests verify preservation of surrounding markdown

**Subtasks:**
1. [ ] Implement UpdateCheckbox function
2. [ ] Implement line-by-line content modification
3. [ ] Implement atomic file save
4. [ ] Test preservation of markdown formatting
5. [ ] Test error cases (invalid line number, no checkbox)

---

### T11: Implement Queue management
> File-based queue: pending → current → complete lifecycle.

**Requires:** T8
**Status:** open

**Done when:**
- [ ] `internal/plan/queue.go` defines `Queue` struct with BaseDir field
- [ ] `Pending() ([]*Plan, error)` lists plans in pending/ sorted by name
- [ ] `Current() (*Plan, error)` returns plan in current/ (nil if empty)
- [ ] `Activate(plan *Plan) error` moves from pending/ to current/
- [ ] `Complete(plan *Plan) error` moves from current/ to complete/
- [ ] `Reset(plan *Plan) error` moves from current/ back to pending/
- [ ] `Status() (*QueueStatus, error)` returns counts for each queue
- [ ] Activate fails if current/ already has a plan (single active plan)
- [ ] Unit tests with temp directories verify all operations

**Subtasks:**
1. [ ] Define Queue and QueueStatus structs
2. [ ] Implement Pending() with directory listing
3. [ ] Implement Current()
4. [ ] Implement Activate() with file move
5. [ ] Implement Complete() and Reset()
6. [ ] Implement Status()
7. [ ] Write unit tests with temp directories

---

### T12: Implement progress file handling
> Read and append to progress files for plan execution history.

**Requires:** T8
**Status:** open

**Done when:**
- [ ] `internal/plan/progress.go` defines progress file operations
- [ ] `ProgressPath(plan *Plan) string` returns `<plan-path-without-ext>.progress.md`
- [ ] `ReadProgress(plan *Plan) (string, error)` reads existing progress (empty string if not exists)
- [ ] `AppendProgress(plan *Plan, iteration int, content string) error` adds timestamped entry
- [ ] Entry format: `## Iteration N (YYYY-MM-DD HH:MM)\n{content}\n`
- [ ] Creates file if not exists
- [ ] Appends without overwriting existing content
- [ ] Unit tests verify format and append behavior

**Subtasks:**
1. [ ] Implement path derivation
2. [ ] Implement ReadProgress
3. [ ] Implement AppendProgress with timestamp
4. [ ] Handle file creation
5. [ ] Write unit tests

---

### T13: Implement feedback file handling
> Handle feedback files for human input during blockers.

**Requires:** T8
**Status:** open

**Done when:**
- [ ] `internal/plan/feedback.go` defines feedback file operations
- [ ] `FeedbackPath(plan *Plan) string` returns `<plan-path-without-ext>.feedback.md`
- [ ] `ReadFeedback(plan *Plan) (string, error)` reads pending feedback section
- [ ] `AppendFeedback(plan *Plan, source string, content string) error` adds timestamped entry to Pending section
- [ ] `MarkProcessed(plan *Plan, entry string) error` moves entry from Pending to Processed
- [ ] File format matches existing: `# Feedback\n## Pending\n...\n## Processed\n...`
- [ ] Unit tests verify section parsing and updates

**Subtasks:**
1. [ ] Implement path derivation
2. [ ] Implement ReadFeedback (parse Pending section)
3. [ ] Implement AppendFeedback
4. [ ] Implement MarkProcessed
5. [ ] Write unit tests with sample files

---

### T14: Add `ralph status` command
> Display queue status and worktree information.

**Requires:** T11, T3
**Status:** open

**Done when:**
- [ ] `ralph status` displays count of plans in each queue (pending, current, complete)
- [ ] Shows current plan name and branch if one is active
- [ ] Shows list of pending plans by name
- [ ] Shows worktree status (count, paths) - placeholder until worktree implemented
- [ ] Colored output: current=green, pending=yellow
- [ ] Returns exit code 0 on success
- [ ] Integration test verifies output format

**Subtasks:**
1. [ ] Create internal/cli/status.go
2. [ ] Implement queue status display
3. [ ] Add worktree status placeholder
4. [ ] Add colored output
5. [ ] Write integration test

---

## Phase 4: Git Operations

### T15: Implement Git interface and basic operations
> Wrapper around git CLI for common operations.

**Requires:** T2
**Status:** open

**Done when:**
- [ ] `internal/git/git.go` defines `Git` interface with Status, Commit, Add, Push, Pull, CurrentBranch, CreateBranch, DeleteBranch, BranchExists, RepoRoot, IsClean
- [ ] `NewGit(workDir string) Git` creates instance for specific directory
- [ ] `Status() (*Status, error)` returns parsed git status (branch, staged, unstaged, untracked, IsClean)
- [ ] `Commit(message string, files ...string) error` stages files and commits
- [ ] `CurrentBranch() (string, error)` returns current branch name
- [ ] `IsClean() (bool, error)` returns true if no uncommitted changes
- [ ] All operations shell out to `git` CLI with proper error handling
- [ ] Integration tests run in temp git repos

**Subtasks:**
1. [ ] Define Git interface
2. [ ] Implement command execution helper
3. [ ] Implement Status() with output parsing
4. [ ] Implement Commit() and Add()
5. [ ] Implement branch operations
6. [ ] Implement RepoRoot() and IsClean()
7. [ ] Write integration tests with temp repos

---

### T16: Implement git worktree operations
> Create, remove, and list git worktrees.

**Requires:** T15
**Status:** open

**Done when:**
- [ ] `CreateWorktree(path, branch string) error` runs `git worktree add`
- [ ] Creates branch if it doesn't exist (based on current HEAD)
- [ ] `RemoveWorktree(path string) error` runs `git worktree remove`
- [ ] `ListWorktrees() ([]WorktreeInfo, error)` parses `git worktree list --porcelain`
- [ ] `WorktreeInfo` contains Path, Branch, Commit
- [ ] CreateWorktree returns specific error if branch already checked out (this is the lock!)
- [ ] Integration tests verify worktree creation and removal

**Subtasks:**
1. [ ] Implement CreateWorktree
2. [ ] Implement branch creation within CreateWorktree
3. [ ] Implement RemoveWorktree
4. [ ] Implement ListWorktrees with porcelain parsing
5. [ ] Define and detect "branch already checked out" error
6. [ ] Write integration tests

---

### T17: Implement WorktreeManager
> High-level worktree management for plans.

**Requires:** T16, T11
**Status:** open

**Done when:**
- [ ] `internal/worktree/manager.go` defines `WorktreeManager` struct
- [ ] `Create(plan *Plan) (*Worktree, error)` creates worktree at `.ralph/worktrees/feat-<plan-name>/`
- [ ] `Remove(plan *Plan) error` removes plan's worktree and optionally deletes branch
- [ ] `Get(plan *Plan) (*Worktree, error)` returns existing worktree or nil
- [ ] `Exists(plan *Plan) bool` checks if worktree exists
- [ ] `Path(plan *Plan) string` returns worktree path for plan
- [ ] Worktree path is gitignored (check `.ralph/worktrees/` in .gitignore)
- [ ] Unit tests with mock Git interface

**Subtasks:**
1. [ ] Define WorktreeManager struct
2. [ ] Implement path derivation
3. [ ] Implement Create with Git.CreateWorktree
4. [ ] Implement Remove
5. [ ] Implement Get and Exists
6. [ ] Write unit tests with mock Git

---

### T18: Implement dependency auto-detection for worktrees
> Detect and install project dependencies in new worktrees.

**Requires:** T17
**Status:** open

**Done when:**
- [ ] `internal/worktree/deps.go` defines `DetectAndInstall(worktreePath string) error`
- [ ] Detects package-lock.json → runs `npm ci`
- [ ] Detects yarn.lock → runs `yarn install --frozen-lockfile`
- [ ] Detects pnpm-lock.yaml → runs `pnpm install --frozen-lockfile`
- [ ] Detects bun.lockb → runs `bun install --frozen-lockfile`
- [ ] Detects composer.lock → runs `composer install`
- [ ] Detects requirements.txt → runs `pip install -r requirements.txt`
- [ ] Detects poetry.lock → runs `poetry install`
- [ ] Detects Gemfile.lock → runs `bundle install`
- [ ] Detects go.sum → runs `go mod download`
- [ ] Detects Cargo.lock → runs `cargo fetch`
- [ ] Returns nil if no lockfile found (skip install)
- [ ] Integration tests with fixture directories

**Subtasks:**
1. [ ] Define lockfile detection order
2. [ ] Implement detection logic
3. [ ] Implement command execution for each package manager
4. [ ] Handle errors (command not found vs install failure)
5. [ ] Create test fixtures
6. [ ] Write integration tests

---

### T19: Implement worktree file sync
> Sync plan files between main worktree and execution worktree.

**Requires:** T17, T12, T13
**Status:** open

**Done when:**
- [ ] `internal/worktree/sync.go` defines sync operations
- [ ] `SyncToWorktree(plan *Plan, worktreePath string) error` copies plan, progress, feedback files to worktree
- [ ] `SyncFromWorktree(plan *Plan, worktreePath string) error` copies plan, progress files back to main
- [ ] Sync preserves file permissions
- [ ] SyncToWorktree also copies .env files based on config.worktree.copy_env_files
- [ ] Missing source files are skipped (not error)
- [ ] Unit tests verify file copying in both directions

**Subtasks:**
1. [ ] Implement SyncToWorktree
2. [ ] Implement SyncFromWorktree
3. [ ] Add .env file copying with config parsing
4. [ ] Handle missing files gracefully
5. [ ] Write unit tests

---

### T20: Implement worktree initialization hooks
> Run custom init commands after worktree creation.

**Requires:** T18, T19
**Status:** open

**Done when:**
- [ ] `internal/worktree/hooks.go` defines `RunInitHooks(worktreePath string, config *Config, mainWorktreePath string) error`
- [ ] Checks for `.ralph/hooks/worktree-init` executable, runs if present
- [ ] Sets `MAIN_WORKTREE` environment variable when running hook
- [ ] If config.worktree.init_commands set, runs those (skips auto-detection)
- [ ] If no hook and no init_commands, falls back to DetectAndInstall
- [ ] Logs each step for debugging
- [ ] Integration test with custom hook script

**Subtasks:**
1. [ ] Implement hook file detection
2. [ ] Implement hook execution with environment
3. [ ] Implement init_commands execution
4. [ ] Implement fallback to auto-detection
5. [ ] Write integration test with hook

---

### T21: Implement orphaned worktree cleanup
> Remove worktrees that no longer have associated plans.

**Requires:** T17, T11
**Status:** open

**Done when:**
- [ ] `Cleanup() ([]string, error)` in WorktreeManager removes orphaned worktrees
- [ ] Worktree is orphaned if: exists in .ralph/worktrees/ but no matching plan in pending/ or current/
- [ ] Does NOT remove worktrees with uncommitted changes (safety)
- [ ] Returns list of removed worktree paths
- [ ] Logs each removal
- [ ] `ralph cleanup` command calls this function
- [ ] Integration test creates orphan and verifies cleanup

**Subtasks:**
1. [ ] Implement orphan detection logic
2. [ ] Implement uncommitted changes check
3. [ ] Implement removal with logging
4. [ ] Create internal/cli/cleanup.go
5. [ ] Write integration test

---

## Phase 5: Claude Execution

### T22: Implement Claude CLI command builder
> Build claude CLI command with all options.

**Requires:** T4
**Status:** open

**Done when:**
- [ ] `internal/runner/command.go` defines `BuildCommand(prompt string, opts Options) *exec.Cmd`
- [ ] Supports: --print (for prompt output), --output-format stream-json
- [ ] Supports: --model flag from opts.Model
- [ ] Supports: --max-tokens from opts.MaxTokens
- [ ] Supports: --allowedTools from opts.AllowedTools (comma-separated)
- [ ] Sets working directory from opts.WorkDir
- [ ] Passes prompt via stdin (not argument, to avoid shell escaping issues)
- [ ] Unit tests verify command construction

**Subtasks:**
1. [ ] Define Options struct
2. [ ] Implement BuildCommand
3. [ ] Handle stdin prompt passing
4. [ ] Add all supported flags
5. [ ] Write unit tests

---

### T23: Implement streaming JSON parser
> Parse Claude CLI streaming JSON output line-by-line.

**Requires:** T22
**Status:** open

**Done when:**
- [ ] `internal/runner/stream.go` defines `StreamParser` for incremental parsing
- [ ] Parses JSON lines from Claude CLI output
- [ ] Extracts content text for real-time display
- [ ] Accumulates full response
- [ ] Handles partial lines (buffer until newline)
- [ ] Ignores non-JSON lines gracefully
- [ ] Unit tests with sample Claude output

**Subtasks:**
1. [ ] Research Claude CLI JSON output format
2. [ ] Define StreamParser struct
3. [ ] Implement line-by-line parsing
4. [ ] Implement content extraction
5. [ ] Handle edge cases (partial lines, non-JSON)
6. [ ] Write unit tests with real output samples

---

### T24: Implement retry logic with exponential backoff
> Retry transient failures with configurable backoff.

**Requires:** T2
**Status:** open

**Done when:**
- [ ] `internal/runner/retry.go` defines `Retrier` struct
- [ ] `Do(fn func() error) error` executes function with retry
- [ ] Configurable: MaxRetries (default 5), InitialDelay (default 5s), MaxDelay (default 60s)
- [ ] Exponential backoff with jitter (±25%)
- [ ] `IsRetryable(err error) bool` identifies transient errors
- [ ] Retryable: context.DeadlineExceeded, rate limit errors, connection errors
- [ ] Non-retryable: invalid arguments, auth failure
- [ ] Logs each retry attempt with delay
- [ ] Unit tests verify backoff timing and retry counts

**Subtasks:**
1. [ ] Define Retrier struct with config
2. [ ] Implement Do() with retry loop
3. [ ] Implement exponential backoff with jitter
4. [ ] Implement IsRetryable error classification
5. [ ] Add logging
6. [ ] Write unit tests

---

### T25: Implement Runner with timeout handling
> Execute Claude CLI with timeout and process management.

**Requires:** T22, T23, T24
**Status:** complete

**Done when:**
- [x] `internal/runner/runner.go` defines `Runner` interface and `CLIRunner` implementation
- [x] `Run(ctx context.Context, prompt string, opts Options) (*Result, error)` executes Claude
- [x] Timeout enforced via context with deadline
- [x] On timeout: sends SIGTERM, waits 5s, sends SIGKILL if needed
- [x] Streams output in real-time during execution
- [x] Returns `Result` with Output, Duration, Attempts
- [x] Integrates with Retrier for transient failures
- [x] Integration test with mock claude script (simulates timeout)

**Subtasks:**
1. [x] Define Runner interface and Result struct
2. [x] Implement CLIRunner.Run()
3. [x] Implement timeout with context
4. [x] Implement process termination (SIGTERM/SIGKILL)
5. [x] Integrate streaming parser
6. [x] Integrate retry logic
7. [x] Write integration test with mock script

---

### T26: Implement completion marker detection
> Detect `<promise>COMPLETE</promise>` in Claude output.

**Requires:** T25
**Status:** complete

**Done when:**
- [x] `Result.IsComplete` set to true when output contains `<promise>COMPLETE</promise>`
- [x] Detection is case-sensitive (exact match)
- [x] Works with marker anywhere in output (not just end)
- [x] Does not false-positive on partial matches or mentions
- [x] Unit tests verify detection in various positions

**Subtasks:**
1. [x] Add IsComplete field to Result
2. [x] Implement detection in output parsing
3. [x] Write unit tests

---

### T27: Implement blocker extraction
> Extract `<blocker>...</blocker>` content from Claude output.

**Requires:** T25
**Status:** complete

**Done when:**
- [x] `internal/runner/blocker.go` defines `Blocker` struct with Description, Action, Resume, Hash
- [x] `ExtractBlocker(output string) *Blocker` parses blocker marker
- [x] Extracts content between `<blocker>` and `</blocker>` tags
- [x] Parses `Description:`, `Action:`, `Resume:` fields if present
- [x] Computes Hash as first 8 chars of MD5 of full blocker content
- [x] Returns nil if no blocker found
- [x] `Result.Blocker` populated when blocker detected
- [x] Unit tests with sample blocker content

**Subtasks:**
1. [x] Define Blocker struct
2. [x] Implement tag extraction regex
3. [x] Implement field parsing
4. [x] Implement hash computation
5. [x] Integrate into Result
6. [x] Write unit tests

---

### T28: Implement completion verification with Haiku
> Verify plan completion using cheaper Haiku model.

**Requires:** T25, T8
**Status:** open

**Done when:**
- [ ] `internal/runner/verify.go` defines `Verify(plan *Plan, runner Runner) (bool, string, error)`
- [ ] Builds verification prompt with plan state (tasks, checkboxes)
- [ ] Runs Haiku model (fast, cheap) via runner with model override
- [ ] Parses yes/no response
- [ ] Returns (true, "", nil) if verified complete
- [ ] Returns (false, reason, nil) if not complete, with extracted reason
- [ ] Uses shorter timeout (60s default) for verification
- [ ] Unit tests with mock runner

**Subtasks:**
1. [ ] Define verification prompt template
2. [ ] Implement Verify function
3. [ ] Implement yes/no parsing
4. [ ] Implement reason extraction
5. [ ] Write unit tests with mock

---

## Phase 6: Core Loop

### T29: Implement iteration context
> Manage context.json for execution state between iterations.

**Requires:** T8
**Status:** open

**Done when:**
- [ ] `internal/runner/context.go` defines `Context` struct with PlanFile, FeatureBranch, BaseBranch, Iteration, MaxIterations
- [ ] `LoadContext(path string) (*Context, error)` reads context.json
- [ ] `SaveContext(ctx *Context, path string) error` writes context.json
- [ ] `NewContext(plan *Plan, baseBranch string, maxIterations int) *Context` creates new context
- [ ] Context path is `.ralph/context.json` in worktree
- [ ] Unit tests verify JSON serialization

**Subtasks:**
1. [ ] Define Context struct with JSON tags
2. [ ] Implement Load/Save
3. [ ] Implement NewContext
4. [ ] Write unit tests

---

### T30: Implement main iteration loop
> Core execution loop: prompt → Claude → verify → commit → repeat.

**Requires:** T25, T26, T27, T28, T29, T6, T15, T12
**Status:** open

**Done when:**
- [ ] `internal/runner/loop.go` defines `IterationLoop` struct
- [ ] `Run(ctx context.Context) error` executes loop until complete or max iterations
- [ ] Each iteration: build prompt → run Claude → check completion → verify if complete → commit
- [ ] Appends to progress file after each iteration
- [ ] Commits all changes (plan, progress) after each iteration
- [ ] Exits with success when verified complete
- [ ] Exits with error when max iterations reached
- [ ] Detects and handles blockers (notifies, continues)
- [ ] 3-second cooldown between iterations
- [ ] Integration test with mock Claude (3 iterations to complete)

**Subtasks:**
1. [ ] Define IterationLoop struct
2. [ ] Implement single iteration logic
3. [ ] Implement loop with termination conditions
4. [ ] Integrate prompt building
5. [ ] Integrate progress file updates
6. [ ] Integrate git commit
7. [ ] Add cooldown delay
8. [ ] Write integration test

---

### T31: Add `ralph run` command
> CLI command to run iteration loop on a plan.

**Requires:** T30
**Status:** open

**Done when:**
- [ ] `ralph run <plan>` executes iteration loop on specified plan file
- [ ] `--max` flag overrides max iterations (default 30)
- [ ] `--review` flag runs plan review before execution (placeholder for now)
- [ ] Validates plan file exists before starting
- [ ] Shows iteration progress (current/max)
- [ ] Exits with code 0 on success, 1 on failure
- [ ] Integration test runs loop on test plan

**Subtasks:**
1. [ ] Create internal/cli/run.go
2. [ ] Implement plan loading and validation
3. [ ] Wire up iteration loop
4. [ ] Add --max flag
5. [ ] Add --review flag (placeholder)
6. [ ] Write integration test

---

## Phase 7: Worker Queue

### T32: Implement worker loop
> Process queue: pending → current → execute → complete.

**Requires:** T30, T11, T17, T19, T20
**Status:** open

**Done when:**
- [ ] `internal/worker/worker.go` defines `Worker` struct
- [ ] `Run(ctx context.Context) error` processes queue continuously
- [ ] `RunOnce(ctx context.Context) error` processes one plan and exits
- [ ] Workflow: take from pending → activate → create worktree → run loop → complete → cleanup worktree
- [ ] Syncs files to/from worktree at appropriate points
- [ ] Runs worktree init hooks after creation
- [ ] Waits and polls when queue is empty (configurable interval)
- [ ] Handles interrupts gracefully (finish current iteration, then stop)
- [ ] Integration test with queue of 2 plans

**Subtasks:**
1. [ ] Define Worker struct
2. [ ] Implement single plan processing
3. [ ] Implement RunOnce
4. [ ] Implement Run with polling
5. [ ] Integrate worktree management
6. [ ] Integrate file sync
7. [ ] Add interrupt handling
8. [ ] Write integration test

---

### T33: Implement completion workflow (PR mode)
> Create PR on completion using `gh` CLI.

**Requires:** T32, T15
**Status:** open

**Done when:**
- [ ] `internal/worker/completion.go` defines `CompletePR(plan *Plan, worktree *Worktree) (string, error)`
- [ ] Pushes branch to origin
- [ ] Creates PR using `gh pr create` with title and body
- [ ] PR title: plan name
- [ ] PR body: includes "Generated by Ralph" footer
- [ ] Returns PR URL on success
- [ ] Falls back gracefully if `gh` not installed (logs manual instructions)
- [ ] Integration test with mock gh

**Subtasks:**
1. [ ] Implement branch push
2. [ ] Implement gh pr create execution
3. [ ] Parse PR URL from gh output
4. [ ] Implement fallback for missing gh
5. [ ] Write integration test

---

### T34: Implement completion workflow (merge mode)
> Merge directly to base branch on completion.

**Requires:** T32, T15
**Status:** complete

**Done when:**
- [x] `internal/worker/completion.go` defines `CompleteMerge(plan *Plan, worktree *Worktree, baseBranch string) error`
- [x] Checks out base branch in main worktree
- [x] Merges feature branch with `git merge --no-ff`
- [x] Pushes base branch to origin
- [x] Deletes feature branch (local and remote)
- [x] Returns error if merge conflicts
- [x] Integration test verifies merge commit

**Subtasks:**
1. [x] Implement checkout of base branch
2. [x] Implement merge with conflict detection
3. [x] Implement push
4. [x] Implement branch deletion
5. [x] Write integration test

---

### T35: Add `ralph worker` command
> CLI command to run worker queue processor.

**Requires:** T32, T33, T34
**Status:** open

**Done when:**
- [ ] `ralph worker` processes queue continuously
- [ ] `ralph worker --once` processes one plan and exits
- [ ] `--pr` flag uses PR mode for completion (default)
- [ ] `--merge` flag uses merge mode for completion
- [ ] `--interval` flag sets poll interval when queue empty (default 30s)
- [ ] Shows current plan being processed
- [ ] Handles Ctrl+C gracefully
- [ ] Integration test processes queue with --once

**Subtasks:**
1. [ ] Create internal/cli/worker.go
2. [ ] Wire up Worker
3. [ ] Add --once flag
4. [ ] Add --pr and --merge flags
5. [ ] Add --interval flag
6. [ ] Add signal handling
7. [ ] Write integration test

---

### T36: Add `ralph reset` command
> Move current plan back to pending.

**Requires:** T11
**Status:** open

**Done when:**
- [ ] `ralph reset` moves plan from current/ to pending/
- [ ] Removes associated worktree if exists
- [ ] Prompts for confirmation
- [ ] `--force` skips confirmation
- [ ] Returns error if no current plan
- [ ] Integration test verifies reset

**Subtasks:**
1. [ ] Create internal/cli/reset.go
2. [ ] Implement reset logic
3. [ ] Add confirmation prompt
4. [ ] Add --force flag
5. [ ] Write integration test

---

## Phase 8: Slack Integration

### T37: Implement Slack webhook notifications
> Simple fire-and-forget webhook notifications.

**Requires:** T4
**Status:** open

**Done when:**
- [ ] `internal/notify/webhook.go` defines `WebhookNotifier` implementing `Notifier` interface
- [ ] `Start(plan *Plan) error` sends start message
- [ ] `Complete(plan *Plan, prURL string) error` sends completion message
- [ ] `Blocker(plan *Plan, blocker *Blocker) error` sends blocker notification
- [ ] `Error(plan *Plan, err error) error` sends error notification
- [ ] Messages formatted with Slack mrkdwn
- [ ] Notifications are async (don't block execution)
- [ ] Errors logged but not propagated
- [ ] Unit tests with mock HTTP server

**Subtasks:**
1. [ ] Define WebhookNotifier struct
2. [ ] Implement HTTP POST to webhook
3. [ ] Implement message formatting
4. [ ] Make notifications async
5. [ ] Write unit tests with httptest

---

### T38: Implement thread tracking
> Track Slack threads per plan for reply threading.

**Requires:** T37
**Status:** open

**Done when:**
- [ ] `internal/notify/threads.go` defines `ThreadTracker` struct
- [ ] `Get(planName string) *ThreadInfo` returns thread info for plan
- [ ] `Set(planName string, info *ThreadInfo) error` saves thread info
- [ ] `ThreadInfo` contains PlanName, ThreadTS, ChannelID, NotifiedBlockers
- [ ] Persists to `.ralph/slack_threads.json`
- [ ] Loads from file on initialization
- [ ] Handles concurrent access with file locking
- [ ] Unit tests verify persistence

**Subtasks:**
1. [ ] Define ThreadTracker and ThreadInfo structs
2. [ ] Implement JSON file persistence
3. [ ] Implement Get/Set
4. [ ] Add file locking for concurrency
5. [ ] Write unit tests

---

### T39: Implement Slack Bot API notifications
> Notifications via Bot API with thread tracking.

**Requires:** T38
**Status:** open

**Done when:**
- [ ] `internal/notify/slack.go` defines `SlackNotifier` implementing `Notifier` interface
- [ ] Uses Bot API (requires bot_token in config)
- [ ] First message to channel creates thread, saves ThreadTS
- [ ] Subsequent messages reply to thread
- [ ] Blocker notifications deduplicated via hash in ThreadInfo.NotifiedBlockers
- [ ] Falls back to webhook if bot_token not configured
- [ ] Unit tests with mock Slack API

**Subtasks:**
1. [ ] Add slack-go dependency
2. [ ] Define SlackNotifier struct
3. [ ] Implement message posting with thread
4. [ ] Implement blocker deduplication
5. [ ] Implement fallback to webhook
6. [ ] Write unit tests with mock

---

### T40: Implement Socket Mode bot for replies
> Handle Slack thread replies and write to feedback files.

**Requires:** T39, T13
**Status:** open

**Done when:**
- [ ] `internal/notify/bot.go` defines `SocketModeBot` struct
- [ ] `Start(ctx context.Context) error` connects to Slack Socket Mode
- [ ] Listens for message events in tracked threads
- [ ] Converts thread replies to feedback file entries
- [ ] Handles reconnection on disconnect
- [ ] Runs as goroutine (doesn't block main execution)
- [ ] Supports global bot mode (config at ~/.ralph/)
- [ ] Integration test with mock Socket Mode

**Subtasks:**
1. [ ] Implement Socket Mode connection
2. [ ] Implement message event handling
3. [ ] Implement feedback file writing
4. [ ] Implement reconnection logic
5. [ ] Implement global bot mode
6. [ ] Write integration test

---

### T41: Integrate notifications into worker
> Wire notifications into worker lifecycle.

**Requires:** T39, T40, T32
**Status:** open

**Done when:**
- [ ] Worker sends Start notification when plan begins
- [ ] Worker sends Complete notification with PR URL when done
- [ ] Worker sends Blocker notification when blocker detected
- [ ] Worker sends Error notification on failure
- [ ] Iteration notifications sent if config.slack.notify_iteration is true
- [ ] Socket Mode bot auto-started if configured
- [ ] Notifications are no-op if Slack not configured
- [ ] Integration test verifies notification calls

**Subtasks:**
1. [ ] Inject Notifier into Worker
2. [ ] Add notification calls at lifecycle points
3. [ ] Auto-start Socket Mode bot
4. [ ] Handle missing configuration
5. [ ] Write integration test

---

## Phase 9: Release & Polish

### T42: Set up GoReleaser
> Cross-platform binary builds and releases.

**Requires:** T3
**Status:** open

**Done when:**
- [ ] `.goreleaser.yaml` configured for linux, darwin, windows (amd64, arm64)
- [ ] Version, Commit, BuildDate injected via ldflags
- [ ] `ralph version` shows correct values from goreleaser build
- [ ] `make build` runs local build
- [ ] `make release-snapshot` creates test release
- [ ] Archives include LICENSE, README.md
- [ ] Checksums generated

**Subtasks:**
1. [ ] Create .goreleaser.yaml
2. [ ] Configure ldflags for version injection
3. [ ] Update main.go with version vars
4. [ ] Create Makefile with build targets
5. [ ] Test snapshot release

---

### T43: Set up Homebrew tap
> Install via `brew install arvesolland/tap/ralph`.

**Requires:** T42
**Status:** open

**Done when:**
- [ ] homebrew-ralph repository created (or configured in goreleaser)
- [ ] Formula generated by goreleaser on release
- [ ] `brew install arvesolland/tap/ralph` installs latest version
- [ ] Formula includes description and homepage
- [ ] Test installation in clean environment

**Subtasks:**
1. [ ] Configure brew section in goreleaser
2. [ ] Set up homebrew tap repository
3. [ ] Test formula generation
4. [ ] Test installation

---

### T44: Create comprehensive README for Go version
> Document installation, usage, migration from bash.

**Requires:** T35
**Status:** open

**Done when:**
- [ ] README.md includes installation instructions (brew, binary download)
- [ ] Quick start section with basic usage
- [ ] Command reference for all subcommands
- [ ] Configuration reference (config.yaml format)
- [ ] Migration guide from bash version
- [ ] Troubleshooting section
- [ ] Badge showing latest release version

**Subtasks:**
1. [ ] Write installation section
2. [ ] Write quick start
3. [ ] Write command reference
4. [ ] Write configuration reference
5. [ ] Write migration guide
6. [ ] Add release badge

---

### T45: Add integration test suite
> Comprehensive tests matching bash test suite.

**Requires:** T35, T21
**Status:** open

**Done when:**
- [ ] Test: single-task - basic completion
- [ ] Test: dependencies - task dependency ordering
- [ ] Test: progress - progress file creation
- [ ] Test: loose-format - non-strict plan format
- [ ] Test: worker-queue - queue management with worktrees
- [ ] Test: dirty-state - dirty main worktree handling
- [ ] Test: worktree-cleanup - orphaned worktree cleanup
- [ ] Tests run with `go test ./... -tags=integration`
- [ ] CI runs integration tests

**Subtasks:**
1. [ ] Create testdata/plans/ with test plans
2. [ ] Implement single-task test
3. [ ] Implement dependencies test
4. [ ] Implement progress test
5. [ ] Implement loose-format test
6. [ ] Implement worker-queue test
7. [ ] Implement dirty-state test
8. [ ] Implement worktree-cleanup test
9. [ ] Configure CI for integration tests

---

### T46: Update CLAUDE.md for Go version
> Document Go architecture alongside bash (during transition).

**Requires:** T44
**Status:** open

**Done when:**
- [ ] CLAUDE.md updated with Go binary usage
- [ ] Go architecture section added
- [ ] Build commands updated
- [ ] Test commands updated
- [ ] Notes about bash deprecation timeline (if applicable)

**Subtasks:**
1. [ ] Add Go section to CLAUDE.md
2. [ ] Update command examples
3. [ ] Document new build process
4. [ ] Add deprecation notes if needed

---

## Discovered

<!-- Tasks found during implementation -->

---

## Completed

*(Move completed tasks here with completion date)*
