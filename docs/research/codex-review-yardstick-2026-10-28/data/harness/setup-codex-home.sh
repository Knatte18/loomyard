#!/usr/bin/env bash
# setup-codex-home.sh <label>...: create one scratch CODEX_HOME per run label, holding a symlink to the operator's auth.json.
# The link is writable through to ~/.codex/auth.json, so a token refresh by Codex would change the operator's file.
# An existing auth.json in the scratch directory, link or file, is replaced.
ROOT=/home/knatte/Code/loomyard-LYMHUB/codex-review-yardstick
for label in "$@"; do
  mkdir -p "$ROOT/.scratch/codex-home-$label"
  ln -sfn "$HOME/.codex/auth.json" "$ROOT/.scratch/codex-home-$label/auth.json"
done
