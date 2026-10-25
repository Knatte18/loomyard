#!/usr/bin/env bash
# claude-reference.sh: print data/claude-findings.tsv and data/claude-usage.tsv from the weft history.
# usage: claude-reference.sh findings|usage
# findings: every Claude round-1 finding of the ten inputs with its severity and the Codex settings whose shared rows in match.tsv name it.
# usage: the round-1 review's wall time and fresh tokens, and its lens forks' fresh tokens, from the round's usage file.
set -euo pipefail
ROOT=/home/knatte/Code/loomyard-LYXHUB/codex-review-bench
WEFT=/home/knatte/Code/loomyard-LYXHUB/loomyard-weft
DATA=$ROOT/docs/research/codex-review-bench-2026-10-25/data
case $1 in
findings)
  printf 'input\tclaude_finding\tseverity\tfound_by\n'
  while read -r id kind weft _; do
    commit=${weft#weft=}
    review=$(mktemp)
    git -C $WEFT show "$commit:_lyx/reviews/$kind/round-1-review.md" > "$review"
    while read -r fid severity _; do
      found_by=$(while IFS=$'\t' read -r m_input m_setting _ m_label m_claude _; do
        [ "$m_input" = "$id" ] && [ "$m_label" = shared ] && [ "$m_claude" = "$fid" ] && echo "$m_setting"
      done < <(tail -n +2 "$DATA/match.tsv") | sort -u | paste -sd, -) || true
      printf '%s\t%s\t%s\t%s\n' "$id" "$fid" "$severity" "$found_by"
    done < <(bash "$DATA/harness/dump.sh" "$review")
    rm "$review"
  done < "$DATA/inputs.txt"
  ;;
usage)
  printf 'input\twall_seconds\tfresh_tokens\tforks\tfork_fresh_tokens\n'
  while read -r id kind weft _; do
    commit=${weft#weft=}
    git -C $WEFT show "$commit:_lyx/reviews/$kind/round-1-usage.yaml" | {
      # The first wall_seconds and fresh_tokens after the top-level review: key are the round's own; the seed block comes later.
      in_review=
      while IFS= read -r line; do
        case "$line" in
          "review:") in_review=1;;
          "seed:"*|"fix:"*) in_review=;;
          "    wall_seconds: "*) [ -n "$in_review" ] && wall=${line#    wall_seconds: };;
          "    fresh_tokens: "*) [ -n "$in_review" ] && fresh=${line#    fresh_tokens: };;
          "    forks: "*) [ -n "$in_review" ] && forks=${line#    forks: };;
          "    fork_fresh_tokens: "*) [ -n "$in_review" ] && fork_fresh=${line#    fork_fresh_tokens: };;
        esac
      done
      printf '%s\t%.0f\t%s\t%s\t%s\n' "$id" "$wall" "$fresh" "$forks" "$fork_fresh"
    }
  done < "$DATA/inputs.txt"
  ;;
esac
