MILL_REVIEW_BEGIN
# Review: self-report Tier 2: per-agent friction notes for unsupervised runs

```yaml
duration_s: 138.0
verdict: REQUEST_CHANGES
reviewer_model: opus
reviewer_self_id: Claude (Opus-class), Anthropic
reviewed_file: _mill/discussion.md
date: 2026-09-12
```

## Findings

### [NIT:scope] Seed-time clearing is outside the In list
**Demoted-from:** BLOCKING
**Section:** §Scope In vs §Decisions "Consumed notes are archived…"
**Issue:** The decision puts the once-per-task clear in `loomshed.Seed`, but `Seed(statusPath, statusLockPath, slug, parent string)` (`internal/loomshed/seed.go:37`) is told no friction path, its sole production caller is `internal/loomcli/run.go:101`, and neither `internal/loomshed` nor `run.go` appears in the Scope In inventory or the Testing section. `lyx loom drive` never calls `Seed` at all, so a drive-only task never clears.
**Fix:** State where the clear actually lands (Seed with a new told parameter, or beside the Seed call in `run.go`), add it to the In list with its test, and say what the drive-only path does.

### [BLOCKING:design] Failure-path disposition of the notes is a TBD
**Section:** §Testing `internal/frictionengine`
**Issue:** "the notes are still archived or still present per whichever the decision fixes — pin it either way" leaves the choice to the plan writer, yet the two branches differ behaviourally: archiving after a `OutcomeTimeout` loses the notes the next run would reflect on, leaving them re-files them (and leaves a possibly half-written report file in the directory, where the next scan counts it as a note).
**Fix:** Decide the failure-path behaviour in §Decisions, including whether the agent's own report file is excluded from the `*.md` note scan.

### [BLOCKING:design] Marker never reaches an already-seeded stencil
**Section:** §Scope In (marker injection) / §Constraints (Stencil Ownership)
**Issue:** `stencilstore.reconcileOne` refreshes `StateUntouched` only in prod mode (dev mode warns and keeps the older copy) and never refreshes `StateEdited`. An existing worktree whose five stencils are operator-edited, or a dev build, keeps templates with no `{{.friction_directive}}`; `FillOptional` ignores an unused values key, so Tier 2 silently produces zero notes with no signal. The discussion addresses `loom.yaml` key migration but not this one.
**Fix:** State the disposition — accepted silent degradation, or a detectable signal (e.g. the composer noting a directive computed but not rendered) — in §Decisions.

### [NIT:consistency] `friction` envelope vocabulary has no "ran, filed nothing" value
**Section:** §Decisions "The reflection step can never change the run's outcome"
**Issue:** The values are `"skipped"`/`"filed"`/`"failed"`, but nothing in Go parses the agent's report file (§Decisions "freeform markdown with no parser"), so `"filed"` is asserted for a run that spawned and decided nothing was worth filing.
**Fix:** Rename the value to something Go can actually observe (e.g. `"reflected"`), or say explicitly that `"filed"` means "reflection ran".

## Verdict

REQUEST_CHANGES
Seed-clear scope gap, undecided failure path, and unaddressed stencil-refresh degradation.
_Note: 1 finding(s) demoted from BLOCKING to NIT by the stage's blocking-class ceiling; current blocking_count is 2._
MILL_REVIEW_END
