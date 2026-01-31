package config

// Defaults returns a new Config with all default values set.
func Defaults() *Config {
	return &Config{
		Project: ProjectConfig{
			Name:        "",
			Description: "",
		},
		Git: GitConfig{
			BaseBranch: "main",
		},
		Commands: CommandsConfig{
			Test:  "",
			Lint:  "",
			Build: "",
			Dev:   "",
		},
		Slack: SlackConfig{
			WebhookURL:      "",
			Channel:         "",
			BotToken:        "",
			AppToken:        "",
			GlobalBot:       false,
			NotifyStart:     true,
			NotifyComplete:  true,
			NotifyIteration: false,
			NotifyError:     true,
			NotifyBlocker:   true,
		},
		Worktree: WorktreeConfig{
			CopyEnvFiles: ".env",
			InitCommands: "",
		},
		Completion: CompletionConfig{
			Mode:              "pr",
			VerificationModel: "claude-3-5-haiku-latest",
		},
	}
}
