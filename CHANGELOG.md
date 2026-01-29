# Changelog

All notable changes to Ralph will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Fixed
- Handle Claude Code CLI hanging bug (GitHub Issue #19060) with timeout-based workaround
- Add real-time streaming output using jq filtering (credit: Matt Pollock)
- Add proper timeout handling for verification calls to prevent infinite hangs

### Changed
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
