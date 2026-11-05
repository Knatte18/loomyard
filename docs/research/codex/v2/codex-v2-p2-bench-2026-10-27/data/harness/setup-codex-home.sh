#!/usr/bin/env bash
# setup-codex-home.sh <run>: create the scratch CODEX_HOME of one bench run, holding a symlink to the operator's auth.json.
# One home per run keeps concurrent runs' rollouts apart.
# The link is writable through to ~/.codex/auth.json, so a token refresh by Codex would change the operator's file.
# An existing auth.json in the scratch directory, link or file, is replaced.
ROOT=/home/knatte/Code/loomyard-LYMHUB/codex-v2-p2-bench
mkdir -p "$ROOT/.scratch/codex-home-$1"
ln -sfn "$HOME/.codex/auth.json" "$ROOT/.scratch/codex-home-$1/auth.json"
