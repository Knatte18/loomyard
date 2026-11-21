#!/usr/bin/env bash
# lib.sh is sourced by the other scripts here and names where a landed run's records live.
# HUB is the hub directory (default: the parent of the prime worktree), PROJECTS Claude Code's projects directory.
# WEFT is the prime's records repository, ARCHIVE the hub's per-run log archive and its gate run records under GATE_RUNS.
here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
prime=${PRIME:-$(git -C "$here" worktree list --porcelain | awk 'NR == 1 { print $2 }')}
HUB=${HUB:-$(dirname "$prime")}
WEFT=${WEFT:-$prime-weft}
ARCHIVE=${ARCHIVE:-$HUB/_bolt/.lym/log-archive}
PROJECTS=${PROJECTS:-$HOME/.claude/projects}

# sessions <slug>: the directory of the run's Claude Code session transcripts.
sessions() { printf '%s/%s-%s' "$PROJECTS" "$(printf '%s' "$HUB" | tr -c 'A-Za-z0-9' '-')" "$1"; }

# session <slug> <id-prefix>: the transcript of one session.
session() { ls "$(sessions "$1")/$2"*.jsonl; }
