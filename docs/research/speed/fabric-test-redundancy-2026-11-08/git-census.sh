#!/usr/bin/env bash
# Usage: git-census.sh <out dir> > gitprocs.tsv
# Prints one line per test the coverage runner traced: test, git processes it started, and how many of them were `git init`.
# The trace2 files of a run are <out dir>/git/<Test>/*, one per git process.
# Needs bash, find, grep and wc; reads only.
set -euo pipefail
export LC_ALL=C

out=$1
for dir in "$out"/git/*/; do
  name=$(basename "$dir")
  processes=$(find "$dir" -type f | wc -l)
  inits=0
  if [ "$processes" -gt 0 ]; then
    inits=$(find "$dir" -type f -print0 | xargs -0 grep -l '"argv":\["git","init"' 2>/dev/null | wc -l || true)
  fi
  printf '%s\t%d\t%d\n' "$name" "$processes" "$inits"
done | sort
