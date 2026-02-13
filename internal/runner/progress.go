package runner

import (
	"regexp"
	"strings"
)

// progressTagRegex matches <progress>...</progress> content.
var progressTagRegex = regexp.MustCompile(`(?s)<progress>(.*?)</progress>`)

// ExtractProgress extracts the progress summary from Claude output.
// Returns empty string if no progress tag is found.
func ExtractProgress(output string) string {
	matches := progressTagRegex.FindStringSubmatch(output)
	if len(matches) < 2 {
		return ""
	}
	return strings.TrimSpace(matches[1])
}
