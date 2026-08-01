# Batch: clone-does-everything

```yaml
task: 'fabric: clone-does-everything + subpath-in-weft + init dissolution'
batch: clone-does-everything
number: 4
cards: 6
verify: go test -tags integration ./internal/fabricengine/... ./internal/fabriccli/...
depends-on: [1, 2, 3]
```

## Batch Scope

This batch makes `lyx fabric clone` do the whole topology job in one shot: clone host + weft, check out the weft primary pairing, materialize the `weft:main` board worktree, resolve the subpath (adopt-or-create), write `.fabric-anchor` + the repo-wide `fabric.yaml` onto `weft:main`, wire all prime-worktree junctions, create `_lyx`/`_lyx/config`, maintain the `.gitignore` `.lyx/` block, and run `configsync.ReconcileAll` once — from the user's perspective a single command.

**Import-cycle constraint (drives the layering).** The config-materialization calls (`configsync.ReconcileFabricAt`, `configsync.ReconcileAll`) must NOT be made from inside `internal/fabricengine`: `configsync` imports `internal/configreg` (configsync.go:13) and `configreg` imports `internal/fabricengine` (configreg.go:12,51 — for `fabricengine.ConfigTemplate`), so a `fabricengine → configsync` edge would close the cycle `fabricengine → configsync → configreg → fabricengine`, which Go rejects. The current orchestrator `internal/initengine` imports both `configsync` and `fabricengine` safely precisely because it sits *above* both. This batch preserves that layering: **`fabricengine.CloneHub` stays git/geometry-focused** (clone, checkout, board worktree, anchor resolve/validate, write the `.fabric-anchor` marker to disk) and returns the paths the caller needs; the **`internal/fabriccli` clone handler** (which already imports `fabricengine` and may import `configsync` without cycling) drives the config-materialization + weft:main commit + junction wiring + reconcile sequence. This keeps "clone does everything" true at the command level while respecting the import graph.

Depends on batch 1 (the `hubgeometry.FabricAnchorName` constant clone writes, and correct anchor-based `Resolve`), batch 2, and batch 3 (`configsync.ReconcileFabricAt`). **The dependency on batch 2 is load-bearing:** clone creates a hub whose `fabric.yaml` lives ONLY at the repo-wide `BoardDir`, never at the per-pair weft base — so `reconcile`/`checkout`/`remove`/`pairs`/`PairInSync` must already read the name-set from `BoardDir` (batch 2 card 7's `RepoWiredNames` migration) before a clone-created hub is exercised, or those verbs break package-wide on every clone. Without the edge, a DAG scheduler could land batch 4 before batch 2 (both otherwise gated only on 1/3), producing exactly that regression. The junction name-set is read via `WiredNames(BoardDir)`. The two `weft:main` writes route through `fabricengine.CommitWeftAt`/`PushWeftAt` per the Weft Git Invariant.

Batch-local decision: adopt-vs-create is detected by whether `.fabric-anchor` already exists in the freshly-materialized board worktree (adopt: read it; create: validate `--subpath` and write it) — mirroring `suffixWeftPrimaryBranch`'s adopt-or-create shape.

Batch-local decision (partial-failure recovery): if the CLI orchestration fails *after* `CloneHub` returns (a config/wire/reconcile step erroring), the hub's git clone is left intact and the handler returns an `output.Err` — the operator completes wiring with `lyx fabric reconcile` (the declarative converge), rather than the handler destructively tearing down a good clone. `CloneHub`'s own internal failures still route through its existing `teardownHub`.

Batch-local decision (accepted limitation): because the `--subpath` CLI flag defaults to `"."` and the adopt-path mismatch check treats `subpath == "" || subpath == "."` as "unset/default" (never erroring), a re-clone with an explicit `--subpath .` against a repo actually anchored at a subpath will silently adopt the recorded subpath rather than error. This is accepted: the mismatch guard's job is to catch a wrong NON-default value (a typo'd real subpath), and `anchor-value-always-explicit` means the record is authoritative on adopt. Precise "explicit-root-vs-unset" detection (a distinct sentinel default) is deliberately not added — it is extra machinery for a benign edge.

## Cards

### Card 14: `CloneHub` takes a subpath and returns the resolved geometry

- **Context:**
  - `internal/fabricengine/boardweft.go`
  - `internal/hubgeometry/hubgeometry.go`
  - `internal/fabricengine/branchname.go`
- **Edits:**
  - `internal/fabricengine/clone.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Change `CloneHub(cwd, hostURL, weftURL string) (hubPath string, err error)` (clone.go:55) to return a result struct instead of a bare `hubPath`: define `type CloneResult struct { HubPath string; Anchor string; BoardDir string; WeftBase string; PrimeCwd string }` and change the signature to `CloneHub(cwd, hostURL, weftURL, subpath string) (CloneResult, error)`. `PrimeCwd` is the resolved prime host worktree path at the anchor (`filepath.Join(hostWorktreePath, anchor)`); `WeftBase` is `filepath.Join(l.WeftWorktree(), l.RelPath)` for the prime; `BoardDir` is `hubgeometry.BoardDir(hubPath)`. Thread the struct return through the existing error paths: every `return "", teardownHub(hubPath, err)` becomes `return CloneResult{}, teardownHub(hubPath, err)`, and the three early no-cleanup errors — empty name (clone.go:60-63), hub-already-exists (clone.go:69-71), and the `os.MkdirAll(hubPath, …)` failure (clone.go:74-76) — return `CloneResult{}, <err>`. Do not yet add the anchor logic — card 15 adds it; for now populate `CloneResult{HubPath: hubPath, Anchor: filepath.Clean(subpath), BoardDir: hubgeometry.BoardDir(hubPath)}` (default `Anchor` to `"."` when `subpath` is empty) on the success path (clone.go:122) so the signature compiles. Update `CloneHub`'s doc comment to state it clones + checks out + materializes the board worktree + resolves/records the anchor, and returns the resolved geometry for the CLI layer to drive config materialization and wiring (which `CloneHub` deliberately does NOT do, to avoid the `fabricengine → configsync` import cycle).
- **Commit:** `refactor(fabricengine): CloneHub takes subpath, returns CloneResult`

### Card 15: Resolve the anchor (adopt-or-create), validate, write `.fabric-anchor` to disk

- **Context:**
  - `internal/fabricengine/boardweft.go`
  - `internal/hubgeometry/hubgeometry.go`
- **Edits:**
  - `internal/fabricengine/clone.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `CloneHub`, after `ensureBoardWorktree(weftPath, hostBranch, boardDir)` (clone.go:117; `boardDir := hubgeometry.BoardDir(hubPath)`), resolve the anchor adopt-or-create and write the marker to the board worktree ON DISK (the CLI commits it — card 16):
  - `markerPath := filepath.Join(boardDir, hubgeometry.FabricAnchorName)`.
  - **Adopt** (re-clone): if `markerPath` exists (`os.Stat` — `ensureBoardWorktree` checked out `weft:main`, which already carries a committed marker), read it (`os.ReadFile`+`TrimSpace`) → `recorded`. If the caller passed a non-default `subpath` (`filepath.Clean(subpath)` is neither `""` nor `"."`) that differs from `recorded`, return a hard error via `teardownHub` (never silently re-anchor). Set the result `Anchor = recorded`. Do NOT rewrite the marker.
  - **Create** (first-ever): marker absent. `anchor := filepath.Clean(subpath)`; empty → `"."`. **Validate the subpath exists in the host worktree**: `os.Stat(filepath.Join(hostWorktreePath, anchor))` must succeed and be a directory — else `teardownHub` with a hard error naming the bad subpath (catches a typo like `backedn`). Write the marker to disk: `os.WriteFile(markerPath, []byte(anchor+"\n"), 0o644)`. Do NOT materialize `fabric.yaml` and do NOT commit here — those are the CLI's job (card 16), because `ReconcileFabricAt` lives in `configsync` (import-cycle constraint).
  - Populate the returned `CloneResult` fully: `Anchor`, `BoardDir`, `PrimeCwd = filepath.Join(hostWorktreePath, anchor)`, and `WeftBase` (resolve a prime layout `l, _ := hubgeometry.Resolve(PrimeCwd)` — the marker now exists so `RelPath` is correct — then `WeftBase = filepath.Join(l.WeftWorktree(), l.RelPath)`). On any failure route through `teardownHub`.
- **Commit:** `feat(fabricengine): resolve and record .fabric-anchor on clone`

### Card 16: fabriccli clone handler orchestrates config, weft:main commit, wiring, reconcile

- **Context:**
  - `internal/fabricengine/clone.go`
  - `internal/fabricengine/junction.go`
  - `internal/fabricengine/junctionnames.go`
  - `internal/fabricengine/weftgit.go`
  - `internal/configsync/configsync.go`
  - `internal/gitignore/gitignore.go`
  - `internal/hubgeometry/hubgeometry.go`
  - `internal/output/output.go`
- **Edits:**
  - `internal/fabriccli/clone.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `internal/fabriccli/clone.go`'s `runCloneWithReset`, after `res, err := fabricengine.CloneHub(cwd, hostURL, weftURL, subpath)` succeeds, drive the config + wiring sequence that `lyx init` used to do — here in the CLI layer so `configsync` stays out of `fabricengine` (import-cycle constraint; `fabriccli` already imports `fabricengine` and may import `configsync`/`gitignore`/`hubgeometry`). In order:
  1. `configsync.ReconcileFabricAt(res.BoardDir, true)` — materialize the repo-wide `fabric.yaml` at `<BoardDir>/_lyx/config/fabric.yaml` (idempotent: a no-op on the adopt path where it is already present);
  2. `fabricengine.CommitWeftAt(res.BoardDir, "fabric clone: record anchor + repo-wide config", fabricengine.SyncOptions{})` then `fabricengine.PushWeftAt(res.BoardDir, fabricengine.SyncOptions{})` — commit+push the `.fabric-anchor` marker and `fabric.yaml` onto `weft:main` through the choke point (wildcard `git add -A` stages the root marker + `_lyx/config/fabric.yaml`; a clean no-op on adopt);
  3. build the prime layout `l, err := hubgeometry.Resolve(res.PrimeCwd)`, load the repo-wide name-set `names, err := fabricengine.WiredNames(res.BoardDir)`, and wire: `fabricengine.WireJunctions(l, filepath.Base(l.WorktreeRoot), names)` (creates host junctions + git-exclude entries; the weft worktree already exists so targets resolve). Do NOT `MkdirAll` a real `_lyx` over the junction WireJunctions seeds;
  4. `gitignore.Ensure(l.Cwd, ".lyx/")` — maintain the `.gitignore` `.lyx/` managed block on the host worktree;
  5. `configsync.ReconcileAll(res.WeftBase, true)` — materialize the per-worktree module configs on the weft side (fabric is already skipped by batch 3).
  On any step erroring, return `output.Err(out, err.Error())` WITHOUT tearing down the clone (per the partial-failure-recovery batch decision — the operator finishes with `lyx fabric reconcile`). On success, emit `output.Ok(out, map[string]any{"hub": res.HubPath, "anchor": res.Anchor})` (card 17 also wires the `--subpath` flag that feeds `subpath` here). Config ownership stays in `configsync`/`configengine`/`gitignore` — the handler only sequences the calls.
- **Commit:** `feat(fabriccli): clone handler orchestrates config, weft commit, wiring, reconcile`

### Card 17: fabriccli reads repo-wide fabric config + clone `--subpath` flag

- **Context:**
  - `internal/hubgeometry/hubgeometry.go`
  - `internal/fabricengine/topology.go`
  - `internal/fabricengine/config.go`
  - `internal/fabricengine/clone.go`
- **Edits:**
  - `internal/fabriccli/clone.go`
  - `internal/fabriccli/fabric.go`
  - `internal/fabriccli/weft_verbs.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Two coupled changes to the fabric CLI layer.
  **(a) Migrate every CLI config-read to the repo-wide `BoardDir`.** Batch 3 stops `configsync.ReconcileAll` from materializing a per-worktree `fabric.yaml`, and clone writes `pathspec`/`branch_prefix` only to `<BoardDir>/_lyx/config/fabric.yaml` — so every CLI site building a `Topology` from `fabricengine.LoadConfig(cwd)` would break (`configengine.Load`'s `"config file … not found"`) on a freshly-cloned/wired worktree, including `reconcile` itself. Update all eight topology-verb sites in `internal/fabriccli/fabric.go` — `runAdd`/`runList`/`runCheckout`/`runPairs`/`runReconcile`/`runPruneWithFlag`/`runCleanupWithFlags`/`runRemoveWithFlag` (fabric.go:280,317,378,411,443,472,501,531) — each of which does `cfg, err := fabricengine.LoadConfig(cwd)` then `top := fabricengine.NewTopology(cfg)`: resolve the layout first (`l, err := hubgeometry.Resolve(cwd)`) and load from the repo-wide base — `cfg, err := fabricengine.LoadConfig(hubgeometry.BoardDir(l.Hub))`. Apply the same migration to the ninth site in `internal/fabriccli/weft_verbs.go:124` (`loadedCfg, err := fabricengine.LoadConfig(weftBaseDir)`), which reads `pathspec` to scope the weft commit: switch its base to `hubgeometry.BoardDir(l.Hub)` (a layout `l` is already resolved in that PersistentPreRunE/handler — reuse it). **Remove the now-orphaned `weftBaseDir := filepath.Join(l.WeftWorktree(), l.RelPath)` declaration (weft_verbs.go:122) and its preceding comment** — it exists solely to feed the `LoadConfig` call and is unused after the base switch, a "declared and not used" compile error otherwise. Keep the `pathspec`-scoping semantics unchanged; only the config's home moves.
  **(b) clone `--subpath`.** In `fabric.go`, add a `--subpath` string flag (default `"."`) to `cloneCmd` alongside the existing `--reset` flag (`cloneCmd.Flags().String("subpath", ".", "...")` after the `Bool("reset", ...)` call, ~fabric.go:89), read it in the `cloneCmd` RunE closure (`subpath, _ := cloneCmd.Flags().GetString("subpath")`), and pass it to `runCloneWithReset`. In `clone.go`, change `runCloneWithReset(out io.Writer, args []string, reset bool) int` to also take `subpath string` and forward it to `fabricengine.CloneHub(cwd, hostURL, weftURL, subpath)` (the card-16 orchestration consumes `res`). Update the clone `Use`/`Long` help prose (fabric.go:64,66-88) to document `--subpath <rel>` (default `.`) and that clone wires everything; leave the fabric.go:85 "run lyx init" line for batch 6's retarget. Every command keeps a non-empty `Short` (CLI/Cobra Invariant).
- **Commit:** `feat(fabriccli): read repo-wide fabric config and add clone --subpath`

### Card 18: CloneHub anchor/geometry tests (git + marker, adopt/mismatch)

- **Context:**
  - `internal/fabricengine/clone.go`
  - `internal/fabricengine/clone_adopt_test.go`
  - `internal/fabricengine/boardweft.go`
  - `internal/hubgeometry/hubgeometry.go`
  - `internal/lyxtest/lyxtest.go`
- **Edits:**
  - `internal/fabricengine/clone_adopt_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Extend `internal/fabricengine/clone_adopt_test.go` (`//go:build integration`) to cover `CloneHub`'s new engine contract (git + anchor + marker on disk; NOT the CLI-driven wiring/commit/reconcile, which card 19 covers end-to-end):
  - **create** path: `CloneHub(..., "backend")` with an existing `backend/` in the host writes `.fabric-anchor`=`backend` to `<BoardDir>/.fabric-anchor` ON DISK and returns `CloneResult` with `Anchor == "backend"`, a `BoardDir` ending in `_board`, `PrimeCwd` ending in `.../backend`, and a non-empty `WeftBase`;
  - **typo** path: `CloneHub(..., "backedn")` (nonexistent dir) returns a hard error and tears down the hub (assert `HubPath` removed) — extend the existing strict-abort teardown coverage;
  - **root default** path: `CloneHub(..., ".")` writes `.` explicitly to the marker and returns `Anchor == "."`;
  - **adopt** path: a second `CloneHub` against a weft remote already carrying a committed `.fabric-anchor`=`backend` reads the recorded subpath and returns `Anchor == "backend"` with no `--subpath`; a conflicting `CloneHub(..., "frontend")` returns a hard error (mismatch); a matching `CloneHub(..., "backend")` succeeds.
  Reuse the existing adopt-path fixture scaffolding (weft-primary branch adopt vs fresh-fork); for the adopt case, construct the weft remote fixture with the marker committed on its primary branch. **Compile-forced update:** the signature change to `(CloneResult, error)` ripples to every pre-existing `CloneHub(...)` call site in this file (adopt, fresh-fork, strict-abort, orphan — four sites); thread the new `subpath` argument and switch each to the `CloneResult` return (read `.HubPath` where the old `hubPath` was used) so the file compiles.
- **Commit:** `test(fabricengine): cover CloneHub anchor resolution and marker write`

### Card 19: CLI clone end-to-end + repo-wide config CLI reads test

- **Context:**
  - `internal/fabriccli/clone.go`
  - `internal/fabriccli/fabric.go`
  - `internal/fabriccli/cli_test.go`
  - `internal/fabriccli/testmain_test.go`
  - `internal/fabricengine/junction.go`
  - `internal/fslink/fslink.go`
  - `internal/hubgeometry/hubgeometry.go`
  - `internal/lyxtest/lyxtest.go`
- **Edits:**
  - `internal/fabriccli/cli_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add a subtest in `internal/fabriccli/cli_test.go` (`//go:build integration`) driving `RunCLI` with `fabric clone --subpath backend <host-url> <weft-url>` against a local two-repo fixture (host with a `backend/` dir, empty-but-existing weft remote), asserting the end-to-end clone-does-everything contract: the JSON envelope contains `"ok":true`, a `"hub"` path, and `"anchor":"backend"`; AND that after the CLI clone the prime worktree's `_lyx`/`_pattern` junctions are wired (`fslink.IsLink` on the host links), `<BoardDir>/.fabric-anchor` and `<BoardDir>/_lyx/config/fabric.yaml` are committed on `weft:main`, and per-worktree module configs were reconciled — i.e. the card-16 CLI orchestration ran. Add a second subtest asserting the default (`fabric clone <host> <weft>` with no `--subpath`) echoes `"anchor":"."`. **Also update the two pre-existing CLI fixtures so their fabric config seeds at the repo-wide `BoardDir` base** (both would break the card-17 migration):
  - `setupCLIRepo` (cli_test.go:30-40): write to `hubgeometry.ConfigFile(hubgeometry.BoardDir(f.Hub), "fabric")` (materializing `<hub>/_board/_lyx/config/fabric.yaml`) instead of `hubgeometry.ConfigFile(f.Hub, "fabric")`, so the topology-verb subtests resolve config from where the migrated `LoadConfig(BoardDir(l.Hub))` sites read it;
  - `TestRunCLI_EnvMapToOption` (cli_test.go:179-212): it seeds fabric config at `fixture.WeftPrime` via `lyxtest.SeedConfig` against a `lyxtest.CopyPaired(t)` fixture (which never materializes a `_board` dir); since card 17 migrates `weft_verbs.go:124` to `LoadConfig(BoardDir(l.Hub))`, seed the config at `hubgeometry.BoardDir(fixture.Hub)` instead (create the `_board`/`_lyx/config/` and write `fabric.yaml` there).
  Without these fixes the migrated verbs fail `not found` and Batch 4's `verify:` breaks. Reuse the package's local-fixture helpers and `HermeticGitEnv` `TestMain`; if the package lacks a two-repo clone fixture helper, build a minimal one inline (a bare host repo with `backend/` committed + a bare weft repo). Keep the existing verb-surface subtests green — the no-arg verb listing changes verb count only in batch 6 (unwire), not here.
- **Commit:** `test(fabriccli): cover CLI clone end-to-end and repo-wide config reads`

## Batch Tests

`verify: go test -tags integration ./internal/fabricengine/... ./internal/fabriccli/...` runs both packages under the integration superset. fabricengine coverage: the edited `clone_adopt_test.go` plus every other clone/weft-git integration test (`commitweftat_test.go`, `syncweft_integration_test.go`) that could regress from the `CloneHub` signature change, and all untagged unit tests. fabriccli coverage: the edited `cli_test.go` (end-to-end CLI clone + the two migrated fixtures) and `pushbypass_integration_test.go`. Whole-package scope is justified because the `CloneHub` `CloneResult` signature change and the CLI-layer orchestration ripple through every caller and fixture in both packages; a scoped single-file run would miss compile-level breakage in sibling tests. The repo-wide `done_gate` catches any other caller of `CloneHub` (there is only `internal/fabriccli`).
