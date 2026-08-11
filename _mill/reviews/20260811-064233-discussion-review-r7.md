MILL_REVIEW_BEGIN
# Review: fabric: live-state integration harness (slice 13)

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude Opus 5 (best-effort self-assessment)
reviewed_file: _mill/discussion.md
date: 2026-08-11
```

## Findings

### [BLOCKING:design] Reset column can never reach a gate refusal
**Section:** `tranche-1-verb-table` → the `CloneHub{Reset:true}` column; Q&A "assert refusal-by-ownership"
**Issue:** `resetHub` refuses both named targets at its own pre-flight (`clone.go:573-577`, `!looksLikeHub`) *before* the `pathRequest` at `:579` exists, and the gate's ownership predicate for `pathOwnershipFabricHub` is the **same** `looksLikeHub` call (`destroy.go:346-350`) — so no `Reset` target can ever produce a gate refusal, and a `RefusedByGate(CheckOwnership)` expectation fails against a correct binary; the two "ownership-shaped targets" are also one shape, since `resetHub` only ever targets `HubPath(cwd, DeriveWarpName(warpURL))`.
**Fix:** State that this column uses `RefusedBefore("is not a fabric hub")`, record that the gate is structurally unreachable for `Reset` (as the doc already does for `remove ..` at `remove.go:45`), and either drop the redundant second target or replace it with one that passes `looksLikeHub`.

### [NIT:scope] Third build-blocking enforcement test unlisted
**Demoted-from:** BLOCKING
**Section:** Scope "In" (owner rows) and Constraints (the two same-commit enforcement updates)
**Issue:** `TestEnforcement_GeometryLiterals` (`internal/lyxcwd/enforcement_test.go:248`, tree-scan at `:541`) walks **every** non-`_test.go` `.go` file from repo root and looks `relDir` up in exact-match `geometryTokenOwners`, which owns `_board`, `-weft`, `-HUB`, `_portals`, `_launchers`, `_lyx`, `.lyx` and contains no `internal/fabricengine/fabrictest` row — yet the doc's own technical context tells a plan writer "`_portals` and `_launchers` are unexported directory names but their paths are `<hub>/_portals/<anchor>/<slug>`", which is exactly a `filepath.Join` literal in `verbs.go`/`states.go`.
**Fix:** Add this test to the same-commit enforcement list, and state the choice: route every geometry path through exported accessors (`PortalsDir`, `LauncherDir`, `HubPath`, `BoardDirName`, `weftname.Suffix`, `lyxdirs.LyxDirName`) or add a `fabrictest` row to `geometryTokenOwners` plus the matching `CONSTRAINTS.md` update.

### [NIT:scope] `Checkout` has no arrangement or target branch
**Demoted-from:** BLOCKING
**Section:** `cross-product-shape` (Arrange enumeration) and `three-expectation-kinds-and-the-scope-table`
**Issue:** The Arrange list names `Remove`, `UnwireJunctions`, `Prune`, `Cleanup`, `Pull` and `Add` but not `Checkout`, and no cell says which branch `Checkout` is handed — a factory-built hub has only the clone's branch, so `Checkout` either no-ops on the current branch or errors on a nonexistent one; that makes the `dirtyWeftUntracked × Checkout` `Proceeds` cell — the sole justification for both the third expectation kind and the tenth state — vacuous, and leaves `Checkout`'s clean-state effect ("prime warp on the branch, weft on the corresponding weft branch") unreachable.
**Fix:** Give `Checkout` an `Arrange` (e.g. an added pair supplying `<prefix><slug>`) and name the branch argument its cells use.

### [NIT:consistency] Sabotage table is split by a paragraph
**Section:** Testing → "Sabotage-proving", rows 1-9
**Issue:** The "Row 3 carries an extra requirement" paragraph is inserted mid-table, so rows 4-9 are an orphaned fragment with no header and will not render as part of the table.
**Fix:** Move that paragraph below the complete nine-row table.

### [NIT:scope] Scope table omits reachable gate dirtiness rows
**Section:** `three-expectation-kinds-and-the-scope-table` ("Verified against the tree")
**Issue:** The table omits `Remove`'s weft-side gate rows (`weftwiring.go:199` `dirtyScopeAll`, `:219` `dirtyCheckedOutBranch`), `Checkout`'s `checkout.go:200` and `Add`'s `add.go:268`/`:292` — none change a tranche-1 cell's derived outcome, but the table reads as exhaustive.
**Fix:** Either add the rows or state that the table covers only the declarations reachable with `force=false`.

## Verdict

REQUEST_CHANGES
Reset column mis-specifies its refusal layer; one enforcement test and Checkout's arrangement are unaddressed.
MILL_REVIEW_END
