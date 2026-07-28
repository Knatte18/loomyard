MILL_REVIEW_BEGIN
# Review: PATTERN wiring: conditional constraint-injection into every agent

```yaml
verdict: APPROVE
reviewer_model: sonnetmax
reviewed_file: C:\Code\loomyard\wts\pattern-wiring\_mill\discussion.md
date: 2026-07-28
```

## Findings

### [NOTE] checkout omitted from Short/Long re-read list
**Section:** Constraints (CLI/Cobra Invariant bullet)
**Issue:** `fabric checkout` (`checkout.go:152`) is a third `WireJunctions` caller — named elsewhere in this same doc, under weft-target-materialisation — but the Constraints section's explicit list of commands needing a Short/Long re-read names only init/undo/reconcile/status/remove, not checkout.
**Fix:** Add `lyx fabric checkout` to that list for completeness. Verified against `fabriccli/fabric.go`: its current Short/Long already speak generically of "junctions" (plural, e.g. "re-pointing junctions in the same operation"), so this is likely a re-read-and-confirm, not a wording change.

### [NOTE] removeHostJunction's generalised loop error policy is unstated
**Section:** remove-tears-down-every-junction
**Issue:** Unlike unwire-generalisation, which explicitly pins "abort on first junction error" for its loop, this decision doesn't say whether the generalised `removeHostJunction` continues past a mid-loop failure on one junction to attempt the next. `Remove`'s call site already discards the return value (`_ = removeHostJunction(l, slug)`), best-effort like the adjacent `removePortal`/`removeLaunchers` calls in the same function, so an abort-on-first-error implementation (by naive analogy to `unseedLyxJunction`) would silently leave a later junction un-removed on an earlier one's failure — quietly undercutting the decision's own stated goal ("tears down every junction, not only `_lyx`").
**Fix:** State explicitly that the loop continues through all junctions best-effort on a per-junction error, matching the call site's existing discard-and-continue treatment.

## Verdict

APPROVE
Two low-severity completeness NOTEs; all decisions, source citations, and rejected alternatives verified accurate against code.
MILL_REVIEW_END
