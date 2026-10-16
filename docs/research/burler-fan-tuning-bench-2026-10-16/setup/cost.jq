# Usage and API list-price cost of one or more Claude Code transcript files (jq -s over the .jsonl).
# Each message is counted once, on its last usage record (streamed messages repeat their id).
# Prices per MTok as in lens-fan-lab-phase1.md: opus in 4 / out 20 / write5m 5 / write1h 8 / read 0.20;
# sonnet 2 / 10 / 2.5 / 4 / 0.20; haiku 0.10 / 0.50 / 0.125 / 0.20 / 0.01.
def price(m):
  if (m|test("opus")) then {i:4,o:20,w5:5,w1:8,r:0.2}
  elif (m|test("sonnet")) then {i:2,o:10,w5:2.5,w1:4,r:0.2}
  else {i:0.1,o:0.5,w5:0.125,w1:0.2,r:0.01} end;
[ .[] | select(.type=="assistant" and .message.usage != null) | {id:.message.id, m:(.message.model // ""), u:.message.usage} ]
| group_by(.id) | map(last)
| map(. as $x | ($x.u.cache_creation.ephemeral_1h_input_tokens // 0) as $w1
    | (($x.u.cache_creation_input_tokens // 0) - $w1) as $w5
    | price($x.m) as $p
    | {in:($x.u.input_tokens//0), out:($x.u.output_tokens//0), w:($x.u.cache_creation_input_tokens//0), r:($x.u.cache_read_input_tokens//0),
       usd: ((($x.u.input_tokens//0)*$p.i + ($x.u.output_tokens//0)*$p.o + $w5*$p.w5 + $w1*$p.w1 + ($x.u.cache_read_input_tokens//0)*$p.r)/1e6)})
| {messages:length, input:(map(.in)|add), output:(map(.out)|add), cache_write:(map(.w)|add), cache_read:(map(.r)|add),
   fresh:(map(.in+.out+.w)|add), usd:(map(.usd)|add*100|round/100)}
