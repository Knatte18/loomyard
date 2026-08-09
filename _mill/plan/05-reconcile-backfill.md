# Batch: reconcile backfill

```yaml
task: 'fabric: store the warp-URL binding in weft:main; fold bootstrap into clone (slice 10)'
batch: 'reconcile backfill'
number: 5
cards: 4
verify: go build ./... && go test -tags integration ./internal/fabricengine/ ./internal/fabriccli/
depends-on: [4]
```

## Batch Scope

This batch delivers the migration path: every hub that exists today is unbound, so `Topology.Reconcile` gains a once-per-hub backfill that writes the record from the warp side's `origin` remote, and the reconcile CLI handler commits and pushes it through `Bolt`.
It is one batch because the engine half, the handler half, and their tests are meaningless apart — the outcome enum is split across the two layers by design, and only the pair proves the split is right.

It is sequenced after batch 4 rather than beside batch 3 because it edits `internal/fabriccli/fabric.go`, which batch 3 also edits.

Batch-local decisions:
- The engine's outcome set is exactly `recorded` / `present` / `diverged` / `skipped` / `deferred`.
  `record_failed` is set only by the CLI, because the commit and push that can fail happen after `Reconcile` has returned.
- A local write failure inside the engine returns `deferred` with the error in the detail, not `record_failed`.
  `deferred` means "not written this pass, retry next reconcile", which is exactly what a failed write leaves behind, and it keeps `record_failed` a CLI-only value as decided.

## Cards

### Card 13: engine-side repo-wide binding backfill

- **Context:**
  - `internal/fabricengine/warpbinding.go`
  - `internal/fabricengine/boardweft.go`
  - `internal/fabricengine/bolt.go`
  - `internal/fabricengine/junctionnames.go`
  - `internal/fabricengine/topology.go`
  - `internal/gitexec/gitexec.go`
  - `internal/lyxcwd/anchor.go`
- **Edits:**
  - `internal/fabricengine/reconcile.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Declare a `WarpBindingOutcome` string type beside `ReconcileAction`, with exported consts following the same naming shape as `ReconcileActionWeftRecreated`:
  `WarpBindingOutcomeRecorded = "recorded"`, `WarpBindingOutcomePresent = "present"`, `WarpBindingOutcomeDiverged = "diverged"`, `WarpBindingOutcomeSkipped = "skipped"`, `WarpBindingOutcomeDeferred = "deferred"`, `WarpBindingOutcomeRecordFailed = "record_failed"`.
  Each carries a doc comment stating its meaning;
  `WarpBindingOutcomeRecordFailed`'s comment states explicitly that `Topology.Reconcile` never returns it and that only the CLI handler sets it, because the commit and push that can fail happen after `Reconcile` returns.

  Add two repo-wide fields to `ReconcileResult`, beside `Pairs`:
  `WarpBinding WarpBindingOutcome` with tag `json:"warp_binding"` and `WarpBindingDetail string` with tag `json:"warp_binding_detail,omitempty"`.
  Their doc comments must record that the check runs exactly once per `Reconcile` call, after the pair loop, and is never reported per-pair — the binding is a once-per-hub fact written to the board directory, so running it inside the per-worktree loop would repeat it N times and leave "which pair owns a repo-wide fact" unanswerable.
  Note also, on the struct tags, that `runReconcile` hand-builds its envelope and never serializes this struct, so the tags document intent rather than driving output.

  Add `func (t *Topology) reconcileWarpBinding(l *lyxcwd.Location) (WarpBindingOutcome, string)`, called exactly once from `Reconcile` after the `for _, entry := range entries` loop completes and before the final `return`, assigning into `result.WarpBinding` and `result.WarpBindingDetail`.
  It never returns an error and can never fail the reconcile — this mirrors the rule already stated in this file for `wireBoardLink`, whose failure is appended as a Detail note and may never downgrade a reconcile verdict.

  Its logic, in order:

  1. Resolve the board directory as `BoardDir(l.HubPath)`.
  2. Read the warp side's remote: `gitexec.RunGit([]string{"remote", "get-url", "origin"}, l.WorktreePath())`.
     The URL is read from the worktree reconcile was invoked from, never from a loop entry.
     A non-nil error, a nonzero exit, or an empty trimmed result means the warp side has no `origin` — return `(WarpBindingOutcomeSkipped, "")`.
     An absent remote is a legitimate state (a synthetic test hub, a locally-initialised warp), not an error condition.
     `git remote get-url` is read-only and therefore outside the Fabric Git Invariant's mutating-warp-git rule, and it lives in `fabricengine` regardless.
  3. `recorded, found := readWarpBinding(boardDir)`.
  4. When `found`:
     - `normalizeWarpURL(recorded) == normalizeWarpURL(origin)` → `(WarpBindingOutcomePresent, "")`.
     - otherwise → `(WarpBindingOutcomeDiverged, detail)` with the record left untouched and no git call made.
       The detail names both URLs.
       When `warpURLTransportIdentity(recorded) == warpURLTransportIdentity(origin)` the detail additionally states that the two spellings differ only by transport and that this is advisory — the record does not describe the transport in use — rather than a fault.
       Divergence is reported, never overwritten and never fatal: the same never-silently-re-point rule clone follows, but reconcile is the repair verb and must not be blocked by an unrelated URL mismatch.
  5. When not `found`:
     - Check the board worktree is clean first: `gitexec.RunGit([]string{"status", "--porcelain"}, boardDir)`.
       A non-nil error or nonzero exit → `(WarpBindingOutcomeDeferred, detail)` naming the failure.
       Non-empty trimmed output → `(WarpBindingOutcomeDeferred, detail)` naming the dirty board.
       This check exists because `Bolt.Commit` is stage-all: safe at clone time, when the board was created seconds earlier, but not at reconcile time, when a long-lived board may carry unrelated uncommitted content a backfill commit would sweep up and push.
       The check is read-only and runs before anything is written, so no half-written state is ever left behind.
     - Otherwise `writeWarpBinding(boardDir, origin)`.
       On failure → `(WarpBindingOutcomeDeferred, detail)` carrying the write error.
       On success → `(WarpBindingOutcomeRecorded, detail)` naming the URL recorded.

  Refusing to write is deliberately the engine's call rather than the handler's: the engine is what writes the file, and refusing to write is strictly better than writing and then discovering the commit cannot be made safely.
  Do not widen `Bolt` with a scoped-pathspec commit — it is deliberately a narrow stage-all handle.
- **Commit:** `feat(fabricengine): backfill the warp binding once per reconcile`

### Card 14: reconcile handler commits, pushes, and reports the binding

- **Context:**
  - `internal/fabricengine/reconcile.go`
  - `internal/fabricengine/bolt.go`
  - `internal/fabricengine/weftgit.go`
  - `internal/gitrepo/push.go`
  - `internal/fabriccli/clone.go`
- **Edits:**
  - `internal/fabriccli/fabric.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Edit `runReconcile` only — leave `runPairs`, `runPruneWithFlag`, and the clone command declaration alone.

  Today the handler returns `output.Ok(out, map[string]any{"pairs": r.Pairs})` and never serializes `ReconcileResult` itself, so the new struct tags alone would have no effect on output.
  Build the payload explicitly instead:

  - Start from local `binding := r.WarpBinding` and `detail := r.WarpBindingDetail`.
  - Construct a `Bolt` over `fabricengine.BoardDir(l.HubPath)` only when `binding` is `WarpBindingOutcomeRecorded` or `WarpBindingOutcomePresent`;
    `diverged`, `skipped`, and `deferred` pass through untouched with no git call at all.
  - Commit only on `recorded`, with a message naming the operation (for example `fabric reconcile: record warp binding`) and a zero-valued `fabricengine.SyncOptions{}`.
    On failure set `binding` to `WarpBindingOutcomeRecordFailed` and replace `detail` with the error text, and do not attempt the push.
  - Push on both `recorded` and `present`.
    The `present` push is what retries a backfill that committed locally but failed to push: without it, the next reconcile sees the record on disk, returns `present`, and a commit-only-on-`recorded` handler would skip it forever.
    It costs nothing when there is nothing to push — `Bolt.Push` reaches `gitrepo.PushCoalesced`, which checks `HasUnpushed` (a purely local rev-list) and returns nil without contacting the remote when HEAD is in sync.
    Add a short comment recording the caveat that `HasUnpushed` treats *no configured upstream* as unpushed, so a board worktree with no upstream attempts a network push on every reconcile.
    That is the adopt path's non-case — a board on an already-existing default branch carries its upstream from the initial clone — but it IS the steady state for a hub bootstrapped against a genuinely empty weft remote, whose board branch is orphan-created with no upstream at all.
    The attempt is harmless (it either succeeds or yields `record_failed` with the error in the detail), but the comment must not claim it never happens.
    On push failure set `binding` to `WarpBindingOutcomeRecordFailed` and set `detail` to a message distinguishing the two paths — on the `present` path it must say a previously committed record could not be pushed.
  - A failed commit or push is non-fatal: the handler still returns `output.Ok` with an unchanged exit code.
    This mirrors the precedent already in `Reconcile` itself, where a board-junction wiring failure is a Detail note that may never downgrade a reconcile verdict, and it keeps offline reconcile working for a fact the next reconcile can persist just as well.
  - Emit `"pairs"` as today, `"warp_binding"` unconditionally as the string form of `binding` (it always holds one of the six values), and `"warp_binding_detail"` only when `detail` is non-empty.

  Update `runReconcile`'s doc comment to describe the new commit/push responsibility and the CLI-only `record_failed` value.
- **Commit:** `feat(fabriccli): commit, push, and report the reconcile warp-binding backfill`

### Card 15: reconcile backfill integration tests

- **Context:**
  - `internal/fabricengine/reconcile.go`
  - `internal/fabricengine/warpbinding.go`
  - `internal/fabricengine/clone.go`
  - `internal/fabricengine/config.go`
  - `internal/fabricengine/bolt.go`
  - `internal/fabricengine/clone_adopt_test.go`
  - `internal/fabricengine/reconcile_stale_registration_test.go`
  - `internal/fabricengine/reconcile_stale_removal_test.go`
- **Edits:** none
- **Creates:**
  - `internal/fabricengine/warpbinding_reconcile_integration_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  New file, first non-empty line `//go:build integration`, `package fabricengine_test`, sharing the package's existing `TestMain` — do not add another.
  File-level comment naming what it proves and which helpers it reuses.

  Add one local fixture helper, `newClonedHubFixture(t *testing.T)`, that builds a genuinely remote-backed hub rather than a synthetic one:
  create warp and weft bare fixtures with `makeBareRemote`;
  call `fabricengine.CloneHub` with both URLs and `ForceBootstrap: true` to materialize the hub;
  seed the repo-wide fabric config at the hub with `seedRepoWideFabricConfig`;
  resolve the prime layout with `lyxcwd.Resolve` on the result's `PrimeCwd`;
  load the config with `fabricengine.LoadConfig` at the board directory and build a `Topology` with `fabricengine.NewTopology`.
  Return whatever the tests need (the `CloneResult`, the layout, the topology, and the two bare paths).
  This shape is required rather than `lyxtest.CopyPairedLocal` because the backfill reads the warp side's `origin` and the push half needs a real weft remote — neither exists in a purely local paired fixture.

  The fixture must leave the board worktree CLEAN, or every `recorded`-outcome test gets `deferred` instead.
  `CloneHub` writes the anchor marker and the fixture seeds the repo-wide config, but neither is committed — `CloneHub` never commits, only the CLI's `Bolt.Commit` does — so the board is dirty the moment the helper returns.
  End `newClonedHubFixture` with a local stage-all commit through `fabricengine.NewBolt(res.BoardDir).Commit(...)` with a zero-valued `fabricengine.SyncOptions{}`, and no push.

  Since `CloneHub` itself writes the record, every test that exercises the absent-record path must delete the board's binding file and commit that deletion first;
  give the helper a companion `unbindHub` that removes the file from the board worktree and commits that deletion the same way, through `fabricengine.NewBolt(boardDir).Commit(...)`.
  Do NOT reach for `commitFileOnBranch` here — it commits through a throwaway scratch clone pushed to the bare remote and never touches an existing local worktree, so it cannot produce a clean board.
  Producing the "unbound hub that predates the binding" state explicitly, rather than assuming it, is the point of the helper.

  Tests:

  - `TestReconcile_BacksFillsBindingOnce` — an unbound wired hub gains the record: `WarpBinding` is `recorded`, the file exists at the board root with the warp `origin` URL, and no pair result carries any binding-related field or detail.
  - `TestReconcile_BacksFillsOnceOnMultiWorktreeHub` — with several warp worktrees present (add one with the package's existing add/worktree path), the record is written once and `WarpBinding` is reported exactly once at the repo-wide level, with the per-pair results untouched.
  - `TestReconcile_NormalizedRecordReportsPresent` — a record differing from the warp `origin` only by a trailing `.git` reports `present`, not `diverged`.
  - `TestReconcile_DivergentRecordIsLeftUntouched` — a genuinely differing record is not overwritten, reports `diverged` with both URLs in the detail, and `Reconcile` still returns a nil error.
  - `TestReconcile_TransportOnlyDifferenceIsAdvisory` — a record spelling the same repo over a different transport reports `diverged` with a detail that says the difference is transport-only and advisory.
  - `TestReconcile_DirtyBoardDefersWrite` — with an unrelated uncommitted file in the board worktree, the outcome is `deferred`, no binding file is written, and the unrelated file is neither committed nor pushed (assert it is still untracked and the board's HEAD is unchanged).
  - `TestReconcile_NoWarpOriginIsSkipped` — after removing the warp side's `origin` remote, the outcome is `skipped` with an empty detail and `Reconcile` still returns a nil error.
  - `TestReconcile_UnpushedRetryPushesOnPresent` — after a backfill whose push failed, restore the remote and run reconcile again: the already-committed record is pushed, the outcome is `present`, and no second commit is created (compare the board's commit count before and after).
    Because this test needs the CLI's commit-and-push half to have run at least once, drive both reconcile passes through the CLI seam rather than `Topology.Reconcile` directly, or leave the pushed-state setup to card 16 and assert here only on the engine's `present` verdict — pick one and say which in the test's doc comment.
  - `TestUnwire_LeavesWarpBindingInPlace` — running unwire on a bound hub leaves the record on the weft side untouched, exactly as it already leaves the anchor marker and the repo-wide config.
    This requires no production change; it exists so a future unwire edit cannot quietly delete the record.

  Read the binding filename through `fabricengine.WarpBindingFileName` in every assertion — never a hardcoded literal.
- **Commit:** `test(fabricengine): cover the reconcile warp-binding backfill and its failure modes`

### Card 16: CLI-level reconcile envelope tests

- **Context:**
  - `internal/fabriccli/fabric.go`
  - `internal/fabricengine/reconcile.go`
  - `internal/fabricengine/warpbinding.go`
- **Edits:**
  - `internal/fabriccli/cli_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Two tests, driven through the `fabriccli.RunCLI` seam so the commit-and-push half of the backfill is actually exercised — the `record_failed` value is set only by the handler and cannot be observed from an engine-only test.
  Build the hub with the file's existing `makeCLICloneWarpBare` and `makeCLICloneWeftBare` helpers and a two-positional clone through the same seam, then delete the binding record from the board worktree and commit that deletion with plain git (the file's existing `lyxtest.MustRun` idiom), so the hub is in the unbound, pre-binding state the backfill exists for and the board is left clean.
  No `--force-bootstrap` is needed: `makeCLICloneWeftBare` creates a genuinely empty bare repo, which is the unborn-HEAD case the weft-candidate guard admits on its own.
  The CLI clone commits the anchor and the repo-wide config through `Bolt` as part of its normal run, so the board is already clean before the deletion commit.

  `TestRunCLI_ReconcileBacksFillsWarpBinding` — running reconcile against that hub exits 0 and returns an envelope carrying `warp_binding` equal to `recorded`, and the record is tracked on the board worktree afterwards.
  Assert the `pairs` key is still present and unchanged in shape — the binding is reported repo-wide, never per-pair.

  `TestRunCLI_ReconcileBackfillFailureIsNonFatal` — point the weft remote at an unreachable path so the push fails, then run reconcile.
  The envelope must report `warp_binding` as `record_failed` with a non-empty `warp_binding_detail`, and the exit code must still be 0.
  The assertion on the exit code is the point: a failed backfill commit or push is non-fatal, mirroring the board-junction precedent that a convenience repair may never downgrade a reconcile verdict.

  Read the binding filename through `fabricengine.WarpBindingFileName`, never a literal.
- **Commit:** `test(fabriccli): cover the reconcile warp-binding envelope and its non-fatal failure`

## Batch Tests

`verify:` is `go build ./... && go test -tags integration ./internal/fabricengine/ ./internal/fabriccli/`.

The tag is required because the only new test file, `internal/fabricengine/warpbinding_reconcile_integration_test.go`, carries `//go:build integration`, and because one test in card 15 may land in `internal/fabriccli/cli_test.go`, which is integration-tagged in full.

Both packages are in scope because this batch changes both layers: the engine's `ReconcileResult` shape and the CLI handler's envelope.
`go build ./...` covers the new exported identifiers reaching their consumers.
The overview's module-wide `go build ./...` repeats that gate at the batch boundary.
