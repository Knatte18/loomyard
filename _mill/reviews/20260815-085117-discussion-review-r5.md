MILL_REVIEW_BEGIN
# Review: Shed: outer phase-FSM skeleton

```yaml
duration_s: 158.0
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: /home/knatte/Code/loomyard/wts/shed/_mill/discussion.md
date: 2026-08-15
```

## Findings

### [BLOCKING:design] Strictness rationale rests on a false premise
**Section:** `strictness-is-scoped-to-the-read-gate` **Issue:** The acceptance argument — an unknown top-level key dropped on the lenient merge "would be rejected by the very next iteration's strict read anyway" — is false in the only window where leniency applies: a key added by an external writer *after* step 1's strict read is stripped by step 5's `writeJSONUnlocked` full-struct marshal (`internal/state/state.go:127-132`, `:47-54`), so the next iteration reads a clean file and never rejects anything; a key present *before* step 1 already hard-errors at the gate. **Fix:** Restate the acceptance on the true ground (silent drop of a key outside `product`, deemed acceptable because `product` is the sanctioned external channel) and say explicitly that such a key is destroyed, not surfaced.

### [BLOCKING:design] Persist-failure test's key assertion is unobservable
**Section:** Testing, "Persist failure" **Issue:** The chosen injection (producer replaces `StatusPath`'s parent with a regular file) destroys the write target, so a hypothetical compensating `state: "failed"` write would fail identically to the persist itself — the named assertion "no `state: 'failed'` write happened" cannot distinguish "Shed did not attempt it" from "the attempt also failed", the same unstageability already conceded for the byte-identical assertion. **Fix:** State what the test actually proves under this injection (non-nil error, no status file re-created) and either drop the no-compensating-write assertion or name an injection that makes the second write observable.

### [NIT:design] Done short-circuit's position in the six-step loop unstated
**Section:** `terminal-state-on-completion-and-rerun` **Issue:** "Returns `RunDone` immediately" reads as pre-lookup, while the same decision's rationale (an empty `current_producer` "would fail the *next* run's step-2 lookup") only holds if the lookup runs first — so a `done` file whose producer list changed since either hard-errors or returns `RunDone`, undecided. **Fix:** Pin the check's exact position relative to steps 1-3 and state the resulting behaviour for a `done` file with an out-of-list `current_producer`.

### [NIT:scope] Status-file and history Go types unnamed
**Section:** Scope ("the status-file Go type"), `run-entrypoint-result` **Issue:** `HistoryEntry` is referenced in `Result` but declared nowhere in `shed.md`'s pinned Go blocks, and the persisted status type has no name anywhere; only the JSON example pins their fields. **Fix:** Name both types and their JSON tags in the `docs-and-roadmap` reconciliation list, as the other struct shapes already are.

## Verdict

REQUEST_CHANGES
Two rationales/assertions contradict verified source behaviour; the rest is sound.
MILL_REVIEW_END
