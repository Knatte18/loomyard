# Batch: residue-rescope

```yaml
task: "Collapse _pattern into _lyx, and un-reserve _raddle as a hub-level name"
batch: "residue-rescope"
number: 2
cards: 2
verify: go test -tags integration -run 'Pull|Residue' ./internal/fabricengine/
depends-on: [1]
```

## Batch Scope

This batch re-scopes fabric's `PatternResidue` detection from the bare `_pattern` directory to the new `_lyx/PATTERN.md` + `_lyx/pattern/` paths, and retires `pull.go`'s private `patternDirName` const by adding the first **production** `internal/fabricengine` -> `internal/pattern` import.
The feature itself is kept in full: `PullResult.PatternResidue`, `PatternResidueEntry`, `patternResidueCommits`, `parsePatternResidueRecords`, and the `fabriccli` JSON field all survive unchanged in shape.
Only the git pathspec argument changes.

It is one batch because the const deletion and the pathspec re-scope are the same edit, and the integration test that pins the behaviour must move with them.

Batch-local decision: the residue pathspec stays deliberately **narrow** — `_lyx/PATTERN.md` and `_lyx/pattern/`, never all of `_lyx`.
Widening it to `_lyx` would make every config and loom-status commit residue, drowning the signal the narrow pathspec exists to produce.

## Cards

### Card 7: Re-scope `patternResidueCommits` onto `pattern.PathspecFile`/`PathspecDir`

- **Context:**
  - `internal/pattern/pattern.go`
  - `internal/fabricengine/index.go`
  - `internal/fabricengine/status.go`
- **Edits:**
  - `internal/fabricengine/pull.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Delete the private const `patternDirName` and its doc comment from `internal/fabricengine/pull.go`.
  Add `"github.com/Knatte18/loomyard/internal/pattern"` to `pull.go`'s import block — this is a real file-level import-block change, not a no-op: `internal/fabricengine` imports `internal/lyxdirs` at package level via `status.go`, but `pull.go` itself currently imports only `errors`, `fmt`, `path/filepath`, `strings`, `internal/gitexec`, and `internal/lock`.
  In `patternResidueCommits`, replace the single trailing pathspec argument `patternDirName` with the two arguments `pattern.PathspecFile` and `pattern.PathspecDir`, so the `args` slice ends `"--", pattern.PathspecFile, pattern.PathspecDir`.
  Rewrite `patternResidueCommits`' doc comment: the range/purpose paragraph now says the commits touch `_lyx/PATTERN.md` or `_lyx/pattern/...` paths; the paragraph currently asserting "The pathspec is patternDirName, NEVER an inline `_pattern` string literal — the Cwd Resolution Invariant reserves that token to its declarer" becomes a statement that the pathspec strings come from `internal/pattern`'s exported constants, which are themselves built from `lyxdirs.LyxDirName`, and that this is a review obligation because `TestEnforcement_GeometryLiterals` matches whole tokens by exact equality and cannot see `"_lyx/PATTERN.md"`.
  Keep the "Separator placement" paragraph and the "RelPath-blind scope (documented limitation)" paragraph, adjusting only the path names inside them — both limitations survive the re-scope unchanged.
  Update the final paragraph's phrase "a real range with zero `_pattern`-touching commits" to name the new paths.
  Also update `PullResult`'s `AnchorWeftSHA` doc comment and the `PullResult` type comment where they describe PATTERN-residue content, if they name `_pattern`.
  Three further `_pattern/...` mentions in this file are doc comments that no other card in the plan reaches, and all three must be corrected here:
  `PullResult.PatternResidue`'s field doc comment at line 67 ("`_pattern/...` paths — content a caller should treat as potentially ..."),
  `PatternResidueEntry`'s type doc comment at line 73 ("names one post-anchor weft commit and the `_pattern/...` paths it touched"),
  and `parsePatternResidueRecords`' doc comment at line 314 ("that commit's changed `_pattern/...` paths").
  Retarget each to the new `_lyx/PATTERN.md` + `_lyx/pattern/...` wording.
  The line-314 edit is **wording only**: do not change `parsePatternResidueRecords`' body, signature, or parsing logic — it is a pure function whose record-boundary handling is unaffected by the re-scope, and its tests are a deliberate regression anchor.
  After this card, `grep -n '_pattern' internal/fabricengine/pull.go` must return nothing.
  Do not rename any exported identifier: `PatternResidue`, `PatternResidueEntry`, and the `fabriccli` JSON field all keep their names.
  Cycle check to state in the commit message: `fabricengine -> pattern -> {lyxcwd, lyxdirs}`, both of which `fabricengine` already imports directly, so this adds no new transitive dependency and leaves the Pattern Leaf Invariant untouched.
- **Commit:** `refactor(fabricengine): scope PATTERN residue detection to the _lyx PATTERN paths`

### Card 8: Re-point and extend the PATTERN-residue integration test

- **Context:**
  - `internal/fabricengine/pull.go`
  - `internal/pattern/pattern.go`
  - `internal/lyxdirs/dirs.go`
- **Edits:**
  - `internal/fabricengine/pull_integration_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `TestPull_IdentifiesPatternResidue`, move the seeded PATTERN-touching weft commit's fixture from `<weft>/_pattern/PATTERN.md` to `<weft>/_lyx/PATTERN.md`, creating the intermediate `_lyx` directory.
  Replace `wantPath := pattern.DirName + "/PATTERN.md"` with `wantPath := pattern.PathspecFile`.
  Update the test's doc comment at line 246 to name the new path.
  There is exactly one other `pattern.DirName` use in this file, at line 254 — `patternDir := filepath.Join(weftFixture.WeftPath, pattern.DirName)`, the fixture directory the seeded commit writes into.
  That is a PATTERN-path use, not a junction name, so it becomes a join onto `lyxdirs.LyxDirName` and the local is renamed off `patternDir`.
  After this card the file must contain no `pattern.DirName` reference at all.
  Add two new sub-tests to the same function:
  first, a commit touching only `_lyx/config/fabric.yaml` must **not** appear in `PatternResidue` — this negative case is the entire justification for keeping the pathspec narrow instead of widening it to `_lyx`, so it is not optional;
  second, a commit touching `_lyx/pattern/<detail>.md` **must** appear in `PatternResidue`, proving `PathspecDir` is wired and not just `PathspecFile`.
  Keep the existing non-PATTERN control commit and its assertion.
  Do not touch any test of `parsePatternResidueRecords` — its record-boundary cases are a deliberate regression anchor and must be left alone.
- **Commit:** `test(fabricengine): pin PATTERN residue against the _lyx paths and its narrow scope`

## Batch Tests

`verify:` is `go test -tags integration -run 'Pull|Residue' ./internal/fabricengine/`.
The `-tags integration` flag is required because `internal/fabricengine/pull_integration_test.go` carries `//go:build integration`.
The `-run` filter scopes execution to the pull/residue tests rather than the whole fabricengine integration suite, which this batch does not touch — the remaining fabricengine files are batch 3's and batch 4's subject.

Card 8 is a TDD candidate: write the re-pointed fixture and the two new sub-tests first, watch the negative `_lyx/config/fabric.yaml` case fail against the old bare-`_pattern` pathspec, then apply card 7.

The module-wide `go vet -tags integration ./...` from the overview is what proves the new `pull.go` import block compiles across every tagged and untagged file in the tree.
