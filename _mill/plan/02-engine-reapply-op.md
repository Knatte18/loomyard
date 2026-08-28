# Batch: engine-reapply-op

```yaml
task: 'reed: watchdog daemon'
batch: 'engine-reapply-op'
number: 2
cards: 9
verify: go test ./internal/reedengine/...
depends-on: [1]
```

## Batch Scope

This batch builds the whole engine-side surface the watch loop calls into, and nothing that loops.
It widens `applyLayoutLocked` so the apply path can hand back the box it used and whether that box was a real observation, and so a caller can ask for the layout half without the focus half; it exposes the `ok` flag `liveBoxLocked` already computes internally; it adds a non-blocking sibling of `withOpLock`; it composes all of that into the new package-internal `reapplyLayout` op, which also carries the hook-availability probe; it teaches `pinGeometryOptionsLocked` to install, replace, or unset the `window-resized` hook and to remove the signal file when the watchdog is off; and it makes an invalid `watchdog` value a loud boot failure.

It is one batch because every card either widens or consumes the same three functions (`liveBoxLocked`, `applyLayoutLocked`, `withOpLock`) inside one package, and `windowsize.go` is edited by two of them.

**External interface batch 3 consumes:** `ReapplyResult` and `(*Engine).reapplyLayout(lastApplied render.Box) (ReapplyResult, error)`.

Batch-local decision: `applyLayoutLocked` keeps its exact current signature and behaviour as a thin wrapper, so no existing caller changes.
All new capability lands on a new `applyLayoutLockedOpts` sibling.

## Cards

### Card 6: expose liveBoxLocked's ok flag via a sibling

- **Context:**
  - `internal/reedengine/render/types.go`
  - `internal/reedengine/lock.go`
  - `CONSTRAINTS.md`
- **Edits:**
  - `internal/reedengine/windowsize.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Split `(*Engine).liveBoxLocked` into a flag-returning sibling plus a delegating wrapper.
  Add `func (e *Engine) liveBoxLockedOK() (render.Box, bool)` carrying today's exact body, returning `false` on both fallback paths (the `display-message` round-trip error and the `parseWindowSize` malformed answer) and `true` only when the parse succeeded.
  Keep both `logger.Warn` calls exactly as they are, unmoved and unreworded.
  Reduce `func (e *Engine) liveBoxLocked() render.Box` to `box, _ := e.liveBoxLockedOK(); return box`, keeping its existing godoc and adding one line stating that it deliberately discards the flag so no existing caller changes.
  Give `liveBoxLockedOK` a godoc stating why the flag matters: this method never reports failure through its box, because a degraded query returns the configured `cfg.Width`/`cfg.Height` pair — a perfectly plausible-looking box — so a caller comparing boxes across calls must be told whether the box was an observation at all.
  Do not change the tmux argv, the format string, or the fallback box.
- **Commit:** `refactor(reed): expose liveBoxLocked's ok flag via liveBoxLockedOK`

### Card 7: let the apply path report its box and skip the focus half

- **Context:**
  - `internal/reedengine/windowsize.go`
  - `internal/reedengine/render/types.go`
  - `internal/reedengine/spawn.go`
  - `internal/reedengine/state.go`
- **Edits:**
  - `internal/reedengine/apply.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add two new types and one new method to `internal/reedengine/apply.go`, and reduce `applyLayoutLocked` to a wrapper.

  Declare:

  ```go
  // applyOpts tunes one applyLayoutLockedOpts call.
  type applyOpts struct {
  	// SkipFocus suppresses the trailing select-pane, applying the layout only.
  	SkipFocus bool
  	// SkipWhenBoxEquals, when non-nil, suppresses select-layout entirely if the
  	// resolved live box is an observation and equals *SkipWhenBoxEquals.
  	SkipWhenBoxEquals *render.Box
  }

  // applyResult reports what one applyLayoutLockedOpts call did.
  type applyResult struct {
  	// Applied is true only when select-layout was actually issued.
  	Applied bool
  	// Box is the box the layout was planned against. Meaningful only when BoxIsLive.
  	Box render.Box
  	// BoxIsLive reports whether Box came from a successful live query rather than
  	// liveBoxLocked's configured fallback, or from no query at all.
  	BoxIsLive bool
  }
  ```

  Add `func (e *Engine) applyLayoutLockedOpts(st *ReedState, live []LivePane, opts applyOpts) (applyResult, error)` carrying today's `applyLayoutLocked` body with four changes, in this order:

  1. The two existing skip guards (`len(live) < 2` and `!anyPlacedStrand(...)`) return `applyResult{}` — that is `Applied: false`, `BoxIsLive: false`, zero `Box` — and `nil`, before any box query, exactly as they return `nil` today.
  2. Replace `box := e.liveBoxLocked()` with `box, boxIsLive := e.liveBoxLockedOK()`.
  3. Immediately after that call, when `opts.SkipWhenBoxEquals != nil` **and** `boxIsLive` **and** `box == *opts.SkipWhenBoxEquals`, return `applyResult{Applied: false, Box: box, BoxIsLive: true}, nil` without issuing `select-layout` or `select-pane`.
     The `boxIsLive` conjunct is load-bearing: a degraded query is not an observation, so a fallback box must never satisfy the guard.
  4. After a successful `select-layout`, when `opts.SkipFocus` is true, return `applyResult{Applied: true, Box: box, BoxIsLive: boxIsLive}, nil` without issuing `select-pane`; otherwise keep today's `focus == ""` early return and `select-pane` call and return the same `applyResult` at both exits.

  Every error return keeps today's wording and wrapping and pairs it with a zero `applyResult`.

  Reduce `func (e *Engine) applyLayoutLocked(st *ReedState, live []LivePane) error` to `_, err := e.applyLayoutLockedOpts(st, live, applyOpts{}); return err`, keeping its entire existing godoc block verbatim and appending one paragraph explaining that the body now lives in `applyLayoutLockedOpts` and that the zero `applyOpts` is exactly today's behaviour — full focus half included — so `spawn.go`'s `reconcileApplyPersistLocked` caller is unchanged.

  Document on `applyOpts.SkipFocus` why it exists: the trailing `select-pane -t focus` targets the strand carrying `Display.Focus` in the persisted table, not whichever pane is live-active, which is right for an operator-invoked op and wrong for an unattended watcher that would otherwise yank the operator's cursor out of the pane they are typing in on every window resize.

  Do not change `planLayout`, `anyPlacedStrand`, `liveIDSet`, `aliveIDSet`, or `paneIDsByTop`.
- **Commit:** `refactor(reed): let applyLayoutLocked report its box and skip the focus half`

### Card 8: add a non-blocking sibling of withOpLock

- **Context:**
  - `internal/lock/lock.go`
  - `internal/reedengine/server.go`
- **Edits:**
  - `internal/reedengine/lock.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add `func (e *Engine) withTryOpLock(fn func() error) (acquired bool, err error)` to `internal/reedengine/lock.go`, beside `withOpLock`.
  Its body mirrors `withOpLock` exactly — `validateToldTmuxIdentity`, then `validateToldAnchorPath`, then `os.MkdirAll(e.stateDir(), 0o755)`, then the lock, then `defer l.Release()`, then the `os.Stat` baseline snapshot with the same `logger.Debug` on stat failure, then `fn()`, then `opLockCompromisedError` with the same two-error wrapping — with exactly one difference: it acquires via `lock.TryAcquireWriteLock(lockPath)` instead of `lock.AcquireWriteLock(lockPath)`, and when that reports `locked == false` it returns `(false, nil)` immediately, before the stat baseline and without calling `fn`.
  Every path that did call `fn` returns `acquired == true`, whatever `fn` returned.
  Every pre-`fn` failure (a validation error, the `MkdirAll` error, the `TryAcquireWriteLock` error) returns `(false, err)`.
  Its godoc must state: that a lock held by someone else is a **deferral**, not a failure, and is reported as `(false, nil)` with no error; why the watcher needs this rather than `withOpLock`, namely that `lock.AcquireWriteLock` blocks with no timeout and this repo's own R5 measurement records a second `lyx reed status` blocking for 11027ms behind a held lock, which is a state the watcher's retry contract cannot describe; and that deferring is correct on the merits too, since whatever holds the lock is another reed op and every reed op ends by re-applying the layout itself.
  It must also state that it keeps both told-geometry pre-flight validations and the post-op lock-compromise check, because those are the reason `withOpLock` is a chokepoint rather than a bare lock acquisition.
  Do not change `withOpLock` or `opLockCompromisedError`.
- **Commit:** `feat(reed): add withTryOpLock, a non-blocking op-lock sibling`

### Card 9: add the reapplyLayout op with its box guard and hook probe

- **Context:**
  - `internal/reedengine/lifecycle.go`
  - `internal/reedengine/apply.go`
  - `internal/reedengine/lock.go`
  - `internal/reedengine/overlay.go`
  - `internal/reedengine/windowsize.go`
  - `internal/reedengine/watchdog.go`
  - `internal/reedengine/spawn.go`
  - `internal/reedengine/render/types.go`
  - `internal/shell/shell.go`
  - `CONSTRAINTS.md`
- **Edits:** none
- **Creates:**
  - `internal/reedengine/reapply.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create `internal/reedengine/reapply.go` in `package reedengine` with a file-header comment stating that this file owns the watchdog's single re-apply op — the only place the watch loop ever reaches tmux — and that the op is structurally `Status()`'s lock-then-load-then-list shape with the layout apply on the end.

  Declare:

  ```go
  // ReapplyResult reports one reapplyLayout call's outcome.
  type ReapplyResult struct {
  	// Deferred is true when the op lock was held by someone else, so nothing ran.
  	Deferred bool
  	// Applied is true only when select-layout was actually issued.
  	Applied bool
  	// Box is the box the layout was planned against. Meaningful only when BoxIsLive.
  	Box render.Box
  	// BoxIsLive reports whether Box was a real observation rather than
  	// liveBoxLocked's configured fallback, or no query at all.
  	BoxIsLive bool
  	// HookInstalled reports whether this session's window-resized hook is
  	// exactly reed's own command string for this worktree's signal path.
  	HookInstalled bool
  	// HookKnown reports whether HookInstalled was decided at all this call.
  	HookKnown bool
  }
  ```

  `ReapplyResult` is exported only because it is the return type of a method reachable from the exported `Engine.Watch` added in batch 3; `reapplyLayout` itself stays unexported.

  Add `func (e *Engine) hookInstalledLocked() (installed bool, known bool)` in this same file.
  On `runtime.GOOS == "windows"` it returns `(false, false)` immediately, issuing **no** round trip — Windows is poll-only unconditionally, because `set-hook`/`run-shell` are absent from `requiredSubcommands` and psmux's support for them is unverified, and a hook that installs but never fires would pin the watcher in signal mode forever with zero self-heal.
  Otherwise it issues `e.tmux.output("show-options", "-v", "-t", exactSessionWindowTarget(e.SessionName()), windowResizedHookName)`.
  On error it `logger.Debug`s and returns `(false, false)`.
  Otherwise it returns `(strings.TrimSpace(out) == resizeHookCommand(shell.ForGOOS(), e.resizeSignalPath()), true)`.
  Its godoc must record two live-verified facts: the readback is `show-options`, **not** `show-hooks`, because hooks are options in tmux 3.6 and `show-hooks` prints nothing for a session-scoped hook that demonstrably fires; and the match is exact against reed's own command string for this worktree's signal path, never merely "some `window-resized` hook exists", because a foreign hook or a sibling worktree's signal path would deliver nothing this watcher can consume.
  It must also state that `show-options` is absent from `requiredSubcommands` and that this is acceptable precisely because every failure shape here yields `known == false` and therefore poll mode, so no capability-probe change is needed and no psmux risk is taken.

  Add `func (e *Engine) reapplyLayout(lastApplied render.Box) (ReapplyResult, error)`.
  Its body is:

  1. `acquired, err := e.withTryOpLock(func() error { ... })`.
  2. Inside the closure, in order: `e.requireSessionLocked()`; `e.loadOrInitStateLocked()`; `e.tmux.listPanes(e.SessionName())` wrapped as `fmt.Errorf("list panes: %w", err)` exactly as `Status()` does; then `installed, known := e.hookInstalledLocked()` recorded onto the result; then `e.applyLayoutLockedOpts(st, live, applyOpts{SkipFocus: true, SkipWhenBoxEquals: &lastApplied})`, whose `applyResult` fields are copied onto the `ReapplyResult`.
  3. When `acquired` is false and `err` is nil, return `ReapplyResult{Deferred: true}` and `nil` — `HookKnown` stays false, so the mode is simply not decided this call.
  4. When `err` is non-nil, return whatever partial result was recorded together with `err`.

  The probe runs **after** `listPanes` and **before** the apply so that a session the apply guards skip (fewer than two panes, or no strand owning a present pane) still decides the mode — otherwise a watcher on such a session could never promote out of poll mode.

  `reapplyLayout` persists nothing: it never calls `SaveState` and never writes `reed.json`.
  Its godoc must state that, must state that it inherits `applyLayoutLockedOpts`'s two session-survival guards rather than re-deriving them (which matters more here than anywhere else, since the watcher fires unattended with no operator watching the envelope), and must state that it owns the box-equality guard itself so the comparison happens under the same lock as the query that produced the box.

  Do not export `reapplyLayout`, do not add a second lock acquisition, and do not query geometry outside `applyLayoutLockedOpts`.
- **Commit:** `feat(reed): add the reapplyLayout op with its box guard and hook probe`

### Card 10: install, replace, or unset the window-resized hook in pinGeometryOptionsLocked

- **Context:**
  - `internal/reedengine/watchdog.go`
  - `internal/reedengine/overlay.go`
  - `internal/reedengine/attach.go`
  - `internal/reedengine/lifecycle.go`
  - `internal/shell/shell.go`
  - `CONSTRAINTS.md`
- **Edits:**
  - `internal/reedengine/windowsize.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Extend `(*Engine).pinGeometryOptionsLocked` in `internal/reedengine/windowsize.go` with a third non-fatal block, placed after the two existing `set-option` pins and using the same `logger.Warn`-and-continue treatment.

  The block:

  1. `enabled, err := watchdogOption(e.cfg.Watchdog)`; on a non-nil error, `logger.Warn` naming the offending value and stating that the watchdog is being treated as off, then set `enabled = false`.
     This function returns nothing and is all-non-fatal by contract, so it takes the unset side rather than propagating; the boot path (card 12) is where an invalid value is loud.
  2. On `runtime.GOOS == "windows"`: install nothing and unset nothing — the hook is never installed there — but still remove the signal file when `enabled` is false, then return.
  3. Otherwise, when `enabled`: `e.tmux.run("set-hook", "-t", target, windowResizedHookName, resizeHookCommand(shell.ForGOOS(), e.resizeSignalPath()))`, `logger.Warn` on error.
     The plain, **replacing** `set-hook` form is mandatory and `-a` must never appear: verified live, four identical plain installs yield exactly one fire per resize while three additional `-a` appends yield four, and this function runs on **every** `AttachArgv` pre-flight as well as at boot, so the append form would cost N `run-shell` spawns per resize after N attaches.
  4. Otherwise (not enabled): `e.tmux.run("set-hook", "-u", "-t", target, windowResizedHookName)`, `logger.Warn` on error, then remove the signal file.
     `set-hook -u` is idempotent and exits 0 whether or not a hook was set (verified live).

  Signal-file removal is `os.Remove(e.resizeSignalPath())` with an `errors.Is(err, fs.ErrNotExist)` check so an absent file is silent; any other error is `logger.Warn`-ed and ignored.

  Reuse the existing `target := exactSessionWindowTarget(e.SessionName())` local already computed at the top of the function.

  Extend the function's godoc to state that it now also owns the `window-resized` hook's whole install/unset lifecycle, that this is the right home because the function already runs both at boot (`lifecycle.go`) and in the attach pre-flight (`attach.go`) — which is what lets a session booted by an older `lyx` pick the hook up on the operator's next attach rather than staying unhealed until a manual `down` + `up` — and that `watchdog: off` must reach the hook as well as the loop, because a kill-switch that leaves the hook installed keeps spawning `run-shell` on every resize to write a signal file nobody reads.

  Do not change either existing `set-option` call.
  Do not make any part of this block fatal.
- **Commit:** `feat(reed): install and unset the window-resized hook alongside the geometry pins`

### Card 11: make an invalid watchdog value a loud boot failure

- **Context:**
  - `internal/reedengine/watchdog.go`
  - `internal/reedengine/mouse.go`
- **Edits:**
  - `internal/reedengine/lifecycle.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `(*Engine).ensureServerAndSessionLocked` (`internal/reedengine/lifecycle.go`), validate the `watchdog` key in the same pre-tmux block that already validates `debug_log` and `mouse`, immediately after the `mouseOption` call and before the `ValidateHeader` call:

  ```go
  	if _, err := watchdogOption(e.cfg.Watchdog); err != nil {
  		return false, nil, err
  	}
  ```

  Precede it with a short comment in the voice of its two neighbours, stating that the boolean is discarded here because this is the one consumer with an error channel and its only job is to make a typo fail `lyx reed up` loudly and by name — the hook install and the watch loop each read the key again and fail safe toward "no watchdog" instead.
  Do not change the `debugLogArgs`, `mouseOption`, `ValidateHeader`, or `probeCapabilityLocked` calls, and do not move the block.
- **Commit:** `feat(reed): fail the boot loudly on an invalid watchdog value`

### Card 12: tier-1 tests for the widened apply path and the try-lock

- **Context:**
  - `internal/reedengine/apply.go`
  - `internal/reedengine/windowsize.go`
  - `internal/reedengine/lock.go`
  - `internal/reedengine/overlay.go`
  - `internal/reedengine/render/types.go`
  - `internal/lock/lock.go`
  - `CONSTRAINTS.md`
- **Edits:**
  - `internal/reedengine/apply_test.go`
  - `internal/reedengine/windowsize_test.go`
  - `internal/reedengine/lock_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Extend the three existing untagged test files, following each file's existing `execHook`-stubbing and fixture style.

  In `apply_test.go`:
  - `applyLayoutLockedOpts` on both inherited guard-skip paths — fewer than two live panes, and two panes with no strand owning a present pane — returns `applyResult{}` with `Applied` false and `BoxIsLive` false, issues no `select-layout`, and returns nil.
  - `applyOpts{SkipFocus: true}` issues `select-layout` and **no** `select-pane`, while `applyOpts{}` on the same fixture issues both — the focus-preservation contract, asserted on the recorded argv.
  - `applyOpts{SkipWhenBoxEquals: &box}` where the scripted `display-message` answer equals `box` issues no `select-layout` and returns `Applied: false, BoxIsLive: true` with `Box` equal to the observed box.
  - The same option where the scripted answer differs issues `select-layout` and returns `Applied: true, BoxIsLive: true`.
  - The degraded case: with `display-message` scripted to error, `SkipWhenBoxEquals` pointing at a box equal to the configured `cfg.Width`/`cfg.Height` fallback still issues `select-layout` and returns `BoxIsLive: false` — a fallback box is not an observation and must never satisfy the guard.
  - The existing `applyLayoutLocked` assertions in this file stay green unchanged; add one asserting the wrapper still issues both `select-layout` and `select-pane`.

  In `windowsize_test.go`:
  - `liveBoxLockedOK` returns `(box, true)` on a well-formed scripted answer, and `(configured-fallback, false)` on both a round-trip error and a malformed answer.
  - `liveBoxLocked` returns the same box as `liveBoxLockedOK` in all three cases.
  - `pinGeometryOptionsLocked` with `Watchdog: "on"` issues a `set-hook` whose argv is exactly `["set-hook", "-t", "=<session>:", "window-resized", <resizeHookCommand>]` — assert the argv contains no `-a` token anywhere.
  - `pinGeometryOptionsLocked` with `Watchdog: "off"` issues `["set-hook", "-u", "-t", "=<session>:", "window-resized"]` and removes a signal file that exists on disk beforehand, and issues no install.
  - `pinGeometryOptionsLocked` with an invalid `Watchdog` value behaves identically to `"off"` and returns normally (no panic, no error — the function returns nothing).
  - A `set-hook` scripted to error is non-fatal: the function still returns, and both preceding `set-option` calls were still attempted.
  - Removing an absent signal file is silent.

  In `lock_test.go`:
  - `withTryOpLock` runs `fn` and reports `(true, nil)` on a free lock.
  - With `reed.lock` already held by a second `lock.AcquireWriteLock` on the same path, `withTryOpLock` reports `(false, nil)`, does **not** call `fn`, and issues no tmux call.
  - `withTryOpLock` propagates `fn`'s error together with `acquired == true`.
  - A told-geometry validation failure reports `(false, err)` without touching the lock file.

  Every new test stays untagged and spawns nothing: drive tmux through `TmuxCmd.execHook` and use `t.TempDir()` for the lock and signal paths.
- **Commit:** `test(reed): cover the widened apply path, liveBoxLockedOK, the hook pin, and withTryOpLock`

### Card 13: tier-1 tests for reapplyLayout and the hook probe

- **Context:**
  - `internal/reedengine/reapply.go`
  - `internal/reedengine/apply.go`
  - `internal/reedengine/lock.go`
  - `internal/reedengine/watchdog.go`
  - `internal/reedengine/overlay.go`
  - `internal/reedengine/lifecycle.go`
  - `internal/reedengine/spawn.go`
  - `internal/reedengine/state.go`
  - `internal/reedengine/render/types.go`
  - `internal/lock/lock.go`
  - `internal/shell/shell.go`
  - `internal/reedengine/apply_test.go`
  - `CONSTRAINTS.md`
- **Edits:** none
- **Creates:**
  - `internal/reedengine/reapply_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create `internal/reedengine/reapply_test.go` (untagged) with a file-header comment naming what it pins.
  Build fixtures the way `apply_test.go` already does — a hand-built `Engine` over `t.TempDir()`, a persisted `ReedState`, and a `TmuxCmd.execHook` that scripts each subcommand's answer and records every argv.

  Cover:

  - **Guard inheritance.** With fewer than two live panes, and with two panes but no strand owning a present pane, `reapplyLayout` issues no `select-layout`, returns `Applied: false, BoxIsLive: false`, and returns nil.
    This is the guard that keeps an unattended watcher from destroying a session's entire pane set.
  - **Focus is never moved.** A successful re-apply issues `select-layout` and no `select-pane`.
  - **Deferral.** With `reed.lock` already held, `reapplyLayout` returns `ReapplyResult{Deferred: true}` and nil, issues **no** tmux call at all, and reports `HookKnown: false`.
  - **Box-equality guard.** A call whose scripted live box equals `lastApplied` issues no `select-layout` and returns `Applied: false, BoxIsLive: true`; a call whose box differs applies.
  - **Degraded box.** With `display-message` scripted to error, `reapplyLayout` returns `BoxIsLive: false` whether or not the fallback box happens to equal `lastApplied`, and — in the happens-to-equal case — still issues `select-layout`.
  - **Hook probe, exact match only.** A table over the scripted `show-options -v` answer, asserting `(HookInstalled, HookKnown)` for each: reed's own exact command string for this worktree's signal path yields `(true, true)`; the empty string (no hook set) yields `(false, true)`; a `window-resized` hook belonging to something else yields `(false, true)`; reed's own command shape but naming a **different** worktree's signal path yields `(false, true)`; a round-trip error yields `(false, false)`.
    "Some `window-resized` hook exists" is the wrong test and is what an obvious implementation writes.
  - **Probe ordering.** On a session the apply guards skip (fewer than two panes), the probe still ran: `HookKnown` is true and `show-options` appears in the recorded argv.
  - **Persists nothing.** A successful re-apply leaves `reed.json`'s bytes byte-identical to what they were before the call.

  A GOOS-conditional Windows assertion is not written here — card 9's Windows branch is a `runtime.GOOS` check that an untagged test on Linux cannot exercise without a build-tagged file; assert instead that on the current GOOS the probe issued exactly one `show-options` round trip, and leave the Windows poll-only behaviour to the code path's own godoc.
- **Commit:** `test(reed): cover reapplyLayout's guards, deferral, box equality, and hook probe`

### Card 14: tier-1 test for the boot-path watchdog validation

- **Context:**
  - `internal/reedengine/lifecycle.go`
  - `internal/reedengine/watchdog.go`
  - `internal/reedengine/mouse_test.go`
  - `internal/reedengine/overlay.go`
- **Edits:**
  - `internal/reedengine/lifecycle_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add an untagged test to `internal/reedengine/lifecycle_test.go` asserting that `ensureServerAndSessionLocked` returns an error naming the offending value when `Config.Watchdog` is invalid (`""`, `"1"`, `"yes"`), and that it does so **before** any tmux round trip — assert the `execHook` recorded zero argv.
  Assert the same fixture with `Watchdog: "on"` and `Watchdog: "off"` does not fail on this check.
  Follow whatever fixture shape the file already uses for the sibling `debug_log`/`mouse` validation tests; if the file has none, build a minimal one that stubs `execHook` and asserts on the returned error alone.
- **Commit:** `test(reed): assert an invalid watchdog value fails the boot before any tmux call`

## Batch Tests

`verify: go test ./internal/reedengine/...` runs the whole untagged reed suite, which is the right scope: every card in this batch edits a file inside that one package, and three of them (cards 6, 7, 10) change functions the existing suite already exercises heavily (`liveBoxLocked`, `applyLayoutLocked`, `pinGeometryOptionsLocked`), so the existing tests staying green is itself a load-bearing part of this batch's verification.
New coverage lands in `apply_test.go`, `windowsize_test.go`, `lock_test.go`, `lifecycle_test.go`, and the new `reapply_test.go`.
No integration-tagged file is added here, so the batch needs no live tmux; the live proof lands in batch 4.
