package prompt

import (
	"embed"
	"fmt"
	"path/filepath"
)

//go:embed prompts/*.md
var embeddedPrompts embed.FS

// loadEmbeddedPrompt loads a prompt from the embedded filesystem.
// templatePath can be a filename like "prompt.md" or a path like "prompts/base/prompt.md".
func loadEmbeddedPrompt(templatePath string) (string, error) {
	// Try direct path under prompts/
	name := filepath.Base(templatePath)
	content, err := embeddedPrompts.ReadFile("prompts/" + name)
	if err == nil {
		return string(content), nil
	}

	return "", fmt.Errorf("template not found: %s", templatePath)
}

// ListEmbeddedPrompts returns a list of all embedded prompt files.
func ListEmbeddedPrompts() ([]string, error) {
	entries, err := embeddedPrompts.ReadDir("prompts")
	if err != nil {
		return nil, err
	}

	var names []string
	for _, entry := range entries {
		if !entry.IsDir() {
			names = append(names, entry.Name())
		}
	}
	return names, nil
}
