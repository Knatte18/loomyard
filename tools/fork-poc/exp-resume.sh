#!/usr/bin/env bash
# exp-resume.sh — why do forks miss the parent's history cache?
#
# Small parent (ONE tool call, few content blocks), then immediately two
# children: a plain --resume continuation and a --fork-session fork.
# Distinguishes the two documented candidate mechanisms:
#   - 20-block lookback geometry (big multi-tool turns outrun the lookback):
#     a small parent should then HIT (cache_read >> static prefix 54,492).
#   - byte divergence when Claude Code re-serializes a reloaded session:
#     the small-parent children still miss; if plain resume hits but fork
#     misses, the divergence is fork-specific.
#
# Env: WT, HUB, SOCK, NONCE. Prints "name<TAB>sid" lines.
set -euo pipefail
HERE="$(dirname "$0")"
P="$HOME/.claude/projects/-home-knatte-Code-lyx-test-HUB-lyx-test"
SCRATCH="$WT/.scratch/fork-poc"

EXP_SID=$("$HERE/spawn.sh" explorer-s "$HERE/prompts/explorer-small.md" --add-dir "$WT")
printf 'explorer-s\t%s\n' "$EXP_SID"

statusfield() { # statusfield <name> <field>
  (cd "$HUB" && lyx mux status) | python3 -c "
import json,sys
for s in json.load(sys.stdin)['strands']:
    if s['name']=='$1': print(s['$2'])"
}

PANE=$(statusfield explorer-s paneId)
"$HERE/wait_stable.sh" "$P/$EXP_SID.jsonl" 300 "shared explorer" "$PANE" >&2

# Kill the parent process so the plain-resume child can own the session id.
GUID=$(statusfield explorer-s guid)
(cd "$HUB" && lyx mux remove "$GUID" >&2)

# Fork child: new session id, inherits history.
F_SID=$("$HERE/spawn.sh" fork-s "$HERE/prompts/probe-small.md" --resume "$EXP_SID" --fork-session)
printf 'fork-s\t%s\n' "$F_SID"

# Plain-resume child: continues the SAME session id; its turn appends to the
# parent transcript. Reuses the prompt spawn.sh rendered for fork-s.
(cd "$HUB" && lyx mux add --name resume-s \
  --cmd "claude \"\$(cat $SCRATCH/fork-s.prompt)\" --resume $EXP_SID" >&2)
printf 'resume-s\t%s\n' "$EXP_SID"
