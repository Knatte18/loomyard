# Batch: header-pane-and-render

```yaml
task: "Built-in operator console pane in mux"
batch: header-pane-and-render
number: 4
cards: 7
verify: go test -tags integration ./internal/muxengine/... ./internal/muxcli/...
depends-on: [3]
```

## Batch Scope

Makes the header a real, persistent tmux/psmux pane: booted before any strand, excluded from strand
accounting/adoption/reconcile, enumerated by the render layer as a fixed-height top band, and
protected so removing the last strand can never tear down the session. Lifecycle and render land in
ONE batch because they are atomically coupled — a header pane that the render layer does not
enumerate is reaped by `applyLayoutLocked`'s `select-layout` (tmux rejects a mismatched pane count;
psmux reaps the extra pane), so neither half is independently verifiable. Batch-local decision:
`planPaneTarget` excludes the header from *adoption* always, and from *split-target* selection except
as the sole-pane fallback (so an add-after-all-strands-removed still has a pane to split — the header
survives the split and render restores its fixed height).

## Cards

### Card 14: MuxState.HeaderPaneID and the launch-command helper

- **Context:**
  - `internal/shell/shell.go`
  - `internal/muxengine/spawn.go`
  - `internal/weftcli/spawn.go`
- **Edits:**
  - `internal/muxengine/state.go`
- **Creates:**
  - `internal/muxengine/headerpane.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add `HeaderPaneID string ` + "`json:\"headerPaneId,omitempty\"`" + ` to `MuxState`
  (godoc: the tmux pane id of the always-present header pane, outside `Strands`, never a strand). In
  `headerpane.go` add a pure helper `func headerLaunchCmd(sh shell.Shell, exe string) string` that
  returns the shell command line running `<exe> mux header --blocking`, composed via `sh.Invoke(exe)`
  + the quoted `mux header --blocking` args (never raw shell syntax — Shell Mechanics Seam). Keep it
  pure (shell + exe injected) so it is host-testable; `os.Executable()` is called by the boot site
  (card 17), not here.
- **Commit:** `feat(muxengine): add HeaderPaneID state and pure header-launch command helper`

### Card 15: Render enumerates the header as a top band

- **Context:**
  - `internal/muxengine/render/layout.go`
  - `internal/muxengine/render/focus.go`
- **Edits:**
  - `internal/muxengine/render/types.go`
  - `internal/muxengine/render/rules.go`
  - `internal/muxengine/render/policy.go`
  - `internal/muxengine/apply.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Thread the header into the render entry point WITHOUT modelling it as a strand.
  Add a `Header` value to `render.Params` (or a new param on `Rules`) carrying `PaneID string` and
  `HeightRows int` (zero `PaneID` means "no header", preserving today's behaviour). In
  `render.Rules`, when a header pane id is present, emit a fixed-height top-band cell at
  `{X:0, Y:0, W:box.W, H:headerHeight}` for that pane id and place the strand stack in the shrunk
  `Box{X:0, Y:headerHeight, W:box.W, H:box.H-headerHeight}` (reuse the existing `buildStackBody`
  against the shrunk box). The emitted `window_layout` must enumerate the header cell + every strand
  cell so the live-pane count matches. In `apply.go` (`planLayout`/`applyLayoutLocked`), pass
  `st.HeaderPaneID` and the configured header height into `Rules`. `partitionByAnchor`/`orderStack`
  are unchanged — the header is injected at the `Rules` seam, not into the strand slice.
- **Commit:** `feat(muxengine): render the header pane as a fixed-height top band`

### Card 16: Header-vs-window height clamp

- **Context:**
  - `internal/muxengine/render/rules.go`
  - `internal/muxengine/render/policy.go`
- **Edits:**
  - `internal/muxengine/render/height.go`
  - `internal/muxengine/render/layout.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add the new window-split clamp (distinct from `clampToFit`, which distributes rows
  among strands inside an already-shrunk box). Add `func clampHeaderHeight(headerRows, windowRows,
  minStackRows int) int` that returns the header height clamped so the strand-stack region keeps a
  floor of at least `minStackRows` total rows (use `MinFullRows` as the region floor, min 1); the
  header yields rows before the stack is starved. Apply it where card 15 computes the header/strand
  split so an oversized `height_rows` can never shrink the strand box below the floor. `clampToFit`
  then distributes within the (possibly clamped) shrunk box as today.
- **Commit:** `feat(muxengine): clamp header height so the strand stack keeps its floor`

### Card 17: Boot and recreate the header pane

- **Context:**
  - `internal/muxengine/spawn.go`
  - `internal/muxengine/overlay.go`
  - `internal/muxengine/reconcile.go`
  - `internal/muxengine/header.go`
  - `internal/muxengine/headerpane.go`
  - `internal/shell/shell.go`
- **Edits:**
  - `internal/muxengine/lifecycle.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `ensureServerAndSessionLocked` (the shared boot path both `Up` (lifecycle.go:390)
  and `Resume` (lifecycle.go:442) call), after the session/initial pane exists and BEFORE any strand
  work: first call `e.ValidateHeader()` and return its error via the normal path (loud, before the pane
  is created) — placing it on the SHARED boot path (not only `Up`) so a bad template is caught eagerly
  on a resume-after-crash too, per the `eager-header-validation` decision. Then ensure the header pane:
  if `st.HeaderPaneID` is empty or not in the live pane set, create it by splitting the initial pane
  with `-c <e.layout.Hub>` (Hub Geometry Invariant — never recompute), running
  `headerLaunchCmd(shell.ForGOOS(), exe)` where `exe` comes from `os.Executable()`, capture the new
  `#{pane_id}` into `st.HeaderPaneID`, and persist. Make this idempotent across `up`/`resume` (recreate
  only when missing). On the reboot path (`lifecycle.go`'s `if booted` block that calls
  `clearAllPaneBindings` from `reconcile.go` to wipe stale strand bindings), also clear `HeaderPaneID`
  in that same block so it is rebuilt fresh — the clear lands in lifecycle.go, not in
  `clearAllPaneBindings` itself. Reuse the existing `send-keys` launch mechanics from `spawn.go`; the
  header pane runs no strand command and is never added to `st.Strands`.
- **Commit:** `feat(muxengine): boot and recreate the persistent header pane at up`

### Card 18: Exclude the header from adoption, split-target, and reconcile

- **Context:**
  - `internal/muxengine/apply.go`
  - `internal/muxengine/state.go`
- **Edits:**
  - `internal/muxengine/spawn.go`
  - `internal/muxengine/reconcile.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Give `planPaneTarget` the header pane id (thread `st.HeaderPaneID` through its
  caller). In the `!anyBound` adoption loop, skip the header pane id — never adopt the header as a
  strand's pane. In the split-target selection, prefer the tallest alive NON-header pane; fall back to
  the header pane only when it is the sole live pane (so an add-after-all-strands-removed still splits
  successfully — the header survives the split, and render/card 16 restores its fixed height). In
  `planReconcile` (`reconcile.go:93-113`), do NOT add `st.HeaderPaneID` to `boundPaneIDs` — that map
  also gates `anyBoundPresent` (reconcile.go:99-106), so folding the header in would make
  `anyBoundPresent` true whenever the header is live and reap operator/foreign panes even with zero
  strands, breaking the documented "no bound content, foreign panes untouched" invariant
  (reconcile.go:43-45). Instead compute a SEPARATE exemption set used only in the reap-skip test — e.g.
  `exemptPaneIDs` = `boundPaneIDs` plus `st.HeaderPaneID` — and change the kill loop's guard from
  `!boundPaneIDs[p.ID]` to `!exemptPaneIDs[p.ID]`, leaving `anyBoundPresent` computed from real strand
  bindings (`boundPaneIDs`) only. The existing `keptDeadPane` last-pane guard is left in place (made
  moot by the permanent header, but harmless).
- **Commit:** `fix(muxengine): protect the header pane from adoption, split, and reconcile reaping`

### Card 19: Exclude the header from strand accounting

- **Context:**
  - `internal/muxengine/state.go`
  - `internal/muxengine/apply.go`
- **Edits:**
  - `internal/muxengine/lifecycle.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Ensure strand accounting never counts the header. The header is not in
  `st.Strands`, so `UpResult{Strands: len(st.Strands)}` (`lifecycle.go:415`), the `Status` per-strand
  loop (`lifecycle.go:922-925`), and the `noSessionMessage`/`strandCount` logic (`lifecycle.go:842-847`)
  are already correct by construction — verify each and add a short godoc note at each site stating the
  header pane is deliberately excluded (so a future edit does not "fix" the count by adding it). If any
  site derived a count from live panes rather than `len(st.Strands)`, subtract the header pane. `Status`
  must still succeed (session is up) when zero strands but the header is alive.
- **Commit:** `refactor(muxengine): document header exclusion from strand accounting`

### Card 20: Header pane + render tests, keepalive regression

- **Context:**
  - `internal/muxengine/headerpane.go`
  - `internal/muxengine/render/rules.go`
  - `internal/muxengine/render/height.go`
  - `internal/muxengine/spawn.go`
  - `internal/muxcli/smoke_lifecycle_test.go`
  - `internal/muxengine/contract_integration_test.go`
  - `docs/modules/loom.md`
- **Edits:**
  - `internal/muxengine/spawn_test.go`
  - `internal/muxengine/reconcile_test.go`
  - `internal/muxengine/render/rules_test.go`
  - `internal/muxengine/render/height_test.go`
  - `internal/muxengine/contract_integration_test.go`
  - `internal/muxcli/smoke_lifecycle_test.go`
- **Creates:**
  - `internal/muxengine/headerpane_test.go`
  - `docs/modules/mux.md`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Hermetic (untagged, no spawn): `headerpane_test.go` covers `headerLaunchCmd`
  (fake shell + exe → expected command string). `spawn_test.go` covers `planPaneTarget`: with a live
  header pane and no bound strand, the header is neither adopted nor chosen as split-target when a
  non-header pane exists, and IS the split-target fallback when it is the sole pane.
  `reconcile_test.go` asserts `planReconcile` keeps the header pane id (not scheduled for kill) even
  when a strand is bound. `render/rules_test.go` asserts the emitted layout enumerates the header band
  cell at top + one cell per strand with the strand box shrunk by header height; `render/height_test.go`
  covers `clampHeaderHeight` (oversized header clamped to keep the stack floor). Real-tmux
  (`contract_integration_test.go`, `//go:build integration`, skips via `exec.LookPath`): flip
  `TestRemoveStrand_SoleStrandEmptiesSessionSucceeds` so that with a header present, removing the last
  strand leaves the session AND header alive and strand count reaches zero. Smoke
  (`smoke_lifecycle_test.go`, `//go:build smoke`): the header pane survives `up`/`add`/`remove` cycles
  and reconcile with strands bound (mirror `TestSmokeUpWithOnlyForeignPanesKeepsSessionUsable`). Update
  `docs/modules/mux.md` with the header construct: not-a-strand keepalive, top-band render, config
  block, and the exclusion seams.
- **Commit:** `test(mux): cover header pane lifecycle, render band, and keepalive regression`

## Rename mechanic

_No `Moves:` in this batch; section included for completeness only — there are no renames to perform._

## Batch Tests

`verify: go test ./internal/muxengine/... ./internal/muxcli/...` runs all hermetic unit tests (cards
14, 16, 20), the render enumeration tests (card 15/20), and — via the `-tags integration` flag — the
`//go:build integration` `contract_integration_test.go` keepalive regression (skips if tmux is absent;
present on this host). The build-tagged smoke
suite (`go test -tags smoke ./internal/muxcli/...`) exercises the full `up`/`add`/`remove` header
survival and is named here for the reviewer but is NOT in the fast per-round `verify:` (it spawns real
sessions and the `--blocking` pane). The module-wide `go build ./...` overview gate catches any
cross-package compile regression from the `render.Params`/`MuxState` signature changes.
