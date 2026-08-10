# Batch: dirtiness-probe

```yaml
task: 'fabric: one ownership-and-dirtiness gate for all destruction (slice 12)'
batch: 'dirtiness-probe'
number: 1
cards: 3
verify: go test ./internal/fabricengine/...
depends-on: []
```

## Batch Scope

This batch delivers the smaller, independent half of the slice: the eight hand-rolled `git status --porcelain` probes in `internal/fabricengine` collapse into one implementation in a new `internal/fabricengine/dirtiness.go`, with the tracked-vs-untracked-inclusive choice becoming a declared parameter instead of a per-site spelling.
It is one batch because the eight sites are mechanical replacements of one another and share a single new file;
splitting them would leave the package with two probe implementations mid-plan, which is the disease.

It runs first and depends on nothing.
The external interface batch 2 consumes is exactly two identifiers: the `dirtyScope` type with its two constants, and `worktreeDirty`.
Nothing in this batch mentions the gate, ownership, containment or force — those arrive in batch 2, which declares its dirtiness checks in terms of the `dirtyScope` values this batch defines.

Batch-local decision beyond `## Shared Decisions`: no call site changes its dirtiness *scope*.
Four sites are tracked-only today and stay tracked-only;
four are untracked-inclusive today and stay untracked-inclusive.
Every refusal message is preserved verbatim, because these eight sites are covered by named integration tests that assert on those messages.
Error *paths* are the one exception, and it is a uniform one: `worktreeDirty` returns a single consolidated error where a site today distinguishes a spawn failure from a nonzero exit, so at those sites the two paths collapse into one and the surviving wording is the spawn-failure form, with the exit code carried inside the wrapped error.
That applies to `add.go`, `checkout.go`, `warpclean.go` and `reconcile.go`;
each card names it at the site rather than leaving it to be inferred.

## Cards

### Card 1: the single dirtiness probe

- **Context:**
  - `internal/fabricengine/prune.go`
  - `internal/fabricengine/warpclean.go`
  - `_mill/discussion.md`
- **Edits:** none
- **Creates:**
  - `internal/fabricengine/dirtiness.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** create `internal/fabricengine/dirtiness.go` in `package fabricengine` holding the package's single `git status --porcelain` implementation.
  Declare `type dirtyScope int` with two unexported constants, `scopeTracked` and `scopeAll`.
  `scopeTracked` means the probe passes `--untracked-files=no`;
  `scopeAll` means it does not.
  Declare `func worktreeDirty(scope dirtyScope, dir string) (dirty bool, detail string, err error)` which runs `gitexec.RunGit` with `[]string{"status", "--porcelain"}` plus `"--untracked-files=no"` when `scope` is `scopeTracked`, in `dir`.
  It returns a non-nil error when the spawn itself failed or the exit code was nonzero, carrying `strings.TrimSpace(stderr)` and the exit code in the message;
  otherwise it returns `strings.TrimSpace(stdout) != ""` as `dirty` and that same trimmed stdout as `detail`.
  `detail` exists because `dirtyReason` in `internal/fabricengine/warpclean.go` returns the porcelain text itself to its caller;
  every other site ignores it.
  The file's header comment must state that this is the package's sole porcelain-status probe, that scope is the caller's declared choice rather than a property of the primitive, and that `git worktree list --porcelain` in `worktreelist.go` is a different command outside this file's remit.
  Add no exported identifier — every consumer is in-package.
  Add nothing to the `gitrepo` package;
  the `dirtiness-probe-stays-fabric-local` decision in `_mill/discussion.md` records why, and its reasoning must be summarised in the file header.
- **Commit:** `refactor(fabricengine): add the single dirtiness probe in dirtiness.go`

### Card 2: migrate the four tracked-only probe sites

- **Context:**
  - `internal/fabricengine/dirtiness.go`
- **Edits:**
  - `internal/fabricengine/add.go`
  - `internal/fabricengine/checkout.go`
  - `internal/fabricengine/prune.go`
  - `internal/fabricengine/pull.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** replace the four hand-rolled `--untracked-files=no` probes with `worktreeDirty(scopeTracked, ...)` calls, preserving each site's existing error text and control flow exactly.
  In `internal/fabricengine/add.go`, the probe at the top of `Topology.Add` against `l.WorktreePath()`: keep both existing error messages (`read warp worktree status at %s: %w` and its git-exit variant) and the `source worktree has uncommitted changes` refusal.
  Because `worktreeDirty` collapses the spawn error and the nonzero-exit error into one returned error, `Add` now has one error path where it had two;
  keep the wording of the spawn-failure form and let the exit-code detail come through the wrapped error.
  In `internal/fabricengine/checkout.go`, the probe at the top of `Topology.Checkout` against `weftWorktree`: same treatment, preserving `check weft status: %w` and the `weft worktree has uncommitted changes; stash or commit before checkout` refusal.
  In `internal/fabricengine/prune.go`, `applyStalePairProtection`: it deliberately treats *any* probe failure as unprotected, so call `worktreeDirty(scopeTracked, weftPath)` and return early when `err != nil`, preserving that documented direction and the existing `pe.Error` text.
  In `internal/fabricengine/pull.go`, `Fabric.warpWorktreeDirty`: keep the method, its name, its signature and its doc comment;
  replace its body with a `worktreeDirty(scopeTracked, f.warpPath)` call, keeping the `fabricengine: git status in %s: %w` error prefix.
  This method stays because `Pull` refuses with the named `ErrWarpDirty` before reaching `ResetHard`, and that named error is asserted by existing tests.
- **Commit:** `refactor(fabricengine): route the four tracked-only probes through worktreeDirty`

### Card 3: migrate the four untracked-inclusive probe sites

- **Context:**
  - `internal/fabricengine/dirtiness.go`
- **Edits:**
  - `internal/fabricengine/remove.go`
  - `internal/fabricengine/warpclean.go`
  - `internal/fabricengine/reconcile.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** replace the four probes that deliberately include untracked files with `worktreeDirty(scopeAll, ...)` calls, preserving each site's existing error text and control flow exactly.
  In `internal/fabricengine/remove.go` there are two: the `!force` warp probe inside `Topology.Remove` against `target`, keeping `check warp worktree status at %s: %w` and the `worktree has uncommitted changes; use --force` refusal;
  and `refuseDirtyWeftWorktree`, keeping its leading `os.Stat` absent-is-not-a-refusal guard, its `check weft worktree status at %s: %w` error and its `weft worktree has uncommitted changes; run "lyx fabric sync" or use --force` refusal.
  The doc comment on `refuseDirtyWeftWorktree` explains that an unreadable weft worktree IS a refusal because the probe once swallowed its own spawn error;
  that property must survive the migration, and `worktreeDirty` returning an error rather than a silent false is what preserves it.
  In `internal/fabricengine/warpclean.go`, `dirtyReason`: keep the function and its `label` parameter, and replace its body with a `worktreeDirty(scopeAll, dir)` call returning `detail`.
  It has two error formats today, a spawn-failure form and an exit-code form, and `worktreeDirty` returns one consolidated error for both — so keep the spawn-failure wording and let the exit code arrive inside the wrapped error, exactly the collapse card 2 concedes for the two sites above.
  Do not widen `worktreeDirty`'s return shape to preserve the second format: one caller wanting a distinct exit-code sentence is not worth handing every caller an exit code to re-format, and the surviving message still carries the code.
  In `internal/fabricengine/reconcile.go`, the board-status check that decides `WarpBindingOutcomeDeferred`: it treats a spawn failure and a nonzero exit identically, so a single `err != nil` branch preserves the existing `board status check failed: %v (exit %d)` behaviour — keep a `board status check failed` prefix carrying the error, and keep the `board worktree has uncommitted changes; backfill deferred to avoid sweeping them into an unrelated commit` deferral text verbatim.
- **Commit:** `refactor(fabricengine): route the four untracked-inclusive probes through worktreeDirty`

## Batch Tests

`verify: go test ./internal/fabricengine/...` runs the package's untagged tier, which is the tier this batch can break fastest — it compiles the whole package and exercises every in-package test.
Scope is deliberately the one package: no file outside `internal/fabricengine` is touched, and the module-wide `go build ./...` at the batch boundary catches any accidental exported-surface change.

No new test is written here.
This batch is a pure consolidation with every site's scope, error text and control flow preserved, so its correctness evidence is the existing suite staying green — in particular the `//go:build integration` files covering `Remove`, `Prune`, `Pull` and `Checkout`, which the batch-boundary run of batch 3's verify command reaches.
A dirtiness scope silently changing at one of the eight sites is exactly what those tests catch, and writing a fresh unit test asserting the new helper's own behaviour would prove less: batch 2's `destroy_test.go` covers the helper directly once the gate has a reason to call it.
