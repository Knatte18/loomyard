#!/usr/bin/env bash
# Usage: usage.sh <transcript.jsonl> [<label>]; prints per session (the session, then each of its forks) the deduplicated token usage and its Opus 5.5 cost.
# Copied from twill-discussion-fork0-bench's usage.sh: messages are deduplicated by id keeping each id's last line, and a fork's copy of the spawn message is excluded.
# Columns: label, messages, output, cache write 5m, cache write 1h, cache read, input, cost USD, first and last assistant timestamps, context at the last message.
# Prices are tools/tokencount/cost.go's claude-opus-5-5 row: input 4, 5m write 5, 1h write 8, read 0.20, output 20 USD per Mtok.
one() {
  jq -s -r --arg name "$2" --rawfile ex "$3" '
    ($ex | split("\n")) as $exl
    | [ .[] | select(.type=="assistant" and .message.usage != null) | select(.message.id as $i | ($exl | index([$i])) == null) ] as $a
    | ($a | map({key: .message.id, value: .message.usage}) | from_entries | to_entries | map(.value)) as $u
    | ($u | map(.output_tokens // 0) | add // 0) as $out
    | ($u | map(.cache_creation.ephemeral_5m_input_tokens // 0) | add // 0) as $w5
    | ($u | map(.cache_creation.ephemeral_1h_input_tokens // 0) | add // 0) as $w1
    | ($u | map(.cache_creation_input_tokens // 0) | add // 0) as $wt
    | ($u | map(.cache_read_input_tokens // 0) | add // 0) as $rd
    | ($u | map(.input_tokens // 0) | add // 0) as $in
    | (if ($w5 + $w1) == 0 then $wt else $w5 end) as $w5f
    | (($in*4 + $w5f*5 + $w1*8 + $rd*0.20 + $out*20) / 1000000) as $cost
    | ($u[-1] // {}) as $l
    | (($l.input_tokens // 0) + ($l.cache_read_input_tokens // 0) + ($l.cache_creation_input_tokens // 0) + ($l.output_tokens // 0)) as $ctx
    | [$name, ($u|length), $out, $w5f, $w1, $rd, $in, ($cost*100|round/100), ($a[0].timestamp // ""), ($a[-1].timestamp // ""), $ctx] | @tsv
  ' "$1"
}
w=$1
label=${2:-session}
T="$(dirname "$0")/tmp"
mkdir -p "$T"
: > "$T/none"
jq -r 'select(.type=="assistant") | .message.id' "$w" | sort -u > "$T/session.ids"
one "$w" "$label" "$T/none"
d="${w%.jsonl}/subagents"
for m in "$d"/*.meta.json; do
  [ -f "$m" ] || continue
  g=${m%.meta.json}.jsonl
  one "$g" "fork:$(jq -r '.description // .agentType' "$m")" "$T/session.ids"
done
