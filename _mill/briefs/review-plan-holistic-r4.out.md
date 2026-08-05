MILL_REVIEW_BEGIN
# Review: fabric: shrink hubgeometry to the minimal illusion primitive (slice 7) — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: fablehigh
reviewer_self_id: Claude (Fable 5, model id claude-fable-5)
reviewed_file: plan/
date: 2026-08-05
```

## Findings

### [BLOCKING] Card 3's Prime deletion leaves three test files unfixed
**Location:** batch 1, card 3 (and card 4)
**Issue:** Deleting the `Prime` field and `WeftRepoRoot()` breaks `hubgeometry/weft_test.go` (`Prime:` literals at :86,166,181,199,281,318; `WeftRepoRoot()` calls at :90,202), `hubgeometry/pattern_test.go:24`, and `reedengine/mouse_boot_integration_test.go:50-55` — the first two are edited by card 4 only for the unrelated `WorktreePath` inlining, and the third is in no batch-1 card at all, so batch 1's `go test ./...`/tagged vet fails.
**Fix:** Add all three to card 3's `Edits:` and state the substitutions (drop `Prime:` from literals; retarget/inline the `WeftRepoRoot` assertions).

### [BLOCKING] `MenuLauncherRel` signature change misses its only caller
**Location:** batch 1, card 3
**Issue:** Card 3 gives `MenuLauncherRel()` a `primeName string` parameter, but its sole production caller `fabricengine/launchers.go:63` (`l.MenuLauncherRel()`) is not in the card's `Edits:` and no source for the argument is named — batch 1 stops compiling.
**Fix:** Add `internal/fabricengine/launchers.go` to card 3's `Edits:` and specify the call becomes `l.MenuLauncherRel(primeName)` sourced via the new `fabricengine.PrimeName(l)`.

### [BLOCKING] Batch 2's verify cannot pass — lyxtest still imports hubgeometry
**Location:** batch 2, card 5 / batch `verify`
**Issue:** The moved `lyxcwd` test files import `internal/lyxtest` (`testmain_test.go`, `anchor_test.go`, `hubgeometry_test.go`), and `lyxtest/lyxtest.go` keeps `hubgeometry.Layout`/`Resolve` until batch 3 card 10 — after the `git mv` that import path no longer exists, so `go vet -tags integration ./internal/lyxcwd/...` fails to type-check the package's test files.
**Fix:** Add `internal/lyxtest/lyxtest.go` to card 5's `Edits:` with the import/qualifier/field retarget (pulling it out of card 10), or narrow batch 2's verify to the `go build` half only.

### [BLOCKING] raddle guard's filename skip breaks at the rename
**Location:** batch 2 card 5 / batch 4
**Issue:** `raddle_guard_test.go:48` skips exactly `d.Name() == "hubgeometry.go"`; card 5 renames the file to `lyxcwd.go` and no card updates the literal, so the guard scans `lyxcwd.go`, finds `_raddle` (:317, :518), and fails at batch 4's first `go test ./...`.
**Fix:** Have card 5's surgical edits update the skip literal to `lyxcwd.go` (and its comment), naming `internal/lyxcwd/raddle_guard_test.go` explicitly.

### [BLOCKING] Batch 5 deletes constructors with lyxcwd's own tests unlisted
**Location:** batch 5, cards 20, 21, 23, 26, 27
**Issue:** `lyxcwd_unit_test.go` exercises `PerchRunsDir` (:44-52), `PlanDir` (:56-64), `BuilderDir` (:68-76), `DotLyxDir`/`LyxDir` (:123-131) and `HubLogsDir` (:135-147), and `lyxcwd_test.go` calls `layout.LyxDir()` (:123, :524) — none of the deleting cards lists either file, so batch 5's `go test ./internal/lyxcwd/...` fails to compile.
**Fix:** Add both files to the relevant cards' `Edits:` and state whether each sub-test moves with its symbol or is deleted as superseded by card 28's anchoring table.

### [BLOCKING] Card 25 deletes `PatternDirName` a batch before its consumers move
**Location:** batch 5 card 25 vs batch 6 card 35
**Issue:** `fabricengine/pull.go:299` plus `fabriccli/cli_test.go:416`, `pull_integration_test.go:258,292`, `unwire_test.go:65,71` and `junction_pattern_integration_test.go` (many) still reference `PatternDirName` until card 35, so batch 5's `go vet ./...` (and `go test ./cmd/lyx/...`) fails the moment card 25 deletes the const.
**Fix:** Keep `PatternDirName` in `lyxcwd` through batch 5 and delete it in card 35, or have card 25 retarget every referencing file to `pattern.DirName`/a `fabricengine` const in the same card.

### [BLOCKING] Lifted HostJunctions tests keep pattern-accessor calls card 35 deletes
**Location:** batch 6, cards 32 and 35
**Issue:** The HostJunctions sub-tests card 32 lifts from `lyxcwd_test.go` assert against `HostPatternLink`/`WeftPatternDirFor`/`WeftPatternDir` (:615-619, :663, :705); card 35 deletes those methods but its `Edits:` names neither `lyxcwd_test.go` nor the card-32-created `hostjunction_test.go`, and neither card states how those assertions are re-expressed.
**Fix:** Have card 32 rewrite the lifted assertions to inline joins (`filepath.Join(..., "_pattern")` via the generic record), or add `hostjunction_test.go` to card 35's `Edits:` with the rewrite named.

### [NIT] `.fabric-anchor` survives in help text and a comment no card touches
**Location:** batch 2 card 7
**Issue:** `fabriccli/fabric.go:266` names `.fabric-anchor` in a cobra `Long` (help-accuracy obligation) and `fabricengine/unwire.go:5` in a comment; card 7 renames the marker but lists neither file, and no later sweep covers them.
**Fix:** Add both files to card 7's `Edits:` for the marker-name correction.

### [NIT] Red-tree Shared Decision cites the wrong batch-2/4 verify commands
**Location:** 00-overview.md, red-intermediate-tree decision
**Issue:** The decision text says batch 2 verifies with `go build ./internal/lyxcwd/... && go vet ./internal/lyxcwd/...` and batch 4 restores `go test ./...`, but the Batch Index and batch files use the `-tags integration` variants and batch 4 adds a tagged run.
**Fix:** Align the decision prose with the actual verify commands (including any change from the batch-2 finding above).

## Verdict

REQUEST_CHANGES
Five batch gates fail as sequenced: deletions and renames outrun their listed call-site and test-file edits.
MILL_REVIEW_END
