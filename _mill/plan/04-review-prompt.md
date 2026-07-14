# Batch: review-prompt

```yaml
task: Investigate the unexplained lyx mux server crash
batch: review-prompt
number: 4
cards: 1
verify: null
depends-on: [3]
```

## Batch Scope

Update the standing adversarial mux review prompt so future review rounds drive the
behaviors this task ships instead of rediscovering them. Depends on batch 3 because the
prompt must describe the final shipped behavior of all three code batches. Pure docs
batch — no runnable surface.

## Cards

### Card 9: Teach mux-review-prompt.md the new behaviors

- **Context:**
  - `docs/reviews/README.md`
  - `_mill/discussion.md`
- **Edits:**
  - `docs/reviews/mux-review-prompt.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Extend the prompt (match its existing tone and structure) so a
  future reviewer exercises the new surfaces:
  1. In the "High-yield focus" invariant list, add entries for: DEBUG LOGGING — with
     `debug_log: 1` (or `LYX_MUX_DEBUG=1`) the boot that spawns the server must leave a
     `tmux-server-*.log` under the hub's `.lyx/logs/`, old logs are pruned to the
     newest 3 total, an invalid value fails the boot loud, and `debug_log: 0` adds no
     flags; DEAD-SERVER HINT — with persisted strands and no session, every verb's
     error must point at `lyx mux resume` (and plain `up` when zero strands persist);
     TOP-BAND LEGIBILITY — the shipped `top_band_rows` default is 3 and a per-strand
     `--top-band-rows` override wins over the config default while the last-top-band
     stretch still applies.
  2. In the "What to TEST" / hand-rolled driving section, add the corresponding drive
     instructions (boot with the env var armed and inspect the hub `.lyx/logs/`;
     kill-server with strands persisted and read each verb's error; `add --anchor top
     --top-band-rows N` and check the applied layout).
  3. Note the boot-winner semantics as a review lens: `debug_log` only matters on the
     boot that actually spawns the shared per-hub server — a reviewer testing from a
     sibling worktree must arm the env var machine-wide or boot from the armed
     worktree.
  4. Where the prompt's "Round context" / deferred-items sections reference the crash
     investigation, keep wording correlation-only per Shared Decision
     no-root-cause-claims (the server-death mechanism remains unexplained; this task
     shipped mitigations and forensic prep).
  Do not rewrite unrelated sections; surgical additions only.
- **Commit:** `Teach mux review prompt the debug-log, resume-hint and top-band behaviors`

## Batch Tests

`verify: null` — pure Markdown batch with no runnable surface. Reviewer-facing quality
gate: the edited prompt must name the exact key (`debug_log`), env var
(`LYX_MUX_DEBUG`), log path (`<hub>/.lyx/logs/`), and both error-message variants so a
cold reviewer can drive them without reading this plan.
