MILL_REVIEW_BEGIN
# Review: PATTERN wiring: conditional constraint-injection into every agent — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-07-28
```

## Findings

### [BLOCKING] docs/shared-libs/hubgeometry.md describes the pre-flip, single-junction `HostJunctions`
**Location:** `docs/shared-libs/hubgeometry.md:124-125`
**Issue:** The "Junction detection methods" section still reads `HostJunctions(slug string) []HostJunction — the list of host junctions (currently _lyx; _pattern follows in a later batch)…` and describes `HostJunctionsHere()` as deriving only from `HostLyxLinkHere()`/`WeftLyxDir()`. Since batch 5's card 15, `internal/hubgeometry.go`'s `HostJunctions`/`HostJunctionsHere` both return two entries (`_lyx` and `_pattern`) — this doc now flatly contradicts the shipped code and misleads any reader consulting it after the flip landed.
**Fix:** Update both bullets to state two entries (`_lyx` then `_pattern`), matching `HostJunctions`'/`HostJunctionsHere`'s current godoc, per this repo's Documentation Lifecycle rule that docs land in the same commit as the change that invalidates them (batch 5's card 15 edited `hubgeometry.go`/`hubgeometry_test.go`/`weft_test.go` but never this doc file).

## Missing context

(none — no NEED_CONTEXT)

## Verdict

REQUEST_CHANGES
One shared-lib doc (`hubgeometry.md`) still describes the pre-flip single-junction `HostJunctions`, contradicting shipped code.
MILL_REVIEW_END
