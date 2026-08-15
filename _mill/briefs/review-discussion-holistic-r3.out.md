MILL_REVIEW_BEGIN
# Review: Shed: outer phase-FSM skeleton

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-15
```

## Findings

### [BLOCKING:design] One persist per iteration or two?
**Section:** `reread-and-merge-persist` + `state-and-error-fields`
**Issue:** Step 5 persists the history append; step 6 writes `state` and advances `current_producer` — the discussion never says whether these are one `UpdateJSON` or two, so a crash between them re-calls the producer and appends a duplicate `history` entry, defeating the step-5 crash-safety claim.
**Fix:** Pin that routing is decided before a single step-5/6 persist (history append + `current_producer` + `state` in one `UpdateJSON` mutate), or state explicitly that a duplicate history entry on crash is accepted.

### [BLOCKING:consistency] "Nothing created on disk" test contradicts run-lock
**Section:** `## Testing`, missing-status-file scenario
**Issue:** The bullet asserts a missing status file leaves "nothing created on disk", but `run-lock` establishes that `Run` `MkdirAll`s both lock parents and creates both lock files before step 1 — so the assertion as written fails; the sibling run-lock-held bullet uses the correct "status file is untouched" wording.
**Fix:** Reword that scenario (and any other "nothing on disk") to assert only that the status file is not created, matching `run-lock`'s own wording correction.

### [BLOCKING:scope] shed.md edit list is under-enumerated
**Section:** `docs-and-roadmap`
**Issue:** It names exactly "three corrections" to `manifest/designs/shed.md`, but at minimum the `product` field is absent from shed.md's status-file JSON example (shed.md:150-162) and `run-lock` separately mandates a fourth correction (shed.md:146's "nothing on disk touched"); the terminal `current_producer`/done-short-circuit and the `MaxBounces` default of 10 are also unstated there.
**Fix:** Replace the closed "three corrections" list with the full inventory of shed.md edits this discussion's decisions imply, or state that the list is illustrative and shed.md must be reconciled against every decision above.

### [NIT:design] Bounce budget's cross-`Run` scope unstated
**Section:** `total-bounce-budget`
**Issue:** The counter is implicitly in-memory per `Run` — the status shape carries no bounces-used field — so a crash-restart or a resumed `blocked` run silently restores the full budget; the decision says "total across the whole run" without saying whether "run" means one `Run` call or the task.
**Fix:** State explicitly that the budget is per-`Run`-call and in-memory, and why that is acceptable.

### [NIT:consistency] Seam invariant omits treadle's transitive-lyxcwd caveat
**Section:** `shed-producer-seam-invariant`
**Issue:** The allowlist permits `internal/logger`, which itself imports `internal/lyxcwd` (`internal/logger/sink.go:21`), so "never `internal/lyxcwd`" is true only of direct imports — the Treadle invariant states this caveat outright and the new one does not; separately, no decision names any use for `logger` in `shedengine`.
**Fix:** Carry treadle's honesty clause into the new invariant's wording, and say whether `shedengine` actually logs anything or whether `logger` should be dropped from the cap.

## Verdict

REQUEST_CHANGES
Persist-ordering gap, a self-contradicting test assertion, and an incomplete shed.md edit list.
MILL_REVIEW_END
