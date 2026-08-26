# Batch: engine-live-geometry

```yaml
task: 'reed: attach doesn''t reconcile session geometry with the terminal'
batch: 'engine-live-geometry'
number: 1
cards: 5
verify: go test ./internal/reedengine/...
depends-on: []
```

## Batch Scope

This batch replaces the config-pinned render box with a told box, and adds the whole geometry-options vocabulary the attach path (batch 2) will consume.
It delivers three things: `planLayout` gains an explicit `render.Box` parameter and stops constructing one from `e.cfg.Width`/`e.cfg.Height`;
a new `windowsize.go` file owns the live-window query, its fallback, the two option pins, and the two `display-message` readbacks with their pure decision halves;
and the boot path pins `status off` and `window-size latest` non-fatally beside the existing `remain-on-exit`/`mouse` pins.

The external interface batch 2 consumes is exactly the exported-within-package surface of `windowsize.go` (`liveBoxLocked`, `pinGeometryOptionsLocked`, `readStatusRowsLocked`, `readWindowSizeLatestLocked`) plus the new `planLayout(st, live, box)` signature.
Nothing in this batch touches the attach argv, either CLI, or `go.mod`.

Batch-local decision, differing from nothing in `## Shared Decisions` but worth stating: the new tmux queries go through `TmuxCmd.output`/`TmuxCmd.run` (which auto-prefix `-L <socket>`), never a fresh `exec.Command` — that is what makes them stubbable through `execHook` at tier 1 and what keeps the socket discipline intact.

## Cards

### Card 1: windowsize.go — live-window query, geometry-option pins, and readbacks

- **Context:**
  - `internal/reedengine/overlay.go`
  - `internal/reedengine/parse.go`
  - `internal/reedengine/mouse.go`
  - `internal/reedengine/lock.go`
  - `internal/reedengine/render/types.go`
- **Edits:** none
- **Creates:**
  - `internal/reedengine/windowsize.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Create `internal/reedengine/windowsize.go` in `package reedengine`, carrying a file-header comment in the style of `mouse.go`/`parse.go` that states what the file owns: the live-window-size query and its fallback, the two geometry option pins, and the two effective-value readbacks the attach path gates on.
  Declare exactly these seven identifiers, all unexported except as noted:
  1. `func parseWindowSize(out string) (w, h int, ok bool)` — a pure parser for `display-message -p '#{window_width} #{window_height}'` output.
     Trim the string, split on `strings.Fields`, and require exactly two fields, both parsing as integers via `strconv.Atoi`, both strictly positive;
     any other shape (empty string, one field, three fields, non-numeric, zero or negative) returns `ok == false`.
     No I/O.
  2. `func (e *Engine) liveBoxLocked() render.Box` — runs `e.tmux.output("display-message", "-p", "-t", exactSessionWindowTarget(e.SessionName()), "#{window_width} #{window_height}")`, feeds the result to `parseWindowSize`, and returns `render.Box{X: 0, Y: 0, W: w, H: h}` on success.
     On an error return, or on `ok == false`, log via `logger.Warn` naming the socket and session and return `render.Box{X: 0, Y: 0, W: e.cfg.Width, H: e.cfg.Height}` — the configured box, exactly today's value.
     The `=<name>:` window-target form is required: a bare `=<name>` is parsed as a pane target and fails with `can't find pane`, so use `exactSessionWindowTarget`, never `exactSessionTarget`.
     Assumes the op lock is already held, per the `Locked` suffix convention this package uses.
  3. `func reservedRowsFromStatus(raw string) (rows int, ok bool)` — a pure mapper from a `#{status}` readback to the number of window rows the tmux status line consumes.
     Trim and lowercase: `"off"` yields `(0, true)`, `"on"` yields `(1, true)`, a string parsing as a non-negative integer via `strconv.Atoi` yields `(that integer, true)`, and anything else — including the empty string and a negative integer — yields `(0, false)`.
  4. `func windowSizeAllowsChain(raw string) bool` — a pure predicate reporting whether a `#{window-size}` readback permits the attach chain.
     Trim and lowercase;
     return true only for the exact value `"latest"`.
     Every other value, including `"manual"`, `"largest"`, `"smallest"` and the empty string, returns false.
  5. `func (e *Engine) pinGeometryOptionsLocked()` — returns nothing.
     Issues `e.tmux.run("set-option", "-t", exactSessionWindowTarget(e.SessionName()), "status", "off")` and `e.tmux.run("set-option", "-w", "-t", exactSessionWindowTarget(e.SessionName()), "window-size", "latest")`.
     Each call's error is logged via `logger.Warn` naming the socket, session and the option, and then ignored;
     the second pin is attempted even when the first failed.
     Both pins are session/window-targeted rather than `-g`, because a session- or window-scoped value set from the operator's `~/.tmux.conf` silently wins over a global set while `set-option` still exits 0 — verified live.
  6. `func (e *Engine) readStatusRowsLocked() (rows int, ok bool)` — runs `e.tmux.output("display-message", "-p", "-t", exactSessionWindowTarget(e.SessionName()), "#{status}")` and returns `reservedRowsFromStatus`'s answer.
     An error from the round trip returns `(0, false)` and logs via `logger.Warn`.
  7. `func (e *Engine) readWindowSizeLatestLocked() bool` — runs `e.tmux.output("display-message", "-p", "-t", exactSessionWindowTarget(e.SessionName()), "#{window-size}")` and returns `windowSizeAllowsChain`'s answer.
     An error from the round trip returns `false` and logs via `logger.Warn`.

  Every one of these tmux interactions is non-fatal, per the Shared Decision geometry-tmux-failures-are-non-fatal-everywhere: none of the four methods returns an `error`, so no caller can be tempted to abort on one.
  Do not add any subcommand to `requiredSubcommands`.
- **Commit:** `feat(reedengine): add the live-window query, geometry option pins, and their readbacks`

### Card 2: planLayout takes an explicit box; applyLayoutLocked resolves the live one

- **Context:**
  - `internal/reedengine/render/rules.go`
  - `internal/reedengine/render/types.go`
  - `internal/reedengine/windowsize.go`
  - `internal/reedengine/apply_test.go`
  - `internal/reedengine/spawn.go`
- **Edits:**
  - `internal/reedengine/apply.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Change `planLayout`'s signature from `func (e *Engine) planLayout(st *ReedState, live []LivePane) (layout, focus string, err error)` to `func (e *Engine) planLayout(st *ReedState, live []LivePane, box render.Box) (layout, focus string, err error)`, and pass `box` straight through to `render.Rules` in place of the `render.Box{X: 0, Y: 0, W: e.cfg.Width, H: e.cfg.Height}` literal it constructs today.
  After this change `planLayout` must reference neither `e.cfg.Width` nor `e.cfg.Height` at all, and must issue no tmux round trip of its own.
  Every other argument to `render.Rules` — the `render.Params` built from `e.cfg.CollapsedStripRows`, `e.cfg.MinFullRows` and `e.cfg.Header.HeightRows`, the header-presence filter, and `paneIDsByTop(live)` — stays exactly as it is.

  Update `planLayout`'s doc comment to say the box is always told to it and that it queries nothing itself, and to name the two callers and their two different box sources (`applyLayoutLocked`'s live query, `AttachArgv`'s told client box, landing in batch 2).

  Restructure `applyLayoutLocked` so its two existing skip guards run FIRST, before anything else:
  move the `if len(live) < 2 { return nil }` and `if !anyPlacedStrand(st.Strands, liveIDSet(live)) { return nil }` checks above the plan, then call `box := e.liveBoxLocked()` and `layout, focus, err := e.planLayout(st, live, box)` beneath them, keeping the plan-error wrap (`fmt.Errorf("plan layout: %w", err)`) and everything below it unchanged.
  This ordering is required, not cosmetic: `liveBoxLocked` is a real `display-message` round trip, so evaluating it as an argument at the top of the function would fire a tmux call on exactly the degenerate paths this function's own doc comment promises to "skip both tmux calls entirely" — and `reconcileApplyPersistLocked` (`spawn.go`) runs this function once per launch on `Resume`, so the wasted round trip would repeat per strand.
  It also makes this call site agree with `AttachArgv`'s ordering in batch 2, which evaluates the same two guards before it plans.

  The one behavioural delta this reorder introduces, stated so it is a decision rather than an accident: a plan error (today only reachable via a strand carrying `render.AnchorOwnWindow`, which `render.Rules` rejects) is no longer returned when a skip guard has already fired.
  That is correct — the guards mean there is nothing to apply, so there is nothing the plan error could have prevented — and nothing pins the old behaviour: `apply_test.go`'s `TestApplyLayoutLocked_SkipsTmuxWhenFewerThanTwoLivePanes` and `TestApplyLayoutLocked_SkipsTmuxWhenNoStrandOwnsAPresentPane` both assert a nil return, and both keep passing.
  Both of those tests also rely on the fixture's nonexistent tmux binary making a stray round trip "fail loudly", which the guards-first order keeps true;
  leave their comments alone, and do not weaken either test.

  Then extend `applyLayoutLocked`'s doc comment with one sentence recording why the live box matters: `select-layout` with a layout string whose dimensions disagree with the live window exits 0 and silently rescales the layout proportionally, so every absolute row budget reed computes (`Header.HeightRows`, `CollapsedStripRows`, `MinFullRows`) was being scaled by `live_height / cfg.Height` on any window that is not exactly `cfg.Height` rows tall.

  Also record the detached-session consequence in that same comment, so a later reader does not read it as a bug: while detached, an over-budget layout string is accepted by `select-layout` and answered by GROWING the window to fit the cells, so a session with no client can end up taller than its configured boot height until the next client attaches and snaps it back.
  Do not change `anyPlacedStrand`, `liveIDSet`, `aliveIDSet`, or `paneIDsByTop`.
- **Commit:** `refactor(reedengine): plan the layout against a told box rather than the pinned config size`

### Card 3: pin status and window-size on the boot path

- **Context:**
  - `internal/reedengine/windowsize.go`
  - `internal/reedengine/mouse.go`
- **Edits:**
  - `internal/reedengine/lifecycle.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  In `ensureServerAndSessionLocked`, immediately after the existing `set-option -g mouse` call and before the final `return true, stripped, nil`, call `e.pinGeometryOptionsLocked()`.
  Add a comment above the call stating that, unlike the two `set-option` calls above it, this one is non-fatal by design: `remain-on-exit` and `mouse` are correctness dependencies, while `status` and `window-size` are geometry-quality options whose absence degrades to tmux's own proportional rescale — a working session — and psmux's support for both is unverified anywhere in this repo, so a capability reed cannot confirm must not be able to take the boot down.
  Note in the same comment that boot options never re-apply to an already-up session (the healthy already-up path returns early, above this block), which is why `AttachArgv` re-pins them in its own pre-flight rather than relying on this call.
  Change nothing else in `lifecycle.go` — not the boot retry loop, not `requireSessionLocked`, not `Status`.
- **Commit:** `feat(reedengine): pin status off and window-size latest at session boot`

### Card 4: tier-1 coverage for the box source, the query fallbacks, and the readbacks

- **Context:**
  - `internal/reedengine/windowsize.go`
  - `internal/reedengine/overlay.go`
  - `internal/reedengine/lock_test.go`
  - `internal/reedengine/generation_test.go`
  - `internal/reedengine/strand_test.go`
  - `internal/reedengine/render/rules.go`
- **Edits:**
  - `internal/reedengine/apply_test.go`
- **Creates:**
  - `internal/reedengine/windowsize_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Create `internal/reedengine/windowsize_test.go` in `package reedengine`, untagged (tier 1), building its engine with the existing `newTestEngine(t)` helper and driving every tmux round trip through `e.tmux.execHook` in the shape `generation_test.go` and `strand_test.go` already use.
  Do not call `exec.Command` anywhere in this file, and do not sleep.
  Cover, as table tests:
  - `parseWindowSize` against `"220 50"`, `"220 50\n"`, `"  220   50  "`, `""`, `"220"`, `"220 50 7"`, `"abc def"`, `"0 50"`, `"220 0"` and `"-1 50"` — the first three parse to `(220, 50, true)`, every other row returns `ok == false`.
  - `liveBoxLocked` with a hook answering `display-message` with a well-formed pair, with garbage, with an empty string, with a non-positive dimension, and with an error — the good case returns the live pair as `render.Box{W: 220, H: 50}`, every degraded case returns `render.Box{W: e.cfg.Width, H: e.cfg.Height}`.
    Set `e.cfg.Width`/`e.cfg.Height` to values distinct from the scripted live pair so a fallback cannot pass by coincidence.
  - `reservedRowsFromStatus` against `"off"`, `"on"`, `"2"`, `"OFF"`, `" on "`, `""`, `"garbage"` and `"-1"` — yielding `(0,true)`, `(1,true)`, `(2,true)`, `(0,true)`, `(1,true)`, `(0,false)`, `(0,false)`, `(0,false)` respectively.
  - `windowSizeAllowsChain` against `"latest"`, `"LATEST"`, `" latest "`, `"manual"`, `"largest"`, `"smallest"` and `""` — true for the first three only.
  - `readStatusRowsLocked` and `readWindowSizeLatestLocked` driven off scripted `display-message` answers and off an error return, asserting the error return yields `(0, false)` and `false` respectively.
  - `pinGeometryOptionsLocked` recording every `set-option` argv the hook receives: assert both pins are issued, that the first carries `-t` with the `=<session>:` target form and the `status off` pair, that the second carries `-w` and the `window-size latest` pair, that neither carries `-g`, and that a first-pin error does not stop the second pin from being issued.

  In `apply_test.go`, update all four existing `e.planLayout(st, live)` call sites to pass an explicit box.
  Each of the three existing tests currently sets `e.cfg.Width`/`e.cfg.Height` and compares against a `render.Rules` call using those same numbers;
  keep those numbers, passing them as `render.Box{X: 0, Y: 0, W: <that width>, H: <that height>}` so each test's expectation is unchanged.
  Then add one new test, `TestPlanLayout_UsesTheToldBoxAndIssuesNoQuery`: set `e.cfg.Width`/`e.cfg.Height` to one pair, install an `execHook` that fails every call and records that it was called at all, plan against a DIFFERENT told box, and assert both that the resulting layout string carries the told box's dimensions (not the configured pair) and that the hook was never invoked.
  This is the seam the two callers' disagreement rests on, so it is asserted rather than inferred.
  Update the `apply_test.go` file-header comment to mention the told-box seam.
- **Commit:** `test(reedengine): cover the told box, the live-size fallbacks, and the option readbacks`

### Card 5: pin the two layout regimes a live box makes reachable

- **Context:**
  - `internal/reedengine/render/rules.go`
  - `internal/reedengine/render/height.go`
  - `internal/reedengine/render/types.go`
- **Edits:**
  - `internal/reedengine/render/rules_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Extend the existing `TestRulesGolden` table in `internal/reedengine/render/rules_test.go` — do not add a parallel table — with two new rows covering the two regimes that a live terminal box makes reachable for the first time.
  With the box pinned at 220x50 the clamps in `height.go` almost never fired;
  a real terminal is routinely 24 or 30 rows, so `clampHeaderHeight` and `clampToFit` now govern the common case.
  1. A **budget-satisfying** row: a `Params` carrying a non-zero `Header` (a `PaneID` and a `HeightRows` above 1) and a box with enough rows for the header band, its one-row divider, every collapsed strip at the configured `CollapsedStripRows`, and every full pane at `MinFullRows`.
     Assert the golden body string shows the header cell at exactly the configured `HeightRows` and each collapsed strip at exactly `CollapsedStripRows` — the absolute row counts tmux's proportional rescale was destroying, and which are only assertable where no clamp fires.
  2. A **clamped** row: the same strand fixture against a box too short for those budgets.
     Assert the clamped golden body rather than the unclamped budgets, and add a companion assertion (a separate test function beside the table is acceptable if the table's shape cannot express it) that no emitted cell height is ever non-positive.
     State in that row's comment that the last-resort branch of `clampToFit` is deliberately permitted to emit cell heights summing to more than `box.H`, per that function's own documented design, so a future reader does not read an over-sum as a defect to fix here.

  Derive both golden strings by reasoning from `height.go`'s documented strict-priority order — strips shrink toward 1 row first, then non-active full panes toward `MinFullRows`, then every remaining donor to 1, and only then the active pane absorbs the rest — and verify them by running the test rather than by asserting whatever the code happens to emit.
  Do not change `render.Rules`, `stackHeights`, `clampHeaderHeight`, or `clampToFit`;
  this task changes which box `render` is handed, never the rules inside it.
- **Commit:** `test(render): pin the budget-satisfying and clamped layout regimes a live box reaches`

## Batch Tests

`verify: go test ./internal/reedengine/...` runs the whole `reedengine` package plus its `render` subpackage, untagged only.
That is the right scope: every file this batch touches lives under `internal/reedengine/`, and the two packages' untagged suites are fast and hermetic (no tmux server, no git).
The files it covers include the two this batch changes (`apply_test.go`, `render/rules_test.go`), the one it creates (`windowsize_test.go`), and every existing sibling suite that could be broken by the `planLayout` signature change or the new boot-path call — `lifecycle_test.go`, `strand_test.go`, `spawn_test.go`, `reconcile_test.go`, `render/height_test.go`.
The `integration`-tagged files in the same package (`contract_integration_test.go`, `mouse_boot_integration_test.go`) are deliberately not run here;
batch 4's verify and the task-wide `done_gate` cover the tagged half.
