# PATTERN-pause-holds-prompts

Typing a line into a session a pause covers: shuttle's send refuses with `ErrPaused` by the target's own slug, read after the idle wait, and a conditioned send's caller reads coverage itself; of the production runners that type a line, only the orch runner is told no pause geometry and reads uncovered, so `lyx orch start`'s start lines are never held; the caller holds it until the resume or claims the session.

The overview line is shorter than this text because of `pattern-entry-line-cap` and the overview's headroom.

- The refusal is a `*PausedError` wrapping `ErrPaused`, a sentinel of its own that never wraps `ErrSessionBusy`; it carries the judged slug and the covering entry, so a caller waits on the slug the refusal judged.
- A refused send types nothing, records nothing and never waits.
  Keys stay outside it: an interrupt, an inject and the pause's own Esc are key deliveries under `PATTERN-pane-text-injection`.
- The tender and creel runners are told the hub's pause geometry with an empty slug, so each send judges its target by the target's own strand name; a prime name reads the empty slug, which only a hub entry covers.

## What each caller does with `ErrPaused`

- The gate loop holds its boundary and its re-prompt.
- The run start and the deferred prompt wait out the pause and resend.
- The seat notice sender retries.
- Creel holds the inject and its queued commands.
- Tender resume claims the session into the refusing pause.
- Tender revive's conditioned send reports the refusal as a send error.
- The start's segment-color line is logged and dropped.
- `lyx loom resume` and `lyx shuttle send` refuse with the pause's way forward.

## Enforcement

Review discipline and the shuttle send tests; no scan enforces it.
