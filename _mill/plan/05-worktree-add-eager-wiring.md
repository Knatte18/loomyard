# Batch: worktree-add-eager-wiring

```yaml
task: 'fabric: clone-does-everything + subpath-in-weft + init dissolution'
batch: worktree-add-eager-wiring
number: 5
cards: 3
verify: go test -tags integration ./internal/fabricengine/...
depends-on: [1, 2]
```

## Batch Scope

This batch folds junction wiring into `Topology.Add` so every new worktree is wired immediately, dropping today's dormant-`_lyx` model (add.go:304). `Add` currently creates the host worktree, the create-or-adopt weft, the portal junction, and launchers, but leaves the `_lyx`/`_pattern` host junctions dormant for a later `lyx init`. After this batch, `Add` wires them eagerly (using the repo-wide junction name-set from `BoardDir`) and `rollbackAdd` unwires them on a mid-add failure. This makes fabric the sole end-to-end wirer, a precondition for deleting `init` in batch 6.

Depends on batch 1 (the wired junction endpoints are `WorktreeRoot`+`RelPath`-anchored; a subpath-anchored hub needs the correct `RelPath` from the recorded anchor) and on batch 2 (which migrates the shared fabricengine test fixtures — `newFabricFixture` and friends — to seed fabric config at the repo-wide `BoardDir`; this batch's Add-via-`WiredNames(BoardDir)` change would otherwise break those shared fixtures, so it must land after batch 2's fixture migration). Reads the repo-wide name-set via `WiredNames(hubgeometry.BoardDir(l.Hub))` (`BoardDir` base is explicit).

Batch-local decision: wiring slots in after launchers (step 10) and before the host push (step 11), so a wiring failure still triggers the existing `rollbackAdd`; the rollback gains a symmetric `UnwireJunctions` call.

## Cards

### Card 20: Fold `WireJunctions` into `Topology.Add`

- **Context:**
  - `internal/fabricengine/junction.go`
  - `internal/fabricengine/junctionnames.go`
  - `internal/hubgeometry/hubgeometry.go`
- **Edits:**
  - `internal/fabricengine/add.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `Topology.Add(l *hubgeometry.Layout, slug string, opts AddOptions) (AddResult, error)` (add.go:83), after `writeLaunchers` (step 10, add.go:255) and before the host push (step 11, add.go:261), wire the new worktree's junctions: load the repo-wide name-set `names, err := WiredNames(hubgeometry.BoardDir(l.Hub))`, then `WireJunctions(l, slug, names)` (host junctions + git-exclude entries; the weft worktree already exists from step 8, so junction targets resolve). On a wiring error, return via the existing `t.rollbackAdd(...)` path (thread the failure exactly as the other post-step-7 failures do). Update the dormant-`_lyx` comment block at add.go:303-305 to state that `Add` now wires the host `_lyx`/`_pattern` junctions eagerly (no dormant state, no `lyx init`), and that `rollbackAdd` removes them on failure.
- **Commit:** `feat(fabricengine): worktree add wires junctions eagerly`

### Card 21: `rollbackAdd` unwires on failure

- **Context:**
  - `internal/fabricengine/junction.go`
  - `internal/fabricengine/weftwiring.go`
  - `internal/fabricengine/junctionnames.go`
  - `internal/hubgeometry/hubgeometry.go`
- **Edits:**
  - `internal/fabricengine/add.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `rollbackAdd(l *hubgeometry.Layout, slug, hostBranch, weftBranch, target string, weftBranchAdopted bool) error` (add.go:306), add a best-effort junction teardown step mirroring the file's existing best-effort cleanup steps (each accumulates the first error, continues past failures). Before or alongside `removeWeftWorktree` (add.go:311), load the repo-wide name-set (`names, _ := WiredNames(hubgeometry.BoardDir(l.Hub))`, tolerating a load error by falling back to `nil` names — best-effort) and call `removeHostJunction(l, slug, names)` (weftwiring.go:137, best-effort continue-past-failure) so a partially-wired worktree does not leave dangling junctions. Keep the existing rollback ordering and first-error-preserved semantics; do not abort the rest of rollback on a junction-removal failure. Since junctions now exist by the time rollback runs, this removal is correct (the old "junction is dormant, rollback skips it" note at add.go:303-305 is superseded by card 20's comment update).
- **Commit:** `feat(fabricengine): rollbackAdd unwires eagerly-wired junctions`

### Card 22: Add eager-wiring tests

- **Context:**
  - `internal/fabricengine/add.go`
  - `internal/fabricengine/add_test.go`
  - `internal/fabricengine/add_rollback_adopt_test.go`
  - `internal/fabricengine/junction.go`
  - `internal/hubgeometry/hubgeometry.go`
  - `internal/lyxtest/lyxtest.go`
- **Edits:**
  - `internal/fabricengine/add_rollback_adopt_test.go`
  - `internal/fabricengine/add_branch_exists_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** **First, migrate the add-path test fixtures to seed fabric config at the repo-wide `BoardDir`** — card 20 folds `WiredNames(hubgeometry.BoardDir(l.Hub))` into `Topology.Add` (hard-failing via `rollbackAdd` on a name-set load error), so any `add_*_test.go` fixture seeding fabric config only at the per-pair weft base now fails. Update `add_rollback_adopt_test.go` and `add_branch_exists_test.go` (and any other `add_*_test.go` this batch's verify surfaces) to seed fabric config at `hubgeometry.ConfigFile(hubgeometry.BoardDir(fixture.Hub), "fabric")`, mirroring batch 2 card 10's shared-fixture migration (batch 2 lands first — this batch depends on it — so if these files reuse the shared `newFabricFixture` helper they already inherit the fix and only add-specific ad-hoc seeding needs changing). Then extend `internal/fabricengine/add_rollback_adopt_test.go` (`//go:build integration`) to assert:
  - a successful `Add` leaves the new worktree's `_lyx` (and `_pattern`, per the repo-wide default `pathspec`) wired as junctions into the paired weft worktree immediately (no dormant state) — assert via `fslink.IsLink` on `l.HostLyxLink(slug)`/`l.HostPatternLink(slug)` and target resolution;
  - a mid-add failure after wiring triggers `rollbackAdd`, which removes those junctions (assert the host links are gone after rollback), while still honoring the adopted-weft-branch preservation the file already covers.
  Reuse the existing adopt/rollback fixtures in the file; inject the mid-add failure using whatever seam the current rollback tests use (or a wiring-stage failure). Do not weaken the existing "rollback never deletes an adopted pre-existing weft branch" assertion (add_rollback_adopt_test.go's core contract). The unit `add_test.go` (slug-validation, Tier-1) stays untouched unless a slug-validation assertion depends on wiring (it does not).
- **Commit:** `test(fabricengine): cover eager add wiring and rollback unwire`

## Batch Tests

`verify: go test -tags integration ./internal/fabricengine/...` runs the whole fabricengine package. Whole-package scope is justified: `add.go` is exercised by several integration tests (`add_rollback_adopt_test.go`, `add_branch_exists_test.go`) and the untagged `add_test.go`, and the eager-wiring change interacts with the junction machinery (`junction.go`/`weftwiring.go`) covered by `junction_repoint_test.go`/`remove_junctions_integration_test.go` — a regression there would only surface at package scope. This matches batch 2's granularity for the same reasons.
