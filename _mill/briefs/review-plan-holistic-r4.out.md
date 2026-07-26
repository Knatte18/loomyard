MILL_REVIEW_BEGIN
# Review: Treadle: shared round-loop engine + perch rewrite — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnetmax
reviewed_file: plan/
date: 2026-07-26
```

## Findings

### [BLOCKING] Card 2 maps RoundSummary/Profile fields without their file in Context
**Location:** Batch 1 (treadle-extraction), Card 2
**Issue:** Requirements says to "map treadleengine.Result onto perch's Result/RoundSummary (Verdict: burlerengine.Verdict(...))" and to call `p.validate(e.cfg)` — both require exact field names/signatures from `internal/perchengine/result.go` and `internal/perchengine/profile.go`, named only in passing ("profile.go and result.go are deliberately untouched") but absent from Card 2's Context/Edits.
**Fix:** Add `internal/perchengine/result.go` and `internal/perchengine/profile.go` to Card 2's Context list.

### [BLOCKING] Card 14's doc.go coverage list outruns its Context
**Location:** Batch 5 (docs-lifecycle), Card 14
**Issue:** `treadleengine/doc.go` must accurately cover gate/ladder/stale-move/token-naming/retry-triage mechanics and name-parameterized diagnostics, but `judge.go`, `gate.go`, `roundfiles.go`, `state.go`, and `engine.go` — where that logic actually lives — are all absent from Card 14's Context (only run.go, runner.go, profile.go, handoff.go, targeting.go are listed).
**Fix:** Add `internal/treadleengine/{judge.go,gate.go,roundfiles.go,state.go,engine.go}` to Card 14's Context.

### [NIT] Treadle's seam allowlist admits an import no moved file uses
**Location:** Batch 1, Card 4 (seam_enforcement_test.go / CONSTRAINTS.md entry)
**Issue:** The allowlist (and the no-burler-import Decision) permits `internal/hubgeometry`, but none of run.go/judge.go/judgeverdict.go/state.go/gate.go/roundfiles.go actually import it today, which sits oddly against the stated "geometry-blind, constructs no _lyx paths" design goal.
**Fix:** Drop `internal/hubgeometry` from the allowlist/CONSTRAINTS.md wording unless a concrete need is identified.

### [NIT] Overview's "All Files Touched" omits the deleted design doc
**Location:** 00-overview.md, "All Files Touched" (relates to Card 14)
**Issue:** Card 14 deletes `manifest/designs/treadle.md`, but that path is not listed anywhere in 00-overview.md's "All Files Touched" summary.
**Fix:** Add `manifest/designs/treadle.md` to that list.

## Verdict

REQUEST_CHANGES
Two Context-completeness gaps (Card 2, Card 14) must be fixed; two NITs are optional polish.
MILL_REVIEW_END
