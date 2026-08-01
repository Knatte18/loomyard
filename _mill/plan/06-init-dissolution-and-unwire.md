# Batch: init-dissolution-and-unwire

```yaml
task: 'fabric: clone-does-everything + subpath-in-weft + init dissolution'
batch: init-dissolution-and-unwire
number: 6
cards: 9
verify: go test -tags integration ./internal/fabricengine/... ./internal/fabriccli/... ./cmd/lyx/... ./internal/loomengine/...
depends-on: [2, 4, 5]
```

## Batch Scope

This batch dissolves `lyx init`. Once clone (batch 4) and add (batch 5) do all wiring and `Resolve` reads the recorded anchor (batch 1), `init` has no remaining job. It: deletes `internal/initengine` and `internal/initcli`; unregisters `init` from `cmd/lyx/main.go` and the help tree; moves the `init --undo` teardown onto a new `lyx fabric unwire` verb (a per-host-worktree full deactivation, distinct from `reconcile`'s config-convergence); retargets the eight `config.go` "run \"lyx init\"" messages to `reconcile` (the correct remedy for an unwired worktree); and sweeps the remaining `lyx init` reference strings/comments across reconcile/junction/weftgit and the CLI/Cobra invariant.

`unwire` enumerates junctions by the on-disk link scan (batch 2's `scanOnDiskJunctionNames`, minus `HubReservedNames()`) so a full deactivation removes every fabric junction present including any stale one absent from `pathspec`; it clears/commits/pushes only the per-worktree weft `_lyx` (never `_pattern`), reverts the `.gitignore` block, and **leaves the repo-wide `weft:main` records (`.fabric-anchor`, `<BoardDir>/_lyx/config/fabric.yaml`) untouched** for a later re-wire.

Depends on batch 2 (the on-disk scan helper + reconcile is the re-wire path referenced by the retargeted messages), batch 4 (clone does everything — precondition for `init` deletion) and batch 5 (add wires eagerly — the other precondition). This batch reaches batch 1 (via 2) and batch 3 (via 4) transitively, so its edits to `reconcile.go` (shared with batch 2) and `fabric.go` (shared with batch 4) are sequential, never parallel.

## Cards

### Card 23: New `Unwire` engine verb in fabricengine

- **Context:**
  - `internal/initengine/undo.go`
  - `internal/fabricengine/fabric.go`
  - `internal/fabricengine/junction.go`
  - `internal/fabricengine/weftwiring.go`
  - `internal/fabricengine/weftgit.go`
  - `internal/fabricengine/reconcile.go`
  - `internal/gitignore/gitignore.go`
  - `internal/hubgeometry/hubgeometry.go`
- **Edits:** none
- **Creates:**
  - `internal/fabricengine/unwire.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create `internal/fabricengine/unwire.go` (`package fabricengine`) with `func Unwire(cwd string) (UnwireVerbResult, error)` and a result type `UnwireVerbResult{ JunctionsRemoved []string; WeftContent string; GitExclude string; Gitignore string }` (mirroring `initengine.UndoResult`, whose fields are `JunctionsRemoved`, `WeftContent` "cleared"/"not_present", `GitExclude` "reverted"/"unchanged", `Gitignore` "reverted"/"unchanged"). Port `initengine.Undo`'s teardown (undo.go:74) with one change — the junction name-set comes from the **on-disk scan**, not a config load:
  1. `l, err := hubgeometry.Resolve(cwd)`; `slug := filepath.Base(l.WorktreeRoot)`.
  2. Enumerate junctions by full on-disk scan: `names, err := scanOnDiskJunctionNames(l.WorktreeRoot, l.RelPath)` (card 8's helper, already excludes `HubReservedNames()`). This removes every fabric junction present — including a stale one absent from the current `pathspec` — unlike a config-driven name-set.
  3. `junctionResult, err := UnwireJunctions(l, slug, names)` (junction.go:207); abort before the weft/gitignore steps on error.
  4. Weft `_lyx` clear/commit/push — port undo.go:122-159 **verbatim in behavior**, preserving the deliberate `_lyx`-only / never-`_pattern` asymmetry (undo.go:60-73). Reproduce undo.go's **three-way** `WeftContent` branch exactly (do not collapse it to two): (a) weft worktree absent → `WeftContent = "not_present"`; (b) weft worktree present but `l.WeftLyxDirFor(slug)` itself absent (`os.Stat` → not-exist — the raw-adopted/dormant pair from `reconcile.go`'s `createDormantWeftForRawHost`, where `_lyx` was never materialized) → **also** `"not_present"` (no `RemoveAll`); (c) `_lyx` present → `os.RemoveAll(l.WeftLyxDirFor(slug))` → `"cleared"`. This matches Card 30's "never-wired host reports `not_present`" assertion. Then unconditionally `opts := EnvSyncOptions()`, `f, err := New(l.WorktreeRoot, l.WeftWorktree())` **checking the error and returning it (`if err != nil { return result, err }`) — undo.go:145-148 checks this `New` error; do not discard it with `_`**, `pathspec := ScopedPathspec(l.RelPath, []string{hubgeometry.LyxDirName})`, `f.CommitWeft(pathspec, "lyx fabric unwire: clear _lyx", opts)`, `PushWeftAt(l.WeftWorktree(), opts)`. The commit pathspec names ONLY `hubgeometry.LyxDirName` — no `_pattern` equivalent.
  5. `.gitignore` revert: `gitignore.Remove(l.Cwd, ".lyx/")` → `"reverted"`/`"unchanged"`; populate `JunctionsRemoved`/`GitExclude` from `junctionResult`.
  Add a doc comment stating `Unwire` is per-host-worktree and never touches the repo-wide `weft:main` records (`.fabric-anchor`, `<BoardDir>/_lyx/config/fabric.yaml`) — those are per-repo facts a later `reconcile` re-wire still needs.
- **Commit:** `feat(fabricengine): add Unwire verb (per-worktree full deactivation)`

### Card 24: Register `lyx fabric unwire`

- **Context:**
  - `internal/fabricengine/unwire.go`
  - `internal/output/output.go`
  - `internal/hubgeometry/hubgeometry.go`
  - `internal/fabriccli/clone.go`
- **Edits:**
  - `internal/fabriccli/fabric.go`
- **Creates:**
  - `internal/fabriccli/unwire.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create `internal/fabriccli/unwire.go` with `func runUnwire(out io.Writer, args []string) int`: `cwd, _ := hubgeometry.Getwd()`; `res, err := fabricengine.Unwire(cwd)`; on error `output.Err(out, err.Error())`; on success `output.Ok(out, map[string]any{"junctions_removed": res.JunctionsRemoved, "weft_content": res.WeftContent, "git_exclude": res.GitExclude, "gitignore": res.Gitignore})` (mirroring initcli's `runUndo` output keys). In `internal/fabriccli/fabric.go`, register the verb alongside the other topology verbs (after `reconcile`/`cleanup`, before the `addWeftVerbs(cmd)` call ~fabric.go:251) using the inline `cmd.AddCommand(&cobra.Command{ Use: "unwire", Short: "<non-empty>", Long: "...", RunE: clihelp.WrapRun(func(out io.Writer, args []string) int { return runUnwire(out, args) }) })` pattern. `Short` must be non-empty (CLI/Cobra Invariant); the `Long` should state it is a full per-worktree deactivation (removes all junctions, clears weft `_lyx`, reverts `.gitignore`) distinct from `reconcile`, and that it leaves the repo's anchor/config intact.
- **Commit:** `feat(fabriccli): register lyx fabric unwire verb`

### Card 25: Delete `initengine` and `initcli`

- **Context:**
  - `internal/fabricengine/unwire.go`
- **Edits:** none
- **Creates:** none
- **Deletes:**
  - `internal/initengine/init.go`
  - `internal/initengine/init_test.go`
  - `internal/initengine/undo.go`
  - `internal/initengine/undo_test.go`
  - `internal/initengine/testmain_test.go`
  - `internal/initcli/initcli.go`
  - `internal/initcli/initcli_test.go`
  - `internal/initcli/testmain_test.go`
- **Moves:** none
- **Requirements:** Delete every file of the initengine and initcli packages (all listed in Deletes). The `Init`/`Undo` responsibilities are fully re-homed: cwd-anchor resolution → recorded subpath (batch 1); junction wiring + `_lyx`/config creation + `.gitignore` block + `configsync.ReconcileAll` → clone (batch 4) and add (batch 5); `--undo` teardown → `fabricengine.Unwire` (card 23). Do not leave stub files. Card 26 removes the `main.go` registration that imports `initcli` (both edits must land in this batch so the tree still compiles).
- **Commit:** `refactor: delete initengine and initcli (dissolved into fabric)`

### Card 26: Unregister `init` from the root and help tree

- **Context:**
  - `internal/fabriccli/fabric.go`
- **Edits:**
  - `cmd/lyx/main.go`
  - `cmd/lyx/helptree_test.go`
  - `cmd/lyx/jsonhelp_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `cmd/lyx/main.go`: remove the `initcli` import (main.go:29); remove `initcli.Command(),` from the `root.AddCommand(...)` block (main.go:117, the first entry); and remove `init, ` from the `root.Long` "Available modules:" list (main.go:87) so it reads `Available modules: board, config, ide, reed, fabric, selfreport, shuttle, burler, perch, builder, scout, webster.`. In `cmd/lyx/helptree_test.go`: remove `"init"` from the hardcoded `requiredModules` slice (helptree_test.go:27-29); and add `"unwire"` to the `fabric` `wantSubs` verb list (helptree_test.go:69-73) alongside the existing `clone, add, list, remove, checkout, pairs, reconcile, prune, cleanup, status, commit, push, pull, sync`. `cmd/lyx/jsonhelp_test.go` was found during implementation to carry its own hardcoded `requiredModules` slice (jsonhelp_test.go:93-95, `"init", "board", "config", "ide", "reed", "selfreport"`) asserting the root `--json` help's `commands` map — this was missed by the original exploration alongside `registration_test.go`/`longlist_test.go`/`drift_test.go`; remove `"init"` from it the same way. The derived guards (`registration_test.go`, `longlist_test.go`, `drift_test.go`) need no edits — they enumerate the live tree and self-adjust once `init` is gone and `unwire` is registered.
- **Commit:** `refactor(cmd/lyx): unregister init from root and help tree`

### Card 27: Retarget the eight `config.go` "run lyx init" messages to reconcile

- **Context:**
  - `internal/fabricengine/reconcile.go`
- **Edits:**
  - `internal/fabricengine/config.go`
  - `internal/boardengine/config.go`
  - `internal/shuttleengine/config.go`
  - `internal/reedengine/config.go`
  - `internal/perchengine/config.go`
  - `internal/builderengine/config.go`
  - `internal/loomengine/config.go`
  - `internal/websterengine/config.go`
  - `internal/loomengine/config_test.go`
  - `internal/fabricengine/config_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In each of the eight `config.go` files, replace the message string `not initialized here; run "lyx init"` (constructed via `fmt.Errorf("not initialized here; run \"lyx init\"")` at fabricengine/config.go:47, boardengine/config.go:66, shuttleengine/config.go:67, reedengine/config.go:101, perchengine/config.go:105, builderengine/config.go:83, loomengine/config.go:64, websterengine/config.go:79) with `not initialized here; run "lyx fabric reconcile"`. These messages fire when a module's config base lacks `_lyx/` (an existing-but-unwired worktree), whose correct remedy is converging wiring via `reconcile`, not re-cloning the hub. Update each accompanying doc-comment line that quotes the old string (fabricengine:40, boardengine:53, shuttleengine:59, reedengine:93, perchengine:97+138, builderengine:75, loomengine:56, websterengine:71) to the new wording. In `internal/loomengine/config_test.go`, update the assertion at config_test.go:113 (`want := \`not initialized here; run "lyx init"\``) to the new string. `internal/fabricengine/config_test.go`'s own `TestLoadConfig_NotInitialized` (found during implementation, missed by the original exploration alongside loomengine's) has the same `strings.Contains(errMsg, "lyx init")` assertion at config_test.go:138-139 — update it to assert `"lyx fabric reconcile"` instead. (The other six engines have no test asserting this exact string per the exploration; the `done_gate` full-suite run is the backstop.)
- **Commit:** `refactor: retarget "run lyx init" config messages to lyx fabric reconcile`

### Card 28: Sweep remaining `lyx init` reference strings and comments

- **Context:**
  - `internal/fabricengine/unwire.go`
- **Edits:**
  - `internal/fabricengine/reconcile.go`
  - `internal/fabricengine/junction.go`
  - `internal/fabricengine/junctionnames.go`
  - `internal/fabricengine/weftgit.go`
  - `internal/fabriccli/fabric.go`
  - `internal/configsync/configsync.go`
  - `internal/fabricengine/junction_pattern_integration_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Update the remaining `lyx init`/`initengine` references so no live user-facing string or dangling comment names a deleted command or package:
  - `internal/configsync/configsync.go:99`: the doc comment naming the deleted initengine package (the seed-only "created" heuristic explanation `…so initengine's \`Applied && len(Added) > 0 && len(Removed) == 0\` 'created' heuristic still fires correctly`) — reword to drop that reference (the package is deleted in card 25), describing the heuristic in its own terms without naming the removed package.
  - `internal/fabricengine/reconcile.go`: the user-facing Detail strings at reconcile.go:230 (`"…created dormant (run lyx init to activate)"`) and reconcile.go:236-239 (`"…run \`lyx fabric add\` or \`lyx init\`"`) — reword to reflect eager wiring (fabric wires at clone/add; the remedy for a drifted pair is `lyx fabric reconcile`, not `lyx init`). The code comments at reconcile.go:43,87,223,235,278 that describe "lyx init's responsibility"/"run lyx init" — update to name the new owner (clone/add wire eagerly; reconcile converges).
  - `internal/fabricengine/junction.go:152`: the `seedLyxJunction` error text ending `…then re-run \`lyx init\` to create the junction` — change to `…then re-run \`lyx fabric reconcile\` to create the junction`.
  - `internal/fabricengine/junctionnames.go:57` and `internal/fabricengine/weftgit.go:233,269,271`: comments referencing the deleted init packages or `lyx init`/`lyx init --undo` — update to reference `fabricengine.Unwire`/clone-add eager wiring as appropriate (these are comments; keep them accurate, do not delete load-bearing rationale).
  - `internal/fabriccli/fabric.go:85`: the clone `Long` line `After cloning, run "lyx init" inside the host worktree to activate junctions and config.` — remove it (clone now does everything) or reword to state clone wires everything automatically.
  - `internal/fabricengine/junction_pattern_integration_test.go`: found during implementation, missed by the original exploration — `TestWireJunctions_RefusesRealHostDirectory` asserts `strings.Contains(msg, "lyx init")` (junction_pattern_integration_test.go:148-149) against `seedLyxJunction`'s real-directory-guard error text, whose remedy card 28's own junction.go edit above retargets to `lyx fabric reconcile`. Update the assertion (and its doc comment at line 111 naming "the re-run-`lyx init` remedy") to `lyx fabric reconcile`.
  Do not touch integration-test comment references that document historical behavior unless they assert a live string.
- **Commit:** `refactor(fabricengine): sweep stale lyx init references to fabric verbs`

### Card 29: Update CONSTRAINTS.md CLI/Cobra Invariant for the command-tree change

- **Context:**
  - `cmd/lyx/main.go`
  - `internal/fabriccli/fabric.go`
- **Edits:**
  - `CONSTRAINTS.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `CONSTRAINTS.md`'s `## CLI / Cobra Invariant` section, remove the stale sentence naming `initcli`/`initengine` (line 70: `` `initcli`/`initengine` follows the standard split (no longer exempt — `lyx init --undo` grew enough core logic that mixing it into the cli package was rot, not simplicity). ``) since both packages are deleted. If the surrounding "Package naming" bullet still reads coherently after removal, leave the rest intact; otherwise trim the now-dangling clause. Optionally note that the `init` module is removed and `lyx fabric unwire` is its teardown successor, if a one-line note fits the section's style. Keep one-line-per-paragraph markdown (no hard-wrap). Do not touch the Hub Geometry Invariant bullet added in batch 1.
- **Commit:** `docs(constraints): drop initcli/initengine from CLI/Cobra invariant`

### Card 30: `lyx fabric unwire` tests (mirror the old undo tests)

- **Context:**
  - `internal/fabricengine/unwire.go`
  - `internal/fabricengine/reconcile.go`
  - `internal/fabricengine/junction.go`
  - `internal/hubgeometry/hubgeometry.go`
  - `internal/hubgeometry/anchor.go`
  - `internal/lyxtest/lyxtest.go`
- **Edits:** none
- **Creates:**
  - `internal/fabricengine/unwire_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create `internal/fabricengine/unwire_test.go` (`//go:build integration`) porting the deleted `initengine/undo_test.go` coverage to `fabricengine.Unwire`:
  - a fully-wired worktree: `Unwire` removes all on-disk fabric junctions (assert `_lyx`/`_pattern` host links gone), and specifically removes a **stale** junction on disk that is absent from the current `pathspec` (proving on-disk-scan enumeration, not config name-set);
  - it clears/commits/pushes the weft `_lyx` only (`WeftContent == "cleared"`), never `_pattern` (assert the weft `_pattern` content survives);
  - it reverts the `.gitignore` `.lyx/` block (`Gitignore == "reverted"`);
  - it is idempotent and no-ops on an unpaired/never-wired host (`WeftContent == "not_present"`, no error);
  - **it leaves the repo-wide `weft:main` records untouched** — assert `<BoardDir>/.fabric-anchor` and `<BoardDir>/_lyx/config/fabric.yaml` still exist after `Unwire`, so a subsequent `Reconcile` can re-wire.
  Build synthetic hubs with `lyxtest`; reuse fixture patterns from batch 2's `reconcile_stale_removal_test.go` for the stale-junction and repo-wide-record setup.
- **Commit:** `test(fabricengine): cover lyx fabric unwire teardown and record preservation`

### Card 31: Retarget SANDBOX-CORE-SUITE S6 off `lyx init` (coupled to the coverage guard)

- **Context:**
  - `cmd/lyx/sandbox_coverage_test.go`
  - `internal/fabriccli/fabric.go`
- **Edits:**
  - `tools/sandbox/SANDBOX-CORE-SUITE.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** This card lives in batch 6 (not the batch-7 docs batch) because it is machine-coupled: `sandbox_coverage_test.go`'s Assert-2 (lines 88-95) fails when a `**Covers:** <module>` tag names a module no longer registered in `newRoot()`, and card 26 unregisters `init` — so the S6 `**Covers:** init` tag (SANDBOX-CORE-SUITE.md:162) must be removed in the same batch or batch 6's `./cmd/lyx/...` verify fails. Retarget scenario **S6 "Subfolder init"** (SANDBOX-CORE-SUITE.md:158-168) to exercise the clone-does-everything / subpath-anchoring contract instead of `lyx init`/`lyx init --undo`: rewrite the `### S6` heading, `**Goal:**`, `**Watch:**`, and `**Durability note:**` to demonstrate a subpath-anchored clone (`lyx fabric clone --subpath <sub>`) resolving `RelPath` from the recorded `.fabric-anchor`, and `lyx fabric unwire` as the teardown; **remove the `**Covers:** init` line** (line 162) since `init` is no longer a registered module (`fabric` is already covered by SANDBOX-FABRIC-SUITE.md, and `unwire` is a fabric verb — no new `Covers:` tag is required, but you may tag the retargeted scenario `**Covers:** fabric` if it drives a fabric verb). Also rewrite the line-33 "not initialized here; run `lyx init`" operating-model paragraph: update the error string to `not initialized here; run "lyx fabric reconcile"` and rephrase the "S6 deliberately runs `lyx init`" exception to describe S6's new subpath-clone demonstration. Keep one-line-per-paragraph markdown.
- **Commit:** `docs(sandbox): retarget CORE S6 off lyx init to clone-does-everything`

## Batch Tests

`verify: go test -tags integration ./internal/fabricengine/... ./internal/fabriccli/... ./cmd/lyx/... ./internal/loomengine/...` spans the four areas this cross-cutting batch touches: fabricengine (new `unwire.go`/`unwire_test.go`, the reference-string sweep, the retargeted `config.go`), fabriccli (new `unwire.go`, `fabric.go` registration, exercised by `cli_test.go`'s verb-surface test which now expects `unwire`), cmd/lyx (the help-tree/registration/longlist/drift guards that must stay green after `init` deletion — this is where a dangling `initcli` reference or a missing help-tree entry fails — plus `sandbox_coverage_test.go`, whose Assert-2 requires card 31's S6 `**Covers:** init` removal), and loomengine (its `config_test.go` string assertion). The broad scope is inherent to init dissolution, which removes a registered module and touches eight engines; it is justified and named here per the per-batch-scoping exception. The repo-wide `done_gate` (`go test ./...`) is the final backstop for the other seven engines' retargeted messages and any missed `initcli`/`initengine` reference.
