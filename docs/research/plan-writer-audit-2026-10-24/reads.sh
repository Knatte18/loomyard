#!/usr/bin/env bash
# usage: reads.sh <slug> <archive-tag>
# Writes $OUT/reads/<slug>.{read,named,summary}: repo files the Plan-Write (title ly:<slug>:plan) session read,
# files relevant.md names, and the overlap shares. File-level; directory args and globs are not expanded.
# Run from anywhere. Env: WEFT=<records repo> (default <repo>/../loomyard-weft); OUT=<work dir> (default <repo>/.scratch/plan-writer-audit/).
# The hub is the repo's parent directory; the task worktree and its Claude Code transcript folder are derived from it.
set -u
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO="$(cd "$HERE/../../.." && pwd)"
HUB="$(dirname "$REPO")"
WEFT="${WEFT:-$REPO/../loomyard-weft}"
OUT="${OUT:-$REPO/.scratch/plan-writer-audit}"
slug=$1; tag=$2
out=$OUT/reads; mkdir -p $out
dir=$HOME/.claude/projects/${HUB//\//-}-$slug
wt=$HUB/$slug/
sessions=$(grep -l "\"customTitle\":\"ly:$slug:plan\"" $dir/*.jsonl 2>/dev/null)
[ -z "$sessions" ] && { echo "$slug no-plan-session"; exit 1; }
git -C "$WEFT" show $tag:_lyx/discussion/relevant.md 2>/dev/null > $out/$slug.relevant.md || { echo "$slug no-relevant.md"; exit 1; }
# named: first backticked token per bullet, trailing slash dropped
grep -E '^- ' $out/$slug.relevant.md | grep -oE '^- `[^`]+`' | sed -E 's/^- `//; s/`$//; s#/$##' | sort -u > $out/$slug.named
{
  jq -r 'select(.type=="assistant") | .message.content[]? | select(.type=="tool_use") |
    if .name=="Read" then "READ\t"+(.input.file_path//"")
    elif .name=="Bash" then "BASH\t"+((.input.command//"")|gsub("\n";" ; "))
    else empty end' $sessions
} > $out/$slug.calls
{
  grep -P '^READ\t' $out/$slug.calls | cut -f2 | sed "s#^$wt##"
  grep -P '^BASH\t' $out/$slug.calls | cut -f2 | grep -E '(^|[ ;|&(])(cat|sed|grep|rg|head|tail|awk|wc|nl|less|diff)( |$)' \
    | grep -oE '[A-Za-z0-9_./-]+\.(go|md|yaml|yml|json|sh|cmd|scm|txt|toml|mod)\b' | sed "s#^$wt##"
} | grep -vE '^(/|_lyx/|\.lyx/|\.scratch|_board|\.\./)' | grep -E '/|^(PATTERN|CLAUDE)\.md$' | sort -u > $out/$slug.read
nr=$(wc -l < $out/$slug.read); nn=$(wc -l < $out/$slug.named)
# a named entry is read if a read file equals it or lies under it; a read file is named if it equals or lies under a named entry
rn=0; while read -r f; do grep -qxF -e "$f" $out/$slug.named || awk -v f="$f" 'index(f,$0"/")==1{h=1} END{exit !h}' $out/$slug.named && rn=$((rn+1)); done < $out/$slug.read
nrd=0; while read -r n; do grep -qxF -e "$n" $out/$slug.read || awk -v n="$n" 'index($0,n"/")==1{h=1} END{exit !h}' $out/$slug.read && nrd=$((nrd+1)); done < $out/$slug.named
echo -e "$slug\t$nr\t$rn\t$nn\t$nrd" | tee $out/$slug.summary
