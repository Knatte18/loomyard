MILL_REVIEW_BEGIN
# Review: invariants and docs for the told-geometry rule

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude Opus 5 (Anthropic)
reviewed_file: _mill/discussion.md
date: 2026-08-18
```

## Findings

### [BLOCKING:design] "never enters tier 1" is false in source
**Section:** Decisions › Told-Geometry Invariant — content, point 2
**Issue:** `preflight.ResolveMode` calls `lyxcwd.Resolve(cwd)` unconditionally (`internal/preflight/predicates.go:111`), and on a plain git repo run from its root `Resolve` *succeeds* and standalone mode is chosen at `boardLyxPresent` (`predicates.go:113-116`) — so a standalone producer invocation does enter tier 1, it merely does not require it to succeed; the same discussion's point 4 makes `ResolveMode` the mandatory trigger, so the two points contradict.
**Fix:** restate point 2 as "a standalone invocation requires none of the three tiers — `ResolveMode` attempts tier 1 and degrades rather than failing", so the invariant does not ship a claim its own mechanism disproves.

### [BLOCKING:consistency] Machine-enforced predicate ignores direct-vs-transitive
**Section:** Decisions › Enforcement basis — named honestly, per package
**Issue:** The membership predicate says a bound package "imports `internal/lyxcwd` in production not at all", yet two of the six machine-enforced entries reach it transitively — `internal/treadleengine` via `logger`/`shuttleengine` (a fact `CONSTRAINTS.md`'s Treadle Runner-Seam Invariant states outright) and `internal/pattern` via `stencilstore` → `logger`; both guards police direct imports only.
**Fix:** scope the predicate and the "genuinely excludes" phrasing to *direct* production imports, matching how the Treadle and Shed invariants already word it.

### [BLOCKING:consistency] Scope and Q&A contradict the overview.md decision
**Section:** Scope › In (bullet 3) and Q&A log (docs/overview.md entry)
**Issue:** Both say "the three missing packages added to the shared-infrastructure list", while the `docs/overview.md` decision explicitly forbids that ("Do **not** add any of the three to the shared-infrastructure parenthetical") on import-graph grounds; a plan writer reading Scope first would implement the rejected option.
**Fix:** reword both the Scope bullet and the Q&A answer to "a separate sentence after the shared-infrastructure sentence, not an entry in it".

### [BLOCKING:consistency] Review-obligation set counted as ten, enumerated as eight
**Section:** Decisions › Enforcement basis; Q&A log; Testing › Manual review obligations
**Issue:** The enumerated review-obligation list has eight members (`planparser`, `configengine`, `shuttleengine`, `reedengine`, `burlerengine`, `perchengine`, `websterengine`, `scoutengine`), but the Q&A and the Testing section both say "ten" — and adding the three predicate-bound outsiders (`batcher`, `stencilstore`, `shedadapters`) would give eleven, so neither reading reconciles.
**Fix:** fix the count to eight in both places, or state explicitly which additional packages the "ten" includes.

### [BLOCKING:decision] Config Strictness spec paragraph has no disposition
**Section:** Decisions › The Config Strictness set-equality guard
**Issue:** The decision flips the **Enforced by** line and keeps the blind-spot bullets, but says nothing about the invariant's long inherited-specification paragraph (`CONSTRAINTS.md:539-541`, "T10 named as its home" plus the guard's full shape), which becomes historical narrative in a file whose own register bans that.
**Fix:** state whether that paragraph is deleted, compressed to a one-line pointer at the new test, or kept verbatim.

### [NIT:consistency] "Leave alone" list names two excluded packages
**Section:** Decisions › `doc.go` audit
**Issue:** The audit's subject set explicitly excludes the two geometry adapters, yet `hubgeom` and `standalonegeom` appear in the same decision's "where told-geometry prose already exists … leave it alone" list.
**Fix:** drop both names from the leave-alone list, since a non-subject package cannot be left alone by an audit that never visits it.

## Verdict

REQUEST_CHANGES
One source-contradicted claim, three internal contradictions, one undisposed artifact.
MILL_REVIEW_END
