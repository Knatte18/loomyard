# Batch: watch-loop

```yaml
task: 'reed: watchdog daemon'
batch: 'watch-loop'
number: 3
cards: 4
verify: go test ./internal/reedengine/...
depends-on: [2]
```

## Batch Scope

This batch delivers the loop itself: a pure, clock-told debounce-and-retry state machine, and `Engine.Watch` — the one exported symbol this whole task adds to the engine — which drives that state machine over the signal file in signal mode, over a slow cycle in poll mode, and promotes itself from the second to the first the moment batch 2's probe reports the hook is installed.

It is one batch because the state machine exists only to serve the loop and the loop's contract is only assertable against the state machine: the never-returns guarantee, the per-event attempt cap, the mode-promotion rule, and the signal-file lifecycle are four faces of one object.

**External interface batch 4 consumes:** `func (e *Engine) Watch(ctx context.Context) error`.

Batch-local decision: `Watch` is a two-line wrapper over an unexported `watchLoop(ctx, watchTiming)` so tests can drive the loop with millisecond timings while production wires the package constants through `watchDefaultTiming()`.
This is what keeps the loop's tests fast and inside the Test Tier Purity Invariant — no untagged test may contain a `time.Sleep(...)` of a second or more, and a test driving the production 2s poll cycle would be exactly that.

## Cards

### Card 15: add the pure debounce-and-retry state machine

- **Context:**
  - `internal/reedengine/watchdog.go`
  - `CONSTRAINTS.md`
- **Edits:** none
- **Creates:**
  - `internal/reedengine/watchloop.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create `internal/reedengine/watchloop.go` in `package reedengine` with a file-header comment stating that this file owns the resize watch loop — its pure decision state in the first half, `Engine.Watch` and its driver in the second (card 16 fills the second half; write only the first half in this card).

  Declare, with godoc on every symbol:

  ```go
  // watchPlan is the decision one signal-mode tick yields.
  type watchPlan int

  const (
  	// watchPlanWait means this tick does nothing.
  	watchPlanWait watchPlan = iota
  	// watchPlanApply means a coalesced re-apply is owed now.
  	watchPlanApply
  )

  // watchTiming carries the loop's tunables as data so a test can drive the loop
  // in milliseconds while production wires the fixed package constants through
  // watchDefaultTiming.
  type watchTiming struct {
  	SignalTick  time.Duration
  	PollCycle   time.Duration
  	Quiet       time.Duration
  	BaseDelay   time.Duration
  	MaxAttempts int
  }

  // watchState is the signal-mode loop's whole mutable decision state: the
  // coalescing debounce window plus the current failure streak. It is pure —
  // it holds no clock and performs no I/O; every method is told the time.
  type watchState struct {
  	quiet       time.Duration
  	baseDelay   time.Duration
  	maxAttempts int

  	pending  bool
  	readyAt  time.Time
  	attempts int
  }
  ```

  Add `func watchDefaultTiming() watchTiming` returning the five package constants from `watchdog.go` (`watchdogSignalTick`, `watchdogPollCycle`, `watchdogDebounceQuiet`, `watchdogRetryBaseDelay`, `watchdogMaxAttempts`) and nothing else — no literals.

  Add `func newWatchState(t watchTiming) *watchState`.

  Add these methods:

  - `func (s *watchState) Signal(now time.Time)` — records a resize signal observed at `now`: sets `pending`, sets `readyAt = now.Add(s.quiet)`, and resets `attempts` to zero.
    Restarting `readyAt` on every signal is what makes the debounce **trailing-edge and coalescing**: a 20-step drag firing 20 signals in one second yields exactly one apply, after the drag settles.
    Resetting `attempts` is required by the failure contract — the streak is per-event, and a fresh resize is a fresh event.
  - `func (s *watchState) Plan(now time.Time) watchPlan` — returns `watchPlanApply` when `s.pending` and `!now.Before(s.readyAt)`, otherwise `watchPlanWait`.
    It mutates nothing, so a tick may call it freely.
  - `func (s *watchState) Succeeded()` — clears `pending`, zeroes `attempts`, and zeroes `readyAt`.
  - `func (s *watchState) Failed(now time.Time) (abandoned bool)` — increments `attempts`.
    While `attempts < s.maxAttempts`, it keeps `pending` set and pushes `readyAt` out to `now.Add(s.baseDelay << (s.attempts - 1))` — the escalating delay — and reports `false`.
    Once `attempts >= s.maxAttempts` it abandons the event: it clears `pending`, zeroes `attempts` and `readyAt`, and reports `true`.
    Its godoc must state that abandoning ends **this event's** retries and never the watcher, and that the next resize signal is itself the next retry trigger.
  - `func (s *watchState) Deferred()` — a documented no-op.
    A tick whose try-lock was unavailable is a deferral, not an attempt and not a failure: `pending`, `readyAt`, and `attempts` are all left exactly as they were, so the watcher reconsiders on the next tick with its budget untouched.
    Its godoc must say the method exists precisely so this rule is a named, testable contract rather than a missing `else` branch.

  Do not add `Engine.Watch`, any `time.Now()` call, any ticker, or any I/O in this card.
- **Commit:** `feat(reed): add the watch loop's pure debounce-and-retry state machine`

### Card 16: add Engine.Watch and the loop driver

- **Context:**
  - `internal/reedengine/watchdog.go`
  - `internal/reedengine/reapply.go`
  - `internal/reedengine/lock.go`
  - `internal/reedengine/lifecycle.go`
  - `internal/reedengine/overlay.go`
  - `internal/reedengine/render/types.go`
  - `internal/reedengine/config.go`
  - `CONSTRAINTS.md`
- **Edits:**
  - `internal/reedengine/watchloop.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add the loop's second half to `internal/reedengine/watchloop.go`.

  Declare a mode type:

  ```go
  // watchMode names how the loop learns about a resize.
  type watchMode int

  const (
  	// watchModePoll re-applies once per cycle and re-probes hook availability
  	// each cycle. It is the safe default: it works whether or not the hook exists.
  	watchModePoll watchMode = iota
  	// watchModeSignal waits on the hook-written signal file and performs no
  	// geometry polling at all.
  	watchModeSignal
  )
  ```

  Add the exported entry point:

  ```go
  // Watch runs reed's resize self-heal loop for this worktree's session.
  func (e *Engine) Watch(ctx context.Context) error
  ```

  Its body is `return e.watchLoop(ctx, watchDefaultTiming())` and nothing else.
  Its godoc must state the four contracts a caller depends on: it **never returns while ctx is live**, including the disabled cases, where it parks internally rather than returning early; it never writes to stdout or stderr and never returns an error out of a failure inside the loop; a non-nil return is only ever `ctx.Err()`-shaped and is for logging, never for display; and it is the only exported symbol this feature adds to the engine — the re-apply op, the state machine, and every helper are package-internal.

  Add `func (e *Engine) watchLoop(ctx context.Context, t watchTiming) error` with this body:

  1. `enabled, err := watchdogOption(e.cfg.Watchdog)`.
     On a non-nil error, `logger.Warn` naming the offending value and stating the watchdog is being treated as off, then set `enabled = false`.
     This consumer has no error channel that a caller could survive — returning here would let the header pane's `RunE` fall through and kill the keepalive — so an invalid value is `off`, never fatal.
  2. When `!enabled`: `logger.Info` that the resize watchdog is disabled for this session, then `<-ctx.Done()` and `return ctx.Err()`.
     Park; do not return early.
  3. Remove any pre-existing signal file: `os.Remove(e.resizeSignalPath())`, silent on `fs.ErrNotExist`, `logger.Warn` on anything else.
     A stale file is either a previous watcher's leftover or a resize that happened while none was running; the session boot that starts this watcher applies the layout itself, so consuming it would only buy a redundant apply.
     Removing it makes the loop's initial state deterministic.
  4. Initialise `mode := watchModePoll`, `state := newWatchState(t)`, and `var lastApplied render.Box`.
     The zero `render.Box` is a deliberate "nothing applied yet" sentinel and needs no companion flag: a live box always has positive `W`/`H`, so it can never equal the zero box, and the first re-apply therefore always runs.
  5. Loop forever over a `time.Ticker`, selecting on `ctx.Done()` (return `ctx.Err()`) and the tick.
     The ticker's period is `t.SignalTick` in signal mode and `t.PollCycle` in poll mode; when the mode changes, stop the old ticker and start one at the new period rather than ticking at the wrong rate.
  6. **Poll-mode tick:** call `e.reapplyLayout(lastApplied)` unconditionally and hand the outcome to the shared handler below.
     Poll mode uses neither the debouncer nor the retry streak — the cycle interval is its own cadence, and this is the fallback platform's only self-heal, so a per-event cap that could stop it permanently must not apply here.
  7. **Signal-mode tick:** `os.Stat(e.resizeSignalPath())`.
     When it exists, **remove it first** (`os.Remove`, silent on `fs.ErrNotExist`, `logger.Warn` on anything else) and then call `state.Signal(now)`.
     Removing before the apply is what makes a resize arriving mid-apply re-signal rather than be swallowed.
     A stat error that is not `fs.ErrNotExist` is `logger.Warn`-ed and treated as "no signal".
     Then, when `state.Plan(now) == watchPlanApply`, call `e.reapplyLayout(lastApplied)` and hand the outcome to the shared handler; otherwise do nothing this tick.
  8. **Shared outcome handler**, applied identically in both modes, in this order:
     - `err != nil` → `logger.Warn` with the socket, session, and error.
       In signal mode, `abandoned := state.Failed(now)`; when `abandoned`, `logger.Warn` once that this resize event is being abandoned after `t.MaxAttempts` attempts and that the watcher remains running and responsive to the next signal.
       In poll mode, do not touch `state`.
       Do not update `lastApplied`, and do not change the mode.
     - `res.Deferred` → `logger.Debug` that the op lock was held and this tick is deferred; in signal mode call `state.Deferred()`.
       Do not update `lastApplied` and do not change the mode: `HookKnown` is false on a deferral, so the mode is simply not decided this tick.
     - otherwise (a real, non-deferred call) → in signal mode call `state.Succeeded()`.
       When `res.BoxIsLive`, set `lastApplied = res.Box`; when it is false, leave `lastApplied` exactly as it was.
       A degraded query is not an observation, so a fallback box must cause neither a spurious permanent skip nor a spurious re-apply loop.
       Then apply the mode rule: when `res.HookKnown && res.HookInstalled && mode == watchModePoll`, promote to `watchModeSignal` — `logger.Info` the promotion, remove any signal file written before the promotion, reset `state` to a fresh `newWatchState(t)`, and restart the ticker at `t.SignalTick`.
       Signal mode **never** demotes and never re-probes: `watchdog: off` unsets the hook while an already-running signal-mode watcher keeps going until the next header-pane rebuild, and a signal-mode watcher with no hook receives no signals and therefore does nothing, which is exactly what the operator asked for.
  9. The loop reads `e.cfg.Watchdog` exactly once, at step 1, and never again.
     Flipping the key on disk changes nothing until the process restarts; that boundary is documented in the template comment card 3 wrote.

  Every log call in this file uses `internal/logger` and includes `"socket", e.Socket()` and `"session", e.SessionName()`, matching the package's existing habit.
  No `fmt.Print*`, no `output.*`, no `panic`, and no `os.Exit` anywhere in this file.
  Do not add a second exported symbol.
- **Commit:** `feat(reed): add Engine.Watch and the resize self-heal loop driver`

### Card 17: tier-1 tests for the state machine

- **Context:**
  - `internal/reedengine/watchloop.go`
  - `internal/reedengine/watchdog.go`
  - `CONSTRAINTS.md`
- **Edits:** none
- **Creates:**
  - `internal/reedengine/watchloop_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create `internal/reedengine/watchloop_test.go` (untagged) with a file-header comment naming what it pins.
  Drive `watchState` with a synthetic clock — a `time.Time` local the test advances by hand — so nothing sleeps and nothing is timing-dependent.
  Build every state under test with `newWatchState`, never by struct literal, and reference `watchDefaultTiming()`'s fields (and through them the `watchdog*` constants) rather than repeating any literal duration.

  Cover:

  - `watchDefaultTiming()` returns exactly the five package constants — this is the assertion that keeps a later tuning change to one line.
  - A single `Signal` yields `watchPlanWait` until the quiet period has elapsed and `watchPlanApply` at and after it.
  - Twenty `Signal` calls at `Quiet/4` intervals yield `watchPlanWait` throughout and exactly one `watchPlanApply`, after the last signal's quiet period — the coalescing contract.
  - A `Signal` arriving inside the quiet period restarts it: the apply is owed relative to the later signal, not the earlier one.
  - A `Signal` arriving while an apply is notionally in flight schedules exactly one follow-up, not a queue: two `Signal` calls before a single `Succeeded` leave the state with at most one owed apply.
  - `Succeeded` clears the owed apply: the next `Plan` at any later time yields `watchPlanWait`.
  - `Failed` escalates and caps: attempts 1 and 2 report `abandoned == false` and push the next apply out by `BaseDelay` then `2*BaseDelay`; attempt 3 (`MaxAttempts`) reports `abandoned == true` and leaves `Plan` yielding `watchPlanWait` forever after.
  - The streak resets on success: `Failed`, `Failed`, `Succeeded`, then a fresh `Signal` and two more `Failed` calls must again report `abandoned == false` — the cap is per streak, not cumulative.
  - The streak resets on a fresh signal: `Failed`, `Failed`, then `Signal`, then two more `Failed` calls must again report `abandoned == false`.
  - `Deferred` changes nothing: taken between two `Failed` calls it leaves the attempt count and the next-apply time untouched, and taken while an apply is owed it leaves it owed.
  - After an exhausted streak, a fresh `Signal` re-arms the state and the very next quiet period yields `watchPlanApply` — the load-bearing assertion that separates the per-event cap from a loop-level cap.
- **Commit:** `test(reed): cover the watch loop's debounce, coalescing, and retry-cap contracts`

### Card 18: tier-1 tests for the loop driver

- **Context:**
  - `internal/reedengine/watchloop.go`
  - `internal/reedengine/watchdog.go`
  - `internal/reedengine/reapply.go`
  - `internal/reedengine/apply.go`
  - `internal/reedengine/lock.go`
  - `internal/reedengine/overlay.go`
  - `internal/reedengine/lifecycle.go`
  - `internal/reedengine/state.go`
  - `internal/reedengine/spawn.go`
  - `internal/reedengine/render/types.go`
  - `internal/lock/lock.go`
  - `internal/reedengine/reapply_test.go`
  - `CONSTRAINTS.md`
- **Edits:**
  - `internal/reedengine/watchloop_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Extend `internal/reedengine/watchloop_test.go` with driver tests that call the unexported `watchLoop` directly, always with a `context.WithCancel` context the test cancels in a `t.Cleanup`, and always with a `watchTiming` whose durations are single-digit milliseconds so no test is slow and no untagged `time.Sleep(...)` reaches a second.
  Reuse `reapply_test.go`'s fixture shape: a hand-built `Engine` over `t.TempDir()`, a persisted `ReedState`, and a recording `TmuxCmd.execHook`.
  Run `watchLoop` in a goroutine and observe it through the recorded argv, the signal file on disk, and a completion channel; never assert on wall-clock timing beyond "eventually, within a bounded poll".

  Cover:

  - **Never returns while ctx is live, disabled case.** With `Watchdog: "off"`, `watchLoop` does not return within a bounded wait, issues no tmux call, and returns only after the context is cancelled.
  - **Never returns while ctx is live, invalid case.** Identical assertions with `Watchdog: "garbage"` — the header tail's contract is that a config typo parks the loop rather than killing the keepalive, so this must not return an error and must not return at all until cancellation.
  - **Stale signal file removed at start.** With a signal file present before the call and `Watchdog: "on"`, the file is gone shortly after the loop starts.
  - **Poll mode by default.** With `show-options` scripted to report no hook, the loop issues repeated `reapplyLayout` cycles at `PollCycle` and never stats its way into signal-mode behaviour.
  - **Mode promotion.** With `show-options` scripted to return reed's own command string, the loop promotes: after promotion it stops issuing per-cycle `reapplyLayout` calls, and it applies only after a signal file appears.
  - **Never demotes.** After a promotion, scripting `show-options` to return the empty string produces no further probe round trips at all — signal mode never re-probes.
  - **Undecided probe does not guess.** With `reed.lock` held for the first few cycles so every call defers, the mode stays poll and no promotion occurs; releasing the lock and then reporting the hook promotes as normal.
  - **Signal consumed by removal, before the apply.** In signal mode, creating the signal file causes exactly one `select-layout` after the quiet period, and the file is gone before that `select-layout` appears in the recorded argv.
  - **Take-effect boundary.** Rewriting `reed.yaml` on disk while the loop runs changes nothing — assert the loop's behaviour is unchanged and that no config read happens after start (drive it by flipping `e.cfg.Watchdog` in the fixture after the loop has begun and asserting the loop keeps running).
  - **Failures never kill the loop.** With `select-layout` scripted to fail every time, the loop is still running and still responsive after an exhausted streak: a fresh signal file still produces a fresh `select-layout` attempt.
  - **Deferral costs no budget.** With the lock held across the whole quiet period, the loop issues no tmux call and, once the lock is released, still applies for the same pending signal.

  Do not use `exec.Command` and do not add a build tag: every one of these runs against `execHook` and a `t.TempDir()` lock file.
- **Commit:** `test(reed): cover the watch loop driver's modes, signal lifecycle, and survival`

## Batch Tests

`verify: go test ./internal/reedengine/...` runs the whole untagged reed suite.
New coverage is entirely in `internal/reedengine/watchloop_test.go`: card 17 pins the pure state machine against a synthetic clock, card 18 pins the driver against the `TmuxCmd.execHook` seam and a real `t.TempDir()` signal file and lock file.
Batch 2's existing tests must stay green unchanged — this batch adds one new file and touches no shipped file other than that.
The scope stays per-batch: nothing outside `internal/reedengine` compiles against anything added here until batch 4 wires the CLI tail.
