#!/usr/bin/env bash
# A8 plan-writer audit, step 1: one tokencount report per loom slug in slugs.txt, 4 in parallel, into $OUT/tc/<slug>.md.
# Writes only under OUT (default <repo>/.scratch/plan-writer-audit/), never beside this script.
# Run from anywhere: docs/research/plan-writer-audit-2026-10-24/run-all.sh
# Env: OUT=<work dir> for the reports and the built tokencount binary.
set -u
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO="$(cd "$HERE/../../.." && pwd)"
OUT="${OUT:-$REPO/.scratch/plan-writer-audit}"; mkdir -p "$OUT/tc"
[ -x "$OUT/tokencount-bin" ] || (cd "$REPO" && go build -o "$OUT/tokencount-bin" ./tools/tokencount) || exit 1
export OUT
cd "$REPO" || exit 1
sort -u "$HERE/slugs.txt" | xargs -P4 -I{} sh -c '"$OUT/tokencount-bin" -detail -out "$OUT/tc/$1.md" "$1" >/dev/null 2>"$OUT/tc/$1.err"' _ {}
echo done > "$OUT/tc/_DONE"
