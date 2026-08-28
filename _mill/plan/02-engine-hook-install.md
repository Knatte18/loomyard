# Batch: engine-hook-install

```yaml
task: "reed: attach's layout computation scales header pane height with terminal height"
batch: "engine-hook-install"
number: 2
cards: 5
verify: go test ./internal/reedengine/...
depends-on: [1]
```

## Batch Scope

This batch is the fix itself: reed computes the fixed-height pins for the box it just laid out against, and installs them as a tmux `window-resized` window hook so the tmux server re-pins those panes after every window-size delta it redistributes.
It is one batch because the five cards are one mechanism — a single mapping seam, a single pure argv builder, its single non-fatal engine wrapper, the two named statements that call it, and the hermetic tests that pin all of the above — and because splitting the two install statements apart would leave a shipped-but-uncalled helper between commits.

It consumes `render.Pin` and `render.FixedHeightPins` from batch 1.
It produces nothing batch 3 consumes as an interface; batch 3 tests and documents the behaviour this batch ships.

Batch-local decision, differing from nothing in the overview: the pure `set-hook` argv builder and its engine wrapper live in `internal/reedengine/windowsize.go` rather than a new file, alongside `pinGeometryOptionsLocked` and the two readbacks, whose `logger.Warn`-and-continue shape they follow exactly.

## Cards

### Card 3: Single mapping seam from state to render inputs, plus the engine pin accessor

- **Context:**
  - `internal/reedengine/state.go`
  - `internal/reedengine/config.go`
  - `internal/reedengine/render/rules.go`
  - `internal/reedengine/render/types.go`
  - `internal/reedengine/apply_test.go`
- **Edits:**
  - `internal/reedengine/apply.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  In `internal/reedengine/apply.go`, add an unexported struct `renderInputs` with fields `strands []render.Strand`, `params render.Params`, and `paneOrder []string`, documented as the single mapping from persisted state plus the live pane set down to the arguments the `render` package takes.

  Add `func (e *Engine) toRenderInputs(st *ReedState, live []LivePane) renderInputs`.
  It performs exactly the mapping `planLayout` performs today, moved rather than duplicated: `presentIDs := liveIDSet(live)`, `toRenderStrands(st.Strands, presentIDs)`, the `headerPaneID := st.HeaderPaneID` blanking when `!presentIDs[headerPaneID]`, the `render.Params{CollapsedStripRows: e.cfg.CollapsedStripRows, MinFullRows: e.cfg.MinFullRows, Header: render.Header{PaneID: headerPaneID, HeightRows: e.cfg.Header.HeightRows}}` assembly, and `paneIDsByTop(live)`.
  It touches no tmux.

  Rewrite `planLayout` to call `toRenderInputs` and then `render.Rules(in.strands, box, in.params, in.paneOrder)`.
  Its signature, its returned values for every input, and its told-box contract are unchanged; every existing test in `apply_test.go` and `attach_test.go` that calls `planLayout` must keep passing with no edit.
  Update `planLayout`'s doc comment to say the mapping now lives in `toRenderInputs` and that the pin path shares it, so the two can never be computed from a different header id than each other.

  Add `func (e *Engine) fixedHeightPins(st *ReedState, live []LivePane, box render.Box) []render.Pin`, which calls `toRenderInputs` and returns `render.FixedHeightPins(in.strands, box, in.params)`.
  Document that it is told its box by the caller exactly as `planLayout` is, queries nothing of its own, and must always be called with the same `st`, `live` and `box` triple the layout for that same call was planned from.
- **Commit:** `refactor(reedengine): route planLayout and the new pin path through one render-input mapping`

### Card 4: Pure set-hook argv builder and its non-fatal engine wrapper

- **Context:**
  - `internal/reedengine/overlay.go`
  - `internal/reedengine/render/rules.go`
  - `internal/reedengine/apply.go`
- **Edits:**
  - `internal/reedengine/windowsize.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  In `internal/reedengine/windowsize.go`, add a pure `func resizePinHookArgvs(session string, pins []render.Pin) [][]string` that returns the full argv sequence rebuilding this session's `window-resized` window-hook array.
  It performs no I/O and no logging.
  The first returned argv is always the clear, emitted even when `pins` is empty: `{"set-hook", "-u", "-w", "-t", exactSessionWindowTarget(session), "window-resized"}`.
  Then one argv per pin, in `pins` order: `{"set-hook", "-w", "-t", exactSessionWindowTarget(session), "window-resized", body}` for the first pin and `{"set-hook", "-a", "-w", "-t", exactSessionWindowTarget(session), "window-resized", body}` for every subsequent pin, where `body` is the single string `fmt.Sprintf("resize-pane -t %s -y %d", pin.PaneID, pin.Height)`.
  The body is one whole argv element; the function must never emit a bare `";"` element, because `set-hook` takes its body as a single argument and a separate `";"` element would terminate the `set-hook` command itself.
  Document that the array encoding (rather than one `";"`-separated command string) exists for failure isolation — verified live on tmux 3.6, a `resize-pane` naming a destroyed pane aborts the rest of a single command list, while array entries are independent — and that the header is always pin index 0 so it fires before any strip pin can go wrong.

  Add `func (e *Engine) installResizePinsLocked(pins []render.Pin)`, which iterates `resizePinHookArgvs(e.SessionName(), pins)` and issues each argv through `e.tmux.run(argv...)`.
  It returns nothing.
  Each failure is logged via `logger.Warn` naming the socket, the session and the error, and then ignored; a failed call never stops the calls after it, so a failed clear still lets the rebuild proceed — the first (non-`-a`) `set-hook` overwrites the array from entry `[0]` regardless.
  Document that this follows the Shared Decision `geometry-tmux-failures-are-non-fatal-everywhere` that already governs `pinGeometryOptionsLocked` in this same file, that the clear is unconditional because reaching a call site means reed has computed an opinion and with zero pins that opinion is "nothing is pinned", and that the whole array is a snapshot rebuilt on every successful apply rather than something recomputed at fire time.
  State the known limitation in the doc comment: a clamp-derived pin is computed for the box at install time, so an operator who shrinks the terminal past a clamp threshold with no intervening reed op keeps a pre-shrink pin, bounded by tmux's own one-row floor and self-correcting on the next reed op.
  Assumes the op lock is already held, like every other `Locked` method in this file.
- **Commit:** `feat(reedengine): add the window-resized resize-pane hook builder and its non-fatal installer`

### Card 5: Install the hook from applyLayoutLocked

- **Context:**
  - `internal/reedengine/windowsize.go`
  - `internal/reedengine/render/rules.go`
  - `internal/reedengine/apply_test.go`
- **Edits:**
  - `internal/reedengine/apply.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  In `applyLayoutLocked`, add exactly one statement, immediately after the `e.tmux.run("select-layout", ...)` call has returned without error and before the `if focus == ""` early return and the `select-pane` call: `e.installResizePinsLocked(e.fixedHeightPins(st, live, box))`.
  Use the same `box` local `planLayout` was already called with on this path, so the pins are computed against the same box the layout was.

  Do not move, reorder, weaken or add to either of this function's two existing guards (`len(live) < 2` and `!anyPlacedStrand(...)`), and do not change the `liveBoxLocked` call's position.
  Both guards, a failed `planLayout`, and a failed `select-layout` must all continue to return before the new statement is reached.

  Extend `applyLayoutLocked`'s doc comment with a short paragraph stating that tmux redistributes every window-size delta evenly across the vertical cells and has no fixed-height pane concept, so no absolute row budget survives a resize on its own; that this function therefore re-installs a `window-resized` hook re-pinning the fixed-height panes after each successful apply; and that a path returning at either guard installs nothing, which is deliberate — reed has applied no layout there, so it has no pins to assert.
- **Commit:** `fix(reedengine): re-pin fixed-height panes via a window-resized hook after each apply`

### Card 6: Install the hook from AttachArgv's pre-flight

- **Context:**
  - `internal/reedengine/windowsize.go`
  - `internal/reedengine/apply.go`
  - `internal/reedengine/render/rules.go`
  - `internal/reedengine/attach_test.go`
- **Edits:**
  - `internal/reedengine/attach.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  In `AttachArgv`'s `withOpLock` closure, hoist the `render.Box` currently constructed inline in the `e.planLayout(st, live, render.Box{X: 0, Y: 0, W: cols, H: rows - reserved})` call into a local variable declared immediately above that call, and pass the local to `planLayout` instead.
  Keep the existing comment explaining why this is the told client box and why `liveBoxLocked` must not be called on this path.

  Then add exactly one statement, immediately after `planLayout` has returned without error and before `chained` is assigned: `e.installResizePinsLocked(e.fixedHeightPins(st, live, box))`, using that same hoisted box local.

  Do not move, reorder, weaken or add to any of the closure's earlier degrade returns — the `cols <= 0 || rows <= 0` return before the lock is taken, `requireSessionLocked`, `readWindowSizeLatestLocked`, `readStatusRowsLocked`, `loadOrInitStateLocked`, `listPanes`, `len(live) < 2`, `!anyPlacedStrand` — and do not change the `reserved` floor clamp.
  Every one of them must continue to precede the new statement.
  `AttachArgv`'s returned argv must be byte-for-byte identical to today's for every input, including when the new statement's `set-hook` calls all fail.

  Extend `AttachArgv`'s doc comment with a sentence stating that the pre-flight also refreshes the session's `window-resized` resize-pin hook, that this is what corrects a later client resize (and, on a session whose earlier apply already installed the hook, a degraded bare attach too), and that a degrade return installs nothing — the uncovered window is a session between `up` and its first placed strand, which has nothing to pin anyway because a lone header pane takes `render.Rules`' sole-cell branch.
- **Commit:** `fix(reedengine): refresh the resize-pin hook in AttachArgv's pre-flight`

### Card 7: Hermetic unit tests for the builder and both install statements

- **Context:**
  - `internal/reedengine/windowsize.go`
  - `internal/reedengine/apply.go`
  - `internal/reedengine/attach.go`
  - `internal/reedengine/overlay.go`
  - `internal/reedengine/lock_test.go`
  - `internal/reedengine/render/rules.go`
  - `internal/reedengine/render/types.go`
  - `internal/reedengine/state.go`
- **Edits:**
  - `internal/reedengine/windowsize_test.go`
  - `internal/reedengine/apply_test.go`
  - `internal/reedengine/attach_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  All new tests are untagged and hermetic: they drive the engine through `TmuxCmd`'s `execHook` seam, spawn no process, and sleep not at all, per the Test Tier Purity Invariant.

  In `internal/reedengine/windowsize_test.go`, add a table-driven test for the pure `resizePinHookArgvs`.
  Cover: zero pins, asserting exactly one argv — the `set-hook -u` clear — and nothing after it, never an empty hook body and never an empty slice; one pin, asserting the clear followed by exactly one `set-hook` argv carrying no `-a`; three pins, asserting the clear, then a non-`-a` `set-hook`, then exactly two `-a` `set-hook` argvs, in the input pin order.
  Assert on every case that each argv carries `-w` and the `exactSessionWindowTarget` form of the session name, that each body element is exactly `resize-pane -t %N -y <n>` for its pin, and that no argv anywhere in the returned sequence contains an element equal to `";"`.

  In `internal/reedengine/apply_test.go`, extend the existing hermetic fixtures to record `set-hook` calls through `execHook` and add tests asserting: a successful apply issues the `set-hook -u` clear and the pin rebuild after the `select-layout` call and before the `select-pane` call, discriminating on the recorded call sequence rather than on call count alone; an apply whose plan yields zero pins — reachable by giving the state a `HeaderPaneID` absent from the live pane set, with no strip strand present, so the mapping blanks the header id — still issues the clear and nothing after it; neither of `applyLayoutLocked`'s two guards reaches any `set-hook` call at all; and a `set-hook` returning an error does not make `applyLayoutLocked` return an error.

  In `internal/reedengine/attach_test.go`, extend `newAttachHook` and `attachRecorder` to record `set-hook` calls and add tests asserting: a known-good pre-flight issues the clear and the pin rebuild after the state and pane list are read and before the argv is returned; every degraded path that yields the bare argv issues no `set-hook` call at all; and a `set-hook` returning an error neither suppresses the chain nor changes a single element of the ten-element chained argv, compared element by element against the same argv built with a non-failing hook.
  While there, correct the doc comment of `TestAttachArgv_NeverMutatesTheSessionOrPersistsState` and, if its assertion is now too broad to be honest, narrow it: `AttachArgv` deliberately does mutate a window option now (the resize-pin hook, alongside the two geometry pins it already set), so the property that test pins is that it issues no pane-set mutation — no `select-layout`, `select-pane`, `kill-pane` or `split-window` — and never writes `reed.json`.
- **Commit:** `test(reedengine): pin the resize-pin hook argv shape and both install statements`

## Batch Tests

`verify: go test ./internal/reedengine/...` runs the untagged tests of both `internal/reedengine` and its `render` leaf.
The whole-package scope is the right one here rather than an over-broad choice: card 3 rewrites `planLayout`, which `apply_test.go`, `attach_test.go` and the attach fixtures all exercise, and cards 5 and 6 add statements to the two functions the majority of this package's hermetic tests drive.
The integration-tagged files in this package are deliberately not built by this command — they need a real tmux and a real pty and are batch 3's scope.
