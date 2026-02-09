# Changelog

All notable changes to Ralph will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [2.0.0] - 2026-02-09

Complete rewrite from bash scripts to a Go binary with ATM (Agent Task Manager) integration.

### Added
- **ATM Integration** - Plans and tasks managed via ATM API instead of filesystem queue
  - `internal/atm/` package wrapping `atm-cli` binary
  - Plan lifecycle: ready -> active -> complete/blocked
  - Task lifecycle: todo -> claimed -> doing -> done/blocked/skipped
  - Agent context bootstrapping via `atm-cli plan context <id>`
  - Progress and feedback entries for inter-iteration memory
- **ATM-based completion verification** - Checks ATM task stats instead of Haiku LLM verification
  - False completion circuit breaker (halts after 5 consecutive false claims)
  - Automatic feedback to ATM when agent falsely claims completion
- **`ralph run --plan <id>`** - Run iteration loop on a specific ATM plan
  - `--completion-mode` flag: pr, merge, or branch
  - `--push` flag: push to remote after each iteration
- **`ralph worker`** - Continuous queue processor polling ATM for ready plans
  - `--once` flag for single plan processing
  - `--pr`, `--merge`, `--branch` completion mode flags
  - `--sync` / `--sync-interval` for pull-from-remote workflow
  - `--push` for push-after-iteration (spot instance safety)
  - `--interval` for configurable poll interval
  - `--max` for configurable max iterations (default 200)
- **`ralph init`** - Interactive project initialization
  - Auto-detection of project type and commands (`--detect`)
  - AI-powered project name/description extraction
  - ATM configuration prompts (project slug, API URL, API token)
  - Specs directory scaffolding
- **`ralph status`** - ATM project status display
  - Active plan with task statistics
  - Available and blocked tasks
  - Recent progress and feedback entries
  - Color output with terminal detection
- **`ralph cleanup`** - Orphaned worktree removal
  - `--dry-run` flag for safe preview
  - Safety: won't remove worktrees with uncommitted changes
- **Branch completion mode** - Push to remote without PR or merge
- **Claude-generated PRs** - Uses Claude to create PR with intelligent description, falls back to basic PR
- **Configurable ATM** - `atm:` config section (project_slug, api_url, api_token, bin_path)
- **Worker config** - `worker:` config section (sync, sync_interval)
- **Fake ATM test infrastructure** - `test/integration/fakeatm/` binary for isolated testing
- **One-task-per-iteration test** - Verifies agent respects iteration boundaries
- **Slack mock server test** - Integration test with captured Slack API requests

### Changed
- **Architecture**: Complete rewrite from bash scripts to Go binary
- **Task management**: ATM API replaces filesystem-based plan queue (plans/pending, current, complete)
- **Completion verification**: ATM task stats replace Haiku LLM verification
- **Context format**: `context.json` uses `planId` (int) instead of `planFile` (string)
- **Prompt placeholders**: Added `{{PLAN_ID}}` and `{{ATM_CONTEXT}}` for ATM data injection
- **Worker defaults**: Max iterations increased to 200 (from 30) for worker mode
- **Integration tests**: Rewritten with fake ATM binary, local bare git origin, and PATH-based atm-cli injection

### Removed
- **Bash scripts**: `ralph.sh`, `ralph-worker.sh`, `ralph-init.sh`, `ralph-reverse.sh`, `ralph-cron.sh`, `ralph-release.sh`, `ralph-update.sh`, `install.sh`
- **File-based queue**: `plans/pending/`, `plans/current/`, `plans/complete/` directories
- **Plan parsing**: `internal/plan/` package (markdown plan parsing, checkbox tracking)
- **Haiku verification**: `internal/runner/verify.go` (replaced by ATM stats checking)
- **Plan bundles**: `ralph plan create` command and bundle scaffolding
- **`ralph reset`** command (use ATM to change plan status)
- **Slack bot directory**: Standalone `slack-bot/` (functionality integrated into `internal/notify/`)
- **Legacy prompts**: `worker_prompt.md`, `plan_reviewer_prompt.md`, `plan-spec.md` (kept only `prompt.md` and `pr_creation_prompt.md`)

---

## [1.1.0] - 2026-01-28

### Added
- Semantic versioning and changelog automation
- `ralph-release.sh` for version bumping with auto-detection
- commit-msg hook for automatic changelog updates
- hooks/install-hooks.sh for easy hook installation

### Fixed
- grep compatibility for summaries containing dashes
- Improved awk script for changelog section handling

### Changed
- Add release instructions to CLAUDE.md
- Trim whitespace from changelog entries
- Use Python for reliable changelog manipulation

## [1.0.0] - 2025-01-28

### Added
- Initial release of Ralph - AI Agent Implementation Loop
- `ralph.sh` - Main implementation loop with fresh context per iteration
- `ralph-worker.sh` - File-based task queue (pending/current/complete)
- `ralph-init.sh` - Project initialization with --detect and --ai modes
- `ralph-reverse.sh` - Codebase-to-specs reverse engineering loop
- Plan review phase with configurable passes
- Progress files for institutional memory
- Claude Code skills: ralph-spec, ralph-plan, ralph-spec-to-plan
- Automatic feature branch management
- PR creation via Claude Code
- Iterative discovery with confidence levels for reverse mode
- Sub-feature support with guidance on when to split features

### Fixed
- macOS compatibility for grep patterns (removed Perl regex dependency)
