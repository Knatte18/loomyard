#!/usr/bin/env bash
# wait_stable.sh TRANSCRIPT_PATH [TIMEOUT_S] [MARKER]
#
# Blocks until TRANSCRIPT_PATH exists, contains at least one assistant
# message after MARKER (when given — guards against the false-stable window
# between the fork-copy flush and the first reply flush: launch + first
# thinking can exceed any fixed quiet window), and its size has been
# unchanged for three consecutive 10s polls. Exits 0 with the final size on
# stdout, 1 on timeout.
set -euo pipefail
P="$1"; TIMEOUT="${2:-600}"; MARKER="${3:-}"
HERE="$(dirname "$0")"
DEADLINE=$((SECONDS + TIMEOUT))
LAST=-1; STREAK=0
while [ "$SECONDS" -lt "$DEADLINE" ]; do
  SIZE=$(stat -c %s "$P" 2>/dev/null || echo -1)
  if [ "$SIZE" -gt 0 ] && [ "$SIZE" -eq "$LAST" ]; then
    STREAK=$((STREAK + 1))
    if [ "$STREAK" -ge 3 ]; then
      if [ -z "$MARKER" ] || [ "$(python3 "$HERE/harvest.py" "$P" "$MARKER" | head -1 | python3 -c 'import json,sys; print(json.load(sys.stdin)["turns"])')" -gt 0 ]; then
        echo "$SIZE"; exit 0
      fi
      STREAK=0
    fi
  else
    STREAK=0
  fi
  LAST="$SIZE"
  sleep 10
done
echo "timeout waiting for $P" >&2
exit 1
