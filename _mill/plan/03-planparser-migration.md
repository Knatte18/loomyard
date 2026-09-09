# Batch: planparser-migration

```yaml
task: "Centralize glyph ref-shape enumeration"
batch: "planparser-migration"
number: 3
cards: 3
verify: go test ./internal/planparser/ ./internal/planglyph/
depends-on: [1, 2]
```

## Batch Scope

This batch migrates every `refKind` dispatch site inside `internal/planparser` onto the batch-1 ledger, routes `rewrite.go`'s open-coded handle-prefix tests through batch 2's `IsHandleRef`, and then deletes the three classifier wrappers (`isPathRef`/`isGlyphRef`/`isHandleRef`) whose caller sets the migration empties — with the compile-driven test rewire and comment updates that deletion forces.
It is one batch because the migration and the wrapper deletion are one logical unit (the wrappers are only deletable once the migration lands) and every card reads the same small registry surface.
After this batch, `internal/planparser` production code outside `classify.go`/`shape.go` contains no `classifyRef` call and no `refKind` comparison — the state batch 7's `refKind` scan pins.

## Cards

### Card 5: Migrate validate.go, containment.go, and handle.go gates onto lookup

- **Context:**
  - `internal/planparser/shape.go`
  - `internal/planparser/classify.go`
  - `internal/planparser/normalize.go`
- **Edits:**
  - `internal/planparser/validate.go`
  - `internal/planparser/containment.go`
  - `internal/planparser/handle.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Replace every direct `classifyRef` call and `refKind` comparison in the three edited files with a `lookup` consultation of the site's registered gate, preserving behavior exactly:
  1. Single-kind filters — `checkBareSymbolTarget` (`gateBareSymbolTarget`: raise the finding when the disposition is `dispFinding`, continue otherwise), `checkDirectoryTarget` (`gateDirectoryTarget`: proceed only on `dispKeep`), `checkGlyphMalformed` (`gateGlyphMalformed`), `checkHandleMalformed`'s Targets/Uses loop (`gateHandleMalformed`), `syntacticContainment` in `internal/planparser/containment.go` (`gateSyntacticContainment`), `handleClaims` and `referencedHandles` in `internal/planparser/handle.go` (`gateHandleClaims`, `gateReferencedHandles`: proceed only on `dispKeep`).
  2. `isFileRenamePair` — consult `gateFileRenamePair` once per pair side; both sides must report `dispKeep` before the existing `parseGlyph`/`IsSelf` refinement runs unchanged.
  3. `checkRenamePairShape` — the to-side arm consults `gateRenameTo` and raises `rename-to-not-handle` on `dispFinding`, rendering the offending kind with `refKindName` from the kind `lookup` returned (no re-classification);
     the from-side arm consults `gateRenameFrom` the same way for `rename-from-not-glyph`;
     the third arm re-uses `gateRenameTo` through `lookup` — "does `p.New` report `dispKeep` on the to-side gate?" — so no raw `refKind` comparison remains, and its `parseGlyph`/`IsSelf` refinement stays inside the from-side's `dispKeep` branch unchanged.
  4. `checkProsaSymbolTarget`'s `language: none` branch — consult `gateProsaPathOnly`: `dispKeep` (a path) continues clean, `dispFinding` raises the existing finding; the glyph-enabled branch keeps its `parseGlyph`/`IsSelf` test untouched.
  Do not alter any finding's Check ID, Detail wording, ordering, or severity — this card is dispatch plumbing only, proven by the untouched validate/containment/handle test suites.
  Keep each site's `isPathRef`-free form ready for card 7: after this card the only remaining wrapper callers are in `internal/planparser/normalize.go` (card 6's job).
- **Commit:** `planparser: migrate validate/containment/handle kind-gates onto the shape ledger`

### Card 6: Migrate normalize.go and rewrite.go; pin the root:-join gate with tests

- **Context:**
  - `internal/planparser/shape.go`
  - `internal/planparser/classify.go`
  - `internal/planparser/handle.go`
- **Edits:**
  - `internal/planparser/normalize.go`
  - `internal/planparser/normalize_test.go`
  - `internal/planparser/rewrite.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  In `internal/planparser/normalize.go`:
  `normalizeRefIfPath` consults `gateNormalizePath` — on `dispKeep` apply `normalizeCardPath`, otherwise return `raw` unchanged;
  `canonicalizeCard`'s canon closure consults `gateCanonicalizePath` the same way before its existing `canonicalizablePath` test.
  This is the migration's highest-risk site (the single `root:`-join gate — its own doc comment says so), so add a dedicated table-driven test to `internal/planparser/normalize_test.go` (new test function, no existing assertion touched) driving `normalizeRefIfPath` with a non-"." root across all four kinds — a path, a bare symbol, a glyph, a `plan:` handle — plus the `//` worktree-root escape, asserting the path is joined and every other shape passes through byte-identical.
  In `internal/planparser/rewrite.go`, replace the two open-coded `strings.HasPrefix(..., HandlePrefix)` tests in the move-line collapse branch with `IsHandleRef` (behavior-identical; this removes rewrite.go's only `plan:`-shape operation ahead of batch 7's scan).
- **Commit:** `planparser: migrate normalize root-join gate and rewrite handle test onto registry helpers`

### Card 7: Delete the three classifier wrappers; rewire tests and comments

- **Context:**
  - `internal/planparser/shape.go`
  - `internal/planparser/handle.go`
- **Edits:**
  - `internal/planparser/classify.go`
  - `internal/planparser/classify_test.go`
  - `internal/planparser/doc.go`
  - `internal/planparser/normalize.go`
  - `internal/planparser/parse.go`
  - `internal/planparser/parse_test.go`
  - `internal/planparser/validate_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Delete `isPathRef`, `isGlyphRef`, and `isHandleRef` from `internal/planparser/classify.go` — after cards 5–6 all three have zero production callers (`isGlyphRef` and `isHandleRef` were already test-only; `isPathRef`'s three callers migrated in cards 5–6).
  Rewire `internal/planparser/classify_test.go`'s wrapper test (`TestIsPathRef_IsGlyphRef_IsHandleRef`): keep every case's input and expectation, re-expressed against `classifyRef` (comparing to the expected `refKind`) and the exported `IsHandleRef`; rename the test accordingly.
  This is the sanctioned behavior-neutral rewire from the overview's Decision: behavior-preservation — no case is dropped or weakened.
  Comment-only updates wherever prose names a deleted wrapper, replacing it with the registry vocabulary (`classifyRef`, the ledger gates, or `IsHandleRef` as appropriate):
  the package doc in `internal/planparser/doc.go` (the classifier sentence listing the three wrappers);
  the file-top and function doc comments in `internal/planparser/normalize.go` that name `isPathRef` (file header, `normalizeCard` doc, `normalizeRefIfPath` doc);
  the canonicalization-ordering comment in `internal/planparser/parse.go`;
  and the comment mentions in `internal/planparser/parse_test.go` and `internal/planparser/validate_test.go`.
  None of these comment edits alters any assertion or any executable line beyond the deletions above.
- **Commit:** `planparser: delete dead classifier wrappers, rewire wrapper test, update comments`

## Batch Tests

`verify: go test ./internal/planparser/ ./internal/planglyph/` — the untouched planparser suite (every crucible regression test included) is the behavior-preservation proof for the whole migration; card 6's new `normalizeRefIfPath` table pins the root:-join gate; the rewired classify test keeps wrapper-equivalent coverage through `classifyRef`/`IsHandleRef`.
