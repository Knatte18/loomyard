# Batch: pattern-path-api

```yaml
task: "Collapse _pattern into _lyx, and un-reserve _raddle as a hub-level name"
batch: "pattern-path-api"
number: 1
cards: 6
verify: go test ./internal/pattern/ ./internal/burlerengine/ ./internal/websterengine/ ./internal/loomengine/ ./cmd/lyx/ && go test -tags integration ./internal/builderengine/
depends-on: []
```

## Batch Scope

This batch moves the PATTERN content model from `_pattern/` into `_lyx/`: `PATTERN.md` at `_lyx/PATTERN.md` and the detail docs under `_lyx/pattern/`.
It reworks `internal/pattern`'s path API onto `lyxdirs.LyxDirName`, exports the two PATTERN path spellings as `PathspecFile`/`PathspecDir` for `internal/fabricengine` to consume in batch 2, rewrites the three agent-facing directive constants' literal relative pointers, and converges every consumer whose assertions depend on those two semantic changes.
It is one batch because the `File()` rewrite and the directive rewrite are a single observable behaviour change: any consumer left behind fails immediately, so they must all move together.

The external interface batch 2 consumes is the pair of exported constants `pattern.PathspecFile` and `pattern.PathspecDir`.

Batch-local decision: the `DirName` const and the `Dir(baseDir)` accessor are deliberately **left in place, unchanged**, in this batch — see the overview's "`pattern.DirName` and `pattern.Dir()` survive until batch 6" Shared Decision.
Do not delete them here, and do not change what `Dir` returns.

## Cards

### Card 1: Widen the Pattern Leaf Invariant to admit `internal/lyxdirs`

- **Context:**
  - `internal/lyxdirs/dirs.go`
  - `internal/lyxdirs/doc.go`
- **Edits:**
  - `internal/pattern/leaf_enforcement_test.go`
  - `CONSTRAINTS.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add the entry `"github.com/Knatte18/loomyard/internal/lyxdirs": true` to the `allowedImports` map in `internal/pattern/leaf_enforcement_test.go`, and update that file's header comment (lines 1-3) so its stated allowed set reads "the standard library, `internal/lyxcwd`, and `internal/lyxdirs`" instead of naming `internal/lyxcwd` alone.
  In `CONSTRAINTS.md`, update the **Pattern Leaf Invariant** section's opening sentence so the allowed production import set is stdlib, `internal/lyxcwd`, and `internal/lyxdirs`; add a sentence stating that `internal/lyxdirs` is admissible because it is a stdlib-only zero-import leaf (its own Lyxdirs Single-Declarer Invariant) and therefore cannot participate in a cycle by construction.
  Do not relax the allowlist to a banlist and do not remove any existing banned-package wording.
  Prose edits follow the repo's semantic-line-break convention.
  This card must land before card 2, because card 2 adds the `internal/lyxdirs` import that `TestLeafInvariant_AllowlistOnly` would otherwise reject.
- **Commit:** `test(pattern): widen Pattern Leaf Invariant allowlist to internal/lyxdirs`

### Card 2: Rework `internal/pattern`'s path API and directive pointers onto `_lyx`

- **Context:**
  - `internal/lyxdirs/dirs.go`
  - `internal/pattern/doc.go`
  - `internal/pattern/leaf_enforcement_test.go`
- **Edits:**
  - `internal/pattern/pattern.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `internal/pattern/pattern.go`, add `"github.com/Knatte18/loomyard/internal/lyxdirs"` to the import block.
  Add an unexported const `patternFileName = "PATTERN.md"` — the single declaration of the filename inside this package — and use it from both `File` and `PathspecFile` so the filename is written exactly once.
  Add two exported consts built from `lyxdirs.LyxDirName` by string concatenation, never `filepath.Join` (git pathspecs are always forward-slashed): `PathspecFile = lyxdirs.LyxDirName + "/" + patternFileName` and `PathspecDir = lyxdirs.LyxDirName + "/pattern"`.
  Give each a doc comment stating that it is the worktree-relative git-pathspec spelling of the PATTERN entry point and of the PATTERN detail-docs directory respectively, that `internal/pattern` is their single declarer, and that `internal/fabricengine` consumes them for the `PatternResidue` pathspec.
  Rewrite `File` to `return filepath.Join(baseDir, lyxdirs.LyxDirName, patternFileName)` — it no longer calls `Dir`.
  Leave `DirName`, `Dir`, and `FileHere` bodies unchanged.
  Rewrite the three directive constants `implementerDirective`, `reviewFixDirective`, and `orchestratorDirective` so every occurrence of `_pattern/PATTERN.md` becomes `_lyx/PATTERN.md` and every occurrence of "detail doc under `_pattern/`" becomes "detail doc under `_lyx/pattern/`".
  These pointers must stay **literal, non-interpolated** relative strings inside the const bodies — never built from `lyxdirs.LyxDirName` or any `Location` field — because the package's own tests and every consumer template test compare the rendered text by fixed-string equality or substring.
  Update the doc comment on `implementerDirective` (line 51) which names `"_pattern/PATTERN.md"` as the literal pointer.
  Add a short comment above the three constants recording that the literal pointers here are deliberately not built from `PathspecFile`, and why.
  Do not introduce the tokens `weft`, `warp`, or a fabric-sense `host` phrase into any new comment text in this package — `internal/pattern` is not a Fabric Vocabulary Invariant owner.
- **Commit:** `refactor(pattern): build PATTERN paths from lyxdirs.LyxDirName and export the pathspec spellings`

### Card 3: Update `internal/pattern`'s package godoc

- **Context:**
  - `internal/pattern/pattern.go`
- **Edits:**
  - `internal/pattern/doc.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In the "# The active check is pure existence" section, change `_pattern/PATTERN.md` to `_lyx/PATTERN.md`.
  The parenthetical crediting the Cwd Resolution Invariant's enforcement test with policing "which package may declare the `_pattern` token" is now false in two ways and must be rewritten: `File` builds from `lyxdirs.LyxDirName`, so the policed token belongs to `internal/lyxdirs`, and `TestEnforcement_GeometryLiterals` matches whole tokens by exact equality and therefore cannot see `_lyx/PATTERN.md` at all.
  State plainly that keeping these spellings built from `lyxdirs.LyxDirName` is a review obligation rather than a machine-enforced one.
  The sentence explaining that the `_pattern/` directory may exist without `PATTERN.md` because `lyx init` always creates the directory must be rewritten for the new layout: `_lyx` always exists, so its presence never implies PATTERN is active.
  In the "# Why the pointer stays relative" section, change the pointer to `_lyx/PATTERN.md` and keep the whole rationale intact.
  Add a short paragraph documenting `PathspecFile` and `PathspecDir`: what they are for, that they are git-pathspec spellings rather than filesystem paths, and that `internal/fabricengine` is their consumer.
- **Commit:** `docs(pattern): update package godoc for the _lyx PATTERN layout`

### Card 4: Converge `internal/pattern`'s own test suite

- **Context:**
  - `internal/pattern/pattern.go`
  - `internal/lyxdirs/dirs.go`
- **Edits:**
  - `internal/pattern/patternpath_test.go`
  - `internal/pattern/pattern_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `internal/pattern/patternpath_test.go`, change the `File` table's expectation so `want` is `filepath.Join(tt.baseDir, lyxdirs.LyxDirName, "PATTERN.md")` rather than a `"_pattern"` join, and add the `internal/lyxdirs` import if absent.
  Leave the `pattern.Dir` table case exactly as it is — `Dir` is unchanged in this batch and is deleted in batch 6.
  The `FileHere` cases at lines 90-91 and 117-118 compare against `pattern.File(...)` and therefore need no expectation change, but re-read them and confirm that remains true.
  Add a table case asserting `pattern.PathspecFile == "_lyx/PATTERN.md"` and `pattern.PathspecDir == "_lyx/pattern"` as exact strings, and a case asserting both are forward-slashed on every platform (no `filepath.Separator`).
  In `internal/pattern/pattern_test.go`, move every fixture path from `<root>/_pattern/PATTERN.md` to `<root>/_lyx/PATTERN.md`: `writePatternFile`'s helper, the `PATTERN.md`-as-a-directory case at line 114, and the nested-`RelPath` case's two plant sites.
  All four existing active-check edge cases — absent, empty file, `PATTERN.md` as a directory, non-`IsNotExist` stat error — must keep passing verbatim with only their fixture paths moved; do not weaken or reword their assertions.
  Update the directive-pointer assertions at lines 159, 172-173 to require the literal `_lyx/PATTERN.md`, and add an assertion that each of the three rendered directives also contains the literal `_lyx/pattern/`.
  Add a negative assertion that no rendered directive contains the substring `_pattern/` — that is what catches a half-done rewrite.
  Keep the existing comment explaining that these are fixed-string comparisons and that an interpolated absolute path would break them.
- **Commit:** `test(pattern): move path and directive expectations onto _lyx`

### Card 5: Converge `cmd/lyx`'s constructor-anchoring assertions

- **Context:**
  - `internal/pattern/pattern.go`
  - `internal/lyxdirs/dirs.go`
- **Edits:**
  - `cmd/lyx/constructoranchoring_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Both `assertPath(t, "pattern.FileHere", pattern.FileHere(l), filepath.Join(anchor, pattern.DirName, "PATTERN.md"))` call sites (lines 83 and 135) must become `filepath.Join(anchor, lyxdirs.LyxDirName, "PATTERN.md")`.
  Add the `internal/lyxdirs` import if it is not already present, and drop the `internal/pattern` import only if `pattern` becomes unused in the file — check the whole file before removing it.
  These two assertions are the reason the const deletion in batch 6 is safe: after this card, `cmd/lyx` no longer names `pattern.DirName`.
- **Commit:** `test(cmd/lyx): anchor pattern.FileHere assertions at _lyx`

### Card 6: Converge the directive-text and PATTERN-fixture consumers

- **Context:**
  - `internal/pattern/pattern.go`
  - `internal/lyxdirs/dirs.go`
- **Edits:**
  - `internal/websterengine/template_test.go`
  - `internal/builderengine/template_test.go`
  - `internal/burlerengine/template_test.go`
  - `internal/loomengine/plan_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `internal/websterengine/template_test.go`, the `t.TempDir()` fixture that materialises a real PATTERN file must create it at `<dir>/_lyx/PATTERN.md` instead of `<dir>/_pattern/PATTERN.md` — create the intermediate `_lyx` directory before the `os.WriteFile` call, and update the header comment at line 47 that describes the fixture.
  Update every `pattern_directive` stub value (lines 146 and 172) and the `requireContains` assertion at line 652, plus the doc comment at line 618, so each names `_lyx/PATTERN.md`.
  In `internal/builderengine/template_test.go` and `internal/burlerengine/template_test.go`, update the single `pattern_directive` stub value in each so it names `_lyx/PATTERN.md`.
  In `internal/loomengine/plan_test.go`, the fixture at line 127 that builds `filepath.Join(worktreeRoot, "_pattern")` and writes `PATTERN.md` inside it must build `filepath.Join(worktreeRoot, lyxdirs.LyxDirName)` instead, adding the `internal/lyxdirs` import.
  This is a fixture-path change, not a junction retarget — the directory exists solely to make `pattern.Directive` report active.
  After this card, `go test ./internal/websterengine/` must pass with no `_pattern` substring remaining anywhere in that file.
- **Commit:** `test(engines): point PATTERN directive and fixture expectations at _lyx`

## Batch Tests

`verify:` runs the untagged tests for `internal/pattern`, `internal/burlerengine`, `internal/websterengine`, `internal/loomengine`, and `cmd/lyx`, then the integration-tagged tests for `internal/builderengine` — the one build-tagged file this batch edits (`internal/builderengine/template_test.go` carries `//go:build integration`).
Every other file this batch edits is untagged, so a plain `go test` reaches it.

The files this scope covers are `internal/pattern/patternpath_test.go`, `internal/pattern/pattern_test.go`, `internal/pattern/leaf_enforcement_test.go`, `internal/burlerengine/template_test.go`, `internal/websterengine/template_test.go`, `internal/loomengine/plan_test.go`, `cmd/lyx/constructoranchoring_test.go`, and `internal/builderengine/template_test.go`.

Card 1 is a TDD candidate in the inverse direction: `TestLeafInvariant_AllowlistOnly` must be widened before card 2 adds the import, or card 2 fails on landing.
Cards 4 and 6 are ordinary TDD candidates — write the new path and pointer expectations first, watch them fail, then apply card 2's rewrite.

The module-wide `go vet -tags integration ./...` from the overview runs after this batch's own verify and is what proves no other package was broken by the `File` semantics change.
