package knowledge

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/arvesolland/ralph/internal/log"
)

const referenceSection = `
## Operational Learnings

Automated development learnings are tracked in ` + "`.claude/learnings.md`" + ` — read it for project-specific gotchas discovered by Ralph.
`

const referenceMarker = "## Operational Learnings"

// EnsureReference checks if CLAUDE.md contains a reference to .claude/learnings.md.
// If not, it appends the Operational Learnings section. Idempotent — if the
// reference already exists, it does nothing. If CLAUDE.md does not exist, it
// logs a warning and returns nil.
func EnsureReference(repoPath string) error {
	claudeMD := filepath.Join(repoPath, "CLAUDE.md")

	content, err := os.ReadFile(claudeMD)
	if err != nil {
		if os.IsNotExist(err) {
			log.Warn("CLAUDE.md not found at %s — skipping learnings reference", claudeMD)
			return nil
		}
		return fmt.Errorf("read CLAUDE.md: %w", err)
	}

	if strings.Contains(string(content), referenceMarker) {
		return nil
	}

	s := string(content)
	if !strings.HasSuffix(s, "\n") {
		s += "\n"
	}
	s += referenceSection

	if err := os.WriteFile(claudeMD, []byte(s), 0o644); err != nil {
		return fmt.Errorf("write CLAUDE.md: %w", err)
	}

	return nil
}
