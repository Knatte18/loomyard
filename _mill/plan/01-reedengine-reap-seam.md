# Batch: reedengine reap seam

```yaml
task: 'reed: per-hub daemon reaps orphaned sessions'
batch: 'reedengine reap seam'
number: 1
cards: 3
verify: go test ./internal/reedengine/
depends-on: []
```

## Batch Scope

This batch delivers the whole `internal/reedengine` side of the task and nothing else: the `Engine.ShellPath()` accessor, the new engine-less exported `ReapSession` plus the two tmux-only halves it is split around, and the untagged tests that pin the halves and the call ordering through `TmuxCmd`'s `execHook` seam.
It is one batch because all three cards are the same small, self-contained addition to one package, and because the exported surface they produce — `ReapSession` and `ShellPath` — is exactly the external interface batch 3 consumes.
It has no dependency on the daemon side and can run in parallel with batch 2.

Batch-local decision beyond `## Shared Decisions`: the new symbols go in `internal/reedengine/overlay.go` beside `ListSessions`, which is the precedent for an engine-less exported function in this package, rather than in `lifecycle.go` where the process-reap helpers live — the split point is about who may call it, not about which helpers it reaches.

## Cards

### Card 1: `Engine.ShellPath()` accessor

- **Context:**
  - `internal/reedengine/config.go`
- **Edits:**
  - `internal/reedengine/lock.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add an exported method `func (e *Engine) ShellPath() string` returning `e.cfg.Shell`, placed immediately after the existing `func (e *Engine) TmuxPath() string` in `internal/reedengine/lock.go` and following its doc-comment shape. `Config`'s field is `Shell string` with yaml tag `shell`. The accessor validates nothing and defaults nothing — an empty `cfg.Shell` is returned as the empty string, per the discussion's `the-daemon-is-told-its-shell` decision that the shell is never validated on this path.
- **Commit:** `feat(reedengine): add Engine.ShellPath accessor`

### Card 2: `ReapSession` and its two tmux-only halves

- **Context:**
  - `internal/reedengine/lock.go`
  - `internal/reedengine/geometry.go`
  - `internal/reedengine/config.go`
  - `internal/reedengine/parse.go`
  - `internal/reedengine/strand.go`
  - `internal/reedengine/lifecycle.go`
  - `internal/reedengine/proctree_linux.go`
  - `internal/reedengine/proctree_windows.go`
- **Edits:**
  - `internal/reedengine/overlay.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add three functions to `internal/reedengine/overlay.go`, placed after the existing `listSessionsVia`.

  `func reapSessionPanes(cmd TmuxCmd, session string) ([]LivePane, error)` returns `cmd.listPanes(session)` and does nothing else — it exists solely so the pane-listing half is drivable through `TmuxCmd`'s `execHook` seam, mirroring `listSessionsVia`'s own reason for existing.

  `func reapSessionKill(cmd TmuxCmd, session string) error` returns `cmd.run("kill-session", "-t", exactSessionTarget(session))`. The `exactSessionTarget` wrapper is mandatory: a bare `-t session` prefix-matches a sibling worktree's session.

  `func ReapSession(tmuxPath, shellPath, socketKey, sessionName string) error` is the one new exported symbol. It runs four steps in this exact order, and the order is load-bearing:

  1. `cmd := NewTmuxCmd(tmuxPath, socketKey)`, then `live, err := reapSessionPanes(cmd, sessionName)`. A non-nil `err` is logged at `Warn` and the reap continues with a nil `live` — a session whose panes cannot be listed is the one most worth killing, so a listing failure never skips the kill.
  2. Build a throwaway engine value — `eng := &Engine{cfg: Config{Tmux: tmuxPath, Shell: shellPath}, geom: Geometry{SocketKey: socketKey, SessionName: sessionName}, tmux: cmd}` — and compute `pids := eng.descendantClosurePIDs(sessionReapRoots(live))`. This runs **before** the kill, while the panes are still alive: `descendantClosurePIDs` derives the closure from live parent links, so a closure computed after `kill-session` reparents the pane children collapses to the root pids alone and silently loses the detached agent descendants the reap exists to kill. `paneProcessTreePIDsLocked` carries this same rule in its own doc comment. The throwaway engine is built by struct literal rather than through `New`, because `New` would rebuild `tmux` from `cfg.Tmux`/`geom.SocketKey` — identical here, but the literal makes it explicit that the value exists only to reach `descendantClosurePIDs`, which is a method only because Windows reads `e.cfg.Shell`.
  3. `killErr := reapSessionKill(cmd, sessionName)`.
  4. `reapPaneChildren(pids, reapExitTimeout)` against the closure captured in step 2.

  `ReapSession` acquires no lock — it never calls `withOpLock` or `withTryOpLock`, so it never reaches the told-geometry validators, which is what makes it legal for a worktree that no longer exists. It touches no state directory and reads no geometry field beyond `SocketKey` and `SessionName`.

  Return value: `ReapSession` returns `killErr`, the `kill-session` error, unwrapped. The pane-listing error from step 1 is not propagated.

  Logging, per CONSTRAINTS.md's Live-Substrate Spawn Observability rule and the overview's `reap-logging-names-the-socket-not-the-hub` decision: one `logger.Info` line immediately before step 3 naming `socket` (the `socketKey` parameter), `session` and the count of `pids` captured in step 2, and one `logger.Info` line after step 4 reporting the outcome with the same `socket`/`session` keys plus the kill error. `reapPaneChildren` itself is left unchanged — it is a shared helper `Engine.Down` also calls, and its existing `Warn` lines for a straggler and a survived force-kill already cover the per-pid failures.

  Give `ReapSession` a doc comment stating that it is the second engine-less exported function in this package, why the closure must precede the kill, and that it holds no lock and persists nothing.
- **Commit:** `feat(reedengine): add engine-less ReapSession for orphaned hub sessions`

### Card 3: untagged tests for the reap seam

- **Context:**
  - `internal/reedengine/overlay.go`
  - `internal/reedengine/strand.go`
  - `internal/reedengine/parse.go`
- **Edits:**
  - `internal/reedengine/overlay_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Extend `internal/reedengine/overlay_test.go` with tests for the new seam, driving every one of them through `TmuxCmd`'s `execHook` field with no live server, exactly as the existing `TestListSessions` does. Update the file's header comment so it describes both what it pinned before and the reap seam it now also pins.

  Cover:

  - `reapSessionKill` issues `kill-session` with an exact-match target: the recorded `execHook` args are `kill-session`, `-t`, `=<name>` — a bare `<name>` third arg fails the test. Assert `capture` is false (it routes through `run`, not `output`).
  - `reapSessionPanes` returns the parsed pane list on a scripted successful listing, and returns the error on a scripted failure.
  - `ReapSession`'s call ordering, at the level decidable without real processes: with an `execHook` recording every call's first arg in sequence, the recorded sequence is `list-panes` **then** `kill-session`.
  - A `list-panes` failure still produces a `kill-session` call — the listing error does not abort the reap.

  Assert that `ReapSession` feeds `sessionReapRoots` the pane list `reapSessionPanes` returned; do not re-test `sessionReapRoots`' own `safeReapRoot` filter, which `internal/reedengine/strand.go`'s existing coverage already owns.

  Nothing here covers `descendantClosurePIDs` or `reapPaneChildren`: both read the live process table and kill real pids, which CONSTRAINTS.md's Test Tier Purity Invariant keeps out of an untagged file. Those are batch 4's tagged assertions. Drive the tests so the pane pids scripted into the `list-panes` response are ones whose closure walk finds nothing to wait on, keeping the untagged run fast and free of real-process dependence.
- **Commit:** `test(reedengine): pin ReapSession's tmux halves and call ordering`

## Batch Tests

`verify: go test ./internal/reedengine/` runs the whole untagged `internal/reedengine` package suite, which is the package this batch's three cards all edit.
The new assertions land in `internal/reedengine/overlay_test.go`;
the run also re-executes the package's existing untagged files (`strand_test.go`, `lifecycle_test.go`, `lock_test.go` and the rest), which is deliberate — card 1 and card 2 both add to files those tests already exercise, and a package-scoped verify is what catches a regression in them.
The scope is one Go package, not the repo, per the overview's `go-native-verify-commands` decision.
