#!/usr/bin/env bash
# Usage: repo-size.sh [first-day] [last-day] > repo-size.tsv
# Prints the size of the parent branch's tree at the end of each UTC day from first-day (default 2026-10-22) to last-day (default today):
# day commit go_files go_lines test_files test_lines test_funcs packages md_lines
# go_lines counts every line of every .go file, test_lines those of the _test.go files among them, test_funcs the top-level Test functions,
# packages the directories holding a .go file, and md_lines every line of every .md file.
# A day with no commit yet on the branch prints nothing.
# Needs bash, awk and git; reads only.
set -euo pipefail
here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
root=$(git -C "$here" rev-parse --show-toplevel)
prime=${PRIME:-$(git -C "$root" worktree list --porcelain | awk 'NR == 1 { print $2 }')}
branch=$(git -C "$prime" symbolic-ref --short HEAD)
first=${1:-2026-10-22}
last=${2:-$(date -u +%Y-%m-%d)}

printf 'day\tcommit\tgo_files\tgo_lines\ttest_files\ttest_lines\ttest_funcs\tpackages\tmd_lines\n'
day=$first
while [[ ! "$day" > "$last" ]]; do
  commit=$(git -C "$prime" log --first-parent -1 --format=%H --until="${day}T23:59:59Z" "$branch")
  if [ -n "$commit" ]; then
    go=$(git -C "$prime" grep -c '' "$commit" -- '*.go' | awk -F: '
      { files++; lines += $NF; dir = $2; sub(/\/[^\/]*$/, "", dir); dirs[dir] = 1; if ($2 ~ /_test\.go$/) { tfiles++; tlines += $NF } }
      END { n = 0; for (d in dirs) n++; printf "%d\t%d\t%d\t%d\t%d", files, lines, tfiles, tlines, n }')
    IFS=$'\t' read -r go_files go_lines test_files test_lines packages <<< "$go"
    test_funcs=$(git -C "$prime" grep -c -E '^func Test[A-Z_]' "$commit" -- '*_test.go' | awk -F: '{ n += $NF } END { print n + 0 }')
    md_lines=$(git -C "$prime" grep -c '' "$commit" -- '*.md' | awk -F: '{ n += $NF } END { print n + 0 }')
    printf '%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n' "$day" "${commit:0:8}" "$go_files" "$go_lines" "$test_files" "$test_lines" "$test_funcs" "$packages" "$md_lines"
  fi
  day=$(date -u -d "$day + 1 day" +%Y-%m-%d)
done
