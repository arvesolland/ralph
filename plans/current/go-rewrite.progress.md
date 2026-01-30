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
