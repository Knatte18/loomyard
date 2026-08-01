# Batch: fabric-warp-methods

```yaml
task: 'fabric: audit and migrate all remaining direct git mutations onto Fabric'
batch: fabric-warp-methods
number: 1
cards: 2
verify: go test -tags integration -run TestFabricWarp ./internal/fabricengine/
depends-on: []
```

## Batch Scope

This batch builds the public Fabric API surface both migrations depend on: four thin, warp-only delegating methods on `*fabricengine.Fabric` (`CheckoutDetached`, `RestoreBranch`, `CurrentBranch`, `ResetHard`), plus a reconciliation of `fabric.go`'s package/struct doc that currently documents the opposite convention, plus a dedicated Tier-2 test covering the four methods directly. Nothing outside `internal/fabricengine` changes here. The external interface the next two batches consume is exactly these four methods: `*fabricengine.Fabric` structurally satisfies `websterengine.WarpBisector` (batch 2) via `CurrentBranch`/`CheckoutDetached`/`RestoreBranch`, and `builderengine.WarpResetter` (batch 3) via `ResetHard`.

Batch-local decision: the four methods live in a new file `warpforward.go` rather than being appended to `fabric.go`, keeping the delegation cluster isolated; the doc reconciliation is the only edit to `fabric.go` itself.

## Cards

### Card 1: Add four warp-only Fabric methods and reconcile fabric.go's package doc

- **Context:**
  - `internal/gitrepo/gitrepo.go`
  - `internal/gitrepo/reset.go`
  - `manifest/designs/fabric-unified-view.md`
- **Edits:**
  - `internal/fabricengine/fabric.go`
- **Creates:**
  - `internal/fabricengine/warpforward.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  - Create `internal/fabricengine/warpforward.go` in `package fabricengine`. It defines exactly four methods on `*Fabric`, each a one-line delegation to the paired `f.Warp` method on the embedded `*gitrepo.Repo`: `func (f *Fabric) CheckoutDetached(sha string) error { return f.Warp.CheckoutDetached(sha) }`; `func (f *Fabric) RestoreBranch(ref string) error { return f.Warp.RestoreBranch(ref) }`; `func (f *Fabric) CurrentBranch() (string, error) { return f.Warp.CurrentBranch() }`; `func (f *Fabric) ResetHard(sha string) error { return f.Warp.ResetHard(sha) }`. `f.Warp` already has all four methods (`internal/gitrepo/gitrepo.go`'s `CheckoutDetached`, `RestoreBranch`, `CurrentBranch`; `internal/gitrepo/reset.go`'s `ResetHard`) — do NOT re-implement or add validation; delegation only. No `Warp`/`Weft` token may appear in any of the four public method names.
  - Each method carries a doc comment stating it operates on warp exclusively (the host repo), that it is a thin delegation to the underlying `gitrepo.Repo` verb, and — for `CurrentBranch` — that it inherits `gitrepo.Repo.CurrentBranch`'s documented rejection of a detached HEAD; for `CheckoutDetached`/`ResetHard`, that the underlying method validates `sha` (`ErrInvalidSHA`) before any git spawn.
  - In `internal/fabricengine/fabric.go`, update the package doc comment (the file-leading comment block, currently ending with "only the genuinely cross-repo operations (Commit, SyncWeft, RevertWithWeft, Pull) get their own method on Fabric") AND the `Fabric` struct doc comment (currently "Warp and Weft are exported so uncoordinated, repo-specific operations go straight through gitrepo with no forwarding-method boilerplate on Fabric itself"). The revised wording must state the actual rule going forward: a single-sided, uncoordinated op gets a named `Fabric` method (rather than direct `f.Warp`/`f.Weft` field access) precisely when it must be callable from OUTSIDE the internal/fabricengine package — preserving the one-repo illusion at the public API boundary — while `f.Warp`/`f.Weft` field access remains correct for uncoordinated ops used only INSIDE the package. Name `CheckoutDetached`/`RestoreBranch`/`CurrentBranch`/`ResetHard` as the warp-only examples of this carve-out. Do not delete the existing description of `Commit`/`SyncWeft`/`RevertWithWeft`/`Pull` as the cross-repo methods; extend the doc, don't replace it wholesale.
  - Do not change `New`, `requireDir`, `SyncOptions`, `EnvSyncOptions`, `ScopedPathspec`, or the `Fabric` struct fields.
- **Commit:** `feat(fabricengine): add warp-only CheckoutDetached/RestoreBranch/CurrentBranch/ResetHard methods`

### Card 2: Tier-2 test covering the four Fabric warp methods

- **Context:**
  - `internal/fabricengine/fabric.go`
  - `internal/fabricengine/warpforward.go`
  - `internal/fabricengine/checkout_rollback_test.go`
  - `internal/fabricengine/reconcile_stale_registration_test.go`
  - `internal/fabricengine/testmain_test.go`
  - `internal/gitrepo/gitrepo.go`
  - `internal/lyxtest`
- **Edits:** none
- **Creates:**
  - `internal/fabricengine/warpforward_integration_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  - Create `internal/fabricengine/warpforward_integration_test.go` with `//go:build integration` as its first line, in `package fabricengine_test` (so it can reuse `newFabricFixture(t)` and `currentBranchOf(t, path)` from `reconcile_stale_registration_test.go`, exactly as `checkout_rollback_test.go` does; the shared `TestMain` in `testmain_test.go` already wires `lyxtest.HermeticGitEnv()`). Import `github.com/Knatte18/loomyard/internal/fabricengine` and `github.com/Knatte18/loomyard/internal/lyxtest`.
  - Build a real paired Fabric per test: `fixture := newFabricFixture(t)`, then `f, err := fabricengine.New(fixture.Layout.WorktreeRoot, fixture.Layout.WeftWorktree())`. All four methods operate on the warp (host) worktree at `fixture.Layout.WorktreeRoot`; make commits there via `lyxtest.MustRun(t, fixture.Layout.WorktreeRoot, "git", ...)`.
  - Every test function is named with the prefix `TestFabricWarp` so the batch `verify:` `-run TestFabricWarp` selects them (e.g. `TestFabricWarp_DetachVerifyRestoreRoundTrip`, `TestFabricWarp_RestoreBranchInvalidRefErrors`, `TestFabricWarp_ResetHardDiscardsCommitsAndWorktreeChanges`, `TestFabricWarp_CurrentBranchErrorsOnDetachedHead`).
  - Coverage required (per the discussion's `new-fabric-method-tests` decision): (a) round-trip — capture `f.CurrentBranch()`, make N commits on warp, `f.CheckoutDetached(<older sha>)` and assert HEAD is at that sha and detached, then `f.RestoreBranch(<captured branch>)` and assert HEAD is back on the original branch; (b) `f.RestoreBranch` on a non-existent ref returns a non-nil error; (c) `f.ResetHard(<older sha>)` after a later commit plus an uncommitted working-tree change discards both — HEAD lands at the older sha and the later file is gone; (d) `f.CurrentBranch()` returns a non-nil error when HEAD is already detached (matching `gitrepo.Repo.CurrentBranch`'s documented detached-HEAD rejection).
  - Use `currentBranchOf(t, fixture.Layout.WorktreeRoot)` and/or `lyxtest.MustRun(...)` `rev-parse`/`symbolic-ref` reads to assert HEAD state; do not add new fixture helpers if the existing ones suffice.
- **Commit:** `test(fabricengine): cover the four warp-only Fabric methods (Tier 2)`

## Batch Tests

`verify: go test -tags integration -run TestFabricWarp ./internal/fabricengine/` compiles the whole `fabricengine` package (production + all integration test files) and runs only the four new `TestFabricWarp*` functions — direct coverage of the four delegating methods' happy and error paths against real git, independent of the webster/builder fakes that never call the real methods. Scoped by `-run` because the full `internal/fabricengine` integration suite is large; the four new methods are the only surface this batch adds.
