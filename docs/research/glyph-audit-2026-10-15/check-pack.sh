#!/usr/bin/env bash
# Check every span entry of every pf fork's pack against the tree at the batch's startSha (batches.tsv).
# Output: pack-check.tsv   columns: agent, proj, card, glyph, file, a, b, class, detail
# class: correct (signature text found inside a-b), correct-decl (declaration of the name found inside a-b),
#        stale (declaration found in the same file at other lines; detail = line shift),
#        wrong (file exists, name not declared in it), nofile (file absent in the tree), notree (startSha not reconstructable)
set -u
here="$(cd "$(dirname "$0")" && pwd)"
R=/home/knatte/Code/loomyard-LYXHUB/loomyard
tmp="$here/.tmp.$$"; mkdir -p "$tmp"
: > "${OUT:-$here/pack-check.tsv}"
: > "$here/pack-unres.tsv"
while IFS=$'\t' read -r agent proj start batch n sha ex rest; do
  tf=$(grep -- "/$agent\$" "$here/webster-files.txt" | head -1)
  ( cd "$HOME/.claude/projects" && jq -s -r -f "$here/pack-entries.jq" "$tf" ) > "$tmp/entries.tsv"
  grep -P '\tunres\t' "$tmp/entries.tsv" | awk -F'\t' -v p="$proj" '{print $1"\t"p"\t"$2"\t"$4"\t"$8}' >> "$here/pack-unres.tsv"
  grep -P '\tspan\t' "$tmp/entries.tsv" | while IFS=$'\t' read -r ag card kind glyph file a b sig; do
    if [ "$ex" != "commit" ]; then
      printf '%s\t%s\t%s\t%s\t%s\t%s\t%s\tnotree\t-\n' "$agent" "$proj" "$card" "$glyph" "$file" "$a" "$b" >> "${OUT:-$here/pack-check.tsv}"; continue
    fi
    if ! git -C "$R" show "$sha:$file" > "$tmp/f.txt" 2>/dev/null; then
      printf '%s\t%s\t%s\t%s\t%s\t%s\t%s\tnofile\t-\n' "$agent" "$proj" "$card" "$glyph" "$file" "$a" "$b" >> "${OUT:-$here/pack-check.tsv}"; continue
    fi
    res=$(awk -v a="$a" -v b="$b" -v sig="$sig" '
      function decl(l, n) {
        return (l ~ ("^func (\\([^)]*\\) )?" n "[(\\[]") || l ~ ("^(type|const|var) " n "([^A-Za-z0-9_]|$)") || l ~ ("^[ \t]+" n "([ \t]+[^ \t(]|[ \t]*=|[ \t]*$)"))
      }
      BEGIN {
        s = sig; t = s
        if (t ~ /^func \(/) { sub(/^func \([^)]*\) /, "", t) } else { sub(/^(func|type|const|var) /, "", t) }
        match(t, /^[A-Za-z0-9_]+/); name = substr(t, 1, RLENGTH)
      }
      { line[NR] = $0 }
      END {
        for (i = a; i <= b && i <= NR; i++) { if (index(line[i], sig) > 0) { print "correct\t0"; exit } }
        for (i = a; i <= b && i <= NR; i++) { if (name != "" && decl(line[i], name)) { print "correct-decl\t0"; exit } }
        for (i = 1; i <= NR; i++) { if (index(line[i], sig) > 0 || (name != "" && decl(line[i], name))) { print "stale\t" (i - a); exit } }
        print "wrong\t" name
      }' "$tmp/f.txt")
    printf '%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n' "$agent" "$proj" "$card" "$glyph" "$file" "$a" "$b" "$res" >> "${OUT:-$here/pack-check.tsv}"
  done
done < "${BATCHES:-$here/batches.tsv}"
rm -rf "$tmp"
wc -l "${OUT:-$here/pack-check.tsv}" "$here/pack-unres.tsv"
