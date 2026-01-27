#!/bin/bash
set -e

# Ralph Release Script
# Creates a new version release by:
# 1. Determining version bump type from changelog
# 2. Bumping VERSION file
# 3. Moving [Unreleased] to versioned section in CHANGELOG.md
# 4. Committing and tagging
#
# Usage:
#   ./ralph-release.sh              # Auto-detect bump type from changelog
#   ./ralph-release.sh patch        # Force patch bump (0.0.X)
#   ./ralph-release.sh minor        # Force minor bump (0.X.0)
#   ./ralph-release.sh major        # Force major bump (X.0.0)

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
VERSION_FILE="$SCRIPT_DIR/VERSION"
CHANGELOG="$SCRIPT_DIR/CHANGELOG.md"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

# Parse arguments
BUMP_TYPE="${1:-auto}"

# Validate bump type
case "$BUMP_TYPE" in
  auto|patch|minor|major)
    ;;
  --help|-h)
    echo "Ralph Release - Version management"
    echo ""
    echo "Usage:"
    echo "  ./ralph-release.sh              Auto-detect bump type"
    echo "  ./ralph-release.sh patch        Bump patch version (0.0.X)"
    echo "  ./ralph-release.sh minor        Bump minor version (0.X.0)"
    echo "  ./ralph-release.sh major        Bump major version (X.0.0)"
    echo ""
    echo "Version detection:"
    echo "  - Breaking Changes section → major"
    echo "  - Added section → minor"
    echo "  - Fixed/Changed/Other → patch"
    exit 0
    ;;
  *)
    echo -e "${RED}Invalid bump type: $BUMP_TYPE${NC}"
    echo "Use: patch, minor, major, or auto"
    exit 1
    ;;
esac

# Check for uncommitted changes
if ! git diff --quiet || ! git diff --cached --quiet; then
  echo -e "${RED}Error: You have uncommitted changes.${NC}"
  echo "Please commit or stash them before releasing."
  exit 1
fi

# Read current version
if [ ! -f "$VERSION_FILE" ]; then
  echo -e "${RED}Error: VERSION file not found${NC}"
  exit 1
fi

CURRENT_VERSION=$(cat "$VERSION_FILE" | tr -d '\n')

# Parse version components
IFS='.' read -r MAJOR MINOR PATCH <<< "$CURRENT_VERSION"

echo -e "${GREEN}========================================"
echo -e "Ralph Release"
echo -e "========================================${NC}"
echo ""
echo "Current version: $CURRENT_VERSION"
echo ""

# Check if there are unreleased changes
if ! grep -q "^## \[Unreleased\]" "$CHANGELOG"; then
  echo -e "${RED}Error: No [Unreleased] section in CHANGELOG.md${NC}"
  exit 1
fi

# Extract unreleased section content
UNRELEASED_CONTENT=$(awk '/^## \[Unreleased\]/{flag=1; next} /^## \[/{flag=0} flag' "$CHANGELOG")

if [ -z "$(echo "$UNRELEASED_CONTENT" | grep -E "^### ")" ]; then
  echo -e "${YELLOW}Warning: No changes in [Unreleased] section${NC}"
  echo "Nothing to release."
  exit 0
fi

# Auto-detect bump type from changelog content
if [ "$BUMP_TYPE" = "auto" ]; then
  if echo "$UNRELEASED_CONTENT" | grep -q "^### Breaking Changes"; then
    BUMP_TYPE="major"
  elif echo "$UNRELEASED_CONTENT" | grep -q "^### Added"; then
    BUMP_TYPE="minor"
  else
    BUMP_TYPE="patch"
  fi
  echo "Auto-detected bump type: $BUMP_TYPE"
fi

# Calculate new version
case "$BUMP_TYPE" in
  major)
    MAJOR=$((MAJOR + 1))
    MINOR=0
    PATCH=0
    ;;
  minor)
    MINOR=$((MINOR + 1))
    PATCH=0
    ;;
  patch)
    PATCH=$((PATCH + 1))
    ;;
esac

NEW_VERSION="$MAJOR.$MINOR.$PATCH"
TODAY=$(date +%Y-%m-%d)

echo "New version: $NEW_VERSION"
echo ""

# Confirm
echo -e "${YELLOW}This will:${NC}"
echo "  1. Update VERSION to $NEW_VERSION"
echo "  2. Move [Unreleased] to [$NEW_VERSION] - $TODAY in CHANGELOG.md"
echo "  3. Commit with message: Release v$NEW_VERSION"
echo "  4. Create git tag: v$NEW_VERSION"
echo ""
read -p "Continue? (y/N) " -n 1 -r
echo ""

if [[ ! $REPLY =~ ^[Yy]$ ]]; then
  echo "Aborted."
  exit 1
fi

# Update VERSION file
echo "$NEW_VERSION" > "$VERSION_FILE"
echo -e "${GREEN}Updated VERSION to $NEW_VERSION${NC}"

# Update CHANGELOG.md
TEMP_CHANGELOG=$(mktemp)

awk -v version="$NEW_VERSION" -v date="$TODAY" '
  /^## \[Unreleased\]/ {
    print "## [Unreleased]"
    print ""
    print "## [" version "] - " date
    next
  }
  { print }
' "$CHANGELOG" > "$TEMP_CHANGELOG"

mv "$TEMP_CHANGELOG" "$CHANGELOG"
echo -e "${GREEN}Updated CHANGELOG.md${NC}"

# Commit and tag
git add "$VERSION_FILE" "$CHANGELOG"
git commit -m "[skip changelog] Release v$NEW_VERSION"
git tag -a "v$NEW_VERSION" -m "Release v$NEW_VERSION"

echo ""
echo -e "${GREEN}========================================"
echo -e "Release v$NEW_VERSION complete!"
echo -e "========================================${NC}"
echo ""
echo "To push the release:"
echo "  git push && git push --tags"
echo ""
echo "To undo (if not pushed):"
echo "  git reset --hard HEAD~1 && git tag -d v$NEW_VERSION"
