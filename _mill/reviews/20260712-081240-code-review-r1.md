I have gathered enough evidence. Here is the final holistic review.

MILL_REVIEW_BEGIN
# Review: Add typed file-ops to lyx's plan-format — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-07-12
```

## Findings

### [BLOCKING] buildercli validate's Long text enumerates only the pre-existing 6 checks
**Location:** `internal/buildercli/validate.go:55-58`
**Issue:** The `Long` text reads "runs every plan-format v2 machine check against it" and then parenthesizes exactly the original six check names (format/approval, index consistency, verify-missing, chain-end-dangling, batch-oversized, scope-malformed) — omitting all twelve `move-*`/`card-*`/`path-missing` checks batches 2-3 added. Grep confirms none of the new check names appear anywhere in this file. This reads as an exhaustive list and is now stale relative to the code's actual behavior; CONSTRAINTS.md's CLI/Cobra Invariant calls stale help exactly this kind of review-blocking defect when observable behavior changed (which it did: `Validate` now runs 18 checks, not 6).
**Fix:** Either drop the parenthetical enumeration entirely (the count-neutral `Short` already avoids the problem) or replace it with a non-exhaustive phrasing ("e.g. format/approval, verify presence, the move-*/card-* structural checks, ...").

## Notes (non-blocking)

- `internal/builderengine/testdata/plan-valid/01-json-flag.md:2` sets `root: .` to satisfy card 4's "one batch with root: set" fixture requirement, but `normalizeCardPath`'s degenerate `root == "."` case is a no-op (behaves identically to no `root:` at all per `plan.go`'s own doc comment), so the fixture doesn't exercise genuine root-joining — only the `//` escape. Real root-joining is separately covered by `plan_test.go`'s `TestParsePlan_RootNormalization`, so this is not a coverage gap, just a slightly weaker fixture demonstration than the worked example in `plan-format.md` (which correctly uses `root: internal/boardcli`).

## Verdict

REQUEST_CHANGES
Excellent, thorough, self-consistent implementation across all five batches; one stale-help finding blocks per CONSTRAINTS.md's help-accuracy rule.
MILL_REVIEW_END
