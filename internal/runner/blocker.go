package runner

import (
	"crypto/md5"
	"encoding/hex"
	"regexp"
	"strings"
)

// blockerTagRegex matches <blocker>...</blocker> content.
var blockerTagRegex = regexp.MustCompile(`(?s)<blocker>(.*?)</blocker>`)

// fieldRegexes for parsing structured fields within blocker content.
var (
	descriptionRegex = regexp.MustCompile(`(?im)^(?:Description:\s*)?(.+?)(?:\n(?:Action:|Resume:)|$)`)
	actionRegex      = regexp.MustCompile(`(?im)^Action:\s*(.+?)(?:\nResume:|$)`)
	resumeRegex      = regexp.MustCompile(`(?im)^Resume:\s*(.+)$`)
)

// ExtractBlocker extracts blocker information from Claude output.
// Returns nil if no blocker tag is found.
func ExtractBlocker(output string) *Blocker {
	matches := blockerTagRegex.FindStringSubmatch(output)
	if matches == nil || len(matches) < 2 {
		return nil
	}

	content := strings.TrimSpace(matches[1])
	if content == "" {
		return nil
	}

	blocker := &Blocker{
		Content: content,
		Hash:    computeBlockerHash(content),
	}

	// Parse structured fields
	blocker.Description, blocker.Action, blocker.Resume = parseBlockerFields(content)

	return blocker
}

// parseBlockerFields extracts Description, Action, and Resume fields from content.
// If the content doesn't have explicit fields, the entire content is used as Description.
func parseBlockerFields(content string) (description, action, resume string) {
	// Try to extract Action field
	if actionMatch := actionRegex.FindStringSubmatch(content); actionMatch != nil {
		action = strings.TrimSpace(actionMatch[1])
	}

	// Try to extract Resume field
	if resumeMatch := resumeRegex.FindStringSubmatch(content); resumeMatch != nil {
		resume = strings.TrimSpace(resumeMatch[1])
	}

	// For description, we need to be careful:
	// If Action: is present, description is everything before it
	// If no Action:, check for Description: prefix
	lines := strings.Split(content, "\n")
	var descLines []string
	inDescription := true

	for _, line := range lines {
		lineLower := strings.ToLower(strings.TrimSpace(line))
		if strings.HasPrefix(lineLower, "action:") {
			inDescription = false
			continue
		}
		if strings.HasPrefix(lineLower, "resume:") {
			inDescription = false
			continue
		}
		if inDescription {
			// Remove "Description:" prefix if present
			trimmedLine := line
			if strings.HasPrefix(lineLower, "description:") {
				trimmedLine = strings.TrimSpace(line[len("description:"):])
			}
			if trimmedLine != "" || len(descLines) > 0 {
				descLines = append(descLines, trimmedLine)
			}
		}
	}

	description = strings.TrimSpace(strings.Join(descLines, "\n"))

	// If no structured description was found but content exists, use the whole content
	if description == "" && action == "" && resume == "" {
		description = content
	}

	return description, action, resume
}

// computeBlockerHash returns the first 8 characters of the MD5 hash of the content.
// This is used for deduplication to avoid Slack notification spam.
func computeBlockerHash(content string) string {
	hash := md5.Sum([]byte(content))
	return hex.EncodeToString(hash[:])[:8]
}

// HasBlocker returns true if the output contains a blocker tag.
func HasBlocker(output string) bool {
	return blockerTagRegex.MatchString(output)
}
