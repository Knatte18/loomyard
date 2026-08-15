MILL_REVIEW_BEGIN
# Review: Shed: outer phase-FSM skeleton

```yaml
duration_s: 170.0
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-15
```

## Findings

### [BLOCKING:design] External writer's locking obligation unstated
**Section:** `field-ownership-split` / `reread-and-merge-persist` / Technical context (`state.UpdateJSON` bullet)
**Issue:** The merge-safety claim ("holds one exclusive lock across read, mutate, and write, which is what makes the merge safe against a concurrent external writer") is stated unconditionally, but `internal/state`'s lock is advisory and keyed on the caller-supplied `lockPath` (`state.go:116`) — it defends only against a writer that takes the *same* `StatusLockPath`. An outside actor setting `pause_requested: true` with a plain `os.WriteFile`, or with a different lock path, can still be clobbered by Shed's persist (and can still clobber Shed's), so the whole-file-clobber hazard `reread-and-merge-persist` exists to close is only closed for lock-cooperating writers. Nothing in the file pins this obligation, while the symmetric `ShedProducer` obligations (cancellation-as-error, only `Done`/`Stuck`) *are* pinned.
**Fix:** Add a decision stating the external-writer contract — any actor writing the status file must go through `state.UpdateJSON`/`WriteJSON` with the same `StatusLockPath` Shed is told — record it in `shed.md`'s status-file section and `shedengine`'s package doc, and qualify the merge-safety sentence accordingly.

### [NIT:decision] Unrecognised persisted `state` value has no disposition
**Demoted-from:** BLOCKING
**Section:** `state-type-and-values` / `terminal-state-on-completion-and-rerun`
**Issue:** `state` is a string type read from a file an external actor seeds, and the design pins behaviour for exactly four of its five values (`done` short-circuits; `blocked`/`failed`/`running` proceed). A value outside the five — a typo, or the `""` a partial seed leaves — has no stated disposition, so a plan writer may hard-error ("never guess a status", the precedent `unknown-current-producer-hard-error` and `unrecognised-outcome-is-an-engine-error` both set for an open string type) or silently proceed as if `running`. Both are defensible from this document.
**Fix:** Pin one disposition explicitly — most consistently, a hard error at the read gate for any `state` outside the five, with `""` either allowed as "unstarted" or rejected, said outright — and add the corresponding test scenario.

### [NIT:decision] `error`/`activity.wait` text unpinned for the no-`OnStuck` block
**Section:** `state-and-error-fields` / `activity-mechanical-fill`
**Issue:** The exhaustion cause pins its text verbatim (`error: "bounce budget exhausted"`), but the other `blocked` cause — `Stuck` with no `OnStuck` target — leaves `error` and hence `wait` as "a short reason", while the testing section asserts `Reason` is set; that is the same unassertable looseness `activity-mechanical-fill` rejected for `last`.
**Fix:** Pin the no-target `blocked` reason string exactly, as the exhaustion string already is, and state that `Result.Reason` and the persisted `error` carry the identical text.

## Verdict

REQUEST_CHANGES
Two open decisions: external-writer lock contract, and unrecognised persisted `state`.
_Note: 1 finding(s) demoted from BLOCKING to NIT by the stage's blocking-class ceiling; current blocking_count is 1._
MILL_REVIEW_END
