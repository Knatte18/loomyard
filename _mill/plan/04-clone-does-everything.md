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
- **Requirements:** Change `CloneHub(cwd, hostURL, weftURL string) (hubPath string, err error)` (clone.go:55) to `CloneHub(cwd, hostURL, weftURL, subpath string) (hubPath string, anchor string, err error)`. Thread the extra return through the existing error paths: every `return "", teardownHub(hubPath, err)` becomes `return "", "", teardownHub(hubPath, err)`, and the early no-cleanup errors (empty name clone.go:60-63, hub-already-exists clone.go:69-71) return `"", "", <err>`. Do not yet add the anchor/wiring logic — card 15 adds it; for now default `anchor` to `filepath.Clean(subpath)` (or `"."` when empty) and return it on the success path (clone.go:122) so the signature is complete and compiles. Update `CloneHub`'s doc comment to state it now also resolves the recorded subpath anchor, wires junctions, and returns the resolved anchor.
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

### Card 17: CLI clone handler gains `--subpath` and echoes the anchor

- **Context:**
  - `internal/fabriccli/fabric.go`
  - `internal/output/output.go`
  - `internal/hubgeometry/hubgeometry.go`
- **Edits:**
  - `internal/fabriccli/clone.go`
  - `internal/fabriccli/fabric.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `internal/fabriccli/fabric.go`, add a `--subpath` string flag (default `"."`) to the `cloneCmd` alongside the existing `--reset` flag (`cloneCmd.Flags().String("subpath", ".", "...")` after the `Bool("reset", ...)` call, ~fabric.go:89), and read it inside the `cloneCmd` RunE closure (`subpath, _ := cloneCmd.Flags().GetString("subpath")`), passing it to `runCloneWithReset`. In `internal/fabriccli/clone.go`, change `runCloneWithReset(out io.Writer, args []string, reset bool) int` to also take `subpath string`; forward it to `fabricengine.CloneHub(cwd, hostURL, weftURL, subpath)` (now returning `(hubPath, anchor, err)`); on success emit `output.Ok(out, map[string]any{"hub": hubPath, "anchor": anchor})`. Update the clone `Use`/`Long` help prose (fabric.go:64,66-88) to document `--subpath <rel>` (default `.`) and that clone now wires everything (no separate `lyx init`); leave the fabric.go:85 "run lyx init" line for batch 6 (it is retargeted there) OR update it here if it falls naturally in the same edit — prefer leaving cross-cutting message retargets to batch 6 to keep this batch's diff focused. Every command still carries a non-empty `Short` (CLI/Cobra Invariant).
- **Commit:** `feat(fabriccli): clone --subpath flag echoes resolved anchor`

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
- **Requirements:** Add a subtest in `internal/fabriccli/cli_test.go` (`//go:build integration`) driving `RunCLI` with `fabric clone --subpath backend <host-url> <weft-url>` against a local two-repo fixture (host with a `backend/` dir, empty-but-existing weft remote), asserting the JSON envelope contains `"ok":true`, a `"hub"` path, and `"anchor":"backend"`. Add a second subtest asserting the default (`fabric clone <host> <weft>` with no `--subpath`, host has a usable root) echoes `"anchor":"."`. Reuse the package's existing local-fixture helpers and `HermeticGitEnv` `TestMain`; if the package lacks a two-repo clone fixture helper, build a minimal one inline (init a bare host repo with a `backend/` committed and a bare weft repo). Keep the existing `cli_test.go` verb-surface subtests green — note the no-arg verb listing will change verb count only in batch 6 (unwire), not here.
- **Commit:** `test(fabriccli): cover clone --subpath anchor echo in JSON envelope`

## Batch Tests

`verify: go test -tags integration ./internal/fabricengine/... ./internal/fabriccli/...` runs both packages under the integration superset. fabricengine coverage: the edited `clone_adopt_test.go` plus every other clone/weft-git integration test (`commitweftat_test.go`, `syncweft_integration_test.go`) that could regress from the `CloneHub` signature change, and all untagged unit tests. fabriccli coverage: the edited `cli_test.go` and `pushbypass_integration_test.go`. Whole-package scope is justified because the `CloneHub` signature change ripples through every caller and fixture in both packages; a scoped single-file run would miss compile-level breakage in sibling tests. The repo-wide `done_gate` catches any other caller of `CloneHub` (there is only `internal/fabriccli`).
