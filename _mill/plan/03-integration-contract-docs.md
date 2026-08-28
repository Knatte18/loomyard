# Batch: integration-contract-docs

```yaml
task: "reed: attach's layout computation scales header pane height with terminal height"
batch: "integration-contract-docs"
number: 3
cards: 4
verify: go test ./internal/reedengine/... && go test -tags integration ./internal/reedengine/...
depends-on: [2]
```

## Batch Scope

This batch proves the fix against a real tmux and a real pty, pins the two wire behaviours the fix's correctness rests on and that no unit test can reach, and lands the documentation the change obliges.
It is one batch because all four cards are downstream of the shipped mechanism and share the same reading surface — `internal/reedengine`'s two integration-tagged test files and its package doc — and because the Documentation Lifecycle requires the module doc to move with the change rather than trail it.

It consumes the behaviour batch 2 ships and exposes no interface of its own.

Batch-local decision, differing from nothing in the overview: the new `TestMultiplexerContract` case drives `window-resized` with `resize-window` on a detached session pinned to `window-size manual`, rather than with a real client.
Verified live on tmux 3.6 in this worktree: with `window-size manual` set on the window, `resize-window -y 60` fires the `window-resized` hook after tmux has already resized the layout, so a `resize-pane -y` inside the hook survives — the identical property a client resize exercises, reachable with no pty and no attached client, which is what keeps the case at home in a file whose every other case runs on a plain scratch socket.

## Cards

### Card 8: Real-tmux, real-pty integration coverage for the resize path

- **Context:**
  - `internal/reedengine/apply.go`
  - `internal/reedengine/attach.go`
  - `internal/reedengine/windowsize.go`
  - `internal/reedengine/overlay.go`
  - `internal/reedengine/parse.go`
  - `internal/reedengine/config.go`
  - `internal/reedengine/state.go`
  - `internal/reedengine/mouse_boot_integration_test.go`
  - `internal/reedengine/contract_integration_test.go`
  - `internal/reedengine/render/rules.go`
- **Edits:**
  - `internal/reedengine/attachgeometry_integration_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Correct the file-level comment of `internal/reedengine/attachgeometry_integration_test.go` and the doc comment of `TestAttachGeometry_ExactLayoutAndRowBudgets`.
  Record that the existing cases' `100x30` client is SHORTER than the 220x50 boot box, so every case in the file before this task exercised a window SHRINK and never the growth path this task is about, and that the claim that the chained `select-layout` running post-attach is what holds the header at its budget is incomplete — it lands the layout verbatim at attach time, but tmux redistributes every later window-size delta evenly across the vertical cells, so the budget is held afterwards by the `window-resized` resize-pin hook, not by the chain.
  Keep the file's Linux-only rationale and the existing `startInPTY` doc comment unchanged.

  Add a case that resizes the client after a healthy chained attach — the case that fails before this task.
  Reuse `setupAttachGeometryFixture`, `startInPTY`, `waitForClientAttached`, `windowSizeNow` and `windowLayoutNow`; add no second harness.
  Attach at one size, confirm the header pane is at `e.cfg.Header.HeightRows` and the collapsed parent at `e.cfg.CollapsedStripRows`, then drive a `unix.IoctlSetWinsize` `TIOCSWINSZ` on the pty master to a materially TALLER size, poll from outside the pty via `windowSizeNow` until the window reports the new height, and assert both budgets still hold.
  Poll with the existing `waitUntil` helper rather than a fixed sleep.

  Add a case covering the path the originally reported ~50-row threshold came from: `AttachArgv(0, 0)`'s bare argv from a client TALLER than the 50-row boot box, asserting the header pane is at `e.cfg.Header.HeightRows` once the attach settles.
  Its comment must state that this case does not exercise an install — `AttachArgv(0, 0)` returns before the lock is even taken — and that it passes on the hook the fixture's own earlier `applyLayoutLocked` already installed, so a reader cannot misread it as proof that the no-size path installs one.

  Add a case pinning the fire-time failure isolation the array encoding buys: from the healthy fixture, kill one strand's pane with `kill-pane` so its strip pin names a destroyed id, then resize the pty and assert the header pane is still at `e.cfg.Header.HeightRows`.

  Every new case stays under this file's existing `//go:build integration && linux` tag, spawns nothing outside `startInPTY` and the engine's own `TmuxCmd`, and relies on `t.Cleanup` for teardown exactly as the existing cases do.
- **Commit:** `test(reedengine): cover the post-attach resize path against a real tmux and pty`

### Card 9: Multiplexer contract case for set-hook and resize-pane

- **Context:**
  - `internal/reedengine/windowsize.go`
  - `internal/reedengine/overlay.go`
  - `internal/reedengine/parse.go`
  - `internal/reedengine/probe.go`
- **Edits:**
  - `internal/reedengine/contract_integration_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Add one section to the existing `TestMultiplexerContract` function, placed after its `select-pane` step and before its `list-sessions` step, so it runs while both `initialPane` and `secondPaneID` are still alive and long before the `kill-pane`/`kill-session` teardown steps.
  Do not add a new top-level test function; this file's self-skip on an absent configured binary is what keeps the addition free for psmux, and it lives on `TestMultiplexerContract` already.

  The section covers the two wire behaviours the fix rests on and that no unit test can assert.

  First, that `set-hook -u` followed by `set-hook` and one `set-hook -a` yields INDEPENDENT array entries.
  Issue `set-hook -u -w -t <window target> window-resized`, then `set-hook -w -t <window target> window-resized "resize-pane -t %99 -y 1"` with a deliberately non-existent pane id as entry `[0]`, then `set-hook -a -w -t <window target> window-resized "resize-pane -t <initialPane.ID> -y 1"` as entry `[1]`.
  Read the array back with `show-hooks -w -t <window target>` and assert its output carries a `window-resized[0]` line and a `window-resized[1]` line, each naming its own pane id — the readback shape verified live on tmux 3.6, one line per entry.
  Use `exactSessionWindowTarget(session)` for every `-t` here, matching the exact-target discipline every other call site in this package follows.

  Second, that a `window-resized` hook fires AFTER tmux has resized the layout, so a `resize-pane -y` inside it survives the resize that triggered it — and that the dead entry `[0]` above does not take entry `[1]` down with it.
  Pin the window to `set-option -w -t <window target> window-size manual` so a detached session can be resized at all, record `initialPane`'s current height via `listPanes`, issue `resize-window -t <window target> -x 80 -y 60`, then poll with the existing `waitUntil` helper until `listPanes` reports the window's panes summing to the new height, and assert `initialPane` is at exactly 1 row while the other pane absorbed the rest.
  Measured live on tmux 3.6 in this worktree with exactly this sequence: an 80x24 two-pane session at heights 12 and 11 resized to 60 rows leaves the pinned pane at 1 and the other at 58, with entry `[0]`'s dead-pane command failing harmlessly.
  Assert the pinned height, not the sibling's exact value.

  Leave the window as the later steps of `TestMultiplexerContract` expect to find it, using two named mechanisms and no ad-hoc readback-and-restore.
  Clear the hook array with `set-hook -u -w -t <window target> window-resized`.
  Drop the `window-size` override with `set-option -uw -t <window target> window-size` — the `-u` unset form, which removes the window-scoped value and lets the inherited one apply again, rather than reading the prior value back and re-setting it.
  Verified live on tmux 3.6 in this worktree: after `set-option -w window-size manual`, `set-option -uw window-size` exits 0 and `display-message -p '#{window-size}'` reads back `latest`, the inherited default.
  Do not add a capture-then-restore idiom; no test in this package has one.
  Comment the section stating that these two verbs are reed's first deliberately OPTIONAL wire surface — absent from `requiredSubcommands` on purpose — so this case documents their semantics rather than gating on their presence.
- **Commit:** `test(reedengine): pin set-hook array independence and window-resized firing order`

### Card 10: Document the resize round-robin, the hook, and the optional wire surface

- **Context:**
  - `internal/reedengine/apply.go`
  - `internal/reedengine/attach.go`
  - `internal/reedengine/windowsize.go`
  - `internal/reedengine/probe.go`
  - `internal/reedengine/state.go`
  - `internal/reedengine/render/rules.go`
  - `internal/reedengine/render/height.go`
  - `internal/reedengine/template_posix.yaml`
- **Edits:**
  - `internal/reedengine/doc.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Three edits to `internal/reedengine/doc.go`, all additions except where stated.
  Do not rewrite the existing "Silent layout rescale" or "The chained attach" bullets: both are accurate as written and document a different mechanism (a mismatched layout STRING) from the one this task fixes (a window RESIZE).

  First, add a new bullet to the load-bearing-behavioral-assumptions list, adjacent to the "Silent layout rescale" bullet.
  It states that tmux has no fixed-height pane concept and hands out a window-size delta one row at a time, round-robin across the vertical cells, so no absolute row budget reed computes survives a resize on its own — measured live on tmux 3.6, a healthy attached session's header went from 1 row to 6 across a 76-to-90-row client resize and to 16 across a further 90-to-120 one.
  It names the answer: a `window-resized` window hook holding one `resize-pane -y` array entry per fixed-height pane, installed by reed and executed by the tmux server, refreshed on every successful apply and in `AttachArgv`'s pre-flight, with the pinned heights coming from `render.FixedHeightPins` — the heights `render` actually placed the cells at, after `clampHeaderHeight` and `clampToFit` — never the raw configured budgets.
  It records that `client-resized` was measured firing BEFORE the layout is resized and so cannot work, and that `window-layout-changed` also fires on reed's own `select-layout`, inviting re-entrancy for no benefit.
  It also records what happens on the paths that install nothing: an apply returning at either of `applyLayoutLocked`'s guards, and every `AttachArgv` degrade return, issue no `set-hook` at all — not even the clear — so a previously installed array survives them deliberately, since a clear with no rebuild behind it would drift on the very next resize.
  Name why that is safe in both guard cases: `resize-pane -y` against a window's sole pane is a verified silent no-op (exit 0, height unchanged), so the `len(live) < 2` case's surviving header pin cannot contradict `render.Rules`' sole-cell branch;
  and in the `!anyPlacedStrand` case — reachable for good via the operator remedy `internal/reedengine/state.go` documents, which deletes `reed.json` while the session keeps running untracked — the surviving array is a benefit, still holding the live header and strips at the budgets reed last computed for them.
  In the same bullet, record that the ~50-row threshold in the original bug report is `template_posix.yaml`'s `height: 50` boot box showing through the BARE (unchained) attach path, not evidence of a miscomputed layout — a synthetic bare attach reproduces the reported table exactly, with 40 and 50 rows leaving the header at 1 and 76 rows taking it to 10 — so a future reader does not re-derive that from scratch.

  Second, add one sentence to the existing "The chained attach" bullet noting that "the layout string lands verbatim with no rescale" holds only until the next window resize, and pointing at the resize-hook bullet above for what holds the budgets afterwards.

  Third, extend the "Subcommand set" paragraph with the required-versus-optional split: the verbs listed there are required, and a binary missing any of them is unusable, while `set-hook` and `resize-pane` are reed's first deliberately OPTIONAL verbs — absent from `requiredSubcommands` on purpose, because gating the capability probe on them would take every reed verb down on a psmux lacking `set-hook`, over a quality-only option already designed to degrade silently, so their absence costs only the resize pin.
  Then REWRITE — not extend — the file's closing sentence, the one currently claiming `requiredSubcommands` "did not grow for any of this … and no new psmux risk".
  This task falsifies its "no new psmux risk" half: both verbs are new to `internal/`, so the wire contract genuinely widens.
  The replacement must say that the probe still does not grow and state that this is now a deliberate trade rather than a free consequence, naming the non-fatal degrade as what pays for it.

  Every edited or added sentence follows the repo's semantic-line-break convention: one sentence per line, no fixed-column hard wrap, plain newlines.
- **Commit:** `docs(reedengine): document the resize round-robin, the resize-pin hook, and the optional wire surface`

### Card 11: Narrow the roadmap's watchdog-daemon resize clause

- **Context:**
  - `internal/reedengine/doc.go`
  - `internal/reedengine/apply.go`
  - `CLAUDE.md`
- **Edits:**
  - `manifest/roadmap.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Edit the **reed: watchdog daemon** item in `manifest/roadmap.md`, narrowing only its resize clause.
  The item stays on the roadmap and its pane-reap half is untouched, including that half's own event-driven-hooks preference and its cheaper-reap-probe prerequisite.

  The clause currently records that an earlier task's discussion considered and REJECTED a one-off tmux `set-hook`+`run-shell` reaction, and claims the resize job as the daemon's.
  Replace that half with: a `window-resized` + `resize-pane` hook reaction is now SHIPPED and holds every fixed-height pane (the header band and each collapsed strip) at its budget across a live resize; what remains for the daemon is re-rendering reed's FULL layout policy after a resize — the equal strand split with the remainder to the active pane, which tmux's round-robin redistribution leaves slightly uneven — since that needs an actor outside tmux, which a hook with no `run-shell` deliberately is not.
  Preserve the surviving daemon rationale: the pane-reap job still needs its own hook class, its intentional-versus-bug-induced policy, and its named prerequisite, so a shared daemon still pays for that infrastructure once rather than per job.

  Do not move any other roadmap line, do not add a Done entry, and do not touch `docs/overview.md` — no module-table or execution-stack change occurs in this task.
  Write the rewritten clause with semantic line breaks, one sentence per line, and leave the parts of the item this card does not rewrite formatted as they are.
- **Commit:** `docs(roadmap): narrow the watchdog daemon's resize clause to the full-policy re-render`

## Batch Tests

`verify:` runs two commands.
`go test ./internal/reedengine/...` re-runs the untagged suite, which must stay green while the two integration-tagged files change — it is the cheap guard that a test-file edit did not break the package's build.
`go test -tags integration ./internal/reedengine/...` is the batch's real gate: it builds and runs `attachgeometry_integration_test.go` (card 8's three new cases plus the three existing ones) and `contract_integration_test.go` (card 9's new `TestMultiplexerContract` section plus every existing case), against the real configured multiplexer binary and a real pty.
The scope is the `internal/reedengine` package and its `render` leaf only, not the repo-wide suite; `pipeline.done_gate` already runs `go test ./... && go test -tags integration ./...` at task completion and covers anything outside it.
Cards 10 and 11 have no runnable surface of their own — they are documentation — and are covered by the same commands only insofar as `doc.go` must keep compiling.
