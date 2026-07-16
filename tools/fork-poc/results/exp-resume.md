# exp-resume: why forks miss the parent's history cache

Small parent (1 tool call), children spawned seconds after idle.

| turn | cache_read | cache_creation |
|---|--:|--:|
| parent turn 1 (fresh session)        | 27,246 | 6,624 |
| parent turn 2 (same live process)    | 33,870 (= 27,246 + 6,624) | 9,504 |
| resume child (plain --resume)        | 27,246 | 16,287 |
| fork child (--fork-session)          | 27,246 | 16,284 |

Findings:
- Static per-cwd prefix = 27,246 tokens. Only the live process reuses its own
  history cache (turn 2 read static + its own turn-1 creation).
- Plain resume and fork miss the ENTIRE parent history identically — with a
  tiny parent (well inside the 20-block lookback) and near-zero delay. This
  refutes both the TTL hypothesis and the 20-block-geometry hypothesis, and
  shows the miss is not fork-specific.
- Root cause: Claude Code re-serializes a reloaded session (resume and fork
  alike) into bytes that do not match the live session's cached prefix.
  History-computation reuse on reload is 0% by construction. Consistent with
  Anthropic's docs, which never promise resume/fork cache reuse and place the
  exact-prefix burden on the client.
- Secondary: re-serialization is deterministic, so sibling forks of the same
  parent CAN read each other's cache. This explains the main run's 54,492
  (static 27,246 + a chunk the earlier M1 fork had cached). Design lever:
  stagger fork launches (fire one, await first token, fire the rest).

## Correction (follow-up experiments s2–s5)

The "re-serialization" conclusion above was WRONG. Further isolation:

| child | flags | cache_read | cache_creation |
|---|---|--:|--:|
| fork, matching --add-dir (s2)   | same as parent | 27,246 | 16,447 |
| sibling fork, within TTL (s3)   | same as parent | 27,246 | 16,447 |
| plain resume, matching --add-dir (s5) | same as parent | **43,611 (FULL HIT)** | **65** |

Root cause, proven:
- Cause A (harness): --add-dir mismatch between parent and children changed
  the system prompt (API diagnostic: cache_miss_reason type=system_changed).
  With flags matched, plain --resume reuses the parent's ENTIRE cache.
- Cause B (structural, fork-only): --fork-session assigns a new session id,
  and the system prompt contains session-unique bytes (the scratchpad path
  embeds the session id). New id -> different system prompt -> system tier
  miss -> messages tier invalidated. Hence CLI forks never reuse the parent's
  history cache, and sibling forks don't share with each other either.
- Claude Code's history re-serialization on reload is byte-faithful; the
  transcript diff (parent vs fork copied region) is identical modulo ids.

Next candidate: built-in fork subagents (Agent tool subagent_type "fork"),
which run inside the parent process with the parent's system prompt and
should sidestep cause B entirely.
