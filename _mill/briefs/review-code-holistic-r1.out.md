MILL_REVIEW_BEGIN
# Review: Add a local-only file category to weft — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-08-27
```

## Findings

### [BLOCKING:consistency] cleanup.go's deleteWeftBranch doc still cites the removed raddle gate
**Location:** `internal/fabricengine/cleanup.go:249-253`
**Issue:** `deleteWeftBranch`'s doc comment says "force is always false here even when Topology.Cleanup was called with force true: --force there answers the folded-back-raddle gate above, evaluated before this call is reached" — but card 18, in this same file, deleted `raddleFoldedBack` and the `Protected = true` branch it fed; `Topology.Cleanup`'s own doc comment (correctly updated) now says force "is reserved and currently consulted by no gate in this verb." This leftover sentence asserts a gate that no longer exists in the file it sits in.
**Fix:** Rewrite the sentence to state that `force` is always false here because `deleteWeftBranch`'s own request never had a force-answerable gate to begin with — `Topology.Cleanup`'s `force` parameter is reserved and unconsulted by any gate in this verb, matching the struct's own doc comment.

### [BLOCKING:consistency] landingdeps.go's CommitStatus comment describes pre-task behavior as current
**Location:** `internal/loomcli/landingdeps.go:53-58`
**Issue:** The doc comment says the status file "is which 'lyx loom run' commits only once, as the seed" and that both landing producers call `CommitStatus` "because fabricengine's merge guard refuses any tracked modification on either side of the pair." Both claims are now false: this very task adds a per-transition `Shed.CommitStatus` seam (`internal/loomcli/wiring.go`) that commits the status file on every producer transition, not once as a seed, and batches 1-2 make the weft a non-merge-participant, so the merge guard no longer inspects weft-side dirt at all. `manifest/designs/loom.md`'s own rewritten landing-checkpoint paragraph (card 31) explicitly says the checkpoint is now "a no-op safety net on the ordinary path rather than the last row's only protection" for exactly this reason — this file's comment was never updated to match and now contradicts it.
**Fix:** Rewrite the comment to match `manifest/designs/loom.md`'s corrected rationale: the per-transition seam now keeps the status file current on the ordinary path, and this landing-time commit is retained only as the sole protection if a product wires `Shed.CommitStatus` as nil — not because the merge guard still inspects the weft.

### [NIT:scope] "four told values" left uncorrected in two spots after card 27 widened ShedPaths to five
**Location:** `internal/loomrecipe/loomrecipe.go:19` and `internal/loomcli/wiring.go:334`
**Issue:** Card 27 required correcting `ShedPaths`' doc comment's field count from four to five (done in the struct's opening sentence, which now lists `CommitStatus`), but the very next paragraph in the same doc comment ("These four cannot travel in shedrecipe.Env...") still says "four." `internal/loomcli/wiring.go`'s own comment on `c.shedPaths` ("carries the four told values shedengine.Shed itself reads") repeats the same stale count.
**Fix:** Update both remaining "four" references to "five" (or "the fields above") now that `CommitStatus` is a fifth told value.

### [NIT:consistency] destroy.go retains a dead pathOwnershipWeftCheckout enum case after its sole constructor was deleted
**Location:** `internal/fabricengine/destroy.go:240,421-425`
**Issue:** Card 11 deleted `ownedWeftCheckout`, the sole constructor for `pathOwnershipWeftCheckout`, stating it is "dead once that request is gone." The enum member and its `resolvePathOwnership` switch case were left behind; with no remaining constructor, no `pathOwnership` value can ever carry that kind, so the case is now unreachable dead code in a file whose whole design philosophy is a closed, fully-constructed enum with no orphaned kinds.
**Fix:** Delete the `pathOwnershipWeftCheckout` enum member and its `resolvePathOwnership` case alongside the constructor card 11 already removed.

## Verdict

REQUEST_CHANGES
Two stale doc comments assert removed behavior (raddle gate, merge-guard weft check) as current; fix before merge.
MILL_REVIEW_END
