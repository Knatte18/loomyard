# Batch: shuttle-detached-runner

```yaml
task: 'webster standalone mode: run refuses to start Master; logs write untracked into target repo'
batch: 'shuttle-detached-runner'
number: 1
cards: 2
verify: go test ./internal/shuttleengine/...
depends-on: []
```

## Batch Scope

This batch delivers the whole `internal/shuttleengine` side of F16: a third told path (`paneCwd`) carried on `Runner` and consumed by the fork audit, and a second constructor (`NewDetachedRunner`) for the legitimately-detached anchor/worktree pair that standalone geometry produces.
Nothing outside `internal/shuttleengine` changes, and no existing caller of `NewRunner` is touched — the two standalone CLI wirings that will call `NewDetachedRunner` live in batch 3, which depends on this batch for the constructor's existence.

The external interface batch 3 consumes is exactly one exported function:

```go
func NewDetachedRunner(reed ReedOps, engine Engine, anchorPath, worktreeRoot, paneCwd string, cfg Config) *Runner
```

Batch-local decision that differs from nothing in `## Shared Decisions`: card 1 is ordered ahead of card 2 deliberately, because `NewDetachedRunner` sets `paneCwd` explicitly and cannot be written until the field exists.

## Cards

### Card 1: carry a told paneCwd on Runner and use it as the fork audit's workdir

- **Context:**
  - `internal/shuttleengine/attach.go`
  - `internal/shuttleengine/rundir.go`
- **Edits:**
  - `internal/shuttleengine/run.go`
  - `internal/shuttleengine/wait.go`
  - `internal/shuttleengine/run_test.go`
  - `internal/shuttleengine/wait_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Add a `paneCwd string` field to the `Runner` struct in `internal/shuttleengine/run.go`, placed immediately after `worktreeRoot`.
  `NewRunner` keeps its exact current signature and sets `paneCwd: anchorPath` in the struct literal it returns, so hub behaviour is byte-for-byte what it is today;
  do not add a `paneCwd` argument to `NewRunner`, and do not add any new validation to `validateToldPaths` in this card.
  Update `Runner`'s own doc comment and `NewRunner`'s doc comment to name the third told path and to state that `NewRunner` derives it from `anchorPath` rather than being told it.
  In `internal/shuttleengine/wait.go`, change `finalize`'s `AuditForks` call to pass `run.runner.paneCwd` in place of `run.runner.anchorPath`.
  That call is the only consumer that moves;
  `runDirRoot(r.cfg, r.anchorPath)`, the `reedengine.LoadState` orphan-sweep lookup, and `FindRun` all keep reading `r.anchorPath` unchanged.
  In `internal/shuttleengine/run.go`, `validateToldPaths`' doc comment currently describes `anchorPath` as "the pane's own process cwd that the fork audit derives the provider's transcript directory from" — rewrite that clause so `anchorPath`'s enumerated consumers are the run-dir root and reed's state lookup only, and the fork audit's workdir is attributed to the new `paneCwd` field instead.
  Leave `validateToldPaths`' body, its three error strings, and its containment clause exactly as they are.
  In `internal/shuttleengine/wait_test.go`, the existing fork-audit assertion compares `call.Workdir` against `runner.anchorPath`;
  it must keep passing, since `newWaitTestRunner` builds through `NewRunner` where `paneCwd == anchorPath`.
  Extend that same test file with a case that builds a runner whose `paneCwd` differs from `anchorPath` and asserts `AuditForks` is handed the pane cwd — construct it by assigning `runner.paneCwd` directly in the test (an in-package test may do so), since `NewDetachedRunner` does not exist until card 2.
  In `internal/shuttleengine/run_test.go`, extend `newTestRunner`'s doc comment to record that the fixture's `paneCwd` is `anchorPath` by construction.
- **Commit:** `fix(shuttle): carry a told paneCwd on Runner and use it as the fork audit workdir`

### Card 2: add NewDetachedRunner for the legitimately-detached anchor/worktree pair

- **Context:**
  - `internal/standalonegeom/reedgeom.go`
  - `internal/hubgeom/hubgeom.go`
  - `CONSTRAINTS.md`
- **Edits:**
  - `internal/shuttleengine/run.go`
  - `internal/shuttleengine/doc.go`
  - `internal/shuttleengine/run_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Add `NewDetachedRunner(reed ReedOps, engine Engine, anchorPath, worktreeRoot, paneCwd string, cfg Config) *Runner` to `internal/shuttleengine/run.go`, immediately after `NewRunner`.
  It builds the same `Runner` literal `NewRunner` builds, except that `paneCwd` is the told parameter rather than a copy of `anchorPath`, and `toldErr` is `validateDetachedToldPaths(anchorPath, worktreeRoot, paneCwd)`.
  Like `NewRunner` it stays total — the verdict is held on `toldErr` and surfaced by every public entry point, never returned from the constructor.
  Add `validateDetachedToldPaths(anchorPath, worktreeRoot, paneCwd string) error` beside `validateToldPaths`.
  It keeps the non-empty and absolute-path checks in the same shape `validateToldPaths` uses, extended to cover `paneCwd` as a third path, and replaces the containment clause with strict disjointness: the call is refused when `anchorPath == worktreeRoot`, when `anchorPath` is `worktreeRoot` or a subdirectory of it, or when `worktreeRoot` is `anchorPath` or a subdirectory of it.
  Compute containment the same way `validateToldPaths` does, with `filepath.Rel` plus a `".."` / `".."+string(filepath.Separator)` prefix test, run in both directions.
  Make no relational assertion at all between `paneCwd` and the other two paths — it is non-empty and absolute and nothing more.
  Every error string this function returns names `NewDetachedRunner` rather than `NewRunner`, so an operator reading a live error can tell which constructor refused.
  Give `NewDetachedRunner` a doc comment that records four things: that its anchor is deliberately outside its worktree root and that this is standalone mode's real geometry rather than a swap;
  that its only sanctioned callers are a standalone CLI's own wiring;
  that disjointness catches hub geometry handed here by mistake in both of hub geometry's shapes — a root-anchored hub gives `anchorPath == worktreeRoot` and a subpath-anchored hub gives an anchor strictly inside the worktree;
  and, stated plainly rather than implied away, that what disjointness cannot catch is a pure swap of two already-disjoint directories, a residual bounded only by there being exactly two call sites, both pinned by wiring tests.
  Give `validateDetachedToldPaths` a doc comment stating why `paneCwd` gets no relational assertion: it equals `anchorPath` under hub geometry and `worktreeRoot` under standalone geometry, so any relation strong enough to be worth asserting would be false in one of the two modes.
  In `internal/shuttleengine/doc.go`, the package doc currently says `NewRunner` validates the pair as "absolute, non-empty, anchor inside-or-equal worktree root" as though that were the package's only construction rule;
  extend that paragraph to name both constructors and both rules, keeping `NewRunner`'s description unchanged and adding the detached one beside it.
  In `internal/shuttleengine/run_test.go`, add a refusal table for `NewDetachedRunner` mirroring `TestNewRunner_RefusesUnusableToldPaths`'s shape — driving every public entry point, not just `Start` — with rows for: empty anchor, empty worktree root, empty `paneCwd`, relative anchor, relative worktree root, relative `paneCwd`, the two paths equal, the anchor strictly inside the worktree, and the worktree strictly inside the anchor.
  Add an acceptance test for `NewDetachedRunner` covering the standalone shape (two disjoint absolute directories) and asserting the told-path verdict is clean through a public entry point rather than by reading the struct field, plus two positive `paneCwd` rows — one equal to `anchorPath` and one equal to `worktreeRoot` — since those are exactly the hub and standalone shapes and neither is refused.
  Add one row to the existing `TestNewRunner_RefusesUnusableToldPaths` table asserting `NewRunner` still refuses the standalone pair, so the two constructors are provably not interchangeable.
  `TestNewRunner_AcceptsHubGeometryShapes` is not edited.
- **Commit:** `feat(shuttle): add NewDetachedRunner for the standalone detached anchor pair`

## Batch Tests

`verify: go test ./internal/shuttleengine/...` runs the package's whole untagged suite, which is where every assertion in both cards lands: `run_test.go` (both refusal tables, the hub-shapes acceptance table, the new detached acceptance table) and `wait_test.go` (the fork-audit workdir cases).
The scope is the single package both cards edit, so no broader run is warranted at this batch boundary;
the overview's module-wide `verify: go build ./...` catches any compile-level fallout in the packages that import `shuttleengine` without paying for their tests here.

The load-bearing proof that hub mode did not move is that `TestNewRunner_RefusesUnusableToldPaths`' pre-existing five rows, `TestNewRunner_AcceptsHubGeometryShapes`, and `wait_test.go`'s existing `call.Workdir != runner.anchorPath` assertion all keep passing without being rewritten.
