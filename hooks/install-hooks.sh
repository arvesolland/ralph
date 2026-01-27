#!/bin/bash
# Install Ralph git hooks
#
# Usage:
#   ./hooks/install-hooks.sh

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT=$(git rev-parse --show-toplevel 2>/dev/null)

if [ -z "$REPO_ROOT" ]; then
  echo "Error: Not in a git repository"
  exit 1
fi

GIT_HOOKS_DIR="$REPO_ROOT/.git/hooks"

echo "Installing Ralph git hooks..."

# Install commit-msg hook
if [ -f "$GIT_HOOKS_DIR/commit-msg" ]; then
  echo "  Backing up existing commit-msg hook..."
  mv "$GIT_HOOKS_DIR/commit-msg" "$GIT_HOOKS_DIR/commit-msg.backup"
fi

cp "$SCRIPT_DIR/commit-msg" "$GIT_HOOKS_DIR/commit-msg"
chmod +x "$GIT_HOOKS_DIR/commit-msg"
echo "  Installed: commit-msg"

echo ""
echo "Done! Hooks installed to $GIT_HOOKS_DIR"
echo ""
echo "The commit-msg hook will automatically update CHANGELOG.md"
echo "when you make commits using conventional commit format:"
echo "  feat: Add new feature      → Added section"
echo "  fix: Fix bug               → Fixed section"
echo "  docs: Update docs          → Documentation section"
echo "  feat!: Breaking change     → Breaking Changes section"
