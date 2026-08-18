# Batch: webster path accessors take a told anchor root

```yaml
task: websterengine + webstercli told-geometry, and Webster standalone entry
batch: webster path accessors take a told anchor root
number: 5
cards: 4
verify: go test ./internal/websterengine/... ./internal/webstercli/... ./cmd/lyx/... && go test -tags integration ./internal/webstercli/...
depends-on: [4]
```

## Batch Scope

`websterengine.Dir`, `ReportsDir`, `ScratchDir` and `PromptsDir` take a `*lyxcwd.Location` today.
This batch changes all four to take a told `anchorRoot string` and updates every caller in the repository, derived from a mechanical repo-wide grep rather than a hand list.
It is deliberately a signature-only batch: every caller passes `l.AnchorPath()`, the exact value the accessors read off the Location today, so no resolved path changes anywhere and the whole tree stays green.
This ordering is what makes batch 6 possible at all — `internal/standalonegeom` has no `Location` to hand these accessors and must call them with a told `<state>` string.
`websterengine` remains the sole declarer of the `_lyx/webster` and `.lyx/webster` subpaths, per the Cwd Resolution Invariant;
only the parameter type changes.

## Cards

### Card 13: Convert the four accessors to `anchorRoot string`

- **Context:**
  - `internal/lyxdirs/dirs.go`
  - `internal/websterengine/geometry.go`
- **Edits:**
  - `internal/websterengine/state.go`
  - `internal/websterengine/doc.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `internal/websterengine/state.go`, change `Dir`, `ReportsDir`, `ScratchDir` and `PromptsDir` from `func(l *lyxcwd.Location) string` to `func(anchorRoot string) string`.
  `Dir` joins `lyxdirs.LyxDirName` and the existing `websterDirName` constant onto `anchorRoot`;
  `ScratchDir` joins `lyxdirs.DotLyxDirName` and `websterDirName` onto `anchorRoot`.
  `ReportsDir(anchorRoot)` must keep composing on `Dir(anchorRoot)` and `PromptsDir(anchorRoot)` on `ScratchDir(anchorRoot)`, so the `_lyx`/`.lyx` mirroring stays derived rather than restated.
  Drop the `internal/lyxcwd` import from this file if nothing else in it uses the package.
  Update each accessor's doc comment to name the told parameter instead of the Location, and keep each one's existing "no other package may construct this path" sentence.
  Fix the file-header comment's claim that callers resolve `websterDir` via the accessors so it reads as told-parameter shaped.
  In `internal/websterengine/doc.go`, the "engine/cli split: webster is fabric-blind" section states that "all `_lyx/webster` path construction lives in internal/lyxcwd (WebsterDir/WebsterReportsDir/WebsterPromptsDir)" — that is already false and becomes conspicuously so here.
  Rewrite those two lines to say that `websterengine` itself declares its own `_lyx/webster` and `.lyx/webster` subpaths through the four told accessors in `state.go`, and that the anchor root they are joined onto is always supplied by the caller, per the Cwd Resolution Invariant.
  Change no other section of `doc.go` in this card.
- **Commit:** `refactor(websterengine): take a told anchor root in the four path accessors`

### Card 14: Convert the accessor unit tests

- **Context:**
  - `internal/websterengine/state.go`
- **Edits:**
  - `internal/websterengine/webstergeom_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** `internal/websterengine/webstergeom_test.go` builds a `*lyxcwd.Location` and passes it to each accessor.
  Keep both existing test functions — the unanchored case and the subpath-anchored one — since together they still pin the `_lyx`-versus-`.lyx` group placement and the join arithmetic.
  In each, build the Location exactly as today, capture `l.AnchorPath()` into a local once, and pass that string to all four accessors.
  Leave every expected value unchanged: the point of this batch is that no computed path moves.
  Add one case driving an accessor with a plain told directory that is not derived from any Location at all, proving the accessors no longer need one — this is the property `internal/standalonegeom` depends on in batch 6.
- **Commit:** `test(websterengine): drive the path accessors with a told anchor root`

### Card 15: Update the `webstercli` accessor call sites

- **Context:**
  - `internal/websterengine/state.go`
- **Edits:**
  - `internal/webstercli/cli.go`
  - `internal/webstercli/cli_test.go`
  - `internal/webstercli/verbs_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `internal/webstercli/cli.go`'s `PersistentPreRunE`, the four assignments to `c.websterDir`, `c.reportsDir`, `c.websterScratchDir` and `c.promptsDir` pass `layout`;
  pass `layout.AnchorPath()` instead.
  Make the identical substitution in the `websterCLI` literal inside `newTestCLI` in `internal/webstercli/cli_test.go` and in the fixture literal in `internal/webstercli/verbs_test.go`, both of which already compute `planparser.PlanDir(layout.AnchorPath())` on the adjacent line and so gain a consistent shape.
  Change nothing else in these three files — the `layout` field itself, `fabricSync`, and `validate` are batch 8's work, and the verb `Deps` constructions are batch 7's.
  The subpath-anchored `PersistentPreRunE` case in `internal/webstercli/verbs_test.go` must keep passing unchanged: it is the non-tautological anchoring proof for this module, and this batch must not weaken it.
- **Commit:** `refactor(webstercli): pass the told anchor root to the webster path accessors`

### Card 16: Update the `cmd/lyx` enforcement-test rows

- **Context:**
  - `internal/websterengine/state.go`
- **Edits:**
  - `cmd/lyx/constructoranchoring_test.go`
  - `cmd/lyx/notransients_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** `cmd/lyx/constructoranchoring_test.go` holds four webster rows in each of its two test functions — `websterengine.Dir` and `ReportsDir` in the `_lyx` group, `PromptsDir` and `ScratchDir` in the `.lyx` group — plus two more webster entries in the map further down the file.
  Rewrite every one of them in place to pass `l.AnchorPath()`, keeping each expected value exactly as it is;
  do not split or retire the file, and do not delete the rows as tautological — they still pin the join arithmetic and the group placement.
  `cmd/lyx/notransients_test.go` holds the same conversion in three places: two rows in its `_lyx` list, two in its `.lyx` list, and one paired `Dir`/`ScratchDir` entry in the mirrored-subpath enforcement table.
  Convert all five identically.
  A textual merge conflict with the sibling task's adjacent `perchengine` rows in `constructoranchoring_test.go` is expected and is resolved by whichever task rebases second;
  do not restructure the file to avoid it.
- **Commit:** `test(cmd/lyx): pass the told anchor root in the webster enforcement rows`

## Batch Tests

`verify:` runs the untagged suites of `./internal/websterengine/...`, `./internal/webstercli/...` and `./cmd/lyx/...` — the three packages holding every caller the repo-wide grep for the four accessors turned up, and no others — plus a chained `go test -tags integration ./internal/webstercli/...`.
That tagged half is required, not defensive: `internal/webstercli/verbs_test.go` carries a `//go:build integration` constraint, so card 15's conversion of its fixture is invisible to the untagged run, and the subpath-anchored pre-run case card 15 must keep passing lives in exactly that file.
That grep is the batch's own completeness argument: `websterengine.Dir`, `ReportsDir`, `ScratchDir` and `PromptsDir` have call sites in exactly `internal/webstercli/cli.go`, `internal/webstercli/cli_test.go`, `internal/webstercli/verbs_test.go`, `internal/websterengine/webstergeom_test.go`, `cmd/lyx/constructoranchoring_test.go` and `cmd/lyx/notransients_test.go`, and every one of them is edited by a card above.
Card 14's new told-directory case is the only genuinely new assertion;
everything else in this batch is a signature change whose regression net is that all three existing suites keep passing with unchanged expected values.
The overview's module-wide `verify:` catches any caller outside these three packages that the grep would have had to miss.
