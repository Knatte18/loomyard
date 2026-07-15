MILL_REVIEW_BEGIN
# Review: Reconsider whether lyx mux needs anchor:top at all — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opushigh
reviewed_file: plan/
date: 2026-07-15
```

## Findings

### [BLOCKING] Card 10 signature change breaks batch-1 policy_test.go
**Location:** Batch 2 card 10 (and batch 1 card 1)
**Issue:** Card 10 changes `partitionByAnchor` to a single `stack` return but its Edits list only `policy.go`; `TestPartitionByAnchor` in `policy_test.go` calls it with two returns (`top, stack := partitionByAnchor(...)` at lines 14/32/45), and card 1 keeps the non-top cases (NotLive filter, own-window) so those two-value calls survive batch 1 — after card 10 batch 2's `go test ./internal/muxengine/...` fails to compile the render test package (assignment mismatch). The "disjoint file set" Shared Decision is therefore inaccurate.
**Fix:** Add `internal/muxengine/render/policy_test.go` to card 10's Edits and migrate `TestPartitionByAnchor` to the single-return signature (keep the exclusion-filter + own-window cases as below-parent-only), or move that test's rewrite into batch 2.

### [MAJOR] resequenceByPaneOrder loses all direct test coverage
**Location:** Batch 1 card 1 (interacts with batch 2 card 11)
**Issue:** Card 1's blanket "no `render.AnchorTop` reference may remain" forces deletion of `TestRulesPaneOrderResequencesCellsToPhysicalOrder` and `TestRulesPaneOrderUnknownIDsKeepIntendedTailOrder` (both built entirely on `AnchorTop` fixtures), yet `resequenceByPaneOrder` is explicitly retained unchanged by card 11 — leaving that positional-reordering logic untested.
**Fix:** Card 1 should re-express both paneOrder tests with below-parent fixtures (e.g. parent+child stack with an inverted `paneOrder`) rather than delete them.

### [NIT] Card 1 cites height_test.go symbol not in Context
**Location:** Batch 1 card 1
**Issue:** Requirements name `height_test.go`'s `TestStackHeightsActiveStrictlyTallestWithSingleAncestor`, but `height_test.go` is not in the card's Context (only `height.go` is).
**Fix:** Add `height_test.go` to Context, or drop the specific test-name reference (it is only a do-not-duplicate note).

### [NIT] config_test.go assertion block line range off by one
**Location:** Batch 1 card 5
**Issue:** The `cfg.TopBandRows != 3` block spans lines 70-72, not "70-71" as stated.
**Fix:** Correct the cited range; the assertion is otherwise unambiguously identified.

## Verdict

REQUEST_CHANGES
A batch-2 signature change breaks a batch-1 test file, violating per-batch green build.
MILL_REVIEW_END
