I have enough to complete the review. Let me compile findings.

The plan is thorough and well-grounded against the source. I verified: `WhereFiles`/`CardCount` have no external consumers (card 1's claim holds); the template/CLI help strings the plan targets exist verbatim; the stencil marker set matches; `pathCovers` lives in digest.go (in card 12's Context); the Batch Index DAG is acyclic with all files present; global card numbering is sequential 1-18; every card's Requirements name stable identifiers all reachable via Context/Edits. Two real gaps surfaced.

MILL_REVIEW_BEGIN
# Review: Add typed file-ops to lyx's plan-format — holistic

```yaml
verdict: APPROVE
reviewer_model: opushigh
reviewed_file: plan/
date: 2026-07-12
```

## Findings

### [NIT] Stale "plan-format v1" godoc left in two files
**Location:** Batch 1 (All Files Touched) / batches 4-5
**Issue:** `internal/builderengine/doc.go:2` ("drives a pinned plan-format v1 plan") and `internal/builderengine/fingerprint.go:21` ("only markdown plan files carry plan-format v1 content") are in-package format references the discussion Scope promises to update, but no card edits them and neither is in "All Files Touched".
**Fix:** Add both files to a card's Edits (natural fit: batch-1 card 1's banner-comment sweep) and retarget the "v1" wording to v2/version-neutral.

### [NIT] plan-unapproved fixture must stay card-check-clean for the exact-count test
**Location:** Batch 1 card 4 (fixtures) vs. batches 2-3
**Issue:** `TestValidate_PlanUnapproved_TripsCheck1` asserts `len(findings)==1` exactly; after batches 2-3 add card-* checks, the rewritten `plan-unapproved` fixture must produce zero card-check findings (all-`none` fields, correct `NN.C`, matching `(C cards)`) or that test breaks. Card 4 says only "v2 syntax, minimal change" without flagging the exact-count constraint.
**Fix:** State in card 4 that `plan-unapproved`/`plan-broken-chain` must remain fully v2-valid so only their designed finding fires (self-caught by batch verify, but worth pinning).

## Verdict

APPROVE
Solid, source-grounded plan; two minor doc/fixture omissions worth folding in.
MILL_REVIEW_END
