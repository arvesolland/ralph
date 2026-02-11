// Package cli provides the command-line interface for ralph.
package cli

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/arvesolland/ralph/internal/config"
	"github.com/arvesolland/ralph/internal/log"
	"github.com/arvesolland/ralph/internal/prompt"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var (
	detectFlag bool
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize a new Ralph project",
	Long: `Initialize a new Ralph project in the current directory.

Creates the .ralph/ configuration directory and specs directory structure.
Optionally auto-detects project settings.`,
	RunE: runInit,
}

func init() {
	initCmd.Flags().BoolVar(&detectFlag, "detect", false, "auto-detect project settings")
	rootCmd.AddCommand(initCmd)
}

func runInit(cmd *cobra.Command, args []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}

	ralphDir := filepath.Join(cwd, ".ralph")
	configPath := filepath.Join(ralphDir, "config.yaml")

	// Check if config already exists — load it as the base so we don't clobber existing values
	reinit := fileExistsInit(configPath)

	// Create directory structure
	dirs := []string{
		ralphDir,
		filepath.Join(ralphDir, "worktrees"),
		filepath.Join(ralphDir, "prompts"),
		filepath.Join(cwd, "specs"),
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
		log.Debug("Created directory: %s", dir)
	}

	// Create .gitignore for worktrees if it doesn't exist
	worktreeGitignore := filepath.Join(ralphDir, "worktrees", ".gitignore")
	if !fileExistsInit(worktreeGitignore) {
		if err := os.WriteFile(worktreeGitignore, []byte("*\n!.gitignore\n"), 0644); err != nil {
			return fmt.Errorf("failed to create worktrees .gitignore: %w", err)
		}
		log.Debug("Created worktrees .gitignore")
	}

	// Copy default prompt to prompts directory if it doesn't exist
	defaultPromptPath := filepath.Join(ralphDir, "prompts", "prompt.md")
	if !fileExistsInit(defaultPromptPath) {
		promptContent, err := prompt.GetEmbeddedPrompt("prompt.md")
		if err != nil {
			log.Warn("Failed to get default prompt: %v", err)
		} else {
			if err := os.WriteFile(defaultPromptPath, []byte(promptContent), 0644); err != nil {
				return fmt.Errorf("failed to create default prompt: %w", err)
			}
			log.Debug("Created default prompt: %s", defaultPromptPath)
		}
	}

	// Add .ralph/ to root .gitignore
	rootGitignore := filepath.Join(cwd, ".gitignore")
	if err := appendToGitignore(rootGitignore, ".ralph/"); err != nil {
		log.Warn("Failed to update .gitignore: %v", err)
	} else {
		log.Debug("Added .ralph/ to .gitignore")
	}

	// Build config: load existing config as base to preserve values, fall back to defaults
	var cfg *config.Config
	if reinit {
		log.Info("Existing config found — preserving current values, filling in missing fields")
		existing, err := config.LoadWithDefaults(configPath)
		if err != nil {
			log.Warn("Failed to load existing config, starting fresh: %v", err)
			cfg = config.Defaults()
		} else {
			cfg = existing
		}
	} else {
		cfg = config.Defaults()
	}

	// Auto-detect if flag is set — only fill in empty command fields
	if detectFlag {
		log.Info("Auto-detecting project settings...")
		detected, err := config.Detect(cwd)
		if err != nil {
			log.Warn("Auto-detection failed: %v", err)
		} else if detected.Language != "" {
			log.Success("Detected %s project", detected.Language)
			if detected.Framework != "" {
				log.Info("  Framework: %s", detected.Framework)
			}

			// Only fill in commands that are currently empty
			if cfg.Commands.Test == "" && detected.Commands.Test != "" {
				cfg.Commands.Test = detected.Commands.Test
				log.Info("  Test command: %s", detected.Commands.Test)
			} else if detected.Commands.Test != "" {
				log.Info("  Test command: %s (kept existing)", cfg.Commands.Test)
			}
			if cfg.Commands.Lint == "" && detected.Commands.Lint != "" {
				cfg.Commands.Lint = detected.Commands.Lint
				log.Info("  Lint command: %s", detected.Commands.Lint)
			} else if detected.Commands.Lint != "" {
				log.Info("  Lint command: %s (kept existing)", cfg.Commands.Lint)
			}
			if cfg.Commands.Build == "" && detected.Commands.Build != "" {
				cfg.Commands.Build = detected.Commands.Build
				log.Info("  Build command: %s", detected.Commands.Build)
			} else if detected.Commands.Build != "" {
				log.Info("  Build command: %s (kept existing)", cfg.Commands.Build)
			}
			if cfg.Commands.Dev == "" && detected.Commands.Dev != "" {
				cfg.Commands.Dev = detected.Commands.Dev
				log.Info("  Dev command: %s", detected.Commands.Dev)
			} else if detected.Commands.Dev != "" {
				log.Info("  Dev command: %s (kept existing)", cfg.Commands.Dev)
			}
		} else {
			log.Info("No project type detected, using defaults")
		}
	}

	// Extract project name and description using AI — only if not already set
	if cfg.Project.Name == "" {
		if os.Getenv("ANTHROPIC_API_KEY") != "" {
			log.Info("Extracting project info...")
			projectInfo, err := extractProjectInfo(cwd)
			if err != nil {
				log.Debug("AI extraction failed: %v", err)
				cfg.Project.Name = filepath.Base(cwd)
				log.Info("  Project name: %s (from folder)", cfg.Project.Name)
			} else {
				if projectInfo.Name != "" {
					cfg.Project.Name = projectInfo.Name
					log.Info("  Project name: %s", cfg.Project.Name)
				} else {
					cfg.Project.Name = filepath.Base(cwd)
					log.Info("  Project name: %s (from folder)", cfg.Project.Name)
				}
				if cfg.Project.Description == "" && projectInfo.Description != "" {
					cfg.Project.Description = projectInfo.Description
					log.Info("  Description: %s", cfg.Project.Description)
				}
			}
		} else {
			cfg.Project.Name = filepath.Base(cwd)
			log.Info("Project name: %s (set ANTHROPIC_API_KEY for AI extraction)", cfg.Project.Name)
		}
	} else {
		log.Info("Project name: %s (kept existing)", cfg.Project.Name)
		if cfg.Project.Description != "" {
			log.Info("Description: %s (kept existing)", cfg.Project.Description)
		}
	}

	// Board configuration — only prompt for fields that are empty
	hasBoardConfig := cfg.Board.ProjectSlug != "" || cfg.Board.APIURL != "" || cfg.Board.APIToken != ""
	if hasBoardConfig {
		fmt.Println()
		log.Info("Board configuration (existing values preserved)")
		if cfg.Board.ProjectSlug != "" {
			log.Info("  Project slug: %s", cfg.Board.ProjectSlug)
		}
		if cfg.Board.APIURL != "" {
			log.Info("  API URL: %s", cfg.Board.APIURL)
		}
		if cfg.Board.APIToken != "" {
			log.Info("  API token: (set)")
		}
	}

	reader := bufio.NewReader(os.Stdin)

	if cfg.Board.ProjectSlug == "" {
		fmt.Println()
		log.Info("Board Task Management Configuration")
		log.Info("(Leave blank to skip - can be configured later in .ralph/config.yaml)")
		fmt.Println()

		fmt.Print("Board Project Slug: ")
		if slug, err := reader.ReadString('\n'); err == nil {
			slug = strings.TrimSpace(slug)
			if slug != "" {
				cfg.Board.ProjectSlug = slug
				log.Info("  Project slug: %s", slug)
			}
		}
	}

	if cfg.Board.APIURL == "" && !hasBoardConfig {
		fmt.Print("Board API URL (e.g., https://board.example.com/api): ")
		if apiURL, err := reader.ReadString('\n'); err == nil {
			apiURL = strings.TrimSpace(apiURL)
			if apiURL != "" {
				cfg.Board.APIURL = apiURL
				log.Info("  API URL: %s", apiURL)
			}
		}
	}

	if cfg.Board.APIToken == "" && !hasBoardConfig {
		fmt.Print("Board API Token: ")
		if token, err := reader.ReadString('\n'); err == nil {
			token = strings.TrimSpace(token)
			if token != "" {
				cfg.Board.APIToken = token
				log.Info("  API token: (set)")
			}
		}
	}

	// Write config file
	configData, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(configPath, configData, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}
	log.Success("Created config: %s", configPath)

	// Create specs INDEX.md if it doesn't exist
	indexPath := filepath.Join(cwd, "specs", "INDEX.md")
	if !fileExistsInit(indexPath) {
		if err := createSpecsIndex(indexPath); err != nil {
			return fmt.Errorf("failed to create specs INDEX.md: %w", err)
		}
		log.Success("Created specs index: %s", indexPath)
	}

	// Print summary
	fmt.Println()
	if reinit {
		log.Success("Ralph config updated (existing values preserved)")
	} else {
		log.Success("Ralph initialized successfully!")
	}
	fmt.Println()
	fmt.Println("Structure:")
	fmt.Println("  .ralph/")
	fmt.Println("    config.yaml      - Project configuration")
	fmt.Println("    prompts/         - Customizable agent prompts")
	fmt.Println("      prompt.md      - Main agent instructions (edit to customize)")
	fmt.Println("    worktrees/       - Execution worktrees (gitignored)")
	fmt.Println("  specs/")
	fmt.Println("    INDEX.md         - Specification index")
	fmt.Println()
	fmt.Println("Next steps:")
	fmt.Println("  1. Edit .ralph/config.yaml to customize settings")
	fmt.Println("  2. Configure Board: set board.project_slug in .ralph/config.yaml")
	fmt.Println("  3. Create plans via board-cli: board-cli plan create <project-slug> --title <name>")
	fmt.Println("  4. Run 'ralph worker' to start processing")

	return nil
}

// fileExistsInit checks if a file exists (local to avoid name collision with config package).
func fileExistsInit(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// appendToGitignore adds an entry to .gitignore if it doesn't already exist.
func appendToGitignore(path, entry string) error {
	// Read existing content if file exists
	var content string
	if data, err := os.ReadFile(path); err == nil {
		content = string(data)
		// Check if entry already exists
		for _, line := range strings.Split(content, "\n") {
			if strings.TrimSpace(line) == entry {
				return nil // Already present
			}
		}
	}

	// Append entry
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	// Add newline before entry if file doesn't end with one
	prefix := ""
	if content != "" && !strings.HasSuffix(content, "\n") {
		prefix = "\n"
	}

	_, err = f.WriteString(prefix + entry + "\n")
	return err
}

// ProjectInfo holds extracted project information.
type ProjectInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// extractProjectInfo uses Claude to extract project name and description.
// It reads CLAUDE.md, agents/agent.md, or falls back to directory listing.
func extractProjectInfo(cwd string) (*ProjectInfo, error) {
	// Build context for Claude
	var context string

	// Try CLAUDE.md first
	claudeMD := filepath.Join(cwd, "CLAUDE.md")
	if data, err := os.ReadFile(claudeMD); err == nil {
		context = fmt.Sprintf("# CLAUDE.md contents:\n\n%s", string(data))
	} else {
		// Try agents/agent.md
		agentMD := filepath.Join(cwd, "agents", "agent.md")
		if data, err := os.ReadFile(agentMD); err == nil {
			context = fmt.Sprintf("# agents/agent.md contents:\n\n%s", string(data))
		} else {
			// Fall back to directory listing
			cmd := exec.Command("ls", "-la")
			cmd.Dir = cwd
			output, err := cmd.Output()
			if err != nil {
				return nil, fmt.Errorf("failed to list directory: %w", err)
			}
			context = fmt.Sprintf("# Directory listing:\n\n%s", string(output))
		}
	}

	// Build the prompt
	prompt := fmt.Sprintf(`Analyze this project and extract the project name and a short description (1-2 sentences).

%s

Respond with ONLY valid JSON in this exact format, no other text:
{"name": "project-name", "description": "Short description of what this project does."}

If you cannot determine a clear name, use an empty string for name.
If you cannot determine a description, use an empty string for description.`, context)

	// Call Claude CLI
	cmd := exec.Command("claude",
		"--model", "claude-sonnet-4-5-latest",
		"--output-format", "text",
		"--max-turns", "1",
		"-p", prompt,
	)
	cmd.Dir = cwd

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("claude command failed: %w (stderr: %s)", err, stderr.String())
	}

	// Parse JSON response
	response := strings.TrimSpace(stdout.String())

	// Try to extract JSON from the response (Claude might add extra text)
	jsonStart := strings.Index(response, "{")
	jsonEnd := strings.LastIndex(response, "}")
	if jsonStart == -1 || jsonEnd == -1 || jsonEnd < jsonStart {
		return nil, fmt.Errorf("no valid JSON found in response: %s", response)
	}
	jsonStr := response[jsonStart : jsonEnd+1]

	var info ProjectInfo
	if err := json.Unmarshal([]byte(jsonStr), &info); err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %w (response: %s)", err, response)
	}

	return &info, nil
}

// createSpecsIndex creates a starter INDEX.md file for specs.
func createSpecsIndex(path string) error {
	content := `# Specifications Index

This directory contains feature specifications for the project.

## Format

Each specification should be in its own directory with a SPEC.md file:

` + "```" + `
specs/
  feature-name/
    SPEC.md          - Main specification document
    assets/          - Supporting diagrams, images, etc.
` + "```" + `

## Specifications

| Name | Status | Description |
|------|--------|-------------|
| *No specifications yet* | - | - |

## Creating a New Specification

1. Create a directory: ` + "`specs/your-feature/`" + `
2. Create the spec file: ` + "`specs/your-feature/SPEC.md`" + `
3. Add entry to this index table
4. Use the ralph-spec skill to manage specifications

## Specification Template

` + "```markdown" + `
# Feature: Your Feature Name

## Overview
Brief description of what this feature does.

## Requirements
- Requirement 1
- Requirement 2

## Technical Design
How it should be implemented.

## Acceptance Criteria
- [ ] Criterion 1
- [ ] Criterion 2
` + "```" + `
`
	return os.WriteFile(path, []byte(content), 0644)
}
