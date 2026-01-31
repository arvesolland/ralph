#!/bin/bash
# Mock claude CLI that exits with an error

# Read stdin (the prompt)
cat > /dev/null

echo "Error: Rate limit exceeded" >&2
exit 1
