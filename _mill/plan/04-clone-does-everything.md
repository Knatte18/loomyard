# Batch: clone-does-everything

```yaml
task: 'fabric: clone-does-everything + subpath-in-weft + init dissolution'
batch: clone-does-everything
number: 4
cards: 6
verify: go test -tags integration ./internal/fabricengine/... ./internal/fabriccli/...
depends-on: [1, 3]
```

## Batch Scope

This batch makes `lyx fabric clone` do the whole topology job in one shot: clone host + weft, check out the weft primary pairing, materialize the `weft:main` board worktree (all existing), then additionally — resolve the subpath (adopt-or-create), write `.fabric-anchor` + the repo-wide `fabric.yaml` onto `weft:main`, wire all prime-worktree junctions, create `_lyx`/`_lyx/config`, maintain the `.gitignore` `.lyx/` block, and run `configsync.ReconcileAll` once. `CloneHub` is modified in place (test-fenced, no parallel entry point) and its signature grows a `subpath` parameter and an `anchor` return value. The CLI handler gains a `--subpath` flag (default `.`) and echoes the resolved anchor in the JSON envelope.

Depends on batch 1 (the `hubgeometry.FabricAnchorName` constant clone writes, and correct anchor-based `Resolve`) and batch 3 (`configsync.ReconcileFabricAt` for the repo-wide `fabric.yaml` materialization). Reuses batch-2-era `WiredNames(BoardDir)` for the repo-wide junction name-set (available today; the base is explicit). The two `weft:main` writes route through `fabricengine.CommitWeftAt`/`PushWeftAt` per the Weft Git Invariant.

Batch-local decision: adopt-vs-create is detected by whether `.fabric-anchor` already exists in the freshly-materialized board worktree (adopt: read it; create: validate `--subpath` and write it) — mirroring `suffixWeftPrimaryBranch`'s adopt-or-create shape.

Batch-local decision (accepted limitation): because the `--subpath` CLI flag defaults to `"."` and the adopt-path mismatch check treats `subpath == "" || subpath == "."` as "unset/default" (never erroring), a re-clone with an explicit `--subpath .` against a repo actually anchored at a subpath will silently adopt the recorded subpath rather than error. This is accepted: the mismatch guard's job is to catch a wrong NON-default value (a typo'd real subpath), and `anchor-value-always-explicit` means the record is authoritative on adopt. Precise "explicit-root-vs-unset" detection (a distinct sentinel default) is deliberately not added — it is extra machinery for a benign edge, and root-vs-subpath re-clones read the record either way.

## Cards

### Card 14: `CloneHub` grows a subpath parameter and anchor return

- **Context:**
  - `internal/fabricengine/boardweft.go`
  - `internal/hubgeometry/hubgeometry.go`
  - `internal/fabricengine/branchname.go`
- **Edits:**
  - `internal/fabricengine/clone.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Change `CloneHub(cwd, hostURL, weftURL string) (hubPath string, err error)` (clone.go:55) to `CloneHub(cwd, hostURL, weftURL, subpath string) (hubPath string, anchor string, err error)`. Thread the extra return through the existing error paths: every `return "", teardownHub(hubPath, err)` becomes `return "", "", teardownHub(hubPath, err)`, and the three early no-cleanup errors — empty name (clone.go:60-63), hub-already-exists (clone.go:69-71), and the `os.MkdirAll(hubPath, …)` failure (clone.go:74-76, `return "", err`) — return `"", "", <err>`. Do not yet add the anchor/wiring logic — card 15 adds it; for now default `anchor` to `filepath.Clean(subpath)` (or `"."` when empty) and return it on the success path (clone.go:122) so the signature is complete and compiles. Update `CloneHub`'s doc comment to state it now also resolves the recorded subpath anchor, wires junctions, and returns the resolved anchor.
- **Commit:** `refactor(fabricengine): CloneHub takes subpath, returns resolved anchor`

### Card 15: Resolve the anchor (adopt-or-create), validate, write the two weft:main files

- **Context:**
  - `internal/fabricengine/boardweft.go`
  - `internal/fabricengine/weftgit.go`
  - `internal/boardengine/sync.go`
  - `internal/configsync/configsync.go`
  - `internal/hubgeometry/hubgeometry.go`
- **Edits:**
  - `internal/fabricengine/clone.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `CloneHub`, after `ensureBoardWorktree(weftPath, hostBranch, boardDir)` (clone.go:117; capture `boardDir := hubgeometry.BoardDir(hubPath)`), resolve the anchor adopt-or-create:
  - `markerPath := filepath.Join(boardDir, hubgeometry.FabricAnchorName)`.
  - **Adopt** (re-clone): if `markerPath` exists (`os.Stat`), read it (`os.ReadFile`+`TrimSpace`) → `recorded`. If the caller passed a non-default `subpath` that differs from `recorded` (compare `filepath.Clean(subpath)` vs `recorded`, treating empty/`.` as root), return a hard error via `teardownHub` (never silently re-anchor). Set `anchor = recorded`. Do NOT rewrite the marker.
  - **Create** (first-ever): marker absent. Set `anchor = filepath.Clean(subpath)`; empty → `"."`. **Validate the subpath exists in the host worktree**: `os.Stat(filepath.Join(hostWorktreePath, anchor))` must succeed and be a directory — else `teardownHub` with a hard error naming the bad subpath (catches a typo like `backedn`). Write the two repo-wide files into the board worktree on disk: `os.WriteFile(markerPath, []byte(anchor+"\n"), 0o644)`, and materialize the repo-wide `fabric.yaml` via `configsync.ReconcileFabricAt(boardDir, true)`. Then commit+push both onto `weft:main` through the choke point: `fabricengine.CommitWeftAt(boardDir, "fabric clone: record anchor + repo-wide config", fabricengine.SyncOptions{})` then `fabricengine.PushWeftAt(boardDir, fabricengine.SyncOptions{})` (the same path board's `Sync` uses; wildcard `git add -A` correctly stages the root marker and `_lyx/config/fabric.yaml`).
  Return `anchor` on success. On any failure route through `teardownHub`.
- **Commit:** `feat(fabricengine): record .fabric-anchor + repo-wide fabric.yaml on clone`

### Card 16: Wire prime-worktree junctions, create `_lyx`, maintain `.gitignore`, ReconcileAll once

- **Context:**
  - `internal/fabricengine/junction.go`
  - `internal/fabricengine/junctionnames.go`
  - `internal/gitignore/gitignore.go`
  - `internal/configsync/configsync.go`
  - `internal/hubgeometry/hubgeometry.go`
- **Edits:**
  - `internal/fabricengine/clone.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `CloneHub`, after the anchor is recorded (card 15), do the wiring that `lyx init` used to do, against the prime host worktree at its resolved anchor. Build the prime layout: `l, err := hubgeometry.Resolve(filepath.Join(hostWorktreePath, anchor))` (now that the marker exists, `Resolve` yields the correct `RelPath`). Then:
  - load the repo-wide junction name-set: `names, err := fabricengine.WiredNames(boardDir)` (reads `<boardDir>/_lyx/config/fabric.yaml`);
  - ensure the weft-side dirs exist and wire: `fabricengine.WireJunctions(l, filepath.Base(l.WorktreeRoot), names)` (idempotent; creates host junctions + git-exclude entries after the weft worktree exists so targets resolve);
  - create the host `_lyx`/`_lyx/config` structure only where wiring did not already provide it (WireJunctions seeds `_lyx` as a junction into weft — do not also `MkdirAll` a real `_lyx` over the junction; follow the ordering `init.go` used: obtain names, then wire, and let `configsync.ReconcileAll(weftBase, true)` populate module configs on the weft side where `weftBase := filepath.Join(l.WeftWorktree(), l.RelPath)`);
  - maintain the `.gitignore` `.lyx/` managed block at the host worktree: `gitignore.Ensure(l.Cwd, ".lyx/")`.
  Run `configsync.ReconcileAll` exactly once for the per-worktree modules (fabric is already skipped by batch 3). Keep config ownership in `configsync` — clone only invokes it. Order matters: weft worktree exists → wire → ReconcileAll → gitignore.
- **Commit:** `feat(fabricengine): clone wires junctions, _lyx, .gitignore, and reconciles config`

### Card 17: fabriccli reads repo-wide fabric config + clone `--subpath` echoes the anchor

- **Context:**
  - `internal/output/output.go`
  - `internal/hubgeometry/hubgeometry.go`
  - `internal/fabricengine/topology.go`
  - `internal/fabricengine/config.go`
- **Edits:**
  - `internal/fabriccli/clone.go`
  - `internal/fabriccli/fabric.go`
  - `internal/fabriccli/weft_verbs.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Two coupled changes to the fabric CLI layer.
  **(a) Migrate every CLI config-read to the repo-wide `BoardDir`.** Batch 3 stops `configsync.ReconcileAll` from materializing a per-worktree `fabric.yaml`, and batch 4's clone writes `pathspec`/`branch_prefix` only to `<BoardDir>/_lyx/config/fabric.yaml` — so every CLI site that builds a `Topology` from `fabricengine.LoadConfig(cwd)` would break (`configengine.Load`'s `"config file … not found"`) on a freshly-cloned/wired worktree, including `reconcile` itself (the exact remedy batch 6's retargeted messages point users to). Update all eight topology-verb sites in `internal/fabriccli/fabric.go` — `runAdd`/`runList`/`runCheckout`/`runPairs`/`runReconcile`/`runPruneWithFlag`/`runCleanupWithFlags`/`runRemoveWithFlag` (fabric.go:280,317,378,411,443,472,501,531) — each of which currently does `cfg, err := fabricengine.LoadConfig(cwd)` then `top := fabricengine.NewTopology(cfg)`: resolve the layout first (`l, err := hubgeometry.Resolve(cwd)`) and load from the repo-wide base — `cfg, err := fabricengine.LoadConfig(hubgeometry.BoardDir(l.Hub))`. Apply the same migration to the ninth site in `internal/fabriccli/weft_verbs.go:124` (`loadedCfg, err := fabricengine.LoadConfig(weftBaseDir)`), which reads `pathspec` to scope the weft commit: switch its base to `hubgeometry.BoardDir(l.Hub)` (a layout `l` is already resolved in that PersistentPreRunE/handler — reuse it). Keep the `pathspec`-scoping semantics unchanged; only the config's home moves.
  **(b) clone `--subpath`.** In `fabric.go`, add a `--subpath` string flag (default `"."`) to `cloneCmd` alongside the existing `--reset` flag (`cloneCmd.Flags().String("subpath", ".", "...")` after the `Bool("reset", ...)` call, ~fabric.go:89), read it in the `cloneCmd` RunE closure (`subpath, _ := cloneCmd.Flags().GetString("subpath")`), and pass it to `runCloneWithReset`. In `clone.go`, change `runCloneWithReset(out io.Writer, args []string, reset bool) int` to also take `subpath string`; forward to `fabricengine.CloneHub(cwd, hostURL, weftURL, subpath)` (now `(hubPath, anchor, err)`); on success emit `output.Ok(out, map[string]any{"hub": hubPath, "anchor": anchor})`. Update the clone `Use`/`Long` help prose (fabric.go:64,66-88) to document `--subpath <rel>` (default `.`) and that clone wires everything; leave the fabric.go:85 "run lyx init" line for batch 6's retarget. Every command keeps a non-empty `Short` (CLI/Cobra Invariant).
- **Commit:** `feat(fabriccli): read repo-wide fabric config and add clone --subpath`

### Card 18: Clone create/adopt/mismatch tests

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
- **Requirements:** Extend `internal/fabricengine/clone_adopt_test.go` (`//go:build integration`) to cover the new clone contract:
  - **create** path: `CloneHub(..., "backend")` with an existing `backend/` in the host writes `.fabric-anchor`=`backend` to `<BoardDir>/.fabric-anchor` and a repo-wide `fabric.yaml` at `<BoardDir>/_lyx/config/fabric.yaml` on `weft:main`, wires the prime junctions, and returns `anchor == "backend"`;
  - **typo** path: `CloneHub(..., "backedn")` (nonexistent dir) returns a hard error and tears down the hub (assert `hubPath` removed) — extend the existing strict-abort teardown coverage;
  - **root default** path: `CloneHub(..., ".")` writes `.` explicitly to the marker and returns `anchor == "."`;
  - **adopt** path: a second `CloneHub` against a weft remote already carrying `.fabric-anchor`=`backend` reads the recorded subpath and returns `anchor == "backend"` without a `--subpath` arg; a conflicting `CloneHub(..., "frontend")` on that same re-clone returns a hard error (mismatch, no silent re-anchor); a matching `CloneHub(..., "backend")` succeeds.
  Reuse the existing adopt-path fixture scaffolding in the file (weft-primary branch adopt vs fresh-fork). Where a test needs a two-URL clone with a pre-seeded weft `.fabric-anchor`, construct the weft remote fixture with the marker committed on its primary branch.
  **Compile-forced update:** card 14's `CloneHub` signature change `(hubPath, err)` → `(hubPath, anchor, err)` ripples to every pre-existing `CloneHub(...)` call site in this file (the adopt, fresh-fork, strict-abort, and orphan tests — four sites). Thread the new `subpath` argument into each call and capture (or discard with `_`) the new `anchor` return so the file still compiles, in addition to adding the new subtests above.
- **Commit:** `test(fabricengine): cover clone create/adopt/mismatch/root-default anchor`

### Card 19: CLI clone handler test

- **Context:**
  - `internal/fabriccli/clone.go`
  - `internal/fabriccli/fabric.go`
  - `internal/fabriccli/cli_test.go`
  - `internal/fabriccli/testmain_test.go`
  - `internal/lyxtest/lyxtest.go`
- **Edits:**
  - `internal/fabriccli/cli_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add a subtest in `internal/fabriccli/cli_test.go` (`//go:build integration`) driving `RunCLI` with `fabric clone --subpath backend <host-url> <weft-url>` against a local two-repo fixture (host with a `backend/` dir, empty-but-existing weft remote), asserting the JSON envelope contains `"ok":true`, a `"hub"` path, and `"anchor":"backend"`. Add a second subtest asserting the default (`fabric clone <host> <weft>` with no `--subpath`, host has a usable root) echoes `"anchor":"."`. **Also update the two pre-existing CLI fixtures so their fabric config seeds at the repo-wide `BoardDir` base** (both currently seed elsewhere and would break the card-17 migration — the masked-regression pattern from the round-1/round-3 reviews):
  - `setupCLIRepo` (cli_test.go:30-40): write to `hubgeometry.ConfigFile(hubgeometry.BoardDir(f.Hub), "fabric")` (materializing `<hub>/_board/_lyx/config/fabric.yaml`) instead of `hubgeometry.ConfigFile(f.Hub, "fabric")`, so the topology-verb subtests resolve config from where the migrated `LoadConfig(BoardDir(l.Hub))` sites now read it;
  - `TestRunCLI_EnvMapToOption` (cli_test.go:179-212): it drives `push` against a `lyxtest.CopyPaired(t)` fixture and seeds fabric config at `fixture.WeftPrime` via `lyxtest.SeedConfig`. Since `CopyPaired` never materializes a `_board` dir, and card 17 migrates `weft_verbs.go:124`'s `LoadConfig(weftBaseDir)` → `LoadConfig(BoardDir(l.Hub))`, `push` would fail `LoadConfig`'s `_lyx/` existence check. Fix the fixture to seed fabric config at `hubgeometry.BoardDir(fixture.Hub)` — create the `_board` dir and its `_lyx/config/` and write `fabric.yaml` there (or `SeedConfig` against that base) — so the weft-verb config load resolves.
  Without these fixes the migrated verbs fail `not found` and Batch 4's own `verify:` breaks on these pre-existing tests. Reuse the package's existing local-fixture helpers and `HermeticGitEnv` `TestMain`; if the package lacks a two-repo clone fixture helper, build a minimal one inline (init a bare host repo with a `backend/` committed and a bare weft repo). Keep the existing verb-surface subtests green — the no-arg verb listing changes verb count only in batch 6 (unwire), not here.
- **Commit:** `test(fabriccli): cover clone --subpath and repo-wide config CLI reads`

## Batch Tests

`verify: go test -tags integration ./internal/fabricengine/... ./internal/fabriccli/...` runs both packages under the integration superset. fabricengine coverage: the edited `clone_adopt_test.go` plus every other clone/weft-git integration test (`commitweftat_test.go`, `syncweft_integration_test.go`) that could regress from the `CloneHub` signature change, and all untagged unit tests. fabriccli coverage: the edited `cli_test.go` and `pushbypass_integration_test.go`. Whole-package scope is justified because the `CloneHub` signature change ripples through every caller and fixture in both packages; a scoped single-file run would miss compile-level breakage in sibling tests. The repo-wide `done_gate` catches any other caller of `CloneHub` (there is only `internal/fabriccli`).
