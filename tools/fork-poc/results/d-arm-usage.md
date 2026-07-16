# D-arm: built-in fork subagents (CLAUDE_CODE_FORK_SUBAGENT=1)

One explorer session (mux strand) explored internal/modelspec, then spawned
three parallel Agent-tool forks (subagent_type omitted, unnamed) with lens
prompts, and reported their outputs verbatim (results/d-arm-parent.md).

Per fork, first request: in=186, cache_creation=11,921, cache_read=51,673
— the parent's FULL live prefix (tools + system + exploration) served from
cache. Compare CLI forks: cache_read 27,246 (static only), cache_creation
~75k. Fork-subagent totals (compute = in+cr+out): 20,047 / 12,109 / 17,805.

| arm | marginal cost per reviewer (compute) | history served as |
|---|--:|---|
| A cold            | ~209k | fresh + own cache |
| B CLI fork        |  ~97k | cache_creation (125%) |
| D fork subagent   |  ~17k | cache_read (10%) |

Parent session total: 236k compute, 24 turns (exploration ~157k + fork
orchestration + receiving three reports). Arm total ≈ 286k vs B2 447k vs
A 628k.

Quality: 18 findings across the three lenses, consistent with the B2/A
clusters (incl. the Resolve zero-value HIGH); NONCE recalled by all three,
zero file re-reads.

Caveats (from docs/research + this run): requires CLAUDE_CODE_FORK_SUBAGENT=1
(v2.1.117+, staged rollout otherwise); forks always run the PARENT's model
(no model-per-fork axis); forks cannot nest; named forks silently lose
context in <=2.1.206 — keep forks unnamed; reviewers are not separate mux
panes (visibility via <session-id>/subagents/*.jsonl on disk); one fork's
subagent transcript recorded an empty final text although its report reached
the parent intact (bookkeeping quirk, not a data loss).
