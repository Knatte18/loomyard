#!/usr/bin/env bash
# Usage: sonnet-writer.sh [dir]
# Prices the darn writer sessions of <dir>/sessions.tsv (default: this script's directory) again at Sonnet 5.5 list price
# for the same tokens, and prints one line: the sessions counted, their recorded cost, their cost at Opus 5.5 and Sonnet
# 5.5 list prices, the ratio, and the share of the Opus cost that is cache reads, cache writes and output.
# List prices in USD per million tokens, from internal/shuttleengine/claudeengine/pricing.go:
#   Opus 5.5:   input 4, cache write 5 (five-minute), cache read 0.20, output 20
#   Sonnet 5.5: input 2, cache write 2.5,             cache read 0.20, output 10
# A session counts when its recorded cost equals its tokens at the Opus prices to within 3%, which leaves out the
# sessions that changed model partway, and the run named by EXCLUDE (default `comment-cleanup`, a one-off run).
# Needs bash and awk; prints to standard output only.
set -euo pipefail
here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
dir=${1:-$here}
exclude=${EXCLUDE:-comment-cleanup}

awk -F'\t' -v exclude="$exclude" '
NR > 1 && $3 == "Darn-Write" && $5 == "claude-opus-5-5" && $2 != exclude {
  opus_cost = ($8 * 4 + $9 * 5 + $10 * 0.2 + $11 * 20) / 1e6
  if ($7 / opus_cost < 0.97) next
  n++; recorded += $7; opus += opus_cost
  sonnet += ($8 * 2 + $9 * 2.5 + $10 * 0.2 + $11 * 10) / 1e6
  reads += $10 * 0.2 / 1e6; writes += $9 * 5 / 1e6; output += $11 * 20 / 1e6
}
END {
  printf "sessions %d, recorded $%.2f, Opus $%.2f, Sonnet $%.2f, ratio %.3f, cache reads %.0f%%, cache writes %.0f%%, output %.0f%% of the Opus cost\n", \
    n, recorded, opus, sonnet, sonnet / opus, 100 * reads / opus, 100 * writes / opus, 100 * output / opus
}
' "$dir/sessions.tsv"
