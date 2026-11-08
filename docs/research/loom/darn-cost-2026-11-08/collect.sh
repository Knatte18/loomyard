#!/usr/bin/env bash
# Usage: collect.sh [out-dir]
# Reads the usage records of every run archived in the records repo since SINCE and writes two tables to <out-dir>:
#   runs.tsv:     tag, slug, landed (UTC date of the tag), recipe, profile, stored_record, lines_added, lines_deleted,
#                 first_start, last_end (the earliest session start and latest session end, UTC)
#   sessions.tsv: tag, slug, row, role, model, engine, cost_usd, input, cache_write, cache_read, output, started, ended
# A tag without _lym/usage/run.json is left out; a tag without landed.json has empty line columns.
# Environment: WEFT names the records repo clone (default <repo root>/../loomyard-weft), SINCE the first UTC date
# (default 2026-10-25).
# Needs bash, git and jq; reads only, apart from the two tables.
set -euo pipefail
here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
root=$(git -C "$here" rev-parse --show-toplevel)
out=${1:-$here}
weft=${WEFT:-$root/../loomyard-weft}
since=${SINCE:-2026-10-25}
mkdir -p "$out"

printf 'tag\tslug\tlanded\trecipe\tprofile\tstored_record\tlines_added\tlines_deleted\tfirst_start\tlast_end\n' > "$out/runs.tsv"
printf 'tag\tslug\trow\trole\tmodel\tengine\tcost_usd\tinput\tcache_write\tcache_read\toutput\tstarted\tended\n' > "$out/sessions.tsv"

git -C "$weft" tag -l 'archive/*' --sort=creatordate --format='%(creatordate:short) %(refname:short)' |
  awk -v since="$since" '$1 >= since { print $2 }' |
  while read -r tag; do
    slug=${tag#archive/}
    slug=${slug%/*}
    files=$(git -C "$weft" ls-tree -r --name-only "$tag" -- _lym/usage 2> /dev/null || true)
    echo "$files" | grep -q '^_lym/usage/run.json$' || continue
    landed=$(git -C "$weft" log -1 --format=%cI "$tag" | cut -c1-10)
    run=$(git -C "$weft" show "$tag:_lym/usage/run.json")
    lines='{}'
    if echo "$files" | grep -q '^_lym/usage/landed.json$'; then
      lines=$(git -C "$weft" show "$tag:_lym/usage/landed.json")
    fi
    sessions=$(mktemp)
    echo "$files" | grep '/session-' | while read -r f; do
      git -C "$weft" show "$tag:$f"
    done | jq -c --arg tag "$tag" --arg slug "$slug" '[$tag, $slug, .row, .role, .model, .engine, .cost_usd, .input_tokens, .cache_write_tokens, .cache_read_tokens, .output_tokens, .started, .ended]' > "$sessions"
    jq -r '@tsv' "$sessions" >> "$out/sessions.tsv"
    first=$(jq -r '.[11]' "$sessions" | sort | head -1)
    last=$(jq -r '.[12]' "$sessions" | sort | tail -1)
    rm -f "$sessions"
    jq -r --arg tag "$tag" --arg slug "$slug" --arg landed "$landed" --arg first "$first" --arg last "$last" --argjson lines "$lines" \
      '[$tag, $slug, $landed, .recipe, .profile, (.stored_record | tostring), ($lines.lines_added // ""), ($lines.lines_deleted // ""), $first, $last] | @tsv' <<< "$run" >> "$out/runs.tsv"
  done
