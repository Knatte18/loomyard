MILL_REVIEW_BEGIN
# Review: fabric: accumulate the result envelope from mutations, not control flow (slice 14)

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-11
```

## Findings

### [BLOCKING:design] Commission check fails on undone mutations
**Section:** `permitted-roots-and-the-oracle` (the two-direction cross-check) + Testing (`Add`/`Checkout` rollback cells)
**Issue:** The commission rule ("every manifest-observable record entry's `Target` must have at least one unfiltered-diff change at or beneath it") compares only before/after manifests, so a mutation performed and then undone inside the same call nets to zero diff while the record correctly carries entries — `Add`'s `worktree_created`/`dir_created`/`link_created` followed by `rollbackAdd`'s removals (`add.go:111` mint, rollback removes worktree, branch and junctions), `Checkout`'s `rollbackSwitch` (`checkout.go:97,110,114`), and `reconcile`'s repoint of a *dangling* link, where `repointLink`→`removeLink` then `fslink.CreateDirLink` restores the same `LinkTarget` and `DiffManifest` therefore emits no `Change` at that path at all. The two cells the Testing section names as the rollback proofs would fail on correct behaviour, exactly the failure the git-state split was introduced to avoid.
**Fix:** State the rule for mutations reverted within the call — e.g. commission applies only to entries with no later record entry restoring/removing the same `Target`, or the direction is asserted against the record's own net effect rather than the manifest diff.

### [BLOCKING:design] Nothing supplies the hub root to the gate's recorder
**Section:** `mutation-entry-shape` (`Target` convention) + `gate-auto-records` (threading)
**Issue:** `Target` is specified as hub-relative `ToSlash` "byte-identical to `CaptureManifest`'s keying... with no normalisation step", but the eight auto-recording sites in `destroy.go` receive absolute paths only — `pathRequest` carries `container` (the parent), never the hub root — and the threaded value is a bare `*Mutations`. The discussion never says who converts, nor that the accumulator carries a hub root. `CloneHub`'s `createExclusiveDir(hubPath)` (`clone.go:225`) also mints the hub root itself, whose hub-relative form is `"."` — a key `CaptureManifest` never emits (it skips `path == hubRoot`) and a `Target` under which every diff change is a segment-wise descendant, trivially satisfying the omission direction for the whole clone cell.
**Fix:** Decide where the hub root enters the recorder (accumulator field set at verb entry, or normalisation at the verb boundary) and state the disposition of a `Target` that is the hub root itself.

### [BLOCKING:decision] `checkForce` disposition under the export contradicts source
**Section:** `check-enum-single-declarer` + Technical context (the two enums)
**Issue:** The discussion says the `checkForce` non-membership rule "moves onto the exported enum's own doc comment" and that a force constant "must not be added to either enum", but `checkForce` already **is** a member of `fabricengine`'s `destructiveCheck` (`destroy.go:39`) and is rendered as `"force"` by `String()` (`:51-52`) — non-membership is true only of `fabrictest`'s copy. It is therefore unstated whether exporting the enum exports three constants or four, and whether the type stays `int`-with-`String()` (needing a marshal step for `refusal.check`, and breaking `fabrictest`'s `string(check)` in `RefusedByGate`, `refusal.go:56`) or becomes string-backed.
**Fix:** State explicitly that `checkForce` stays unexported (or is exported and documented as never-constructible) and which backing the exported type has.

## Verdict

REQUEST_CHANGES
Oracle's commission direction and the `Target`/enum-export mechanics need resolving before planning.
MILL_REVIEW_END
