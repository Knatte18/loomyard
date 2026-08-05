MILL_REVIEW_BEGIN
# Review: fabric: shrink hubgeometry to the minimal illusion primitive (slice 7)

```yaml
verdict: GAPS_FOUND
reviewer_model: opushigh
reviewer_self_id: Claude Opus 4.x-class model (Anthropic); exact build not self-verifiable
reviewed_file: _mill/discussion.md
date: 2026-08-05
```

## Findings

### [GAP] Out-of-package test call sites unenumerated

**Section:** `seven-leak-fixes`, `config-path-move`, `batching`
**Issue:** Every relocation list is production-only, but test files in other packages call the same symbols and break compilation: `websterengine/audit_test.go:46,96,205` and `loomengine/preflight_integration_test.go:318,414` call `WeftWorktree()`/`HostLyxLink(slug)`, and `configcli/configcli_integration_test.go:111` calls `WeftWorktreePath(slug)` — all privatized in batch 3; separately `hubgeometry.ConfigFile|ConfigDir|LyxDirName|DotEnv` appears 228 times across 34 `*_test.go` files (`configsync_test.go` ×31, `configengine/config_test.go` ×21, `configcli_test.go` ×20), plus `BoardDir`/`FabricAnchorName`/`WeftSiblingPath` test callers in `boardcli`, `perchcli`, `buildercli`, `fabriccli`.
**Fix:** Enumerate the out-of-package test callers per batch (naming the `fabricengine`/`weftname`/`configengine` symbol each adopts) and fold that count into batch 1's budget, which today only allows for "~60 struct literals"; `config-path-move`'s dismissal of the 115/85 grep counts as "inflated by tests" understates the edit, since those test lines still must change.

### [GAP] `_board` re-wire skips reconcile's missing-weft branch

**Section:** `board-junction` (placement decision)
**Issue:** The decision says the link is "wired unconditionally" yet pins the call inside the `weftWorktreeExists` branch next to `applyStaleRemoval` (`reconcile.go:152`); the other branch, `reconcileMissingWeft` (`reconcile.go:130-132`, `164-197`), performs no junction wiring at all — a pair whose weft was just recreated or adopted therefore gets no `_board` link on that pass, and nothing diagnoses the absence because the link is deliberately unmonitored.
**Fix:** State explicitly whether the missing-weft branch is excluded on purpose (repair deferred to the next reconcile run) or whether the wiring is hoisted above the `weftWorktreeExists` check, and add the corresponding test row.

### [NOTE] Two moving symbols named in no destination list

**Section:** `weft-junction-move`
**Issue:** `WeftWorktreePath` (`hubgeometry.go:481`, 9 production `fabricengine` call sites: `add.go:111,144`, `weftwiring.go:66,86,125`, `prune.go:61`, `remove.go:54`, `status.go:86`, `reconcile.go:104`) and `HostLyxLinkHere` (`:530`) appear in no enumeration; the "all `Weft*`/`Host*Link`" wording implies them, but `HostLyxLinkHere` does not match the `Host*Link` glob and every sibling list is exhaustive.
**Fix:** Add both to the move list explicitly so the batch-3 diff is fully specified.

### [NOTE] `fabriccli` is an unlisted `weftname` consumer

**Section:** `weftname-leaf`
**Issue:** The decision names `fabricengine` and `lyxtest` as the two importers, but `fabriccli/fabric.go:45,67,78` builds cobra `Long` help text from `hubgeometry.WeftSuffix` in production code — a third consumer that must switch to `weftname.WeftSuffix` in batch 1.
**Fix:** Record `fabriccli` as a `weftname` importer (or state that it sources the suffix from `fabricengine` instead).

## Verdict

GAPS_FOUND
Test call sites unbudgeted; `_board` re-wire placement contradicts its own "unconditional" rule.
MILL_REVIEW_END
