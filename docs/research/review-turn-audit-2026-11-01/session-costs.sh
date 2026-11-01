#!/usr/bin/env bash
# Usage: session-costs.sh > session-costs.tsv
# One row per Discussion or Plan review-segment session of slugs.txt, main sessions and their fork sub-agents:
# slug session parent kind role round model start end wall_s turns ctx_mean ctx_peak cost_usd cost_sonnet_usd ctx_first read_usd write_usd output_usd
# The roles are discussion|plan x burler-review|burler-fix|bouncer-seed|bouncer-judge, and a fork's role carries "+fork".
# parent is the main session a fork belongs to, "-" for a main session.
# round numbers the main sessions of one slug and role by start time, from 1; a fork takes its parent's round.
# A fork transcript opens with its parent's spawning message, replayed; the script leaves that message out, since the parent already counts it.
# model is the one model the session's messages name; a session that names several is a fault the script stops on.
# wall_s is the seconds from the session's first to its last message.
# A turn is one assistant message id; ctx is a turn's input + cache write + cache read tokens,
# ctx_mean and ctx_peak are the mean and maximum over the turns, and ctx_first is the first turn's.
# cost_usd prices each turn at the Claude API list price of its model, in USD per million tokens, from internal/shuttleengine/claudeengine/pricing.go:
# opus-5-5 input 4, cache write 5 (five-minute) or 8 (one-hour), cache read 0.20, output 20;
# sonnet-5-5 input 2, cache write 2.5 or 4, cache read 0.20, output 10.
# cost_sonnet_usd prices the same tokens at the sonnet-5-5 row: what the session would have cost with the same turns and context on Sonnet.
# read_usd, write_usd and output_usd split cost_usd into the cache-read, cache-write and output parts; the rest is uncached input.
# The script stops on any other model, since the price rows are fixed.
# Environment: PROJECTS names the transcript root (default ~/.claude/projects).
set -euo pipefail
here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
projects=${PROJECTS:-$HOME/.claude/projects}

# shellcheck disable=SC2016
stats='
  def price($m): if $m == "claude-opus-5-5" then {i: 4, w5: 5, w1: 8, r: 0.20, o: 20}
    elif $m == "claude-sonnet-5-5" then {i: 2, w5: 2.5, w1: 4, r: 0.20, o: 10}
    else error("unpriced model " + $m) end;
  def writeusd($p; $u):
    ($u.cache_creation.ephemeral_1h_input_tokens // 0) as $w1h
    | (($u.cache_creation_input_tokens // 0) - $w1h) as $w5m
    | ($w5m * $p.w5 + $w1h * $p.w1) / 1e6;
  def usd($p; $u):
    ($u.input_tokens // 0) * $p.i / 1e6 + writeusd($p; $u)
    + ($u.cache_read_input_tokens // 0) * $p.r / 1e6 + ($u.output_tokens // 0) * $p.o / 1e6;
  [ .[] | select(.type == "assistant" and .message.id != null) ] as $all
  | (if $kind == "fork" then ($all | sort_by(.timestamp) | .[0].message.id) else "" end) as $replayed
  | ($all | map(select(.message.id != $replayed))) as $recs
  | ($recs | map(.message.model) | unique) as $models
  | if ($models | length) != 1 then error("session names models " + ($models | join(","))) else . end
  | $models[0] as $m
  | price($m) as $p
  | ($recs | group_by(.message.id) | map(max_by(.message.usage.output_tokens // 0)) | sort_by(.timestamp)) as $turns
  | ($turns | map(.message.usage | (.input_tokens // 0) + (.cache_creation_input_tokens // 0) + (.cache_read_input_tokens // 0))) as $ctx
  | ([ .[] | select(.timestamp != null) | .timestamp ] | sort) as $ts
  | [ $m, $ts[0], $ts[-1],
      ($ts[-1] | sub("\\.[0-9]+Z$"; "Z") | fromdate) - ($ts[0] | sub("\\.[0-9]+Z$"; "Z") | fromdate),
      ($turns | length), (($ctx | add) / ($ctx | length) | floor), ($ctx | max),
      ($turns | map(usd($p; .message.usage)) | add),
      ($turns | map(usd(price("claude-sonnet-5-5"); .message.usage)) | add),
      $ctx[0],
      ($turns | map((.message.usage.cache_read_input_tokens // 0) * $p.r / 1e6) | add),
      ($turns | map(writeusd($p; .message.usage)) | add),
      ($turns | map((.message.usage.output_tokens // 0) * $p.o / 1e6) | add) ]
  | @tsv'

emit() { # file slug session parent kind role
  local file=$1 slug=$2 session=$3 parent=$4 kind=$5 role=$6
  printf '%s\t%s\t%s\t%s\t%s\t%s\t' "$slug" "$session" "$parent" "$kind" "$role" 0
  jq -rs --arg kind "$kind" "$stats" "$file"
}

rows=$(while read -r slug; do
  dir=$projects/-home-knatte-Code-loomyard-LYMHUB-$slug
  for f in "$dir"/*.jsonl; do
    title=$(jq -r 'select(.type == "custom-title") | .customTitle' "$f" | tail -1)
    role=${title##*:}
    [[ $role =~ ^(discussion|plan)-(burler-review|burler-fix|bouncer-seed|bouncer-judge)$ ]] || continue
    sid=$(basename "$f" .jsonl)
    emit "$f" "$slug" "$sid" - main "$role"
    for sub in "$dir/$sid"/subagents/*.jsonl; do
      [ -e "$sub" ] || continue
      emit "$sub" "$slug" "$(basename "$sub" .jsonl)" "$sid" fork "$role+fork"
    done
  done
done < "$here/slugs.txt")

# Number the main sessions of a slug and role by start time (field 8), and give each fork its parent's round.
awk -F'\t' -v OFS='\t' '
  { line[NR] = $0; slug[NR] = $1; sid[NR] = $2; kind[NR] = $4; role[NR] = $5; start[NR] = $8 }
  END {
    for (i = 1; i <= NR; i++) if (kind[i] == "main") {
      rank = 1
      for (j = 1; j <= NR; j++) if (kind[j] == "main" && slug[j] == slug[i] && role[j] == role[i] && (start[j] < start[i] || (start[j] == start[i] && sid[j] < sid[i]))) rank++
      round[sid[i]] = rank
    }
    print "slug", "session", "parent", "kind", "role", "round", "model", "start", "end", "wall_s", "turns", "ctx_mean", "ctx_peak", "cost_usd", "cost_sonnet_usd", "ctx_first", "read_usd", "write_usd", "output_usd"
    for (i = 1; i <= NR; i++) {
      split(line[i], f, "\t")
      f[6] = (kind[i] == "main") ? round[sid[i]] : round[f[3]]
      printf "%s", f[1]; for (k = 2; k <= 19; k++) printf "\t%s", f[k]; printf "\n"
    }
  }' <<< "$rows" | { read -r header; printf '%s\n' "$header"; sort -t$'\t' -k1,1 -k5,5 -k6,6n -k4,4r -k8,8; }
