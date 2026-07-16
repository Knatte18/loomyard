MILL_REVIEW_BEGIN
# Review: Built-in operator console pane in mux

```yaml
verdict: APPROVE
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-07-16
```

## Findings

### [NOTE] repo "always non-empty" ignores empty-Prime path
**Section:** three-part text pipeline (item 2) / Technical context (Hub root)
**Issue:** `Resolve` leaves `Prime` = "" when `List` returns no `Main` entry, and `filepath.Base("")` is `"."` — non-empty but a wrong repo name, contradicting the "Always non-empty" claim (verified `hubgeometry.go:126-142`).
**Fix:** State the plan's expectation that a resolved Layout always has a Main worktree, or have the `Repo` setter guard the empty-Prime case explicitly.

### [NOTE] tokenvocab leaf/acyclic direction is review-only, unlike peers
**Section:** Dependency graph / Constraints
**Issue:** `stencil` staying a pure leaf and `tokenvocab → {hubgeometry, stencil}` only are asserted, but no machine-enforced import test is planned, whereas `modelspec`/`lyxtest` leaf invariants each ship an enforcement test — and tokenvocab is explicitly "general and shared" (loom will import it), inviting future import creep.
**Fix:** Decide whether tokenvocab warrants a leaf-enforcement test + CONSTRAINTS.md entry, or note that review-obligation discipline is deliberate here.

## Verdict

APPROVE
Scope, decisions, failure modes, and testing are all resolved; only two non-blocking notes remain.
MILL_REVIEW_END
