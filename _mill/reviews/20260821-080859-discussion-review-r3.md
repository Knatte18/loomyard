MILL_REVIEW_BEGIN
# Review: Shed-setup validity checker

```yaml
duration_s: 150.0
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude Opus 5 (Anthropic)
reviewed_file: _mill/discussion.md
date: 2026-08-21
```

## Findings

### [BLOCKING:design] `unexpected-terminal` has no answer for a never-Done row
**Section:** Finding kinds → `unexpected-terminal`; "What this catches in a mis-wired perch"
**Issue:** The check requires every reachable non-terminal row to carry a non-empty `OnDone`, but a `Burler` row is specified (roadmap:36) never to return `Done`, so its author has no meaningful target — and the natural value, `OnDone: <its Bouncer>`, is precisely the wiring the discussion declares indistinguishable from the uncatchable mis-wiring.
**Fix:** State the intended disposition for a row that never returns `Done` — what value such a row is expected to set and why that does not defeat the check — so the plan writer and the three perch tasks are not left to invent it.

### [NIT:consistency] `done-cycle` bullet contradicts itself on emission
**Demoted-from:** BLOCKING
**Section:** Finding kinds → `done-cycle`
**Issue:** The bullet opens "reported as **one finding per member row**" and closes "One finding per distinct cycle, naming its members." — the second sentence is the pre-round-2 rule the round-2 Q&A (Q&A log, last entry) explicitly replaced, and it also contradicts the `done-cycle` row of the `Producer`/`Target` table.
**Fix:** Delete the trailing "One finding per distinct cycle, naming its members." sentence.

### [NIT:consistency] Kind count still says six in three places
**Section:** Finding kinds heading; Single severity; Q&A log entry 3
**Issue:** Scope names eight kinds and eight are enumerated, but the decision heading reads "six", the severity rationale says "all six checks"/"none of the six checks", and the Q&A answer still lists the superseded `bad-endpoints`.
**Fix:** Update the heading and the two "six" references to eight; the Q&A entry can stay if the later entry supersedes it explicitly.

### [NIT:consistency] Roadmap-coverage claim true for only one of three tasks
**Section:** "What this catches in a mis-wired perch" → `Burler` handing back via `OnDone`
**Issue:** The discussion claims "the roadmap's own wording for each of the three review-producer tasks already states the requirement in verdict terms"; `manifest/roadmap.md:36` (Discussion-Review) does, but the Plan-Review (:45) and Webster-Review (:50) items only say "same shape as `Discussion-Review producer` above" and never restate `Stuck`, never `Done`.
**Fix:** Soften to name the one item that states it and note the other two incorporate it by reference.

### [NIT:scope] loomshed's own seam allowlist disposition unstated
**Section:** Constraints
**Issue:** The Constraints section pins "do not add `shedcheck` to `shedengine`'s allowlist" but says nothing about `internal/loomshed/seam_enforcement_test.go`'s `loomshedAllowedImports`, which the new invariant test sits next to; it walks non-test files only, so no entry is needed — unstated, a plan writer may add one.
**Fix:** Add one line saying the new test is a test-file import and `loomshedAllowedImports` is untouched.

## Verdict

REQUEST_CHANGES
Two blockers: an unresolved `unexpected-terminal` rule for never-Done rows, and a self-contradicting `done-cycle` bullet.
_Note: 1 finding(s) demoted from BLOCKING to NIT by the stage's blocking-class ceiling; current blocking_count is 1._
MILL_REVIEW_END
