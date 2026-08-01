# Batch: webster-bisect-migrate

```yaml
task: 'fabric: audit and migrate all remaining direct git mutations onto Fabric'
batch: webster-bisect-migrate
number: 2
cards: 3
verify: go test -tags integration -run 'TestIntegrationStage|TestBisectAndEscalate|TestShouldRunIntegration' ./internal/websterengine/
depends-on: [1]
```

## Batch Scope

Migrate `internal/websterengine`'s in-process bisect/verify path off the raw `gitrepo.New(deps.WorktreeRoot)` construction and onto a `*fabricengine.Fabric` handle, via a narrow consumer-side interface `WarpBisector`. `bisect`, `checkoutAndVerify`, and `BisectAndEscalate` stop depending on the concrete `*gitrepo.Repo`; production constructs a real `*Fabric` inline in `runIntegrationStage`; tests inject a `*gitrepo.Repo` over their existing scratch worktree. Behavior is unchanged — bisect still detaches, verifies in-process, and restores the branch on warp only. This batch touches only `internal/websterengine` (production + one integration test file). Depends on batch 1 for the four `Fabric` methods that make `*Fabric` satisfy `WarpBisector`.

## Cards

### Card 3: Define WarpBisector and retype the bisect functions off *gitrepo.Repo

- **Context:**
  - `internal/gitrepo/gitrepo.go`
  - `internal/fabricengine/warpforward.go`
- **Edits:**
  - `internal/websterengine/integration.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  - In `internal/websterengine/integration.go`, define an exported interface `WarpBisector` with exactly three methods, matching `*gitrepo.Repo`'s and `*fabricengine.Fabric`'s signatures verbatim: `CurrentBranch() (string, error)`, `CheckoutDetached(sha string) error`, `RestoreBranch(ref string) error`. Doc-comment it as the warp-only git surface the in-process bisect drives, structurally satisfied by both `*gitrepo.Repo` (tests) and `*fabricengine.Fabric` (production).
  - Change the receiver/parameter type on all three functions from `repo *gitrepo.Repo` to `repo WarpBisector`: `bisect(repo WarpBisector, shas []string, verifyCmd string, worktree string) (offendingIndex int, err error)`, `checkoutAndVerify(repo WarpBisector, sha, verifyCmd, worktree string) (bool, error)`, and `BisectAndEscalate(repo WarpBisector, shas, labels []string, verifyCmd, worktree, websterDir string, st *State) error`. The method call bodies (`repo.CurrentBranch()`, `repo.RestoreBranch(branch)`, `repo.CheckoutDetached(sha)`) are unchanged — the interface has the same method set.
  - Remove the now-unused `github.com/Knatte18/loomyard/internal/gitrepo` import from `integration.go` (it was used only for the `*gitrepo.Repo` parameter types just retyped). Leave every other import (`errors`, `fmt`, `os`, `os/exec`, `path/filepath`, `runtime`, `time`, and the internal/planparser import) untouched. Do NOT add a `fabricengine` import here — `integration.go` depends only on the local `WarpBisector` interface.
  - No behavior change: the bisect algorithm, the empty/single-SHA degrade shapes, the deferred `RestoreBranch`, and the escalation record are all identical.
- **Commit:** `refactor(websterengine): retype bisect path onto a WarpBisector interface`

### Card 4: Construct the Fabric handle inline in runIntegrationStage via a nil-defaulted RunDeps seam

- **Context:**
  - `internal/websterengine/integration.go`
  - `internal/hubgeometry/hubgeometry.go`
  - `internal/webstercli/weft.go`
  - `internal/fabricengine/fabric.go`
- **Edits:**
  - `internal/websterengine/runlevel.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  - Add one field to the `RunDeps` struct: `Bisector WarpBisector`. Doc-comment it as the bisect-repo seam — nil (the production default) makes `runIntegrationStage` construct a real `*fabricengine.Fabric` inline via `fabricengine.New(deps.Layout.WorktreeRoot, deps.Layout.WeftWorktree())`; a test injects a `*gitrepo.Repo` fake so the bisect path never requires a paired weft fixture. Mirror the existing `Clock` field's nil-selects-production doc style.
  - In `runIntegrationStage`, replace the line `repo := gitrepo.New(deps.WorktreeRoot)` with: use `deps.Bisector` when non-nil; otherwise construct `f, err := fabricengine.New(deps.Layout.WorktreeRoot, deps.Layout.WeftWorktree())` and return the error if non-nil (propagate it exactly as the surrounding `return err` sites do — this is inside `runIntegrationStage`, which returns `error`), then use `f` as the `WarpBisector`. Pass the resolved handle as the first argument to `BisectAndEscalate(...)`; the remaining arguments (`shas`, `labels`, `plan.Verify`, `deps.WorktreeRoot`, `deps.WebsterDir`, `st`) are unchanged — `deps.WorktreeRoot` stays the verify-command cwd.
  - Swap imports: remove `github.com/Knatte18/loomyard/internal/gitrepo` (line 28 — it is used only at the replaced construction site; confirm via grep that `gitrepo` has no other use in `runlevel.go`) and add `github.com/Knatte18/loomyard/internal/fabricengine`.
  - Reference pattern for the construction call: `internal/webstercli/weft.go`'s `weftCommit` builds `fabricengine.New(layout.WorktreeRoot, weftWorktree)` from a `*hubgeometry.Layout` the same way; `Layout.WeftWorktree()` (`internal/hubgeometry/hubgeometry.go`) derives the weft path from the layout. In production every hub-managed webster worktree has a paired weft on disk, so `New` succeeds; a missing weft would surface as a real error here (accepted behavior change — bisect previously never constructed a weft-aware handle).
- **Commit:** `refactor(websterengine): construct bisect Fabric handle inline via RunDeps.Bisector seam`

### Card 5: Inject a gitrepo bisector at the newRunFixture level

- **Context:**
  - `internal/gitrepo/gitrepo.go`
- **Edits:**
  - `internal/websterengine/runlevel_test.go`
  - `internal/websterengine/integration_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  - In `newRunFixture` (`internal/websterengine/runlevel_test.go`), add `Bisector: gitrepo.New(worktree)` to the `websterengine.RunDeps{...}` struct literal it builds (the `worktree` scratch-repo path is already in scope there). This gives EVERY test built on `newRunFixture` a real warp-only `WarpBisector` over its scratch worktree — so both integration-stage tests that reach `runIntegrationStage`'s handle construction, `TestIntegrationStage_FailingForkTriggersBisectAndEscalates` AND `TestIntegrationStage_FailedSuite_DoneOutcomeFailsLoud`, exercise real detached-checkout + in-process verify + branch-restore instead of hitting `fabricengine.New`'s weft stat-check against the fixture's empty-`Hub` Layout. This is the fixture-level analog of builder Card 9's `Resetter` injection in `newSpawnFixture` — inject once, cover every consumer, rather than per-test. Add the `github.com/Knatte18/loomyard/internal/gitrepo` import to `runlevel_test.go`.
  - The other `newRunFixture` consumers never reach the bisect handle construction: `TestIntegrationStage_SkipsWhenPlanHasNoVerify` returns on the no-verify branch, `TestIntegrationStage_PassingForkFinishesNormally` returns on the OK-report branch, and the two `MissingReport` tests return before the construction at runlevel.go:845 — so the injected handle is simply unused there. Harmless.
  - In `integration_test.go`, `TestBisectAndEscalate_EmptySHAsDegradesGracefully` calls `websterengine.BisectAndEscalate(nil, nil, nil, ...)`: an untyped `nil` still satisfies the new `WarpBisector` parameter and is never dereferenced (bisect returns on the empty-shas guard before touching `repo`), so it compiles and passes unchanged. Update only its doc comment's "a nil `*gitrepo.Repo` is never dereferenced" phrasing to "a nil `WarpBisector` is never dereferenced". This doc-comment change is the ONLY edit to `integration_test.go` (the Bisector injection now lives in `runlevel_test.go`), so `integration_test.go` needs no new import.
  - Do not weaken or delete any existing assertion in either file. Both files are `//go:build integration`, so they are exempt from the Test Tier Purity guard and the batch-4 production-source regression guard (which scans non-`_test.go` files only).
- **Commit:** `test(websterengine): inject a gitrepo WarpBisector at the newRunFixture level`

## Batch Tests

`verify: go test -tags integration -run 'TestIntegrationStage|TestBisectAndEscalate|TestShouldRunIntegration' ./internal/websterengine/` compiles the package (proving the production migration builds with the swapped imports) and runs the integration-stage suite — including `TestIntegrationStage_FailingForkTriggersBisectAndEscalates` (the real-git bisect+restore path now driven through the injected `WarpBisector`) and `TestBisectAndEscalate_EmptySHAsDegradesGracefully` (the nil-interface degrade). Scoped by `-run` to the integration-stage tests this batch touches; the broader `internal/websterengine` suite is not affected by these signature/seam changes.
