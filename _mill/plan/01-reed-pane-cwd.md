# Batch: reed pane cwd

```yaml
task: websterengine + webstercli told-geometry, and Webster standalone entry
batch: reed pane cwd
number: 1
cards: 5
verify: go test ./internal/reedengine/... ./internal/hubgeom/... && go test -tags integration ./internal/reedengine/...
depends-on: []
```

## Batch Scope

This batch splits `reedengine.Geometry.AnchorPath`'s two unrelated jobs — the base `stateDir` joins `.lyx` onto, and the cwd every tmux pane is spawned with — by adding a `PaneCwd string` field and consuming it at the two spawn sites.
It is one batch because the field addition, its two consumers, its single production constructor (`hubgeom.ReedGeometry`), and every test `Geometry` literal that would otherwise spawn with `-c ""` all have to land together for the reed suites to stay green.
Everything here is hub-neutral by construction: `hubgeom.ReedGeometry` sets `PaneCwd` to `l.AnchorPath()`, the exact value both spawn sites already used, so no resolved path changes in any real worktree.
The external interface batch 6 consumes is the `PaneCwd` field itself — `internal/standalonegeom` sets it to the standalone target directory while `AnchorPath` stays `<state>`.

## Cards

### Card 1: Add `PaneCwd` to `reedengine.Geometry`

- **Context:**
  - `internal/reedengine/lifecycle.go`
  - `internal/reedengine/doc.go`
- **Edits:**
  - `internal/reedengine/geometry.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add a `PaneCwd string` field to `reedengine.Geometry`, declared immediately after `AnchorPath`.
  Four doc sites in `internal/reedengine/geometry.go` change in this card:
  the file-header sentence at the top, which says "the seven-field struct" and becomes "the eight-field struct";
  `AnchorPath`'s own field comment, which currently reads "the base stateDir joins onto for reed.json/reed.lock, and the cwd every pane is spawned with" and must drop the pane-cwd half, since the spawn sites read `PaneCwd` after card 2;
  the type-level comment naming `hubgeom.ReedGeometry` as the hub-mode answer, which stays accurate and is left alone in this batch;
  and the new `PaneCwd` field's own comment, which must state that it is the cwd every tmux pane is spawned with, that it equals `AnchorPath` in hub mode and the standalone target directory in standalone mode, and that there is deliberately **no zero-value fallback** — an empty `PaneCwd` must never silently mean `AnchorPath`, or a caller that forgets the field spawns in the wrong directory with nothing to catch it.
  Add no constructor, no validator, and no default to this file — it states the contract only.
- **Commit:** `feat(reedengine): add Geometry.PaneCwd, the pane spawn cwd told apart from AnchorPath`

### Card 2: Read `PaneCwd` at the two tmux spawn sites

- **Context:**
  - `internal/reedengine/geometry.go`
- **Edits:**
  - `internal/reedengine/lifecycle.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `internal/reedengine/lifecycle.go`, the `spawnSession` closure's `new-session` argv passes `"-c", e.geom.AnchorPath`;
  change it to `"-c", e.geom.PaneCwd` and reword the adjacent comment so it stops describing `-c` as pinning the pane cwd "to the invoking worktree cwd" and instead names `Geometry.PaneCwd` as the told value.
  In `ensureHeaderPaneLocked`, the header `split-window` call passes `"-c", e.geom.AnchorPath`;
  change it to `"-c", e.geom.PaneCwd`.
  Do not touch `stateDir`, which joins `lyxdirs.DotLyxDirName` onto `e.geom.AnchorPath` and must keep reading `AnchorPath`.
  Change no other `e.geom.AnchorPath` reference in the file.
- **Commit:** `refactor(reedengine): spawn tmux panes with Geometry.PaneCwd`

### Card 3: `hubgeom.ReedGeometry` sets `PaneCwd`

- **Context:**
  - `internal/reedengine/geometry.go`
- **Edits:**
  - `internal/hubgeom/hubgeom.go`
  - `internal/hubgeom/hubgeom_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** `ReedGeometry` gains `PaneCwd: l.AnchorPath()` in the returned `reedengine.Geometry` literal — the exact value both spawn sites read before card 2, so this is byte-identical to today's behaviour in anchored and subpath-anchored hubs alike.
  Extend `TestReedGeometry` in `internal/hubgeom/hubgeom_test.go` to assert `PaneCwd == l.AnchorPath()`, and add a second table row using an unanchored fixture (`AnchorRel` of `"."`) alongside the existing subpath-anchored one so both anchoring shapes are pinned.
  The subpath-anchored row must additionally assert `PaneCwd != WorktreeRoot`, since `WorktreeRoot` is `l.WorktreePath()` here and the two coincide only at `AnchorRel == "."` — that assertion is what catches a later "simplification" that repoints the spawn sites at `WorktreeRoot`.
- **Commit:** `feat(hubgeom): set Geometry.PaneCwd to AnchorPath, preserving today's pane cwd`

### Card 4: Give every reed `Geometry` test literal an explicit `PaneCwd`

- **Context:**
  - `internal/reedengine/geometry.go`
  - `internal/reedengine/header_test.go`
- **Edits:**
  - `internal/reedengine/lock_test.go`
  - `internal/reedengine/contract_integration_test.go`
  - `internal/reedengine/mouse_boot_integration_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** These test files build `reedengine.Geometry` literals directly rather than through `hubgeom` — they cannot import it without an import cycle — so no hub-mode default reaches them and an unset `PaneCwd` would spawn panes with `-c ""`.
  Add an explicit `PaneCwd` entry to the three literals in `internal/reedengine/lock_test.go`, the two in `internal/reedengine/contract_integration_test.go`, and the one in `internal/reedengine/mouse_boot_integration_test.go`.
  In the two integration files, set `PaneCwd` to the same value the literal already gives `AnchorPath`, so the live-tmux scenarios spawn exactly where they do today.
  In `newTestEngine` in `internal/reedengine/lock_test.go`, set `PaneCwd` to a *distinct* directory — `filepath.Join(hub, "pane")` — so that card 5's assertion cannot pass by coincidence;
  set the other two `lock_test.go` literals to their own `AnchorPath` value.
  `internal/reedengine/header_test.go` also holds a `Geometry` literal, but it populates only `RepoName` and `HubPath` for header-token rendering and reaches no spawn site, so it deliberately gets no `PaneCwd` row;
  do not edit that file.
- **Commit:** `test(reedengine): give every spawning Geometry literal an explicit PaneCwd`

### Card 5: Pin the header split-window pane cwd to `PaneCwd`

- **Context:**
  - `internal/reedengine/lifecycle.go`
  - `internal/reedengine/lock_test.go`
  - `internal/reedengine/geometry.go`
- **Edits:**
  - `internal/reedengine/lifecycle_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add a hermetic test to `internal/reedengine/lifecycle_test.go` that builds an engine via `newTestEngine`, installs an `e.tmux.execHook` capturing the `split-window` argv the way `TestEnsureHeaderPaneLocked_RebuildRejectsSilentSplitFailure` already does, drives `ensureHeaderPaneLocked`, and asserts the captured argv carries `-c` followed by the engine's `PaneCwd` and **not** its `AnchorPath`.
  The hook must also answer `list-panes` so the split target resolves, and must return a genuinely new pane id from `split-window` so the silent-split guard does not reject the call.
  The `new-session` spawn site is not reachable from this seam — it builds its argv and runs it through `exec.Command` directly rather than through `e.tmux` — so state in the test's doc comment that the `new-session` half of the same change is covered by the tagged reed suites this batch's `verify:` also runs.
- **Commit:** `test(reedengine): pin the header split-window cwd to Geometry.PaneCwd`

## Batch Tests

`verify:` runs `go test ./internal/reedengine/... ./internal/hubgeom/...` for the untagged suites plus `go test -tags integration ./internal/reedengine/...` for the tagged ones.
The tagged half is not optional here: cards 4 edits `contract_integration_test.go` and `mouse_boot_integration_test.go`, both `//go:build integration`, and those two files are the only place the `new-session` spawn site is exercised at all, since that call bypasses the `e.tmux` seam a hermetic test could hook.
The repo's own `pipeline.done_gate` already runs `go test -tags integration ./...`, so the tagged suites are expected to be runnable in this environment.
Card 3 covers `hubgeom.ReedGeometry` at both anchoring shapes;
card 5 covers the header split site hermetically;
card 4's distinct `PaneCwd` in `newTestEngine` is what makes card 5's assertion non-tautological.
