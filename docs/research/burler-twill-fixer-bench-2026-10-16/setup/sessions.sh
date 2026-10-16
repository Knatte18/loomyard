#!/usr/bin/env bash
# sessions.sh <pair>: lists every Claude session of the pair with its role, read from the session's first lines
# (fixer, reviewer, bouncer/judge, other), and prints usage.sh's row for it and each of its forks.
S=/home/knatte/Code/loomyard-LYXHUB/loomyard/.scratch/burler-twill-fixer-bench/setup
P=$HOME/.claude/projects/-home-knatte-Code-loomyard-LYXHUB-$1
for t in $(ls -tr "$P"/*.jsonl); do
  # The prompt the session was pointed at names its role: the fixer reads its fix step, the reviewer its explore step.
  if grep -q -F 'instruction-fix.md' "$t"; then role=fixer
  elif grep -q -E '\.lyx/burler/review-[0-9]+/' "$t"; then role=reviewer
  elif grep -q -i -F 'bouncer' "$t"; then role=bouncer
  else role=other
  fi
  "$S/usage.sh" "$t" "$role:$(basename "$t" .jsonl | cut -c1-8)"
done
