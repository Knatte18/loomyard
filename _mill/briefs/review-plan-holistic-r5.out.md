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

### [BLOCKING] Card 31 privatizes Weft* with four caller files unlisted
**Location:** batch 6, card 31 (vs cards 32/35)
**Issue:** Deleting `WeftWorktreePath`/`WeftLyxDir`/`WeftLyxDirFor` breaks `fabricengine/hook_test.go:233` (`l.WeftWorktreePath(slug)` — in NO batch-6 card at all), `junction_repoint_test.go:53,155` (`l.WeftLyxDirFor(slug)`), `junction_pattern_integration_test.go:74,470` and its Lyx-row `targetFor` at `:386` (`l.WeftLyxDir()` — card 32 quotes only the `:385` `linkFor` half), and `unwire_test.go:104` — the last three files sit in cards 32/35 with no Weft* rewrite named, so batch 6's `go test ./internal/fabricengine/...` fails to compile.
**Fix:** Add `hook_test.go`, `junction_repoint_test.go`, `junction_pattern_integration_test.go` and `unwire_test.go` to card 31's `Edits:` and name each rewrite onto the relocated `weftWorktreePath(l, slug)`/`weftLyxDirFor(l, slug)` forms.

### [BLOCKING] Card 31's "no external fallout" claim is false — configcli test calls WeftWorktreePath
**Location:** batch 6, cards 30/31
**Issue:** Card 31 states `WeftWorktreePath` has "9 production call sites, all inside fabricengine … a pure in-package privatization with no external fallout", but `configcli/configcli_integration_test.go:111` calls `f.Layout.WeftWorktreePath(slug)` from outside the package, and card 30's accessor set (`WeftWorktree(l)`, `WeftLyxDir(l)`) has no slug-parameterized form to retarget it onto.
**Fix:** Have card 30 name the rewrite (e.g. `weftname.SiblingPath(f.Layout.HubPath, slug)` or an exported slug-form accessor) and correct card 31's claim.

### [BLOCKING] Card 32 deletes HostLyxLink with two caller files uncovered
**Location:** batch 6, card 32
**Issue:** `fabricengine/add_rollback_adopt_test.go:142,211` call `l.HostLyxLink(slug)` and the file is not in card 32's `Edits:` (cards 31/33 list it for Weft*/portal edits only), and `loomengine/preflight_integration_test.go:414` calls `f.Layout.HostLyxLink(slug)` out-of-package — card 32 re-declares it as unexported `hostLyxLink`, card 30 provides no replacement accessor, and card 35 rewrites only that table's `HostPatternLink` row.
**Fix:** Add `add_rollback_adopt_test.go` to card 32 with the in-package `hostLyxLink(l, slug)` rewrite, and name the loomengine rewrite (generic join `filepath.Join(fabricengine.WorktreePath(l, slug), l.AnchorRel, configengine.LyxDirName)` or an exported accessor) in card 30 or 32.

### [BLOCKING] Card 34's BoardDir move misses five caller files in batch 6's verify scope
**Location:** batch 6, card 34
**Issue:** Moving `BoardDir` out of `lyxcwd` breaks `buildercli/weft_integration_test.go:43,58`, `webstercli/weft_integration_test.go:35,55`, `perchcli/run_integration_test.go:30,44`, `loomengine/preflight_integration_test.go:59` and `fabricengine/config_driven_junctions_integration_test.go:107` — none is in card 34's `Edits:`, all five packages are in batch 6's `go vet`/`go test -tags integration` gates.
**Fix:** Add the five files to card 34's `Edits:` with the `fabricengine.BoardDir` qualifier retarget.

### [BLOCKING] Card 18's completion grep is unsatisfiable and misses topology.go
**Location:** batch 4, card 18
**Issue:** The required check `grep -rln 'hubgeometry\|\bLayout\b' … must return nothing` can never pass: `\bLayout\b` matches the deliberately retained `PairedFixture.Layout` field and the webster/builder deps-struct `Layout` fields plus their many readers, several listed comment fixes land only in batches 5–6 (`websterengine/report.go` card 22, `pattern/doc.go` card 25, `logger/logger.go` card 26, `boardengine/config.go`/`configsync.go`/`fabricengine/doc.go` card 34), and `fabricengine/topology.go:4,13` names `hubgeometry` in comments yet appears in no card at all.
**Fix:** Add `topology.go` to card 18's sweep list, narrow the verification to qualifier/import uses (e.g. `loomyard/internal/hubgeometry\|hubgeometry\.`), and exempt or restate the mentions scheduled for batches 5–6.

### [NIT] "WeftRaddleDir has zero callers anywhere" is false
**Location:** batch 6, card 31
**Issue:** `weft_test.go:115` calls `layout.WeftRaddleDir()` with a `wantWeftRaddleDir` table column (`:23,38,53,68`); the card's lift instruction says sub-tests are "rewritten against the relocated functions", but this one has no relocated function.
**Fix:** State that the `WeftRaddleDir` sub-test and its table column are deleted with the symbol, and correct the zero-callers claim to zero production callers.

### [NIT] Card 11 instructs godoc edits in files it does not list
**Location:** batch 3, card 11
**Issue:** The requirement says to correct godoc in `logger/logger.go:48,409` and `pattern/doc.go:13`, but neither file is in card 11's `Edits:`/`Context:`, and the same corrections are already owned by cards 26 and 25.
**Fix:** Drop those two files from card 11's requirement (leaving `planparser/parse.go`, which is listed), or add them to its `Edits:`.

## Verdict

REQUEST_CHANGES
Batch 6's privatization cards outrun their listed callers; card 18's verification grep cannot pass.
MILL_REVIEW_END
