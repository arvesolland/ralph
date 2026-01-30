#!/bin/bash
# Mock claude CLI that outputs successful stream-json response

# Read stdin (the prompt)
cat > /dev/null

# Output stream-json format
echo '{"type":"assistant","message":{"content":[{"type":"text","text":"Hello! I am completing the task."}]}}'
sleep 0.1
echo '{"type":"assistant","message":{"content":[{"type":"text","text":" <promise>COMPLETE</promise>"}]}}'
sleep 0.1
echo '{"type":"result","result":"completed"}'

exit 0
