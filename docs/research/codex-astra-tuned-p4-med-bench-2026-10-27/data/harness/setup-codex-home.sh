#!/usr/bin/env bash
# setup-codex-home.sh: create the scratch CODEX_HOME the bench runs use, holding a symlink to the operator's auth.json.
# The link is writable through to ~/.codex/auth.json, so a token refresh by Codex would change the operator's file.
# An existing auth.json in the scratch directory, link or file, is replaced.
ROOT=/home/knatte/Code/loomyard-LYMHUB/codex-astra-tuned-p4-med-bench
mkdir -p "$ROOT/.scratch/codex-home"
ln -sfn "$HOME/.codex/auth.json" "$ROOT/.scratch/codex-home/auth.json"
