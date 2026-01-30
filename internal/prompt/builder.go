package prompt

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/arvesolland/ralph/internal/config"
)

// Builder builds prompts by substituting placeholders in templates.
type Builder struct {
	// config is the loaded configuration
	config *config.Config

	// configDir is the path to the .ralph directory
	configDir string

	// promptsDir is the path to the prompts/base directory (for external prompts)
	promptsDir string
}

// NewBuilder creates a new prompt builder.
func NewBuilder(cfg *config.Config, configDir string, promptsDir string) *Builder {
	return &Builder{
		config:     cfg,
		configDir:  configDir,
		promptsDir: promptsDir,
	}
}

// placeholderRegex matches {{PLACEHOLDER}} syntax
var placeholderRegex = regexp.MustCompile(`\{\{([A-Z_]+)\}\}`)

// Build loads a template and substitutes all placeholders.
// templatePath can be:
//   - An absolute path to a template file
//   - A relative path from the prompts directory
//   - A template name (e.g., "prompt.md")
//
// overrides allows providing additional placeholder values that take precedence over config.
func (b *Builder) Build(templatePath string, overrides map[string]string) (string, error) {
	// Load the template content
	content, err := b.loadTemplate(templatePath)
	if err != nil {
		return "", err
	}

	// Build the substitution map
	subs := b.buildSubstitutions(overrides)

	// Replace all placeholders
	result := placeholderRegex.ReplaceAllStringFunc(content, func(match string) string {
		// Extract the placeholder name (without the {{ }})
		name := match[2 : len(match)-2]

		// Check for override first
		if val, ok := subs[name]; ok {
			return val
		}

		// Unknown placeholder - leave as-is for forward compatibility
		return match
	})

	return result, nil
}

// loadTemplate loads a template from the filesystem or embedded prompts.
func (b *Builder) loadTemplate(templatePath string) (string, error) {
	// Try as absolute path first
	if filepath.IsAbs(templatePath) {
		content, err := os.ReadFile(templatePath)
		if err == nil {
			return string(content), nil
		}
		// If absolute path fails, fall through to embedded prompts
	}

	// Try from external prompts directory
	if b.promptsDir != "" {
		externalPath := filepath.Join(b.promptsDir, templatePath)
		content, err := os.ReadFile(externalPath)
		if err == nil {
			return string(content), nil
		}
	}

	// Fall back to embedded prompts
	return loadEmbeddedPrompt(templatePath)
}

// buildSubstitutions creates a map of all placeholder substitutions.
func (b *Builder) buildSubstitutions(overrides map[string]string) map[string]string {
	subs := make(map[string]string)

	// Config-based substitutions
	if b.config != nil {
		subs["PROJECT_NAME"] = b.config.Project.Name
		subs["PROJECT_DESCRIPTION"] = b.config.Project.Description
		subs["TEST_COMMAND"] = b.config.Commands.Test
		subs["LINT_COMMAND"] = b.config.Commands.Lint
		subs["BUILD_COMMAND"] = b.config.Commands.Build
		subs["DEV_COMMAND"] = b.config.Commands.Dev
	}

	// Load .ralph/*.md override files
	if b.configDir != "" {
		subs["PRINCIPLES"] = b.loadOverrideFile("principles.md")
		subs["PATTERNS"] = b.loadOverrideFile("patterns.md")
		subs["BOUNDARIES"] = b.loadOverrideFile("boundaries.md")
		subs["TECH_STACK"] = b.loadOverrideFile("tech-stack.md")
	}

	// Apply explicit overrides (highest precedence)
	for k, v := range overrides {
		subs[k] = v
	}

	return subs
}

// loadOverrideFile loads a file from the configDir, returning empty string if not found.
func (b *Builder) loadOverrideFile(filename string) string {
	path := filepath.Join(b.configDir, filename)
	content, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(content))
}
