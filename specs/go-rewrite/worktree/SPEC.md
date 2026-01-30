# Feature: Git & Worktree

**ID:** F1.4
**Status:** planned
**Requires:** F1.2

## Summary

Git operations wrapper and worktree-based isolation for plan execution. Each plan runs in its own git worktree to prevent branch-switching conflicts in the main worktree.

## Goals

- Wrap common git operations (status, commit, push, branch)
- Create/remove git worktrees
- Implement three-layer locking (file + worktree + directory)
- Auto-detect and install dependencies in worktree
- Copy .env files to worktree
- Run custom init hooks/commands
- Sync files between main worktree and plan worktree
- Clean up orphaned worktrees

## Non-Goals

- Replace git CLI entirely
- Implement git operations from scratch
- Support non-git version control

## Design

### Git Interface

```go
type Git interface {
    // Basic operations
    Status() (*Status, error)
    Commit(message string, files ...string) error
    Add(files ...string) error
    Push(remote, branch string) error
    Pull(remote, branch string) error

    // Branch operations
    CurrentBranch() (string, error)
    CreateBranch(name, base string) error
    DeleteBranch(name string) error
    BranchExists(name string) (bool, error)

    // Worktree operations
    CreateWorktree(path, branch string) error
    RemoveWorktree(path string) error
    ListWorktrees() ([]WorktreeInfo, error)

    // Info
    RepoRoot() (string, error)
    IsClean() (bool, error)
}

type Status struct {
    Branch     string
    Staged     []string
    Unstaged   []string
    Untracked  []string
    IsClean    bool
}

type WorktreeInfo struct {
    Path   string
    Branch string
    Commit string
}
```

### Worktree Manager

```go
type WorktreeManager struct {
    git       Git
    baseDir   string // .ralph/worktrees/
    config    *config.Config
}

func (m *WorktreeManager) Create(plan *Plan) (*Worktree, error)
func (m *WorktreeManager) Remove(plan *Plan) error
func (m *WorktreeManager) Get(plan *Plan) (*Worktree, error)
func (m *WorktreeManager) Cleanup() ([]string, error) // Remove orphaned
func (m *WorktreeManager) SyncToWorktree(plan *Plan) error
func (m *WorktreeManager) SyncFromWorktree(plan *Plan) error

type Worktree struct {
    Path   string
    Branch string
    Plan   *Plan
}
```

### Dependency Detection

Check for lockfiles and run appropriate install:

| Lockfile | Command |
|----------|---------|
| package-lock.json | `npm ci` |
| yarn.lock | `yarn install --frozen-lockfile` |
| pnpm-lock.yaml | `pnpm install --frozen-lockfile` |
| bun.lockb | `bun install --frozen-lockfile` |
| composer.lock | `composer install` |
| requirements.txt | `pip install -r requirements.txt` |
| poetry.lock | `poetry install` |
| Gemfile.lock | `bundle install` |
| go.sum | `go mod download` |
| Cargo.lock | `cargo fetch` |

### File Sync

Files to sync TO worktree (at creation):
- Plan file
- Progress file
- Feedback file
- .env files (configurable)

Files to sync FROM worktree (after each iteration):
- Plan file (updated checkboxes)
- Progress file (new entries)

### Worktree Init Hooks

1. Copy .env files (from config.worktree.copy_env_files)
2. Run `.ralph/hooks/worktree-init` if executable
3. Run config.worktree.init_commands if set
4. Otherwise, auto-detect and install dependencies

### Key Files

| File | Purpose |
|------|---------|
| `internal/git/git.go` | Git interface and CLI wrapper |
| `internal/git/status.go` | Status parsing |
| `internal/worktree/manager.go` | Worktree lifecycle |
| `internal/worktree/deps.go` | Dependency detection/install |
| `internal/worktree/sync.go` | File synchronization |
| `internal/worktree/hooks.go` | Hook execution |

## Gotchas

- `git worktree add` fails if branch already checked out elsewhere - this IS the lock
- Worktree path must be outside repo root or in gitignored directory
- Removing worktree requires branch to not be checked out in main
- .env files may contain secrets - handle carefully
- Sync must be atomic to avoid partial state
- Cleanup should not remove worktrees with uncommitted changes

---

## Changelog

- 2026-01-31: Initial spec
