# Feature: Config & Prompt

**ID:** F1.1
**Status:** planned
**Requires:** —

## Summary

Configuration loading from YAML files and prompt template building with placeholder injection. This replaces the fragile ~100-line bash YAML parser with Go's native yaml.Unmarshal and provides robust template processing.

## Goals

- Parse .ralph/config.yaml with full YAML spec compliance
- Handle inline comments correctly (the source of bash bugs)
- Support nested key access (e.g., `project.name`)
- Load override files from .ralph/*.md (principles, patterns, boundaries, tech-stack)
- Build prompts with `{{PLACEHOLDER}}` substitution
- Auto-detect project settings (language, framework, package manager)
- Provide sensible defaults for all optional config

## Non-Goals

- Support TOML or other config formats
- Hot-reload config changes during execution
- Config validation beyond type checking

## Design

### Config Structure

```go
type Config struct {
    Project    ProjectConfig    `yaml:"project"`
    Git        GitConfig        `yaml:"git"`
    Commands   CommandsConfig   `yaml:"commands"`
    Slack      SlackConfig      `yaml:"slack"`
    Worktree   WorktreeConfig   `yaml:"worktree"`
    Completion CompletionConfig `yaml:"completion"`
}

type ProjectConfig struct {
    Name        string `yaml:"name"`
    Description string `yaml:"description"`
}

type GitConfig struct {
    BaseBranch string `yaml:"base_branch"`
}

type CommandsConfig struct {
    Test  string `yaml:"test"`
    Lint  string `yaml:"lint"`
    Build string `yaml:"build"`
    Dev   string `yaml:"dev"`
}

type SlackConfig struct {
    WebhookURL     string `yaml:"webhook_url"`
    Channel        string `yaml:"channel"`
    BotToken       string `yaml:"bot_token"`
    AppToken       string `yaml:"app_token"`
    GlobalBot      bool   `yaml:"global_bot"`
    NotifyStart    bool   `yaml:"notify_start"`
    NotifyComplete bool   `yaml:"notify_complete"`
    NotifyIteration bool  `yaml:"notify_iteration"`
    NotifyError    bool   `yaml:"notify_error"`
    NotifyBlocker  bool   `yaml:"notify_blocker"`
}

type WorktreeConfig struct {
    CopyEnvFiles string `yaml:"copy_env_files"`
    InitCommands string `yaml:"init_commands"`
}

type CompletionConfig struct {
    Mode string `yaml:"mode"` // "pr" or "merge"
}
```

### Prompt Building

Templates use `{{PLACEHOLDER}}` syntax. Placeholders:

| Placeholder | Source |
|-------------|--------|
| `{{PROJECT_NAME}}` | config.project.name |
| `{{PROJECT_DESCRIPTION}}` | config.project.description |
| `{{PRINCIPLES}}` | .ralph/principles.md content |
| `{{PATTERNS}}` | .ralph/patterns.md content |
| `{{BOUNDARIES}}` | .ralph/boundaries.md content |
| `{{TECH_STACK}}` | .ralph/tech-stack.md content |
| `{{TEST_COMMAND}}` | config.commands.test |
| `{{LINT_COMMAND}}` | config.commands.lint |
| `{{BUILD_COMMAND}}` | config.commands.build |

### Project Detection

Auto-detect from files present:
- package.json → Node.js, extract scripts
- composer.json → PHP
- pyproject.toml / requirements.txt → Python
- go.mod → Go
- Cargo.toml → Rust
- Gemfile → Ruby

### Key Files

| File | Purpose |
|------|---------|
| `internal/config/config.go` | Config struct and loading |
| `internal/config/defaults.go` | Default values |
| `internal/config/detect.go` | Project auto-detection |
| `internal/prompt/builder.go` | Template loading and placeholder substitution |
| `internal/prompt/templates.go` | Embedded default prompts |

## Gotchas

- YAML inline comments after quoted strings: `name: "value" # comment` - must not include comment in value
- Empty config file should not error, just use defaults
- Missing .ralph/ directory is valid (use all defaults)
- Prompt files may not exist - use empty string for missing placeholders

---

## Changelog

- 2026-01-31: Initial spec
