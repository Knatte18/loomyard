# Batch: remove-empties-session

```yaml
task: "lyx mux remove errors when it empties the last session"
batch: "remove-empties-session"
number: 1
cards: 5
verify: go test -tags "integration smoke" ./internal/muxengine/ ./internal/muxcli/
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

### Card 3: Correct the last-pane assumption per-binary in comment, doc.go, and the smoke test

- **Context:**
  - `internal/muxengine/overlay.go`
- **Edits:**
  - `internal/muxengine/strand.go`
  - `internal/muxengine/doc.go`
  - `internal/muxcli/smoke_lifecycle_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** The remain-on-exit-corpses-the-last-pane claim is
  **binary-dependent**, not universally false (see the overview Shared Decision
  "the last-pane assumption is BINARY-DEPENDENT"). Reconcile all three in-tree
  encodings so none contradicts another:
  (a) In `strand.go`, rewrite the `kill-pane` loop's comment (the block
  currently asserting "killing a session's LAST pane does not remove it — under
  remain-on-exit psmux corpses it as pane_dead=1 ... keeping the session alive")
  so it no longer states that claim *universally*. State it per backend: on
  **tmux** killing a session's true last pane **destroys the session** (and, if
  it was the server's only session, the server exits); on **psmux** it corpses
  the pane and the session survives. Then state that `RemoveStrand` handles both
  via the `hasSession` re-probe — swallowing the resulting `applyErr` as an
  expected success only when the session is confirmed gone (the tmux case).
  (b) In `doc.go`, under "Load-bearing behavioral assumptions", refine the
  existing "Dead-pane adoption via remain-on-exit" bullet (which currently
  implies the corpse behavior holds universally — scope it to the non-last pane
  and/or psmux) and add the corrected last-pane assumption as a **per-binary**
  statement: tmux destroys the last-pane session; psmux corpses it (verified by
  `internal/muxcli/smoke_lifecycle_test.go`'s
  `TestSmokeRemoveLastStrandThenAddRunsTheNewCommand`). State the exit-code
  dependency explicitly: `has-session`/`list-panes` exit **1** for "no server
  running" (the same exit-1 the reproduction showed from `listPanes`), which
  `hasSession` (`overlay.go`) maps to `(false, nil)` — this is what lets
  `RemoveStrand`'s re-probe classify the emptied session on tmux.
  Cross-reference the existing "Async kill-server / probe-always-exits-0" bullet
  and state the contrast: `list-sessions`/`kill-server` exit **0** regardless of
  server state (cannot distinguish "no server" from "server dying
  asynchronously"), whereas `has-session`/`list-panes` reliably surface exit 1
  for "no server" — which is *why* the fix re-probes with `hasSession` and not
  `list-sessions`. Do NOT call psmux last-pane behavior "unverified" — the smoke
  test verifies it.
  (c) In `internal/muxcli/smoke_lifecycle_test.go`, add a brief caveat to
  `TestSmokeRemoveLastStrandThenAddRunsTheNewCommand`'s doc comment noting that
  its corpse-pane premise is **psmux-specific** and that tmux behaves oppositely
  (destroys the last-pane session), pointing at `RemoveStrand`'s
  emptied-session swallow. Do NOT change the test's assertions or build tag —
  they correctly pin psmux behavior; only the comment is reconciled.
- **Commit:** `docs(muxengine): document last-pane behavior as binary-dependent`

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
  Coverage note (state this in the test's doc comment): this regression
  exercises the Card 2 swallow branch **only on last-pane-destroy backends
  (tmux, the POSIX default per `template_posix.go`)** — there, without the fix,
  `RemoveStrand` would return the `no server running` error, so a passing test
  genuinely covers the swallow. On corpse backends (psmux/Windows) the same
  scenario passes via `RemoveStrand`'s normal path (the session survives), so
  the swallow is not reached; the pure-helper unit test (Card 4) covers the
  decision logic on every platform. Do not assert on which internal branch was
  taken — assert only the observable contract (nil error, zero persisted
  strands).
- **Commit:** `test(muxengine): integration regression for removing the last strand`

## Batch Tests

`verify: go test -tags "integration smoke" ./internal/muxengine/ ./internal/muxcli/`
compiles and runs both affected packages with **both** build tags. This scope is
required, not over-broad, because the batch touches files behind two different
build tags in two packages:
- `./internal/muxengine/` with `-tags integration` covers the hermetic helper
  test (Card 4, normal build) and the end-to-end regression (Card 5, behind
  `//go:build integration`).
- `./internal/muxcli/` with `-tags smoke` compiles the smoke test whose comment
  Card 3 edits (`smoke_lifecycle_test.go`, behind `//go:build smoke`) — without
  the `smoke` tag and the `muxcli` package on the command line, a break in that
  edit would never be caught (the original single-package verify could not see
  it — this closed a review BLOCKING). It is also what surfaces any contradiction
  the fix would otherwise introduce in the muxcli encoding of the assumption.

Both build-tagged end-to-end tests self-skip when their binary is absent: the
muxengine integration test skips without `cfg.Psmux` (on this POSIX box that is
tmux, so it runs and exercises the swallow); the muxcli smoke test skips without
psmux (absent here, so it compiles but skips). So the command is safe to run
repeatedly during mill-go's per-round verify. Go's test unit is the package, so
single-file scoping is not available without brittle `-run` filters; the two
named packages are the minimal correctly-scoped set (not a repo-wide run).

Key assertions: Card 4 pins the four-way classification of
`removalEmptiedSession` on every platform; Card 5 pins that `RemoveStrand` of the
sole non-hidden strand returns success and leaves a zero-strand `mux.json` (and,
on tmux, genuinely covers the swallow branch — see Card 5's coverage note).
