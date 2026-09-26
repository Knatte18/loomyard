MILL_REVIEW_BEGIN
# Review: Loom persists done only after post-run friction reflection

```yaml
duration_s: 120.5
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewed_file: _mill/discussion.md
date: 2026-09-26
```

## Findings

### [BLOCKING:design] New paused-after-landing state is unaddressed
**Section:** Decisions → "Mechanism: a terminal recipe row" / "Row always returns Done".
**Issue:** `stepLocked`'s step-3 pause check (`internal/shedengine/run.go`) runs before every producer call, so a `lyx loom pause` requested during `Finalize` now persists `state: paused` at `Friction-Reflect` on an already-landed task, and batten's `innerRunProducer.Call` treats `StatePaused` as a hard error. Today `Finalize`'s Done persists `done` and bypasses the flag.
This contradicts the discussion's own rationale that a landed task must never read halted to batten, and it applies even with Tier 2 off, where the row is a no-op.
**Fix:** State the disposition explicitly: either accept it (document that resuming the child finishes the run, and that batten must then be resumed) or rule it out, with the reasoning, plus a test pinning whichever behaviour is chosen.

### [NIT:scope] LoomFrictionLock doc missing from the reword list
**Section:** Technical context → "Comments that describe reflection as running after the run lock is released".
**Issue:** `internal/loomengine/config.go`'s `LoomFrictionLock` doc also says "the reflection fires after that return, so for the whole of the reflection agent's life the run lock reads as free", but the list omits it.
**Fix:** Add it to the list and reword it to refer to the blocked path only.

### [NIT:decision] Where the nil-closure refusal lives is left open
**Section:** Decisions → "How the row reaches loomcli's reflection".
**Issue:** "either in the `"FrictionReflect"` registry entry or in the producer's own constructor" leaves two alternatives for the plan writer to pick.
**Fix:** Name one site. `publishEntry`'s delegate-to-constructor split is the cited precedent, which points to the constructor.

## Verdict

REQUEST_CHANGES
The pause-during-Finalize window creates a paused state on a landed task that batten rejects, and no decision covers it.
MILL_REVIEW_END
