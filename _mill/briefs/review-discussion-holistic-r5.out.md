MILL_REVIEW_BEGIN
# Review: fabric: unify warp + weft into one git-coordination module

```yaml
verdict: GAPS_FOUND
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-07-25
```

## Findings

### [GAP] RevertWithWeft resets warp before resolving weft target
**Section:** Decisions / RevertWithWeft
**Issue:** The decision says "warp is reset to warpSHA first, then weft to the corresponding point" with "error only when no older correspondence exists" — but if the lookup runs after the warp reset (as the design sketch's steps 1→2→3 do), the no-correspondence error path leaves warp already hard-reset, and a failure of the second (weft) reset leaves the pair inconsistent with no stated rollback (warp's own `Checkout` is explicitly all-or-nothing with host-rollback-on-weft-failure).
**Fix:** State that correspondence resolution (exact / gap-with-range / no-older → error) happens before any reset so the error path mutates nothing, and specify the partial-failure posture if the second reset fails (mirror Checkout's rollback or document leave-and-report).

### [NOTE] Clone "exactly replicates warp" vs the branch-naming delta
**Section:** Decisions / Clone: full parity
**Issue:** Clone is said to replicate warp's CloneHub "behavior exactly," yet the Branch-naming decision makes clone establish `main-weft` (warp clones mirrored same-name branches), so clone is not exact — it carries the one deliberate delta.
**Fix:** Cross-reference the two decisions so a plan writer reads "exactly" as scoped to the three-repo/teardown/resolvedBoardURL parity, with the weft primary checked out on `<host>-weft`, normalized by the differential clone test.

## Verdict

GAPS_FOUND
One ordering/partial-failure gap in RevertWithWeft; otherwise a mature, well-grounded discussion.
MILL_REVIEW_END