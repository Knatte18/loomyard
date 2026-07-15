# Batch: remove-empties-session

```yaml
task: "lyx mux remove errors when it empties the last session"
batch: "remove-empties-session"
number: 1
cards: 5
verify: go test -tags integration ./internal/muxengine/
depends-on: []
```

## Batch Scope

This batch delivers the entire fix in one Sonnet-sized unit: it is confined to
the `internal/muxengine` package and the change is small and cohesive. It adds a
pure decision helper, wires it into `RemoveStrand`'s post-reconcile error branch
so an emptied-session outcome resolves to success (persisting the emptied
state), corrects the false load-bearing comment and the `doc.go` assumptions
list, and adds both a hermetic unit test for the helper and an
`//go:build integration` end-to-end regression. There is no external interface a
later batch consumes — the helper is unexported and package-local. All batch
decisions follow `## Shared Decisions` in the overview; no batch-local
deviations.

## Cards

### Card 1: Add pure `removalEmptiedSession` decision helper

- **Context:**
  - `internal/muxengine/state.go`
  - `internal/muxengine/apply.go`
  - `internal/muxengine/render/types.go`
- **Edits:**
  - `internal/muxengine/strand.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add an unexported pure helper
  `func removalEmptiedSession(remaining []Strand, sessionGone bool) bool` to
  `strand.go`. It returns `true` iff `sessionGone` is `true` AND no strand in
  `remaining` is non-hidden — i.e. every strand in `remaining` satisfies
  `s.Display.Anchor == render.AnchorHidden` (an empty `remaining` slice
  therefore returns `true` when `sessionGone`). Mirror the existing
  `anyPlacedStrand` filter in `apply.go` (`s.Display.Anchor != render.AnchorHidden`)
  so the two share one notion of "expected to own a live pane". The helper takes
  no engine receiver and makes no psmux call — it is fully hermetic. Add a godoc
  comment stating it classifies whether a remove drained the session of every
  strand that should still own a live pane, so a confirmed-gone session is an
  expected terminal state rather than a failure.
- **Commit:** `feat(muxengine): add removalEmptiedSession decision helper`

### Card 2: Swallow the emptied-session error in RemoveStrand

- **Context:**
  - `internal/muxengine/spawn.go`
  - `internal/muxengine/lifecycle.go`
  - `internal/muxengine/overlay.go`
  - `internal/muxengine/state.go`
- **Edits:**
  - `internal/muxengine/strand.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `RemoveStrand` (`strand.go`), replace the tail
  `if applyErr != nil { return applyErr }` (the block immediately after the
  `reapPaneChildren(reapPIDs, reapExitTimeout)` call) so that when
  `applyErr != nil`: re-probe the session with
  `up, herr := e.psmux.hasSession(e.SessionName())`, compute
  `sessionGone := herr == nil && !up`, and if
  `removalEmptiedSession(st.Strands, sessionGone)` is `true`, persist the pruned
  state via `SaveState(e.layout.DotLyxDir(), st)` — returning a wrapped error if
  `SaveState` fails (e.g. `fmt.Errorf("save state after emptying session: %w", err)`)
  — then set `result = removed` and `return nil`. Otherwise `return applyErr`
  unchanged. Do NOT move or remove the existing `reapPaneChildren` call; it must
  still run before this branch exactly as today (the removed pane's subtree is
  dying asynchronously either way). Key the swallow off `hasSession` +
  `removalEmptiedSession`, never off the `applyErr` string.
- **Commit:** `fix(muxengine): treat emptied last session as success in RemoveStrand`

### Card 3: Correct the kill-pane comment and doc.go assumptions list

- **Context:** none
- **Edits:**
  - `internal/muxengine/strand.go`
  - `internal/muxengine/doc.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** (a) In `strand.go`, rewrite the `kill-pane` loop's comment
  (the block currently asserting "killing a session's LAST pane does not remove
  it — under remain-on-exit psmux corpses it as pane_dead=1 ... keeping the
  session alive") so it no longer states that false claim; instead state the
  corrected tmux behavior (killing a session's true last pane destroys the
  session) and that `RemoveStrand` now treats an emptied session as an expected
  success via the `hasSession` re-probe. (b) In `doc.go`, under "Load-bearing
  behavioral assumptions", refine the existing "Dead-pane adoption via
  remain-on-exit" bullet (which currently implies the corpse behavior holds
  universally) and add the corrected assumption: on tmux, killing a session's
  *true last* pane destroys the session, and if it was the server's only session
  the server then exits. State the exit-code dependency explicitly:
  `has-session`/`list-panes` exit **1** for "no server running" (the same exit-1
  the reproduction showed from `listPanes`), which `hasSession` (`overlay.go`)
  maps to `(false, nil)` — this is what lets `RemoveStrand`'s re-probe classify
  the emptied session. Cross-reference the existing "Async kill-server /
  probe-always-exits-0" bullet and state the contrast: `list-sessions`/
  `kill-server` exit **0** regardless of server state (cannot distinguish "no
  server" from "server dying asynchronously"), whereas `has-session`/`list-panes`
  reliably surface exit 1 for "no server" — which is *why* the fix re-probes with
  `hasSession` and not `list-sessions`. Note the Windows/psmux last-pane behavior
  as unverified.
- **Commit:** `docs(muxengine): correct last-pane remain-on-exit assumption`

### Card 4: Hermetic unit test for removalEmptiedSession

- **Context:**
  - `internal/muxengine/strand.go`
  - `internal/muxengine/state.go`
  - `internal/muxengine/render/types.go`
- **Edits:**
  - `internal/muxengine/strand_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add a table-driven `go test` for `removalEmptiedSession`
  covering all four cases: (1) `sessionGone=true` + empty `remaining` → `true`;
  (2) `sessionGone=true` + `remaining` all `render.AnchorHidden` → `true`;
  (3) `sessionGone=true` + at least one remaining strand with a non-hidden
  anchor (e.g. `render.AnchorTop`) → `false`; (4) `sessionGone=false` (any
  `remaining`) → `false`. Construct `Strand` values inline with only the
  `Display.Anchor` field set as needed — the helper reads nothing else. Place
  the test alongside the existing `removeStrandLocked` tests in
  `strand_test.go`.
- **Commit:** `test(muxengine): cover removalEmptiedSession classification`

### Card 5: Integration regression for emptying the last session

- **Context:**
  - `internal/muxengine/strand.go`
  - `internal/muxengine/lifecycle.go`
  - `internal/muxengine/state.go`
  - `internal/muxengine/lock_test.go`
  - `internal/muxengine/overlay.go`
- **Edits:**
  - `internal/muxengine/contract_integration_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add an `//go:build integration` test function to
  `contract_integration_test.go` that reproduces the bug end-to-end against a
  real Engine. It must: build a `Config` via `LoadConfig(tmpDir, "mux")` (seed
  with `seedMuxConfig`, as `TestMultiplexerContract` does) and self-skip with
  `t.Skipf` when `exec.LookPath(cfg.Psmux)` fails (same self-skip guard as
  `TestMultiplexerContract`, so the suite stays green where the binary is
  absent); construct a real `*hubgeometry.Layout` rooted at a `t.TempDir()`
  (mirror `newTestEngine`'s `Layout{Cwd, WorktreeRoot, Hub}` shape in
  `lock_test.go`, but with the real `cfg`) and build the engine via
  `New(cfg, layout)`; register a `t.Cleanup` that best-effort tears the server
  down (via the engine's `Down`, or a raw `kill-server` on the engine socket, so
  no scratch server leaks). Then drive: `e.Up()`; `e.AddStrand(AddSpec{...})`
  with exactly one non-hidden strand (a `Cmd` such as a long-lived sleep/shell,
  `Display` anchored non-hidden, e.g. `render.AnchorTop`), capturing the
  returned `Strand.GUID`; then `e.RemoveStrand(guid, false)`. Assert
  `RemoveStrand` returns a nil error and its `Removed` names the removed guid.
  Then assert persistence: reload state via `LoadState(layout.DotLyxDir())` and
  confirm the persisted `MuxState` has zero strands (guards the
  resurrect-on-resume regression). Because pane/session teardown is async, poll
  where needed using the existing `waitUntil` helper rather than asserting once.
- **Commit:** `test(muxengine): integration regression for removing the last strand`

## Batch Tests

`verify: go test -tags integration ./internal/muxengine/` compiles and runs the
whole `muxengine` package test suite **including** the `//go:build integration`
files. This scope is required (not an over-broad choice): the fix spans a
hermetic helper (Card 1/4, run by the normal build) and the end-to-end
regression (Card 5, gated behind the `integration` build tag), so only a
tags-`integration` package run exercises both in one command. Go's test unit is
the package, so a single-file scope is not available without brittle `-run`
filters; running the one affected package is the idiomatic, correctly-scoped
choice and stays well within the muxengine package (not a repo-wide run). The
integration test self-skips when the configured multiplexer binary is absent, so
this command is safe to run repeatedly during mill-go's per-round verify even on
a box without tmux. Key assertions: Card 4 pins the four-way classification of
`removalEmptiedSession`; Card 5 pins that `RemoveStrand` of the sole non-hidden
strand returns success and leaves a zero-strand `mux.json`.
