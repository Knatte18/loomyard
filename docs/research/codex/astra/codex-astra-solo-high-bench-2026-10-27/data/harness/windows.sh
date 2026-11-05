#!/usr/bin/env bash
# windows.sh <rollout-file>: print the 5-hour and weekly used_percent of the rollout's last token_count event, space-separated.
# Prints nothing when the rollout has no token_count event, as for a run the plan refused.
grep '"token_count"' "$1" | tail -1 | grep -o '"primary":{[^}]*}\|"secondary":{[^}]*}' \
  | grep -o '"used_percent":[0-9.]*' | cut -d: -f2 | tr '\n' ' ' | sed 's/ $//'
echo
