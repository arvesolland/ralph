# Ralph shell aliases
# Add to your shell config with:
#   echo 'source /path/to/ralph/aliases.sh' >> ~/.zshrc
# Or run: ./install-aliases.sh

# Install Ralph in a project
alias ralph-install='curl -fsSL https://raw.githubusercontent.com/arvesolland/ralph/main/install.sh | bash'
alias ralph-install-ai='curl -fsSL https://raw.githubusercontent.com/arvesolland/ralph/main/install.sh | bash -s -- --ai'

# Update Ralph scripts (run from project root)
alias ralph-update='./scripts/ralph/ralph-update.sh'

# Run Ralph (run from project root)
alias ralph='./scripts/ralph/ralph.sh'
alias ralph-init='./scripts/ralph/ralph-init.sh'
