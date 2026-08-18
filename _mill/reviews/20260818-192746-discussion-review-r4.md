MILL_REVIEW_BEGIN
# Review: invariants and docs for the told-geometry rule

```yaml
duration_s: 144.0
verdict: APPROVE
reviewer_model: opus
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-18
```

## Findings

### [NIT:consistency] ResolveMode has a third, non-degrading branch
**Demoted-from:** BLOCKING
**Section:** Decisions → "Told-Geometry Invariant — content", point 2
**Issue:** The text says a standalone pre-run "degrades to standalone mode on either outcome — a failed resolve, or a successful one with no board-level lyx directory (`predicates.go:113-119`)", but `ResolveMode` has a third branch the cited range stops short of: `predicates.go:123-131` returns the original `ErrCwdOutsideAnchor` (mode `0`, refuse) when cwd is a subdirectory of a wired hub worktree, and `doc.go`/comments at `predicates.go:100-101` state "a non-nil error means refuse: it is surfaced verbatim and is never degraded to standalone".
**Fix:** State three outcomes — degrade on `ErrNotAGitRepo`, degrade on a successful resolve with no board-level `_lyx`, degrade on `ErrCwdOutsideAnchor` outside a wired hub, but **refuse** on `ErrCwdOutsideAnchor` inside one — so the invariant's "requires none of the three" carries its real exception rather than a two-outcome claim that is false in source.

### [NIT:consistency] "nine packages already carry prose" vs seven enumerated
**Section:** Decisions → "`doc.go` audit", Rationale; and Q&A log entry 5
**Issue:** The decision enumerates exactly seven packages whose told-geometry prose is left alone (`shuttleengine`, `reedengine`, `pattern`, `perchengine`, `websterengine`, `planparser`, `scoutengine`), while the rationale and the Q&A answer both say "nine".
**Fix:** Reconcile the count to the enumeration (or name the two additional packages), since the enumeration is what the plan writer will execute.

### [NIT:consistency] Config Strictness deletion cites the wrong line span
**Section:** Decisions → "The Config Strictness set-equality guard", disposition paragraph
**Issue:** It cites `CONSTRAINTS.md:520-523` with "the guard-shape paragraph at line 521" and implies the known-blind-spot sentence must be rescued out of it; in the file the Enforced-by bullet spans 520-522, and the blind-spot line is already its own separate bullet at 519, untouched by the deletion.
**Fix:** Repoint the citation to 520-522 and note the blind-spot bullet at 519 already stands alone, so no rescue step is needed.

## Verdict

APPROVE
One source-inaccurate mode-resolution claim would land verbatim in CONSTRAINTS.md.
_Note: 1 finding(s) demoted from BLOCKING to NIT by the stage's blocking-class ceiling; current blocking_count is 0._
MILL_REVIEW_END
