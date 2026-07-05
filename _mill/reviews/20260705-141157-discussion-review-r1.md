MILL_REVIEW_BEGIN
# Review: Build internal/shuttle: one LLM agent via a swappable engine

```yaml
verdict: GAPS_FOUND
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-07-05
```

## Findings

### [GAP] Empty OutputFiles collapses done/asking
**Section:** Decisions § Outcome classification
**Issue:** With `spec.OutputFiles` empty, `done` = Stop + (vacuously all-files-exist) = Stop alone, so `asking` becomes unreachable; whether `run` requires at least one output file (CLI flag) is unspecified.
**Fix:** State whether OutputFiles is mandatory, and define the classification when it is empty (e.g. treat every Stop as `asking`, or reject empty).

### [NOTE] Orphan-sweep can delete an in-flight run
**Section:** Decisions § Run directory and cleanup
**Issue:** Sweeping run dirs whose guid is "not in mux.json" will delete a concurrently-starting run's dir in the window after `run.json` is written but before `AddStrand` persists the strand.
**Fix:** Guard the sweep (dir mtime/age threshold, or only sweep dirs older than startup_timeout) so a just-created run is never reaped.

### [NOTE] send after `asking` has no waiter across processes
**Section:** Decisions § In-agent interrupt
**Issue:** Once blocking `run` returns `asking` (Stop received, strand kept), a later CLI `send <guid>` from another terminal injects keys but no process re-enters the wait loop to classify the next outcome.
**Fix:** Clarify that send/interrupt are only meaningful against a live blocking run, or note there is no re-wait path for an already-returned `asking`.

## Verdict
GAPS_FOUND
Verdict contract underspecified for empty OutputFiles; two concurrency/lifecycle notes.
MILL_REVIEW_END
