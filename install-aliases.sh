#!/bin/bash
# Install Ralph aliases to your shell config

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ALIASES_FILE="$SCRIPT_DIR/aliases.sh"

# Detect shell config file
if [ -n "$ZSH_VERSION" ] || [ "$SHELL" = "/bin/zsh" ]; then
  SHELL_RC="$HOME/.zshrc"
elif [ -n "$BASH_VERSION" ] || [ "$SHELL" = "/bin/bash" ]; then
  SHELL_RC="$HOME/.bashrc"
else
  SHELL_RC="$HOME/.profile"
fi

SOURCE_LINE="source \"$ALIASES_FILE\""

# Check if already added
if grep -qF "$ALIASES_FILE" "$SHELL_RC" 2>/dev/null; then
  echo "Ralph aliases already installed in $SHELL_RC"
else
  echo "" >> "$SHELL_RC"
  echo "# Ralph aliases" >> "$SHELL_RC"
  echo "$SOURCE_LINE" >> "$SHELL_RC"
  echo "Added to $SHELL_RC:"
  echo "  $SOURCE_LINE"
fi

echo ""
echo "Aliases available after restart or running:"
echo "  source $SHELL_RC"
echo ""
echo "Commands:"
echo "  ralph-install      Install Ralph in a project"
echo "  ralph-install-ai   Install with AI config"
echo "  ralph-update       Update Ralph scripts"
echo "  ralph              Run Ralph on a plan file"
echo "  ralph-init         Initialize/reconfigure Ralph"
