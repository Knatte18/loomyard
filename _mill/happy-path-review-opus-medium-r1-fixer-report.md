# happy-path fixer report — opus-medium-r1

Review: `_mill/happy-path-review-opus-medium-r1.md` (committed `db795663f` before any source change).

## Fixed

| Finding | Severity | Commit(s) | What |
|---|---|---|---|
| F1 | BLOCKING | `242d46983` | `fabricengine.CloneHub` names the weft primary (`<warp branch>-weft`) and `_board` after the warp prime's checked-out branch, not the weft clone's unborn HEAD. Regression test `TestCloneHub_EmptyWeftRemoteWithForeignDefaultBranch` (integration). |
| F3 | MEDIUM | `5550dd00d` | `landingshed.Finalize` pushes the parent branch after a successful parent-side merge (`parentMerger.PushBranch`, honouring `PushSkipped`); a failed push is Stuck with a push-by-hand reason. Test `TestFinalize_PushesParentAfterMerge`; `landingshed` doc updated. |
| F4 | MEDIUM | `efc1f7053` | Partial: `lyx loom start`/`lyx start` and `lyx shed seed` help, and the driver launch prompt, name loomyard's `ly` plugin as the ly-drive skill's source. Provisioning the skill without the plugin: NOT-FIXED-THIS-ROUND (below). |
| F5 | BLOCKING | `c4f0b1a39`, `3d59b14c6` | ly-drive writes step envelopes to a private `mktemp -d` directory, never under the drive directory. The follow-up commit keeps the new sentence recipe-blind (`TestLyDriveSkill_IsRecipeBlind`). |
| F6 | MEDIUM | `f51cb430f` | `yamlengine` carries lists whole through `Reconcile` and `SetValues`, and `--set key=[...]` replaces a list whole (`[]` included). Tests `TestSetValues_ListKeys`, `TestSetValues_ListKeyRejectsNonListValue`, `TestReconcile_CarriesListsWhole`; `lyx config --help` shows the list form. |
| F8 | NIT | `655bcb6be` | ly-drive says to wait on the background job's own completion or PID, never a command-line pattern match. |
| F9 | MEDIUM | `761dc64a5` | `lyx shed seed --help` example seeds loom as its bootstrap does (`self`, `--param parent=<branch>`) and says a pre-seed must match. |

## Not fixed

- **F7** (LOW, NOT-FIXED-THIS-ROUND): a Stuck row with no `on_stuck` reports only `stuck with no OnStuck target`; carrying a producer's reason into `Status.Reason` changes the `shedengine.ShedProducer` seam every producer implements and needs its own design step.
- **F4 remainder** (NOT-FIXED-THIS-ROUND): `lyx` still cannot make the skill available to a driver session on a machine without the `ly` plugin installed; the session falls back to searching the filesystem.
  Shipping the skill bytes with the binary would add a third `//go:embed` registry, which the Stencil Ownership Invariant does not allow without a design decision on where the skill lives.

## Test commands

(pending)

## Final re-drive

(pending)

## Changed files

(pending)

### Re-drive log

- Hub4 (`$HOME/crucible-happy-path/opus-medium-r1/hub4`), dev binary `bc9004f74`, weft bare left on its host-default `master` HEAD (re-verifies F1): clone → weft prime `main-weft`, `_board` on `main`; `lyx config landing --set 'require_pr_to_base=[]'` → ok (F6).
- go driver, task `add-sub`: `board upsert` → `fabric add add-sub` → `shed seed self --recipe loom --driver go --param parent=main` → `loom start --no-attach` → `done` at 15:55 (history 18), no intervention.
  Bare `warp.git` `main` = `bc9c17f Add Sub to calc` (F3); prime `main...origin/main` in sync.
