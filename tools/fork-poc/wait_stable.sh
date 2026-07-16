#!/usr/bin/env bash
# wait_stable.sh TRANSCRIPT_PATH [TIMEOUT_S]
#
# Blocks until TRANSCRIPT_PATH exists and its size has been unchanged for
# three consecutive 10s polls (the session has gone idle after its turn).
# Exits 0 with the final size on stdout, 1 on timeout.
set -euo pipefail
P="$1"; TIMEOUT="${2:-600}"
DEADLINE=$((SECONDS + TIMEOUT))
LAST=-1; STREAK=0
while [ "$SECONDS" -lt "$DEADLINE" ]; do
  SIZE=$(stat -c %s "$P" 2>/dev/null || echo -1)
  if [ "$SIZE" -gt 0 ] && [ "$SIZE" -eq "$LAST" ]; then
    STREAK=$((STREAK + 1))
    if [ "$STREAK" -ge 3 ]; then echo "$SIZE"; exit 0; fi
  else
    STREAK=0
  fi
  LAST="$SIZE"
  sleep 10
done
echo "timeout waiting for $P" >&2
exit 1
