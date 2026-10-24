#!/usr/bin/env bash
# A8 plan-writer audit, step 2: second pass of run-all.sh over the slugs in sample-input.tsv (runs started 2026-10-15 or later),
# skipping reports already written, then gate-runs-once. Writes only under OUT (default <repo>/.scratch/plan-writer-audit/).
# Run from anywhere: docs/research/plan-writer-audit-2026-10-24/run-priority.sh
# Env: OUT=<work dir> for the reports, the slug lists and the built tokencount binary.
set -u
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO="$(cd "$HERE/../../.." && pwd)"
OUT="${OUT:-$REPO/.scratch/plan-writer-audit}"; mkdir -p "$OUT/tc"
[ -x "$OUT/tokencount-bin" ] || (cd "$REPO" && go build -o "$OUT/tokencount-bin" ./tools/tokencount) || exit 1
export OUT
cd "$REPO" || exit 1
cut -f1 "$HERE/sample-input.tsv" | sort -u > "$OUT/slugs-priority.txt"
for s in $(cat "$OUT/slugs-priority.txt"); do [ -s "$OUT/tc/$s.md" ] || echo "$s"; done > "$OUT/slugs-todo.txt"
echo gate-runs-once >> "$OUT/slugs-todo.txt"
xargs -P6 -I{} sh -c '"$OUT/tokencount-bin" -detail -out "$OUT/tc/$1.md" "$1" >/dev/null 2>"$OUT/tc/$1.err"' _ {} < "$OUT/slugs-todo.txt"
echo done > "$OUT/tc/_DONE_PRIORITY"
