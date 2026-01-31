#!/bin/bash
# Mock claude CLI that simulates a hanging/slow execution

# Read stdin (the prompt)
cat > /dev/null

# Output some initial content
echo '{"type":"assistant","message":{"content":[{"type":"text","text":"Starting work..."}]}}'

# Sleep for a long time to simulate hanging
sleep 60

echo '{"type":"result","result":"completed"}'

exit 0
