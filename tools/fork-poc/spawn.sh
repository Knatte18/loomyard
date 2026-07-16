#!/usr/bin/env bash
# spawn.sh NAME PROMPT_FILE [extra claude args...]
#
# Renders PROMPT_FILE ({{WT}} -> $WT, {{NONCE}} -> $NONCE) into
# $WT/.scratch/fork-poc/NAME.prompt, generates a session UUID, and spawns a
# mux strand in the sandbox hub ($HUB) running claude with that prompt and
# the preassigned --session-id. The pane's bash expands $(cat ...) so the
# prompt file's content becomes the initial claude prompt.
#
# Env: WT (loomyard worktree, required), HUB (sandbox host repo, required),
#      NONCE (optional). Appends "NAME<TAB>SID" to sessions.tsv; prints SID.
set -euo pipefail
WT="${WT:?set WT to the loomyard worktree}"
HUB="${HUB:?set HUB to the sandbox host repo}"
SCRATCH="$WT/.scratch/fork-poc"
NAME="$1"; PROMPT="$2"; shift 2
mkdir -p "$SCRATCH"
SID="$(uuidgen)"
sed -e "s|{{WT}}|$WT|g" -e "s|{{NONCE}}|${NONCE:-}|g" "$PROMPT" > "$SCRATCH/$NAME.prompt"
(cd "$HUB" && lyx mux add --name "$NAME" \
  --cmd "claude --session-id $SID $* \"\$(cat $SCRATCH/$NAME.prompt)\"" >&2)
printf '%s\t%s\n' "$NAME" "$SID" >> "$SCRATCH/sessions.tsv"
echo "$SID"
