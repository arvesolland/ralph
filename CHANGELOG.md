# Changelog

All notable changes to Ralph will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- Smart AI update: `ralph-update.sh --ai` regenerates only stub/placeholder config files
- Auto-add new config sections (slack, worktree) to existing config.yaml during update
- Slack bot files included in install.sh and ralph-update.sh
- Slack configuration template in default config.yaml
- Global Slack credentials detection during install
- Human input/blocker system: agent can signal `<blocker>` when human action required
- Slack notification for blockers (`notify_blocker` config option, default: true)
- Feedback file (`<plan>.feedback.md`) for human responses to blockers
- Blocker deduplication to avoid Slack notification spam
- Slack Socket Mode bot for handling thread replies (`slack-bot/`)
- Thread tracking for blocker notifications (enables Slack reply → feedback file)
- Worktree initialization: auto-install dependencies when creating plan worktrees
- Support for `.ralph/hooks/worktree-init` custom hook script
- Auto-detection for npm, yarn, pnpm, bun, composer, pip, poetry, bundle, go, cargo
- Automatic `.env` file copying to worktrees (configurable via `worktree.copy_env_files`)
- Config option `worktree.init_commands` for custom initialization commands
- `ralph-worker.sh --reset` to move current plan back to pending and start fresh

### Fixed
- use directory instead of deprecated folder in goreleaser config
- use goreleaser v2 and enable homebrew tap token
- Verification loop now writes detailed failure reasons to feedback file (prevents infinite "incomplete tasks" loops)
- Handle Claude Code CLI hanging bug (GitHub Issue #19060) with timeout-based workaround
- Add real-time streaming output using jq filtering (credit: Matt Pollock)
- Add proper timeout handling for verification calls to prevent infinite hangs

### Changed
- sync plan checkboxes with actual state (T44-T46 complete)
- update progress log for T43 blocker status
- add integration test suite (T45)
- update CLAUDE.md for Go version (T46)
- comprehensive README for Go version (T44)
- add Homebrew tap config to GoReleaser (T43 partial)
- add GoReleaser and Makefile for builds (T42)
- integrate notifications into worker (T41)
- implement Slack Socket Mode bot for replies (T40)
- implement Slack Bot API notifications (T39)
- implement Slack thread tracking (T38)
- implement Slack webhook notifications (T37)
- add ralph reset command (T36)
- add ralph worker command (T35)
- implement completion workflow merge mode (T34)
- implement completion workflow PR mode (T33)
- implement worker queue loop (T32)
- add ralph run command (T31)
- implement iteration loop (T30)
- implement iteration context management (T29)
- implement completion verification with Haiku (T28)
- implement blocker extraction (T27)
- implement Runner interface with timeout handling (T25, T26)
- implement retry logic with exponential backoff (T24)
- implement streaming JSON parser (T23)
- implement Claude CLI command builder (T22)
- implement orphaned worktree cleanup (T21)
- implement initialization hooks (T20)
- implement file sync operations (T19)
- implement dependency auto-detection (T18)
- implement WorktreeManager (T17)
- implement worktree operations (T16)
- implement Git interface and basic operations
- add ralph status command
- implement feedback file handling
- implement progress file handling
- implement queue management for plan lifecycle
- implement checkbox update and atomic save
- implement task extraction from plans
- implement Plan struct and parsing
- add ralph init command
- implement prompt template builder
- implement project auto-detection
- implement Config struct and YAML loading
- implement structured logging with level filtering and color support
- initialize Go module and project structure
- Add Go rewrite spec and implementation plan
- Add Slack notification when implementation phase starts
- Use queue plan path for plan name in notifications
- Sync .ralph/config.yaml to worktree during init
- Add Slack notification for plan review start
- Include PR URL in Slack completion notification
- Handle inline YAML comments in config_get
- macOS sed compatibility in Slack message escaping
- Clarify worktree plan location in reviewer prompt
- Update CHANGELOG for smart AI update feature
- Add smart AI update to ralph-update.sh
- Add Slack bot to install and update scripts
- Auto-detect global Slack credentials
- Add global Slack bot mode and auto-start
- Add Slack bot for human input handling
- Add worktree initialization and reset command
- Add worktree-based plan isolation
- Update CHANGELOG.md to include recent changes
- Add git pull and --review to ralph-cron.sh
- Add ralph-cron.sh wrapper for scheduled runs
- Add Slack notifications section to CLAUDE.md
- Add optional Slack webhook notifications
- Merge base branch into existing feature branches
- Stash untracked files during branch switch
- Fix commit count check after merge-to-main flow
- Let Claude attempt to resolve merge conflicts
- Update worker-queue test for merge-to-main flow
- Merge feature branch to main after plan completion
- Increase default max iterations to 50
- Add --review flag to worker for plan review
- Simplify plan reviewer prompt and add spec alignment
- Improve error detection and plan file preservation
- Update prompt to commit plan and progress files together
- Add retry logic and prevent progress files from being treated as plans

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
