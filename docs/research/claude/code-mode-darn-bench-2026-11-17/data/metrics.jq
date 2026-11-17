def price: if (.model|test("opus")) then {i:4,w5:5,w1:8,r:0.2,o:20} elif (.model|test("sonnet")) then {i:2,w5:2.5,w1:4,r:0.2,o:10} else {i:1,w5:1.25,w1:2,r:0.1,o:5} end;
[inputs | select(.type=="assistant" and .message.usage != null)]
| group_by(.message.id)
| map(last) as $msgs
| {
  turns: ($msgs|length),
  input: ($msgs|map(.message.usage.input_tokens)|add),
  cache_write: ($msgs|map(.message.usage.cache_creation_input_tokens)|add),
  cache_read: ($msgs|map(.message.usage.cache_read_input_tokens)|add),
  output: ($msgs|map(.message.usage.output_tokens)|add),
  cost: ($msgs|map(. as $m | ($m.message|price) as $p | ($m.message.usage) as $u
      | ($u.input_tokens*$p.i + ($u.cache_creation.ephemeral_5m_input_tokens // 0)*$p.w5 + ($u.cache_creation.ephemeral_1h_input_tokens // 0)*$p.w1 + $u.cache_read_input_tokens*$p.r + $u.output_tokens*$p.o)/1000000)|add),
  models: ($msgs|map(.message.model)|unique),
  first: ($msgs|map(.timestamp)|min),
  last: ($msgs|map(.timestamp)|max),
  peak_context: ($msgs|map(.message.usage.input_tokens + .message.usage.cache_creation_input_tokens + .message.usage.cache_read_input_tokens)|max)
}
