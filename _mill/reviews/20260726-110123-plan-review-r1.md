MILL_REVIEW_BEGIN
# Review: fabric: cutover -- rewire consumers onto fabric, delete warp/weft — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opushigh
reviewed_file: plan/
date: 2026-07-26
```

## Findings

### [BLOCKING] Comment sweep under-scoped vs. card 27 zero-diff gate
**Location:** Batch D3 cards 23/25/26 + card 27; Batch A cards 1/2
**Issue:** Card 23 rewords "Adapted from warpengine's X.go" provenance in only 6 fabricengine files, but ~17 other non-test fabricengine files carry identical deleted-module comments (`remove.go`, `list.go`, `portals.go`, `ancestors.go`, `checkout.go`, `config.go`, `hostclean.go`, `add.go`, `status.go`, `reconcile.go`, `launchers.go`, `launcher_content.go`, `weftwiring.go`, `prune.go`, `topology.go`, `junction.go`, `drift.go`), plus `initengine/init.go:41` and `undo.go:7,35,38,64`; none are in any card's Edits, yet card 27's Tier-2 `grep -rnw 'warpengine|weftengine' --include='*.go'` flags them all while card 27 is declared "zero diff / Commit: none".
**Fix:** Expand card 23's Edits (and add init.go/undo.go to a batch-A/D3 card) to sweep every fabricengine file bearing a `warpengine`/`weftengine` provenance/mirror comment, so card 27 genuinely stays zero-diff.

### [NIT] Context omits fabricengine files defining named helpers
**Location:** Batch A cards 1, 2, 4, 5; Batch B card 12
**Issue:** Cards name `WireJunctions`/`UnwireJunctions` (junction.go), `HostClean` (hostclean.go), `PairInSync` (drift.go), `NewTopology`/`Topology.Add`/`AddOptions` (topology.go, add.go), but Context lists only `fabricengine/fabric.go` (+weft_verbs.go); the shared "signature gotchas" decision supplies the mappings, so cold-start risk is low but the Context field is technically incomplete.
**Fix:** Add the defining files (junction.go, hostclean.go, drift.go, topology.go, add.go) to the relevant cards' Context, or note explicit reliance on the Shared Decisions mapping.

### [NIT] Wrong module count in batch B narrative
**Location:** Batch B scope ("thirteen modules to eleven"); card 11 note
**Issue:** `configreg.Modules()` currently has 12 rows, so dropping warp+weft yields 10, not "thirteen to eleven"; the card mechanic ("delete two rows / drop by two") is correct, only the stated count is off.
**Fix:** Correct the narrative count to 12 → 10.

## Verdict

REQUEST_CHANGES
Card 27's zero-diff gate is unreachable: the comment sweep leaves many deleted-module references unswept.
MILL_REVIEW_END
