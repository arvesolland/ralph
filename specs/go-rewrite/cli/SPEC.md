# Feature: CLI & Release

**ID:** F1.6
**Status:** planned
**Requires:** F1.1, F1.2, F1.3, F1.4, F1.5

## Summary

Command-line interface using Cobra, structured logging, and cross-platform release automation with GoReleaser. This ties all components together into a single binary.

## Goals

- Intuitive CLI with subcommands (run, worker, init, status)
- Structured logging with --verbose flag
- Colored output for terminal
- Cross-platform builds (linux, darwin, windows; amd64, arm64)
- Homebrew tap for easy installation
- Version command with build info
- Backward-compatible with bash script behavior

## Non-Goals

- GUI
- Daemon mode
- System service installation

## Design

### Command Structure

```
ralph                         # Show help
ralph run <plan>              # Run iteration loop on plan
ralph run <plan> --max 10     # Limit iterations
ralph run <plan> --review     # Review plan before execution

ralph worker                  # Process queue continuously
ralph worker --once           # Process one plan and exit
ralph worker --pr             # Create PR on completion (default)
ralph worker --merge          # Merge to base on completion

ralph status                  # Show queue and worktree status
ralph cleanup                 # Remove orphaned worktrees
ralph reset                   # Reset current plan to pending

ralph init                    # Interactive project setup
ralph init --detect           # Auto-detect and configure
ralph init --ai               # AI-assisted setup

ralph version                 # Show version, commit, build date
ralph help <command>          # Command-specific help
```

### Global Flags

```
--config, -c     Config file path (default: .ralph/config.yaml)
--verbose, -v    Verbose output
--quiet, -q      Suppress non-error output
--no-color       Disable colored output
```

### Logging

```go
type Logger interface {
    Debug(msg string, fields ...Field)
    Info(msg string, fields ...Field)
    Warn(msg string, fields ...Field)
    Error(msg string, fields ...Field)
}

// Colors (when terminal)
// Debug: gray
// Info: default
// Warn: yellow
// Error: red
// Success: green (custom level)
```

### Version Info

```go
var (
    Version   = "dev"     // Set by goreleaser
    Commit    = "none"    // Set by goreleaser
    BuildDate = "unknown" // Set by goreleaser
)

func VersionCmd() {
    fmt.Printf("ralph %s (%s) built %s\n", Version, Commit[:7], BuildDate)
}
```

### GoReleaser Config

```yaml
builds:
  - main: ./cmd/ralph
    binary: ralph
    ldflags:
      - -s -w
      - -X main.Version={{.Version}}
      - -X main.Commit={{.Commit}}
      - -X main.BuildDate={{.Date}}
    goos: [linux, darwin, windows]
    goarch: [amd64, arm64]

archives:
  - format: tar.gz
    name_template: "ralph_{{ .Version }}_{{ .Os }}_{{ .Arch }}"
    files: [LICENSE, README.md]

brews:
  - repository:
      owner: arvesolland
      name: homebrew-ralph
    homepage: https://github.com/arvesolland/ralph
    description: Autonomous AI development loop orchestration
    install: bin.install "ralph"

checksum:
  name_template: "checksums.txt"

changelog:
  sort: asc
  filters:
    exclude: ["^docs:", "^test:", "^chore:"]
```

### Key Files

| File | Purpose |
|------|---------|
| `cmd/ralph/main.go` | Entry point, version vars |
| `internal/cli/root.go` | Root command, global flags |
| `internal/cli/run.go` | Run command |
| `internal/cli/worker.go` | Worker command |
| `internal/cli/init.go` | Init command |
| `internal/cli/status.go` | Status command |
| `internal/cli/version.go` | Version command |
| `internal/log/logger.go` | Logging implementation |
| `.goreleaser.yaml` | Release configuration |
| `Makefile` | Build, test, release targets |

## Gotchas

- Windows paths use backslashes - use filepath.Join everywhere
- Colors don't work in all Windows terminals - detect and disable
- GoReleaser needs GITHUB_TOKEN for releases
- Homebrew formula needs separate repo (homebrew-ralph)
- Version must be semantic (vX.Y.Z) for goreleaser

---

## Changelog

- 2026-01-31: Initial spec
