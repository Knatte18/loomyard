# Batch: seeded-subtree-commit

```yaml
task: Deploy cited spec/design docs to target repos like stencils
batch: seeded-subtree-commit
number: 3
cards: 3
verify: go test ./internal/fabricengine/... ./internal/stencilcli/... ./cmd/lyx/... && go test -tags integration ./internal/fabricengine/...
depends-on: [2]
```

## Batch Scope

This batch generalises `fabricengine.CommitSeededStencils` from "commits the stencils subtree" to "commits a named seeded subtree", so that batch 4 can call it a second time for the specs subtree instead of adding a sibling verb.
It changes no behaviour: both existing callers pass the stencils subtree and land exactly the commit they land today.

It is one batch because the signature change and its two production callers must move together — a partial move does not compile — and because the two things being generalised (the pathspec prefix and the mutation record's absolute directory) are generalised in the same function body.

The external interface batch 4 consumes is the new signature.
The Mutation Record Invariant is what forces the second parameter rather than one: generalising only the pathspec prefix would file a seeded spec under the stencils directory in the `*Mutations` record, which is a false record of what was written.

Batch-local decision: the function keeps its name.
Renaming it would touch every caller and every test for no gain, and its doc comment already carries the burden of saying what it now commits.

## Cards

### Card 9: Generalise CommitSeededStencils over a seeded subtree

- **Context:**
  - `internal/fabricengine/fabric.go`
  - `internal/fabricengine/junctionnames.go`
  - `internal/fabricengine/stencilhistory.go`
- **Edits:**
  - `internal/fabricengine/stencilcommit.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Change `CommitSeededStencils` in `internal/fabricengine/stencilcommit.go` to take the seeded subtree as told parameters rather than deriving it from the package-private stencils constants.

  The current signature is `func CommitSeededStencils(hub string, writtenRelPaths []string, message string, rec *Mutations) (res StencilSeedResult, err error)`.
  The new signature inserts two parameters after `hub`:

  ```go
  func CommitSeededStencils(hub, subtreeRel, subtreeDir string, writtenRelPaths []string, message string, rec *Mutations) (res StencilSeedResult, err error)
  ```

  `subtreeRel` is the board-repo-relative, slash-separated prefix the written paths hang off (for the stencils pass, `path.Join(lyxdirs.LyxDirName, stencilsDirName)`); `subtreeDir` is the same subtree's absolute on-disk directory (for the stencils pass, `StencilsDir(hub)`).

  Inside the body, replace the local `stencilsRel := path.Join(lyxdirs.LyxDirName, stencilsDirName)` computation with the told `subtreeRel`, and pass it to `ScopedPathspec` unchanged — the call stays `ScopedPathspec(subtreeRel, writtenRelPaths)`, so the Fabric Git Invariant's positive-only-file-list requirement is satisfied in exactly one place, as it is today.

  Replace the mutation-record append's `filepath.Join(StencilsDir(hub), relPath)` with `filepath.Join(subtreeDir, relPath)`.
  This is the second generalisation and it is not optional: without it every seeded spec would be recorded as having been written under the stencils directory, which the Mutation Record Invariant's "an executor appends its primitive only after it observably changed state" makes a false record rather than a cosmetic one.

  Everything else in the function is unchanged: the empty-`writtenRelPaths` early return with `Committed: false` and no lock, the board write lock acquisition and release, the deferred `res.Mutations = rec.Snapshot()`, the `gitrepo.New(BoardDir(hub)).StageAndCommit` call, the `KindCommitCreated` append against `BoardDir(hub)`, and the never-pushes behaviour.
  `StencilSeedResult` keeps its name and its two fields.

  Rewrite the function's doc comment and the file's leading comment for the widened contract: it commits whatever seeded subtree it is told about — the stencils subtree, or the deployed-specs subtree — under one board write lock with a pathspec confined to that subtree, and it is called once per seeded subtree rather than once per process.
  Name the two parameters' relationship explicitly in the doc comment: `subtreeDir` must be the absolute directory `subtreeRel` names relative to the board repository, and a caller passing a mismatched pair produces a correct commit with a false mutation record.
  Do not add a helper that derives one from the other — every caller already has both values in hand.

  In the same file, add the two exported accessors the callers need for the `subtreeRel` argument, so no production file outside `internal/fabricengine` re-joins the `_lyx` literal itself — the Lyxdirs Single-Declarer Invariant bars that:

  ```go
  func StencilsSubtreeRel() string { return path.Join(lyxdirs.LyxDirName, stencilsDirName) }

  func SpecsSubtreeRel() string { return path.Join(lyxdirs.LyxDirName, specsDirName) }
  ```

  Give both a doc comment naming them as the board-repo-relative prefixes `CommitSeededStencils`' own `subtreeRel` parameter takes, and pairing each with the absolute-directory accessor a caller passes alongside it — `StencilsDir` for the first, `SpecsDir` for the second.
  `specsDirName` is declared by batch 2 in `internal/fabricengine/junctionnames.go`; this card consumes it and does not redeclare it.

  Leave `internal/fabricengine/stencilhistory.go` alone: it serves `lyx stencil diff`, which deliberately does not cover specs.
- **Commit:** `refactor(fabricengine): generalise CommitSeededStencils over a seeded subtree`

### Card 10: Move both production callers to the new signature

- **Context:**
  - `internal/fabricengine/stencilcommit.go`
- **Edits:**
  - `cmd/lyx/stencilseed.go`
  - `internal/stencilcli/cli.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Update the two production call sites to the widened signature, with no behavioural change at either.

  In `cmd/lyx/stencilseed.go`, `seedStencilsAt` currently calls `fabricengine.CommitSeededStencils(hub, written, "lyx: seed stencils", fabricengine.NewMutations(filepath.Dir(hub)))`.
  Pass the stencils subtree explicitly, using the two accessors card 9 added rather than re-joining any path literal at the call site.

  The hub call then reads `fabricengine.CommitSeededStencils(hub, fabricengine.StencilsSubtreeRel(), fabricengine.StencilsDir(hub), written, "lyx: seed stencils", fabricengine.NewMutations(filepath.Dir(hub)))`.
  Note `seedStencilsAt` already has `baseDir := fabricengine.StencilsDir(hub)` in scope — pass that variable rather than recomputing the call.

  In `internal/stencilcli/cli.go`, the `sync` subcommand's `fabricengine.CommitSeededStencils(l.HubPath, written, "lyx: seed stencils", rec)` call takes the same treatment, passing `fabricengine.StencilsSubtreeRel()` and the `stencilsDir` variable already in scope there.

  Do not change either caller's message string, its mutation-record construction, its error handling, or its envelope.
  Do not add a specs call at either site — batch 4 does that.
- **Commit:** `refactor(fabricengine): thread the seeded subtree through both commit callers`

### Card 11: Extend the commit integration test to a second subtree

- **Context:**
  - `internal/fabricengine/stencilcommit.go`
  - `internal/fabricengine/junctionnames.go`
- **Edits:**
  - `internal/fabricengine/stencilcommit_integration_test.go`
  - `internal/fabricengine/stencilhistory_integration_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Update `internal/fabricengine/stencilcommit_integration_test.go` for the widened signature, and add the coverage the generalisation actually needs.

  Update the existing `TestCommitSeededStencils_EmptyInputIsNoOp` and `TestCommitSeededStencils_ScopedCommitExcludesUnrelatedDirt` calls to pass `StencilsSubtreeRel()` and `StencilsDir(hub)`, preserving each test's current assertions exactly — they are the regression guard that the stencils path did not change.

  Add `TestCommitSeededStencils_SecondSubtreeCommitsAndRecordsItsOwnDirectory`, which drives the same function with `SpecsSubtreeRel()` and `SpecsDir(hub)` over a file written under the specs subtree, and asserts two things:
  the commit lands (a non-empty SHA and `Committed: true`), and the resulting mutation record's file-written entry names a path under `SpecsDir(hub)` rather than under `StencilsDir(hub)`.
  The second assertion is the one that fails if a future edit reverts the mutation-record half of the generalisation while leaving the pathspec half in place — the exact partial regression card 9's doc comment warns about, and the one a passing commit would otherwise hide.

  There is a THIRD call site, in a different file, and it must move in this same card or batch 3's own verify fails to compile: `internal/fabricengine/stencilhistory_integration_test.go`'s `seedStencil` helper calls the verb at the old arity.
  Update that one call to pass `StencilsSubtreeRel()` and `StencilsDir(hub.Path)`, changing nothing else in that file — it exercises stencil history, not the commit verb's own contract, and its existing assertions are unrelated to this generalisation.
  Card 9's instruction to leave the stencil-history production file alone covers a different file and does not cover this test.

  Keep both files' `//go:build integration` tag and their existing hub fixture construction through `internal/hubforge`, per the hubforge Fabric-Fixture Invariant — no hand-assembled hub.
- **Commit:** `test(fabricengine): pin the seeded-subtree generalisation in both halves`

## Batch Tests

`verify: go test ./internal/fabricengine/... ./internal/stencilcli/... ./cmd/lyx/... && go test -tags integration ./internal/fabricengine/...` covers the signature change and both callers.

The untagged half compiles and tests the three packages the cards touch: `internal/fabricengine` (card 9), and `internal/stencilcli` plus `cmd/lyx` (card 10), which is where a missed call site shows up as a compile failure rather than a test failure.

The `-tags integration` half is required rather than optional: card 11 edits a file carrying the `//go:build integration` constraint, so the untagged invocation never compiles it and a broken assertion there would pass unnoticed.
It is scoped to `internal/fabricengine` alone, not the repository, because that is the only package whose tagged tests this batch changes; the repository-wide tagged sweep is the configured done gate's job, not this batch's.
