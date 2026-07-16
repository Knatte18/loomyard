#!/usr/bin/env bash
# instafork.sh N — cache-reuse probe: spawn a fresh explorer, and the moment
# it goes idle spawn N forks of it (same generic review prompt). The point is
# to keep explorer-to-fork delay inside the 5-minute cache TTL, unlike the
# main run where manual choreography let the 5m entries expire (forks then
# read only the 1h-cached 54,492-token prefix). Compare the forks'
# first-turn cache_read/cache_creation against that run.
#
# Env: WT, HUB, SOCK (tmux socket), NONCE. Prints "name<TAB>sid" lines.
set -euo pipefail
N="${1:-3}"
HERE="$(dirname "$0")"
P="$HOME/.claude/projects/-home-knatte-Code-lyx-test-HUB-lyx-test"

EXP_SID=$("$HERE/spawn.sh" explorer-c "$HERE/prompts/explorer.md" --add-dir "$WT")
printf 'explorer-c\t%s\n' "$EXP_SID"

PANE=$( (cd "$HUB" && lyx mux status) | python3 -c "
import json,sys
for s in json.load(sys.stdin)['strands']:
    if s['name']=='explorer-c': print(s['paneId'])")

"$HERE/wait_stable.sh" "$P/$EXP_SID.jsonl" 600 "shared explorer" "$PANE" >&2

for i in $(seq 1 "$N"); do
  SID=$("$HERE/spawn.sh" "c1-$i" "$HERE/prompts/review-b1.md" --resume "$EXP_SID" --fork-session)
  printf 'c1-%s\t%s\n' "$i" "$SID"
done
