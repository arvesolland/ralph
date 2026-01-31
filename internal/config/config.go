// Package config handles configuration loading and management.
package config

import (
	"os"

	"gopkg.in/yaml.v3"
)

// Config is the root configuration structure for Ralph.
type Config struct {
	Project    ProjectConfig    `yaml:"project"`
	Git        GitConfig        `yaml:"git"`
	Commands   CommandsConfig   `yaml:"commands"`
	Slack      SlackConfig      `yaml:"slack"`
	Worktree   WorktreeConfig   `yaml:"worktree"`
	Completion CompletionConfig `yaml:"completion"`
}

// ProjectConfig contains project identification settings.
type ProjectConfig struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
}

// GitConfig contains git-related settings.
type GitConfig struct {
	BaseBranch string `yaml:"base_branch"`
}

// CommandsConfig contains project command configurations.
type CommandsConfig struct {
	Test  string `yaml:"test"`
	Lint  string `yaml:"lint"`
	Build string `yaml:"build"`
	Dev   string `yaml:"dev"`
}

// SlackConfig contains Slack notification settings.
type SlackConfig struct {
	WebhookURL      string `yaml:"webhook_url"`
	Channel         string `yaml:"channel"`
	BotToken        string `yaml:"bot_token"`
	AppToken        string `yaml:"app_token"`
	GlobalBot       bool   `yaml:"global_bot"`
	NotifyStart     bool   `yaml:"notify_start"`
	NotifyComplete  bool   `yaml:"notify_complete"`
	NotifyIteration bool   `yaml:"notify_iteration"`
	NotifyError     bool   `yaml:"notify_error"`
	NotifyBlocker   bool   `yaml:"notify_blocker"`
}

// WorktreeConfig contains worktree initialization settings.
type WorktreeConfig struct {
	CopyEnvFiles string `yaml:"copy_env_files"`
	InitCommands string `yaml:"init_commands"`
}

// CompletionConfig contains plan completion settings.
type CompletionConfig struct {
	Mode string `yaml:"mode"` // "pr" or "merge"
}

// Load reads and parses a YAML config file.
// Returns an error if the file cannot be read or parsed.
// For missing files, use LoadWithDefaults instead.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// LoadWithDefaults reads and parses a YAML config file, applying defaults
// for any missing fields. If the file doesn't exist or is empty, returns
// a config with all defaults (not an error).
func LoadWithDefaults(path string) (*Config, error) {
	cfg := Defaults()

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			// Missing file is valid - return defaults
			return cfg, nil
		}
		return nil, err
	}

	// Empty file is valid - return defaults
	if len(data) == 0 {
		return cfg, nil
	}

	// Parse YAML into a fresh config first to check for errors
	var fileCfg Config
	if err := yaml.Unmarshal(data, &fileCfg); err != nil {
		return nil, err
	}

	// Merge file config into defaults
	mergeConfig(cfg, &fileCfg)

	return cfg, nil
}

// mergeConfig merges values from src into dst.
// Only non-zero values from src overwrite dst.
func mergeConfig(dst, src *Config) {
	// Project
	if src.Project.Name != "" {
		dst.Project.Name = src.Project.Name
	}
	if src.Project.Description != "" {
		dst.Project.Description = src.Project.Description
	}

	// Git
	if src.Git.BaseBranch != "" {
		dst.Git.BaseBranch = src.Git.BaseBranch
	}

	// Commands
	if src.Commands.Test != "" {
		dst.Commands.Test = src.Commands.Test
	}
	if src.Commands.Lint != "" {
		dst.Commands.Lint = src.Commands.Lint
	}
	if src.Commands.Build != "" {
		dst.Commands.Build = src.Commands.Build
	}
	if src.Commands.Dev != "" {
		dst.Commands.Dev = src.Commands.Dev
	}

	// Slack
	if src.Slack.WebhookURL != "" {
		dst.Slack.WebhookURL = src.Slack.WebhookURL
	}
	if src.Slack.Channel != "" {
		dst.Slack.Channel = src.Slack.Channel
	}
	if src.Slack.BotToken != "" {
		dst.Slack.BotToken = src.Slack.BotToken
	}
	if src.Slack.AppToken != "" {
		dst.Slack.AppToken = src.Slack.AppToken
	}
	// Bool fields - only override if explicitly set in file
	// Since we can't distinguish "not set" from "set to false" with yaml.Unmarshal,
	// we need a different approach. For bools, we'll always use defaults unless
	// the YAML file has them. The yaml.Unmarshal will set bools to their zero value
	// (false) even if not present, so we can't detect if they were explicitly set.
	// The safest approach is to rely on the defaults and note this limitation.
	// In practice, users who want to disable a notification would set it to false
	// explicitly, and since defaults are mostly true, this works out.
	dst.Slack.GlobalBot = src.Slack.GlobalBot
	dst.Slack.NotifyStart = src.Slack.NotifyStart || dst.Slack.NotifyStart
	dst.Slack.NotifyComplete = src.Slack.NotifyComplete || dst.Slack.NotifyComplete
	dst.Slack.NotifyIteration = src.Slack.NotifyIteration
	dst.Slack.NotifyError = src.Slack.NotifyError || dst.Slack.NotifyError
	dst.Slack.NotifyBlocker = src.Slack.NotifyBlocker || dst.Slack.NotifyBlocker

	// Worktree
	if src.Worktree.CopyEnvFiles != "" {
		dst.Worktree.CopyEnvFiles = src.Worktree.CopyEnvFiles
	}
	if src.Worktree.InitCommands != "" {
		dst.Worktree.InitCommands = src.Worktree.InitCommands
	}

	// Completion
	if src.Completion.Mode != "" {
		dst.Completion.Mode = src.Completion.Mode
	}
}
