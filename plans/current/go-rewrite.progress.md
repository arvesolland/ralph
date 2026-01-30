# Progress: Go Rewrite

Iteration log - what was done, gotchas, and next steps.

---
### Iteration 1: T1 - Initialize Go module and project structure
**Completed:**
- Created `go.mod` with module `github.com/arvesolland/ralph` and Go 1.22
- Created directory structure: `cmd/ralph/`, `internal/{cli,config,plan,runner,git,worktree,prompt,notify,log}/`
- Added placeholder `.go` files in each internal package with package declarations
- Created `cmd/ralph/main.go` that prints "ralph dev"
- Updated `.gitignore` to ignore `ralph`, `*.exe`, `dist/`
- Verified `go build ./cmd/ralph` succeeds

**Gotcha:** `go mod init` defaulted to Go 1.19, manually updated to 1.22 to meet requirements.

**Next:** T2 - Implement structured logging (depends on T1, now complete)

---
### Iteration 2: T2 - Implement structured logging
**Completed:**
- Created `internal/log/log.go` with `Logger` interface (Debug, Info, Success, Warn, Error, SetLevel, SetOutput, SetColorEnabled)
- Implemented `ConsoleLogger` struct with thread-safe logging (uses sync.Mutex)
- Added level filtering: LevelDebug < LevelInfo == LevelSuccess < LevelWarn < LevelError
- Added ANSI color support with TTY detection: Debug=gray, Info=default, Success=green, Warn=yellow, Error=red
- Default logger writes to stderr with timestamps in `HH:MM:SS` format
- Created package-level convenience functions (Debug, Info, Success, Warn, Error)
- Created `internal/log/log_test.go` with comprehensive tests for level filtering and color codes
- All tests pass (7 test functions covering levels, colors, formatting)

**Gotcha:** Note that `log.go` is used instead of `logger.go` for idiomatic Go naming (the file defines the main `log` package functionality). The interface is still named `Logger` as specified.

**Next:** T3 - Set up Cobra CLI framework with root command (depends on T2, now complete)

---
### Iteration 3: T3 - Set up Cobra CLI framework with root command
**Completed:**
- Added `github.com/spf13/cobra` v1.10.2 dependency (brings in `spf13/pflag` and `mousetrap`)
- Created `internal/cli/root.go` with:
  - Root command with description of Ralph's purpose
  - Global persistent flags: `--config/-c`, `--verbose/-v`, `--quiet/-q`, `--no-color`
  - `PersistentPreRun` hook that configures logging based on flags
  - `Execute()` function for main.go to call
- Created `internal/cli/version.go` with:
  - `version` subcommand showing Version, Commit, BuildDate
  - Variables set up for ldflags injection at build time (defaults to "dev"/"unknown")
- Updated `cmd/ralph/main.go` to call `cli.Execute()`
- Removed placeholder `internal/cli/cli.go` file
- Verified all acceptance criteria:
  - `ralph --help` shows usage with all global flags
  - `ralph version` shows "ralph version dev" with commit/build info
  - Unknown commands return helpful error message with exit code 1
- All existing tests pass

**Gotcha:** None - Cobra setup is straightforward.

**Next:** T4 - Implement Config struct and YAML loading (depends on T3, now complete)

---
### Iteration 4: T4 - Implement Config struct and YAML loading
**Completed:**
- Added `gopkg.in/yaml.v3` v3.0.1 dependency
- Created `internal/config/config.go` with:
  - All config structs matching spec: `Config`, `ProjectConfig`, `GitConfig`, `CommandsConfig`, `SlackConfig`, `WorktreeConfig`, `CompletionConfig`
  - `Load(path string) (*Config, error)` - reads and parses YAML file
  - `LoadWithDefaults(path string) (*Config, error)` - applies defaults for missing fields, handles missing/empty files gracefully
  - `mergeConfig()` helper for merging file config into defaults
- Created `internal/config/defaults.go` with sensible defaults:
  - `git.base_branch: "main"`
  - `completion.mode: "pr"`
  - `worktree.copy_env_files: ".env"`
  - Slack notifications: start/complete/error/blocker enabled, iteration disabled
- Created `internal/config/config_test.go` with 10 test functions covering:
  - Valid config loading
  - Missing file error (for Load) vs defaults (for LoadWithDefaults)
  - Invalid YAML parsing
  - YAML inline comments (the bash bug source)
  - Nested key access
  - Empty file handling
  - Partial config with defaults merge
  - All field types (strings, bools)
- All tests pass (10 tests)
- Build succeeds

**Gotcha:** Bool fields in YAML can't distinguish "not set" from "set to false" with standard yaml.Unmarshal. Used OR merge for notification bools that default to true, so if file sets them to false, it won't override defaults unless explicitly handled. In practice this is acceptable since users wanting to disable notifications will set them explicitly.

**Next:** T5 - Implement project auto-detection (depends on T4, now complete)

---
### Iteration 5: T5 - Implement project auto-detection
**Completed:**
- Created `internal/config/detect.go` with:
  - `DetectedConfig` struct with Language, Framework, PackageJSON, Commands fields
  - `Detect(dir string) (*DetectedConfig, error)` - detects project type from files
  - Node.js detection from package.json with script extraction (test/lint/build/dev)
  - Framework detection (Next.js, Nuxt, Vite) from config files
  - Go detection from go.mod with golangci-lint support
  - Python detection from pyproject.toml or requirements.txt with flake8/ruff support
  - PHP detection from composer.json with Laravel framework and PHPUnit/PHPStan support
  - Rust detection from Cargo.toml with cargo test/build/clippy
  - Ruby detection from Gemfile with Rails framework and RuboCop support
- Created test fixtures in `internal/config/testdata/detect/`:
  - `node/` - basic Node.js project
  - `node-nextjs/` - Next.js project with next.config.js
  - `go/` - Go module project
  - `python/` - Python project with pyproject.toml
  - `python-requirements/` - Python project with requirements.txt
  - `php/` - PHP Composer project
  - `php-laravel/` - Laravel project with artisan file
  - `rust/` - Rust Cargo project
  - `ruby/` - Ruby Bundler project
  - `ruby-rails/` - Rails project with config/application.rb
  - `empty/` - empty directory for no-detection test
- Created `internal/config/detect_test.go` with 12 test functions covering all languages and edge cases
- All 22 tests pass (10 config tests + 12 detect tests)

**Gotcha:** Detection order matters - Node.js is checked first because package.json is common in polyglot repos. Each detector returns nil for "not this language" vs error for "this language but malformed".

**Next:** T6 - Implement prompt template builder (depends on T4, now complete)

---
### Iteration 6: T6 - Implement prompt template builder
**Completed:**
- Created `internal/prompt/builder.go` with:
  - `Builder` struct with config, configDir, promptsDir fields
  - `NewBuilder(cfg *Config, configDir, promptsDir string) *Builder` constructor
  - `Build(templatePath string, overrides map[string]string) (string, error)` method
  - `placeholderRegex` matching `{{PLACEHOLDER}}` syntax
  - All placeholders supported: PROJECT_NAME, PROJECT_DESCRIPTION, PRINCIPLES, PATTERNS, BOUNDARIES, TECH_STACK, TEST_COMMAND, LINT_COMMAND, BUILD_COMMAND, DEV_COMMAND
  - Missing override files (principles.md, etc.) return empty string, not error
  - Unknown placeholders left as-is for forward compatibility
  - Template loading: absolute path → external prompts dir → embedded fallback
- Created `internal/prompt/templates.go` with:
  - `//go:embed prompts/*.md` directive for embedding templates
  - `loadEmbeddedPrompt(templatePath string)` function
  - `ListEmbeddedPrompts()` utility function
- Copied all prompts from `prompts/base/*.md` to `internal/prompt/prompts/` for embedding
- Removed placeholder `internal/prompt/prompt.go` file
- Created `internal/prompt/builder_test.go` with 9 test functions:
  - TestBuilder_Build_WithConfig - config-based placeholders
  - TestBuilder_Build_WithOverrideFiles - .ralph/*.md file loading
  - TestBuilder_Build_MissingOverrideFiles - empty string for missing files
  - TestBuilder_Build_UnknownPlaceholders - unknown left as-is
  - TestBuilder_Build_WithExplicitOverrides - override map precedence
  - TestBuilder_Build_EmbeddedPrompt - embedded prompt fallback
  - TestBuilder_Build_AbsolutePath - absolute path support
  - TestListEmbeddedPrompts - embedded listing
  - TestPlaceholderRegex - regex pattern matching
- All 31 tests pass (22 existing + 9 new)
- Build succeeds, `ralph version` works

**Gotcha:** The `Build` method signature differs slightly from spec - it's a method on Builder rather than taking Config as parameter. This is cleaner since Builder already holds config reference and allows for additional state/caching if needed later.

**Next:** T7 - Add `ralph init` command (depends on T5, T6 - both now complete)

---
### Iteration 7: T7 - Add `ralph init` command
**Completed:**
- Created `internal/cli/init.go` with complete init command implementation:
  - `ralph init` creates full directory structure: `.ralph/`, `.ralph/worktrees/`, `plans/{pending,current,complete}/`, `specs/`
  - Creates `.ralph/config.yaml` from defaults
  - Creates `.ralph/worktrees/.gitignore` to exclude worktrees from git
  - `--detect` flag runs project auto-detection and populates config with detected commands
  - Existing config file prompts for confirmation before overwrite (y/N prompt)
  - Creates `specs/INDEX.md` with comprehensive starter template
  - Prints helpful summary with next steps
- Created `internal/cli/init_test.go` with 4 integration tests:
  - `TestRunInit_CreatesDirectoryStructure` - verifies all directories and files created
  - `TestRunInit_WithDetection` - tests Node.js auto-detection populates config
  - `TestRunInit_PreservesExistingSpecs` - verifies existing INDEX.md not overwritten
  - `TestSpecsIndexContent` - verifies INDEX.md template has essential sections
- All 35 tests pass (4 cli + 22 config/detect + 9 prompt)
- Build succeeds, `ralph init --help` shows command and `--detect` flag

**Gotcha:** Used existing `config.Defaults()` function rather than creating new `NewDefaultConfig()` to avoid duplication.

**Next:** T8 - Implement Plan struct and parsing (depends on T4, now complete)

---
### Iteration 8: T8 - Implement Plan struct and parsing
**Completed:**
- Created `internal/plan/plan.go` with:
  - `Plan` struct with Path, Name, Content, Tasks, Status, Branch fields
  - `Task` struct placeholder (for T9)
  - `Load(path string) (*Plan, error)` reads plan file and extracts all fields
  - `deriveName()` extracts plan name from filename (e.g., "go-rewrite.md" -> "go-rewrite")
  - `extractStatus()` with regex to find `**Status:** value` in content, defaults to "pending"
  - `deriveBranch()` creates git branch name from plan name (e.g., "go-rewrite" -> "feat/go-rewrite")
  - `sanitizeBranchName()` converts special characters: lowercase, spaces->hyphens, removes special chars, collapses multiple hyphens
- Created test fixtures in `internal/plan/testdata/`:
  - `valid-plan.md` - plan with explicit status
  - `no-status.md` - plan without status field (tests default)
  - `my plan (v2).md` - plan with special characters in filename (tests sanitization)
- Created `internal/plan/plan_test.go` with 8 test functions:
  - TestLoad_ValidPlan, TestLoad_MissingStatus, TestLoad_SpecialCharactersInName, TestLoad_NonexistentFile
  - TestDeriveName, TestExtractStatus, TestSanitizeBranchName, TestDeriveBranch
- All 43 tests pass (8 new + 35 existing)

**Gotcha:** The Task struct is defined as a placeholder here but will be fully implemented in T9. The regex for status extraction uses `(?m)` for multiline mode to match at start of any line.

**Next:** T9 - Implement task extraction from plans (depends on T8, now complete)

---
### Iteration 9: T9 - Implement task extraction from plans
**Completed:**
- Created `internal/plan/task.go` with:
  - `Task` struct with Line, Text, Complete, Requires, Subtasks, Indent fields
  - `checkboxRegex` matching `- [ ]` or `- [x]` patterns with indentation capture
  - `requiresRegex` for "requires: T1, T2" pattern matching (case-insensitive)
  - `ExtractTasks(content string) []Task` parses markdown content and extracts all checkboxes
  - `extractRequires(text string) []string` extracts task IDs from requires clause
  - `buildTaskTree(flat []Task) []Task` converts flat task list to nested tree based on indentation
  - `CountComplete(tasks []Task) int` recursively counts completed tasks
  - `CountTotal(tasks []Task) int` recursively counts all tasks
  - `FindNextIncomplete(tasks []Task, completedIDs map[string]bool) *Task` finds next actionable task
- Updated `internal/plan/plan.go`:
  - Removed duplicate Task struct placeholder
  - Updated `Load()` to call `ExtractTasks()` and populate Tasks field
- Created `internal/plan/task_test.go` with 12 test functions:
  - TestExtractTasks_SimpleTasks, TestExtractTasks_NestedSubtasks, TestExtractTasks_WithDependencies
  - TestExtractTasks_MixedCompleteIncomplete, TestExtractTasks_EmptyContent, TestExtractTasks_NoCheckboxes
  - TestExtractTasks_LineNumbers, TestExtractRequires, TestCountComplete, TestCountTotal
  - TestFindNextIncomplete, TestExtractTasks_RealWorldPlan
- All 55 tests pass (12 new + 43 existing)

**Gotcha:** The checkbox regex only matches dash-style lists (`- [ ]`), not numbered lists (`1. [ ]`). The actual plan format uses dashes for checkboxes, so this is correct behavior. Numbered lists in the plan are for subtask descriptions, not checkboxes.

**Next:** T10 - Implement checkbox update in plans (depends on T9, now complete)

---
### Iteration 10: T10 - Implement checkbox update in plans
**Completed:**
- Updated T9 status to `complete` (was still marked `open` from previous iteration)
- Created `internal/plan/update.go` with:
  - `UpdateCheckbox(content string, lineNum int, complete bool) (string, error)` - modifies checkbox at specific line
  - `checkboxUpdateRegex` that captures everything before/after the checkbox for exact preservation
  - `ErrNoCheckbox` and `ErrInvalidLine` error types for proper error handling
  - `Plan.SetCheckbox(lineNum int, complete bool)` convenience method that updates Content and re-extracts Tasks
  - `Save(plan *Plan) error` with atomic write (temp file + rename) to prevent corruption
- Created `internal/plan/update_test.go` with 13 test functions covering:
  - Complete/uncomplete operations
  - Whitespace preservation (various indent levels, tabs, extra spaces)
  - Error cases (no checkbox on line, invalid line numbers)
  - Surrounding markdown preservation
  - Save operations (create, overwrite, permission preservation, atomic write verification)
  - SetCheckbox integration with Plan struct
- All 68 tests pass (55 existing + 13 new)

**Gotcha:** None - straightforward implementation. Used regex capture groups to preserve exact formatting around the checkbox.

**Next:** T11 - Implement Queue management (depends on T8, which is complete)

---
### Iteration 11: T11 - Implement Queue management
**Completed:**
- Created `internal/plan/queue.go` with:
  - `Queue` struct with BaseDir field for base directory containing queue subdirectories
  - `QueueStatus` struct with PendingCount, CurrentCount, CompleteCount, PendingPlans, CurrentPlan
  - Error types: `ErrQueueFull`, `ErrNoCurrent`, `ErrPlanNotInPending`, `ErrPlanNotInCurrent`
  - `NewQueue(baseDir string) *Queue` constructor
  - `Pending() ([]*Plan, error)` lists plans in pending/ sorted by name, skips progress/feedback files
  - `Current() (*Plan, error)` returns plan in current/ (nil if empty, error if multiple)
  - `Activate(plan *Plan) error` moves from pending/ to current/, checks for queue full
  - `Complete(plan *Plan) error` moves from current/ to complete/
  - `Reset(plan *Plan) error` moves from current/ back to pending/
  - `Status() (*QueueStatus, error)` returns counts and names for each queue
  - Helper `listPlans(dir string)` for directory scanning with .md filtering
- Created `internal/plan/queue_test.go` with 17 test functions:
  - TestNewQueue, TestQueue_Pending, TestQueue_Pending_SkipsNonMdFiles
  - TestQueue_Pending_SkipsProgressAndFeedback, TestQueue_Current_Empty, TestQueue_Current_WithPlan
  - TestQueue_Activate, TestQueue_Activate_QueueFull, TestQueue_Activate_NotInPending
  - TestQueue_Complete, TestQueue_Complete_NotInCurrent, TestQueue_Reset, TestQueue_Reset_NotInCurrent
  - TestQueue_Status, TestQueue_FullLifecycle, TestQueue_NonExistentDirectory
- All 85 tests pass (68 existing + 17 new)
- Build succeeds, `ralph version` works

**Gotcha:** The `listPlans()` helper needs to skip `.progress.md` and `.feedback.md` files which are sibling files to plans, not plans themselves. Used suffix matching for this.

**Next:** T12 - Implement progress file handling (depends on T8, which is complete)

---
### Iteration 12: T12 - Implement progress file handling
**Completed:**
- Created `internal/plan/progress.go` with:
  - `ProgressPath(plan *Plan) string` returns `<plan-path-without-ext>.progress.md`
  - `ReadProgress(plan *Plan) (string, error)` reads existing content, returns empty string if not exists
  - `AppendProgress(plan *Plan, iteration int, content string) error` adds timestamped entry with current time
  - `AppendProgressWithTime()` variant for testing with explicit timestamp
  - `CreateProgressFile(plan *Plan) error` creates file with header if not exists
  - Entry format: `## Iteration N (YYYY-MM-DD HH:MM)\n{content}\n`
  - All functions create parent directories if needed
- Created `internal/plan/progress_test.go` with 10 test functions:
  - TestProgressPath (3 subtests: simple, nested, multiple dots)
  - TestReadProgress_NonExistent, TestReadProgress_Existing
  - TestAppendProgress_NewFile, TestAppendProgress_ExistingFile, TestAppendProgress_MultipleIterations
  - TestCreateProgressFile_NewFile, TestCreateProgressFile_AlreadyExists
  - TestAppendProgress_CreatesParentDirectory, TestProgressPath_PreservesDirectory
- All 95 tests pass (85 existing + 10 new)
- Build succeeds

**Gotcha:** None - straightforward implementation. Added `AppendProgressWithTime` for deterministic tests.

**Next:** T13 - Implement feedback file handling (depends on T8, which is complete)

---
### Iteration 13: T13 - Implement feedback file handling
**Completed:**
- Created `internal/plan/feedback.go` with:
  - `FeedbackPath(plan *Plan) string` returns `<plan-path-without-ext>.feedback.md`
  - `ReadFeedback(plan *Plan) (string, error)` reads pending section content
  - `extractPendingSection()` helper to parse `## Pending` section from file content
  - `AppendFeedback(plan *Plan, source string, content string) error` adds timestamped entry
  - `AppendFeedbackWithTime()` variant for testing with explicit timestamp
  - `insertIntoPendingSection()` helper to insert entries in correct location
  - `MarkProcessed(plan *Plan, entry string) error` moves entry from Pending to Processed
  - `moveEntryToProcessed()` helper for the actual move logic
  - `CreateFeedbackFile(plan *Plan) error` creates file with proper structure
  - File format: `# Feedback: {name}\n\n## Pending\n...\n\n## Processed\n...`
- Created `internal/plan/feedback_test.go` with 16 test functions:
  - TestFeedbackPath (3 subtests: simple, nested, dots in name)
  - TestReadFeedback_NonExistent, TestReadFeedback_Existing, TestReadFeedback_EmptyPendingSection
  - TestAppendFeedback_NewFile, TestAppendFeedback_ExistingFile, TestAppendFeedback_NoSource
  - TestMarkProcessed_Success, TestMarkProcessed_EntryNotFound, TestMarkProcessed_FileNotExists
  - TestCreateFeedbackFile_NewFile, TestCreateFeedbackFile_AlreadyExists
  - TestExtractPendingSection (5 subtests: normal, empty, no processed, no pending, with comments)
  - TestFeedbackPath_PreservesDirectory
- All tests pass (95 existing + 16 new = 111 total across plan package)
- Build succeeds

**Gotcha:** None - implementation follows the same patterns as progress.go with section-aware parsing.

**Next:** T14 - Add `ralph status` command (depends on T11 and T3, both complete)

---
### Iteration 14: T14 - Add `ralph status` command
**Completed:**
- Created `internal/cli/status.go` with:
  - `statusCmd` Cobra command with description and usage info
  - `runStatus()` function that displays queue status
  - Reads from `plans/` directory (pending, current, complete subdirectories)
  - Uses `plan.Queue` from T11 for status retrieval
  - Displays current plan name and branch (feat/<name>) in green
  - Displays pending plans count and list in yellow
  - Displays complete plans count
  - Worktree status placeholder (not yet implemented - T17+)
  - Color output with TTY detection and --no-color flag support
  - Graceful handling of missing plans directory
- Created `internal/cli/status_test.go` with 6 integration tests:
  - TestRunStatus_NoPlanDirectory - graceful handling when no plans/
  - TestRunStatus_EmptyQueue - empty queue display
  - TestRunStatus_WithCurrentPlan - current plan name and branch display
  - TestRunStatus_WithPendingPlans - pending list with multiple plans
  - TestRunStatus_OutputFormat - all sections present with correct counts
  - TestRunStatus_ExitCode - returns nil (exit 0) on success
- All 117 tests pass (111 existing + 6 new)
- Build succeeds, `ralph status` works correctly

**Gotcha:** None - used direct fmt.Print for status output instead of log package since log adds timestamps/levels which aren't appropriate for status display.

**Next:** T15 - Implement Git interface and basic operations (depends on T2, which is complete)

---
### Iteration 15: T15 - Implement Git interface and basic operations
**Completed:**
- Created `internal/git/git.go` with:
  - `Git` interface defining all required operations: Status, Add, Commit, Push, PushWithUpstream, Pull, CurrentBranch, CreateBranch, DeleteBranch, DeleteRemoteBranch, BranchExists, Checkout, Merge, RepoRoot, IsClean, WorkDir
  - `CLIGit` struct implementing the interface by shelling out to git CLI
  - `NewGit(workDir string) Git` constructor
  - `Status` struct with Branch, Staged, Unstaged, Untracked fields and IsClean() method
  - `run()` helper for command execution with trimmed output
  - `runRaw()` helper for commands needing exact output (like status --porcelain)
  - Error types: ErrNotGitRepo, ErrUncommittedChanges, ErrBranchNotFound, ErrBranchExists, ErrMergeConflict
- Created `internal/git/git_test.go` with 19 integration tests:
  - TestNewGit, TestStatus_CleanRepo, TestStatus_WithChanges
  - TestAdd, TestAdd_EmptyFiles, TestCommit, TestCommit_NothingToCommit
  - TestCurrentBranch, TestCreateBranch, TestCreateBranch_AlreadyExists
  - TestDeleteBranch, TestBranchExists, TestCheckout, TestCheckout_BranchNotFound
  - TestRepoRoot, TestRepoRoot_NotGitRepo, TestIsClean, TestMerge
  - TestStatus_IsCleanMethod
- All tests run against real temporary git repos created with setupTestRepo()
- All 136 tests pass (117 existing + 19 new)

**Gotcha:**
- The `run()` helper does `strings.TrimSpace()` which broke status parsing (leading space in ` M file.txt` was trimmed). Added `runRaw()` for status --porcelain parsing.
- In repos with no commits, `git rev-parse --abbrev-ref HEAD` fails. Used `git symbolic-ref --short HEAD` with fallback.
- macOS `/tmp` is symlink to `/private/var/...` - used `filepath.EvalSymlinks()` in tests for comparison.
- `git commit` with nothing to commit outputs to stdout, not stderr.

**Next:** T16 - Implement git worktree operations (depends on T15, now complete)

---
### Iteration 16: T16 - Implement git worktree operations
**Completed:**
- Added worktree operations to `internal/git/git.go`:
  - `CreateWorktree(path, branch string) error` - creates worktree, auto-creates branch if needed
  - `RemoveWorktree(path string) error` - removes worktree, force-removes if has changes
  - `ListWorktrees() ([]WorktreeInfo, error)` - parses `git worktree list --porcelain`
  - `WorktreeInfo` struct with Path, Branch, Commit, Bare fields
  - `ErrBranchAlreadyCheckedOut` and `ErrWorktreeNotFound` error types
- Added 11 integration tests to `internal/git/git_test.go`:
  - TestCreateWorktree_NewBranch, TestCreateWorktree_ExistingBranch
  - TestCreateWorktree_BranchAlreadyCheckedOut, TestCreateWorktree_BranchCheckedOutInOtherWorktree
  - TestRemoveWorktree, TestRemoveWorktree_NotFound, TestRemoveWorktree_WithChanges
  - TestListWorktrees, TestListWorktrees_Multiple, TestWorktreeInfo
- All 30 git tests pass, all 147 total tests pass

**Gotcha:** The worktree list porcelain output includes `bare` and `detached` markers that need special handling. Detached HEAD means branch field stays empty.

**Next:** T17 - Implement WorktreeManager (depends on T16 and T11, both now complete)
