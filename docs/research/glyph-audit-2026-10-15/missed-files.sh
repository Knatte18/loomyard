#!/usr/bin/env bash
# Files a pf fork edited or read that the pack gave no span for, split by whether the file existed at the batch's startSha.
# Output: missed-files.tsv  columns: agent, proj, relpath, edits, writes, reads, existed_at_start(yes|no|unknown)
set -u
here="$(cd "$(dirname "$0")" && pwd)"
R=/home/knatte/Code/loomyard-LYXHUB/loomyard
: > "$here/missed-files.tsv"
while IFS=$'\t' read -r agent proj start batch n sha ex rest; do
  tf=$(grep -- "/$agent\$" "$here/webster-files.txt" | head -1)
  ( cd "$HOME/.claude/projects" && jq -s -r -f "$here/pack-entries.jq" "$tf" ) | awk -F'\t' '$3=="span"{print $5}' | sort -u > "$here/.spans.txt"
  ( cd "$HOME/.claude/projects" && jq -s -r -f "$here/touched-files.jq" "$tf" ) | while IFS=$'\t' read -r ag f e w r; do
    case "$f" in _lyx/*|.lyx/*|/*) continue;; esac
    if grep -qxF -- "$f" "$here/.spans.txt"; then continue; fi
    if [ "$ex" = "commit" ]; then
      if git -C "$R" cat-file -e "$sha:$f" 2>/dev/null; then x=yes; else x=no; fi
    else x=unknown; fi
    printf '%s\t%s\t%s\t%s\t%s\t%s\t%s\n' "$agent" "$proj" "$f" "$e" "$w" "$r" "$x" >> "$here/missed-files.tsv"
  done
done < "$here/batches.tsv"
wc -l "$here/missed-files.tsv"
