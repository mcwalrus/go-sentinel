#!/usr/bin/env bash
set -e

# Choo Choo Ralph - Autonomous coding loop
# Usage: ./ralph.sh [max_iterations] [--verbose|-v]

# Default iteration limit. One iteration = one task attempt.
# For testing, start smaller: ./ralph.sh 10
MAX_ITERATIONS=100
VERBOSE_FLAG=""

# Parse arguments
for arg in "$@"; do
  case "$arg" in
  --verbose | -v)
    VERBOSE_FLAG="--verbose"
    ;;
  [0-9]*)
    MAX_ITERATIONS="$arg"
    ;;
  esac
done
iteration=0

echo "Starting Ralph loop (max $MAX_ITERATIONS iterations)"

while [ $iteration -lt $MAX_ITERATIONS ]; do
  echo ""
  echo "=== Iteration $((iteration + 1)) ==="
  echo "---"

  # Check if any ready work is available (no blockers, not in_progress by another agent)
  available=$(bd ready --assignee=ralph -n 100 --json 2>/dev/null | jq -r 'length')

  if [ "$available" -eq 0 ]; then
    echo "No ready work available. Done."
    exit 0
  fi

  echo "$available ready task(s) available"
  echo ""

  # Let Claude see available work, pick one, claim it, and execute
  claude --dangerously-skip-permissions --output-format stream-json --verbose -p "
Run \`bd ready --assignee=ralph -n 100 --sort=priority\` to see available tasks.

Also run \`bd list --status=in_progress --assignee=ralph\` to see what tasks other Ralph agents are currently working on.

Decide which task to work on next. Selection criteria:
1. Priority - higher priority tasks are more important
2. Avoid conflicts - if other Ralph agents have tasks in_progress, you MUST pick a completely different epic. Do NOT work on any task that is a child, parent, or sibling of an in-progress task. Stay away from the entire epic tree that another Ralph is working on.
3. If all high-priority epics are being worked on by other Ralphs, pick a lower-priority epic that is completely unrelated

Pick ONE task, claim it with \`bd update <id> --status in_progress\`, then execute it according to its description.

One iteration = complete the task AND all its child tasks (if any).

IMPORTANT: After the task and all children are done (or if blocked), EXIT immediately. Do NOT pick up another top-level task. The outer loop will handle the next iteration.
" 2>&1 | "$(dirname "$0")/ralph-format.sh" $VERBOSE_FLAG || true

  ((iteration++)) || true
done

echo ""
echo "Reached max iterations ($MAX_ITERATIONS)"
