// Package cli provides the command-line interface for ralph.
package cli

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/arvesolland/ralph/internal/config"
	"github.com/arvesolland/ralph/internal/log"
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

Creates the .ralph/ configuration directory, plan queue directories,
and specs directory structure. Optionally auto-detects project settings.`,
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

	// Check if config already exists
	if fileExistsInit(configPath) {
		if !confirmOverwrite(configPath) {
			log.Info("Initialization cancelled")
			return nil
		}
	}

	// Create directory structure
	dirs := []string{
		ralphDir,
		filepath.Join(ralphDir, "worktrees"),
		filepath.Join(cwd, "plans", "pending"),
		filepath.Join(cwd, "plans", "current"),
		filepath.Join(cwd, "plans", "complete"),
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

	// Build config
	cfg := config.Defaults()

	// Auto-detect if flag is set
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

			// Merge detected settings into config
			if detected.Commands.Test != "" {
				cfg.Commands.Test = detected.Commands.Test
				log.Info("  Test command: %s", detected.Commands.Test)
			}
			if detected.Commands.Lint != "" {
				cfg.Commands.Lint = detected.Commands.Lint
				log.Info("  Lint command: %s", detected.Commands.Lint)
			}
			if detected.Commands.Build != "" {
				cfg.Commands.Build = detected.Commands.Build
				log.Info("  Build command: %s", detected.Commands.Build)
			}
			if detected.Commands.Dev != "" {
				cfg.Commands.Dev = detected.Commands.Dev
				log.Info("  Dev command: %s", detected.Commands.Dev)
			}
		} else {
			log.Info("No project type detected, using defaults")
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
	log.Success("Ralph initialized successfully!")
	fmt.Println()
	fmt.Println("Created structure:")
	fmt.Println("  .ralph/")
	fmt.Println("    config.yaml      - Project configuration")
	fmt.Println("    worktrees/       - Execution worktrees (gitignored)")
	fmt.Println("  plans/")
	fmt.Println("    pending/         - Plans waiting to be executed")
	fmt.Println("    current/         - Currently executing plan")
	fmt.Println("    complete/        - Completed plans")
	fmt.Println("  specs/")
	fmt.Println("    INDEX.md         - Specification index")
	fmt.Println()
	fmt.Println("Next steps:")
	fmt.Println("  1. Edit .ralph/config.yaml to customize settings")
	fmt.Println("  2. Create a plan: ralph plan create <name>")
	fmt.Println("  3. Run 'ralph worker' to start processing")

	return nil
}

// fileExistsInit checks if a file exists (local to avoid name collision with config package).
func fileExistsInit(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// confirmOverwrite asks the user to confirm overwriting an existing file.
func confirmOverwrite(path string) bool {
	fmt.Printf("Config file already exists: %s\n", path)
	fmt.Print("Overwrite? [y/N]: ")

	reader := bufio.NewReader(os.Stdin)
	input, err := reader.ReadString('\n')
	if err != nil {
		return false
	}

	input = strings.TrimSpace(strings.ToLower(input))
	return input == "y" || input == "yes"
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
