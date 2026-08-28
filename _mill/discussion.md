# Discussion: reed: watchdog daemon

```yaml
task: 'reed: watchdog daemon'
slug: reed-watchdog-daemon
status: discussing
parent: main
```

## Problem

reed computes a correct layout — fixed one-row header, proportional strand stack, `collapsed_strip_rows`/`min_full_rows` budgets, mother/child collapse — and applies it on every reed op.
The moment the operator resizes or moves their terminal window while attached, tmux's own proportional rescale takes over and reed's layout is abandoned, permanently, with nothing watching to re-apply it.
`lyx reed up` from a second connection restores it, so the layout computation is fine;
what is missing is automatic re-application.

**Why now:** the reed sandbox suite's M7 scenario verified the gap live and reproducibly.
Attach at 127x50 gives a correct layout (header height 1);
resizing the client to 100x65 pushes the header to height 6 and stretches the strands to fill;
waiting 4+ seconds changes nothing;
`lyx reed up` restores it;
resizing again to 150x70 breaks it again (header height 3).
Resizing a terminal window is an extremely common operator action, so reed's layout is wrong most of the time in practice.

This activates `manifest/roadmap.md`'s existing **reed: watchdog daemon** Someday item, whose stated scope is two self-heal jobs — automatic pane-reap and resize-geometry reconciliation.
Only the resize half is built here (see **Scope: Out** and the `resize-self-heal-only` decision).

## Scope

**In:**

- A watch loop hosted inside the existing header-pane process (`lyx reed header --blocking`), which today runs `blockForever()` and nothing else.
- A session-scoped tmux `window-resized` hook, set alongside the two existing geometry option pins, whose only job is to touch a signal file.
- A signal file under the worktree's ephemeral `.lyx/` directory, created by the hook and consumed by the watch loop.
- A trailing-edge debounce that coalesces a drag's burst of resize events into a single re-apply.
- A new public engine op that re-plans and re-applies the layout against the live window, under the existing `reed.lock`.
- A polling fallback for multiplexers where the hook could not be installed (psmux).
- A single `watchdog: on|off` key in `reed.yaml`, defaulting to `on`.
- Suppressing the header process's own stderr logging so the watch loop can never paint over the rendered header text.
- Tier-1 tests for every pure part, a `integration && linux` pty test that reproduces M7 end to end, and a new sandbox-suite scenario.

**Out:**

- **Automatic pane-reap**, the roadmap item's other half.
  `planReconcile` already reaps deterministically on every reed op;
  the roadmap's added value there is a policy distinguishing a bug-induced pane from an intentional scratch pane, which does not exist yet and is a design task of its own.
- **Cheapening the reap probe** (the pwsh + `Win32_Process` WMI enumeration the roadmap names as that half's prerequisite).
  It is Windows-only work, unverifiable from this Linux box, and nothing in the resize path calls it.
- **A standalone supervised daemon process** — no PID file, no liveness check, no restart policy, no `Down` teardown change.
  The watcher's whole lifecycle is the header pane's lifecycle.
- **Any new CLI command.**
  No `lyx reed watch`, no `lyx reed watchd`.
  `cmd/lyx/helptree_test.go` and `seamsignature_test.go` must stay green unchanged.
- **The Slack relay** (`reed: daemon Slack relay`, a separate roadmap item explicitly split out so it never blocks the self-heal work).
- **`internal/reedengine/render`'s layout algebra.**
  The watcher changes *when* `render.Rules` is invoked, never what it computes.
- **Reacting to anything but a resize** — no strand-death watching, no focus following, no CC-hook integration.
- **Changing `window-size`, the status-line pin, `mouse`, or `remain-on-exit`.**

## Decisions

### watchdog-lives-in-the-header-pane-process

- Decision: the watch loop runs inside the existing per-session header-pane process, replacing `blockForever()` in `internal/reedcli/header.go`'s `--blocking` tail.
  No new process, no new command, no new lifecycle.
- Rationale: reed already boots exactly one permanent, always-on `lyx` process per session — the header pane, whose entire body today is a sleep loop.
  It is created and healed by `ensureHeaderPaneLocked`, is exempt from every strand-accounting, adoption, split-target and reconcile path, dies with the session, and is already a registered holder of the CLI/Cobra Invariant's interactive-handoff exception.
  It also already holds a fully-resolved `*reedengine.Engine` from `reedcli`'s `PersistentPreRunE`, so it needs nothing told to it.
  A supervised daemon would be a genuine architectural first for this repo (`internal/boardengine` is documented as "one-shot, daemonless", and the `reed-attach-geometry-reconcile` discussion rejected a standalone watcher on the grounds that it is "a daemon reed has deliberately never been") and would owe a PID file, liveness probing, restart policy, teardown in `Down`, and its own sandbox coverage — all to host a loop that the header pane hosts for free.
- Rejected: a separate detached daemon spawned by `Up` (all of the above cost, no behavioural gain);
  no long-lived process at all, with the hook running `lyx reed <verb>` directly (see `debounce-in-the-watcher`).

### window-resized-is-the-event-source

- Decision: the event source is a **session-scoped** tmux hook on `window-resized`, set with `set-hook -t '=<session>:'`.
  Not `client-resized`, not `window-layout-changed`, not SIGWINCH, not a geometry poll.
- Rationale: all four alternatives were probed live on this box (tmux 3.6, real pty, real attaching client).
  On a client resize the hooks fire in the order `client-resized` → `window-layout-changed` → `window-resized`, and `client-resized` reports the **stale** pre-resize window size (127x50 when the client had already become 100x65), so it cannot plan a correct layout.
  `window-layout-changed` is self-triggering: reed's own `select-layout` would re-fire it, giving an infinite loop.
  `window-resized` fires exactly once per settled size, after the window already has the new geometry, on both growth and shrink.
  The hook is session-scoped rather than `-g` on purpose — the tmux server is shared per hub across sibling worktrees, so a global hook would fire every worktree's watcher for every other worktree's resize.
- Rejected: **SIGWINCH in the header process** — attractive because it needs no tmux surface at all and no psmux risk, but disproved live.
  With the header pinned to one row, growing the window (50 → 51, 52, 55, 60) resizes the header (1 → 2, 3, 4, 6 rows — this *is* the M7 bug) and delivers SIGWINCH every time, but **shrinking** (60 → 59, 58, 55, 45, 30) leaves the header at one row and delivers nothing, while the strand budgets below it are silently violated (at 30 rows the bottom strand had been squeezed to 2 rows).
  A watcher that self-heals only on growth is worse than none, because the operator would learn to trust it.
  Also rejected: a `display-message` geometry poll as the *primary* source (a permanent subprocess spawn per session per tick, roughly 23ms per call, for an event that fires a handful of times a day).

### hook-touches-a-signal-file

- Decision: the hook command is the cheapest possible thing — a backgrounded `run-shell -b` that creates/truncates a signal file at `<stateDir()>/reed-resize.signal`.
  The watch loop consumes the signal by **removing the file**, so existence alone is the signal and no timestamp comparison is involved.
  The removal happens *before* the re-apply, so a resize arriving mid-apply re-signals rather than being swallowed.
  **Full lifecycle**, so nothing about the file is left to inference:
  - **At watcher start:** any pre-existing file is removed before the loop begins.
    A stale file is either a leftover from a previous watcher in this session or a resize that happened while none was running;
    both are already answered by the fact that the session boot which starts the watcher applies the layout itself, so consuming the stale signal would only buy a redundant apply.
    Removing it makes the loop's initial state deterministic.
  - **On `watchdog: off`:** `pinGeometryOptionsLocked` removes the file in the same non-fatal block where it unsets the hook, so turning the watchdog off leaves neither a hook that writes nor a file that lingers.
  - **At `Down`:** left alone, and harmless — the file is meaningless without a live session, it lives under the ephemeral `.lyx` tree that the Durable-vs-Ephemeral State Invariant already designates as disposable, and the next watcher start removes it regardless.
    No teardown code is added for it.
- Rationale: the hook must do as little as possible because it fires once per resize step;
  a `touch`-equivalent is roughly a millisecond, while spawning `lyx` is orders of magnitude more.
  The watcher's steady-state cost is then an `os.Stat` (microseconds) per tick and **zero** subprocesses — nothing is spawned until an actual resize happens.
  Existence-as-signal is immune to filesystem mtime granularity, which a modification-time comparison is not.
  The file is ephemeral and per-worktree, so `.lyx/` (via the existing `Engine.stateDir()`) is its only correct home under the Durable-vs-Ephemeral State Invariant, and one signal file per worktree means sibling worktrees on the shared server cannot collide.
  `run-shell` must carry `-b`: without it the tmux **server** blocks while the command runs.
- Rejected: the hook invoking `lyx reed <relayout verb>` directly (a 20-step drag was measured to fire 20 hook events in one second, so this would spawn 20 `lyx` processes all contending for `reed.lock`);
  a unix socket or FIFO the watcher listens on (fully event-driven and no tick at all, but adds an IPC endpoint to `.lyx/` and owes a named-pipe story for Windows, for no measurable gain over a microsecond stat);
  the hook writing a tmux user option the watcher reads back (reading it costs a `display-message` round trip per tick, i.e. exactly the polling cost the signal file exists to avoid).

### hook-set-in-pingeometryoptionslocked

- Decision: the `window-resized` hook is installed in `Engine.pinGeometryOptionsLocked` (`windowsize.go`), alongside the existing `status off` and `window-size latest` pins, with the same non-fatal `logger.Warn`-and-continue treatment.
- Rationale: that function already runs in exactly the two places this hook needs to be set — the boot path and the attach pre-flight — and the attach-pre-flight call exists precisely because boot options never re-apply to an already-up session (`ensureServerLocked` returns early on the healthy already-up path, above the `set-option` block).
  Installing the hook there means a session booted by an older `lyx` picks it up on the operator's next attach, instead of staying unhealed until a manual `down` + `up`.
  It is already the non-fatal, geometry-quality-option block, which is the right severity: `set-hook` and `run-shell` are **not** in `requiredSubcommands` (`probe.go`) and psmux support for both is unverified, so a failure to install must degrade rather than fail the op.
- Rejected: the fresh-boot path only, beside `remain-on-exit`/`mouse` (leaves every already-running session unhealed);
  the watcher installing its own hook at startup (appealing, since the consumer would own its event source, but it puts a tmux write outside the op lock).

### debounce-in-the-watcher

- Decision: trailing-edge debounce with a fixed quiet period in the 150–250ms range, coalescing.
  Every signal restarts the quiet timer;
  the re-apply fires once, after the resizing stops.
  Both the tick interval and the debounce window are compile-time constants, not config.
- Rationale: measured live — a 20-step drag (one size change per 50ms) fires 20 `window-resized` events in one second.
  Applying on each would mean 20 layouts, each immediately invalidated by the next resize and each fighting tmux's own rescale, all serialised through `reed.lock`.
  Trailing-edge collapses that to one apply after the drag settles, and the layout is only wrong *during* the drag, which is when the operator is still dragging and not reading.
- Rejected: leading-edge plus rate limiting (first frame corrects fast, but every mid-drag apply is wasted work);
  a fixed-interval reconcile that applies whenever the observed box differs from the last applied one (simplest, but couples correction latency to the tick and reintroduces the geometry poll).

### reapply-layout-is-a-new-public-engine-op

- Decision: a new public `Engine` method — shape `ReapplyLayout() (<result>, error)` — that re-plans and re-applies the layout against the live window.
  Its body is the existing composition, in the existing order: a **non-blocking** op-lock acquisition (see `watcher-never-blocks-on-the-op-lock`) → `requireSessionLocked` → `loadOrInitStateLocked` → `tmux.listPanes` → the layout apply.
  It persists nothing.
  It applies the layout **only** — it does not issue the trailing `select-pane` (see `reapply-never-moves-focus`).
- Rationale: every piece already exists and the op is structurally identical to `Status()` (`lifecycle.go:1154`), which is the same lock-then-load-then-list shape.
  `applyLayoutLocked` (`apply.go:141`) already queries the live box itself via `liveBoxLocked` and already carries both session-destruction guards — the `len(live) < 2` skip and the `anyPlacedStrand` refusal that stops a zero-pane layout string from destroying every pane in the session.
  Reusing it means the watcher inherits those guards rather than re-deriving them, which matters more here than anywhere else: the watcher fires unattended, without an operator watching the envelope.
  `applyLayoutLocked` mutates no state, so no `SaveState` and no `reed.json` write is involved — the watcher is read-only with respect to persisted state.
- Rejected: shelling out to `lyx reed up` (re-execs the binary, re-probes capability, and re-boots substrate on every resize, from a process that already holds a live engine);
  running `Up`'s full body in-process (does far more work per resize, including `ensureHeaderPaneLocked` on the very pane the watcher is running in);
  exporting `applyLayoutLocked` directly (it documents "assumes the op lock is already held", so an unlocked caller would violate its contract).

### reapply-never-moves-focus

- Decision: the watcher's re-apply issues `select-layout` and stops there.
  It never issues the trailing `select-pane`.
  `applyLayoutLocked` (`apply.go:141`) therefore gains a way to run its layout half without its focus half — a parameter or a sibling that returns after `select-layout` — with every existing caller keeping today's full behaviour, focus included.
- Rationale: `applyLayoutLocked` ends with `select-pane -t focus` (`apply.go:162`), where `focus` comes from the persisted strand table's `Display.Focus` flag via `render/focus.go`, **not** from whichever pane is live-active.
  That is correct for an operator-invoked op — the operator asked reed to render, so snapping to the declared focus strand is the answer to their request — and wrong for an unattended watcher, which fires while the operator is typing.
  Resizing a terminal window would otherwise yank the cursor out of the pane being typed in, mid-keystroke, every single time, turning a layout fix into an input-stealing bug that is far more disruptive than the misdrawn layout it corrects.
  Skipping it costs nothing the operator wants: after a resize, input focus stays exactly where they left it, which is the behaviour they would ask for if asked.
- Rejected: reading the live active pane (`#{pane_active}`) before the apply and restoring it after (two extra round trips per apply, and it still races the operator's own pane switches during the apply window — restoring a pane they just deliberately left);
  accepting the focus steal with a note (the disruption is worse than the defect);
  making it a config key (no operator would choose "steal my cursor on every resize").

### self-apply-does-not-retrigger-and-is-guarded-anyway

- Decision: reed's own `select-layout` does not fire `window-resized`, so no self-trigger loop exists.
  Independently of that, the watcher records the box it last applied against and **skips the apply when a signal arrives and the live box equals the last successfully-applied box.**
- Rationale: this was the one rejection in `window-resized-is-the-event-source` that rested on an untested assumption, since `apply.go`'s own comment documents that a detached over-budget `select-layout` **grows the window** — a real geometry change, which is exactly what `window-resized` reports.
  Probed live and the answer is clean: `select-layout` fired the hook **zero** times in every case — attached, detached, re-applying an identical layout, and the documented detached-grow case where a 60-row layout string applied to a 40-row window grew the window to 60 rows (exit 0, panes 20/20/18) and still fired nothing.
  So `window-resized` tracks *client-driven* window size changes, not layout-driven ones, which is precisely the property that makes it usable and `window-layout-changed` unusable.
  The box-equality guard is kept anyway, and cheaply: it costs one `display-message` the apply path already performs, it breaks any self-trigger loop on a future tmux or on psmux without depending on this probe holding there, and it also suppresses the redundant re-apply when two signals coalesce imperfectly.
  It must compare against the last **successfully-applied** box, never the last observed one, or a failed apply would be skipped on retry.
  The box the guard compares is the one `ReapplyLayout` **returns**, not one the watcher queries for itself — so the result type is concrete: `ReapplyResult{Applied bool, Box render.Box, BoxIsLive bool}`.
  `BoxIsLive` is load-bearing, because `liveBoxLocked` (`windowsize.go:42`) never reports failure: on a round-trip error or a malformed answer it `logger.Warn`s and returns the configured `cfg.Width`/`cfg.Height` pair, which is a perfectly plausible-looking box.
  A degraded query is therefore **not an observation**: when `BoxIsLive` is false the watcher neither updates its last-applied box nor treats the comparison as meaningful, so a fallback box can cause neither a spurious permanent skip (fallback happens to equal the last applied box) nor a spurious re-apply loop (fallback differs from it forever).
  This needs `liveBoxLocked` to expose the `ok` it already computes internally — a sibling returning `(render.Box, bool)`, with today's method delegating to it and discarding the flag, so no existing caller changes.
- Rejected: relying on the probe alone with no guard (correct on tmux 3.6 today, unverified on psmux, and a silent infinite loop is the worst possible failure mode for an unattended loop inside the session keepalive);
  clearing the signal file again after each apply to swallow self-fired signals (would also swallow a genuine resize that arrived during the apply).

### watcher-never-blocks-on-the-op-lock

- Decision: the watcher acquires the op lock **non-blocking**, via a try-lock sibling of `withOpLock` built on the existing `lock.TryAcquireWriteLock` (`internal/lock/lock.go:31`).
  A lock held by someone else is a **deferral**, not an attempt and not a failure: the watcher re-arms the pending-resize state and reconsiders on the next tick, leaving the attempt counter and backoff delay untouched.
  Deferrals are bounded by the same per-event budget in wall-clock terms only — an event whose lock never frees is eventually dropped and left to the next signal, with one log line.
- Rationale: `withOpLock` uses `lock.AcquireWriteLock` (`lock.go:21`), which blocks with **no timeout** — the repo's own R5 measurement records a second `lyx reed status` blocking for 11027ms behind a held lock.
  A watcher blocked there is in a state the failure policy does not describe: it is not retrying, not failing, and not observing signals, and the tier-1 assertion that the loop stays responsive after an exhausted streak could not be written against it.
  Deferring is not merely safe here but actively correct: the thing holding the lock is another reed op, and every reed op ends by re-applying the layout itself — so the work the watcher is waiting to do is about to be done for it.
  Try-lock also removes any possibility of the watcher's own lock wait interfering with an operator-invoked `up`/`add`/`remove`, which is a stronger guarantee than a timeout would give.
- Rejected: blocking with the existing `withOpLock` (unbounded wait inside the session keepalive, and it makes the watcher a lock-contention participant against operator-invoked ops);
  blocking with a stated deadline (needs a new timeout-capable lock API where a try-lock already exists, and still contends);
  running lock-free (the layout apply mutates the live tmux session and races every other reed op — the lock exists for exactly this).

### hook-availability-decides-poll-fallback

- Decision: at startup the watcher determines whether its hook is installed with a single `show-hooks`-class round trip.
  If it is, the watcher runs signal-file-driven with no geometry polling.
  If it is not, the watcher falls back to a slow `liveBoxLocked`-style geometry poll, re-applying only when the observed box differs from the last applied one.
  **The mode is not fixed for the process's life.**
  While in poll mode the watcher re-probes hook availability as part of each poll cycle and promotes itself to signal-driven mode the moment the hook appears;
  signal-driven mode never re-probes and never demotes itself.
- Rationale: this is what makes the design correct on psmux without betting on it.
  `set-hook`/`run-shell` are absent from `requiredSubcommands` and unverified on the Windows port, so the capability must be discovered at runtime rather than assumed or required.
  One round trip at startup is free;
  the alternative — always polling as a safety net — reintroduces the permanent per-session subprocess cost on the platform where hooks *do* work.
  The poll-mode re-probe is free for the same reason: that mode is already making a round trip per cycle, so checking hook presence alongside it costs nothing extra, and it closes a real hole in this design's own migration story — the hook is installed at attach pre-flight, so a watcher that started on a hook-less already-up session would otherwise sit in poll mode forever even after the operator attaches and the hook appears.
  The reverse direction needs no handling: nothing in reed ever removes the hook except `watchdog: off`, which stops the watcher outright.
- Rejected: adding `set-hook`/`run-shell` to `requiredSubcommands` (the capability probe would then refuse to boot reed at all on any multiplexer lacking them, i.e. betting the entire Windows path on unverified psmux behaviour, to gain nothing on Linux);
  always polling in addition to the hook (permanent cost for a case the startup probe can decide once);
  probing once and never again (leaves the attach-pre-flight migration path permanently stuck in the degraded mode).

### watchdog-single-toggle-no-tunables

- Decision: one new `reed.yaml` key, `watchdog`, accepting `on`/`off` and defaulting to `on`, added to both `template_posix.yaml` and `template_windows.yaml` with an `${env:LYX_REED_WATCHDOG:-on}` default in the same shape as `mouse`.
  Debounce window and tick interval stay constants in code.
  **`off` disables the whole mechanism, not just the loop**, in all three of its parts: the watch loop does not start, `pinGeometryOptionsLocked` does not install the hook, and — because `pinGeometryOptionsLocked` is an `*Engine` method and already holds `e.cfg` — it actively **unsets** any hook already present on the session, so flipping to `off` and running any op that reaches that function (a boot, or an attach pre-flight) leaves no orphan hook writing signal files nobody consumes.
  The poll fallback is part of the loop and is disabled with it.
  The unset is non-fatal like every other call in that function.
  **An invalid value is a hard error on exactly one path, and treated as `off` on the other two.**
  The validator itself is a pure `watchdogOption(raw) (bool, error)` mirroring `mouseOption` (`mouse.go:14`) exactly — trimmed, lowercased, `on`/`off` only, everything else including the empty string returning an error — but its three consumers cannot all react the same way, because two of them have no error channel:
  - `ensureServerAndSessionLocked` (`lifecycle.go:176`), the boot path that already validates `mouse` — **hard error**, so `lyx reed up` fails loudly and names the bad value.
    This is the one place an operator reliably sees it.
  - `pinGeometryOptionsLocked` (`windowsize.go:90`), reached from boot and from `AttachArgv`'s pre-flight (`attach.go:80`) — the function returns nothing and is all-non-fatal by contract, so it **takes the unset side**: an invalid value is treated as `off`, the hook is unset rather than installed, and a `logger.Warn` records why.
    This path is only reachable with an invalid value at all when the operator edits `reed.yaml` *after* the session booted, since boot itself would have refused;
    unsetting is the conservative half, because it stops signals rather than starting them, and the operator's next `up` delivers the loud error.
  - the header-pane watch loop in `reedcli/header.go`'s `--blocking` tail — **declines to start**, logs, and falls through to the plain block-forever keepalive.
    It never errors out of the pane.
- Rationale for the split: a hard error in the header tail would kill the keepalive the header pane exists to provide, directly contradicting `watch-loop-failures-are-never-fatal` — a config typo would take down the pane that keeps the session alive, which is far worse than self-heal being off.
  Meanwhile `pinGeometryOptionsLocked` cannot report an error even if it wanted to.
  So exactly one consumer is loud, and it is the one the operator is watching;
  the other two fail safe toward "no watchdog" and say so in the log.
- Rationale: this is new always-on background infrastructure that touches every session on the box, which is a materially different risk profile from the status-line pin whose "no requirement behind it — YAGNI" rejection would otherwise be the governing precedent.
  An operator who hits a watchdog bug needs a way to turn it off that is not "downgrade `lyx`".
  `mouse` is the precedent for a reed behaviour pinned explicitly in both directions with an env override, and it is the precedent for the invalid-value behaviour too: a sibling key in the same file validated two different ways would be the inconsistency, and a silently-ignored typo in a self-heal kill-switch is exactly the failure an operator would not notice until the layout stopped healing.
  `off` has to reach the hook install as well as the loop, because a kill-switch that leaves the hook behind still spends tmux surface on every resize and keeps writing a signal file with no reader — which is both misleading to anyone inspecting the session and a slow leak of `run-shell` invocations.
  Note the existing reconcile semantics: an already-materialised `reed.yaml` keeps whatever value it holds, since config reconcile is key-based and never rewrites a value, so existing hubs adopt the key via `lyx config reconcile` exactly as `debug_log` and `mouse` documented before it.
- Rejected: no key at all (no kill-switch for new always-on infrastructure);
  a `watchdog:` block with `enabled` + `debounce_ms` + tick (two of the three keys have no requirement behind them, and a wrong `debounce_ms` is a support burden with no upside);
  `off` stopping only the loop and leaving the hook installed (orphan hook, orphan signal file, no reader);
  degrading an invalid value silently to `on` (diverges from `mouse`'s validation in the same config file, and hides a typo in the one key an operator reaches for when something is already wrong).

### watch-loop-failures-are-never-fatal

- Decision: every failure inside the watch loop is logged and swallowed.
  The loop never exits, never returns an error to the pane, and never lets the header process die.
  **What is bounded is one event's retries, never the watcher.**
  A single debounced re-apply gets at most a small fixed number of attempts (N, on the order of 3) with an escalating delay between them;
  once those N are spent the event is abandoned with one log line and the watcher goes straight back to waiting for the next signal.
  Both the attempt counter and the escalating delay reset on any successful apply **and** on the arrival of the next resize signal, so the cap is per failure-streak, not cumulative over the process's life.
- Rationale: this is the `geometry-tmux-failures-are-non-fatal-everywhere` Shared Decision applied to the same surface it was written for, and the header pane's entire reason for existing is to be an always-on keepalive that keeps the session (and the substrate the next `add` needs) alive no matter what.
  A watchdog that can kill the header pane would convert a cosmetic layout failure into a session-survival failure — strictly worse than the bug being fixed.
  The per-event cap is what satisfies the Live-Substrate Spawn Observability rule that a retry loop "caps attempt COUNT, not only elapsed time" **literally**, without the cap ever halting self-heal: an unbounded retry on a persistently-failing tmux would spin against the substrate forever, while a bounded one costs at most N attempts and then falls back to the next resize event, which is itself a retry trigger.
  This is what reconciles the cap with "the loop never exits" — the two counters govern different things, and nothing about exhausting a streak stops the watcher from acting on the next signal.
- Rejected: stopping the **loop** after N consecutive failures and falling back to `blockForever()` (the pane survives, but self-heal silently stops for the rest of the session with no way for the operator to notice — this is the alternative the per-event cap above is deliberately not);
  exiting the process on error (corpses the header pane, which is then only healed on the next `up`/`resume`, deliberately breaking the keepalive);
  no retry at all, i.e. a cap of exactly 1 (simplest and constraint-satisfying, but a transient tmux hiccup would then leave the layout wrong until the next resize or reed op, where a bounded retry recovers within seconds);
  capping only the growth of the backoff delay while retrying indefinitely (this is precisely the "only elapsed time" reading CONSTRAINTS.md forbids);
  declaring the constraint inapplicable on the grounds that the watch loop spawns no process (arguable, since it issues only `TmuxCmd` round trips, but weakening an enforced constraint to avoid defining a three-attempt cap is a bad trade).

### header-blocking-tail-discards-logger-output

- Decision: before entering the watch loop, the `--blocking` tail rebinds the logger's stderr sink to a discarding writer via `logger.SetOutput`.
  The durable log sink is unaffected.
- Rationale: the header pane's stdout/stderr **is** its visible screen — `--blocking` paints the rendered header text there and then must never write to it again.
  `internal/logger`'s default stderr threshold is `Warn` (`logger.go`'s `init` sets `levelVar` to `slog.LevelWarn`), and the watcher will reach `Warn` call sites in *already-shipped* code: `liveBoxLocked` logs `Warn` on a failed or malformed window-size query, and `pinGeometryOptionsLocked` logs `Warn` on a failed pin.
  Without this, the first degraded tmux round trip paints a slog line over the operator console.
  Discarding only the stderr half is the right cut: the durable handler is enabled unconditionally at `Info` and above and writes to the hub log file, so nothing is lost for diagnosis — it just stops being *drawn*.
- Rejected: restricting the watch loop to `Debug`/`Info` levels only (does not help, because the offending `Warn` calls are in shipped engine code the watcher calls into);
  appending a shell redirect to the header launch command (shell-dialect-dependent, and `headerLaunchCmd` builds a portable string via `internal/shell`).

### resize-self-heal-only

- Decision: this task ships the resize half of the roadmap item and the host the pane-reap half will later occupy.
  Pane-reap and the reap-probe cheapening are not in scope.
- Rationale: the resize defect is verified, reproducible on demand, and unmitigated.
  Pane-reap is already deterministic on every reed op via `planReconcile` (`reconcile.go`), so the roadmap's added value there is specifically the *policy* distinguishing a bug-induced pane from an intentional scratch pane — a design question with no answer written down anywhere, which would be decided badly if bolted onto this task.
  Its stated prerequisite, cheapening the reap probe, is Windows-only (`proctree_windows.go`'s `Win32_Process` enumeration) and cannot be measured or validated from this Linux box;
  the Linux seam already reads `/proc` directly and is cheap.
  Nothing in the resize path calls the reap probe, so no ordering dependency is violated by deferring it.
- Rejected: shipping both halves per the roadmap item (bundles an unverifiable Windows prerequisite and an unwritten policy into a task whose own half is ready);
  shipping pane-reap without the probe-cheapening prerequisite (knowingly ships the per-poll pwsh + WMI cost the roadmap names as the blocker).

## Technical context

**The header pane, and why it is the host.**

- `internal/reedengine/headerpane.go` builds the launch line: `headerLaunchCmd` composes `<exe> reed header --blocking` through `internal/shell`, and `headerLaunchLine` returns `""` when `underTest` is true.
- `internal/reedengine/lifecycle.go:515` passes `testing.Testing()` as `underTest`, so under **any** `go test` the header pane is left as a bare shell.
  This is deliberate — CONSTRAINTS.md's "Never re-exec `os.Executable()` under `go test`" — and it means **no Go test can exercise a header-hosted watch loop by booting a header pane.**
  The tier-2 test therefore drives the loop in-process (see **Testing**), which needs no re-exec at all.
- `internal/reedcli/header.go` holds the `--blocking` tail: it prints `"\x1b[2J\x1b[H" + text` and calls `blockForever()`, which sleeps in an hour-long loop rather than `select {}` to avoid Go's deadlock detector.
  This is the one envelope-exempt tail the command has;
  everything fallible already runs pre-flight, on the envelope.
- `ensureHeaderPaneLocked` (`lifecycle.go:448`) is the only rebuild path: a header whose keepalive dies is deliberately kept as an enumerable corpse by `planReconcile` and healed — corpse killed, fresh header split back in at the physical top — on the next `Up`/`Resume`.
  So a crashed watcher self-heals on the next reed op, with no supervision code.
- `reedcli`'s `PersistentPreRunE` (`cli.go`) already resolves cwd → location → config → `hubgeom.ReedGeometry` → `*reedengine.Engine` for every verb including `header`, so the watcher needs nothing told to it that the command does not already hold.

**The engine seam the watcher calls.**

- `Engine.applyLayoutLocked` (`apply.go:141`) is the whole re-apply: it skips both tmux calls when `len(live) < 2` or when `!anyPlacedStrand(...)`, then resolves the live box via `e.liveBoxLocked()`, plans via `planLayout`, and issues `select-layout` followed by `select-pane`.
  Both skips are session-survival guards, documented in place as verified live: a layout string enumerating zero panes is accepted by tmux (exit 0) and answered by destroying every pane in the session.
- `Engine.Status()` (`lifecycle.go:1154`) is the structural template for the new op: `withOpLock` → `requireSessionLocked` → `loadOrInitStateLocked` → `tmux.listPanes(session)`.
- `Engine.withOpLock` (`lock.go`) is non-reentrant and is the single chokepoint carrying the told-geometry pre-flight (`validateToldTmuxIdentity`, `validateToldAnchorPath`) plus the post-op lock-compromise check.
  The watcher must acquire it exactly once per re-apply, never nest.
  Its acquisition is `lock.AcquireWriteLock`, which **blocks with no timeout** — the try-lock sibling the watcher needs is `lock.TryAcquireWriteLock` (`internal/lock/lock.go:31`), which already exists and returns `(*FileLock, bool, error)`.
  The watcher's try-lock path must keep both pre-flight validations and the post-op compromise check, since those are the reason `withOpLock` is a chokepoint rather than a bare lock acquisition.
- `applyLayoutLocked`'s trailing `select-pane -t focus` (`apply.go:162`) targets the strand carrying `Display.Focus` in the persisted table (resolved by `render/focus.go`, bottom-most wins on ties), which is unrelated to whichever pane is live-active.
  Splitting the layout half from the focus half is what lets the watcher re-apply without moving the operator's cursor.
- `mouseOption` (`mouse.go:14`) is the shape the new `watchdog` validator copies: a pure, I/O-free `(string) (T, error)` that trims, lowercases, accepts exactly two values, and errors on everything else including the empty string, with the caller performing the tmux round trip.
- `Engine.stateDir()` (`lifecycle.go:33`) returns `<AnchorPath>/.lyx` — the home for `reed.lock`, `reed.json`, and now the resize signal file.
- `Engine.liveBoxLocked` (`windowsize.go:42`) is `display-message -p -t '=<session>:' '#{window_width} #{window_height}'` with a fallback to `cfg.Width`/`cfg.Height`;
  `parseWindowSize` is its pure half and is the model for any new parse the poll fallback needs.
- `Engine.pinGeometryOptionsLocked` (`windowsize.go:90`) is where the hook install belongs.
  Note its existing shape: each `set-option` error is `logger.Warn`-ed and then ignored, and the second pin is attempted even when the first failed.
- `exactSessionWindowTarget(session)` builds the `=<name>:` form. This is load-bearing and not stylistic — tmux prefix-matches a bare `-t` name when no exact match exists, so on the shared per-hub server a bare name issued from one worktree can silently address a prefix-sharing sibling's session (verified live, tmux 3.6).
  The hook's `set-hook -t` must use this form.
- `TmuxCmd.run`/`TmuxCmd.output` (`overlay.go`) are the only exec paths and both auto-prefix `-L <socket>`.
  Every new tmux call goes through them, never a fresh `exec.Command`.
  `TmuxCmd.execHook` is the white-box seam a tier-1 test stubs to drive a composed engine call site against scripted tmux output with no live server.

**Live tmux facts established for this task (tmux 3.6, Linux, real pty, real attaching client).**

| Probe | Result |
| --- | --- |
| `set-hook`, `run-shell`, `show-hooks` in `list-commands` | present on tmux 3.6; **absent from `requiredSubcommands`**, psmux support unverified |
| Attach at 127x50 with `window-size latest` | window becomes exactly 127x50 |
| Live pty resize 127x50 → 100x65 | window follows to 100x65 (M7's premise reproduced) |
| Hook order on that resize | `client-resized` (reports stale 127x50) → `window-layout-changed` (100x65) → `window-resized` (100x65) |
| Session-scoped `set-hook -t '=<session>:' window-resized` | fires; no `-g` needed, so sibling worktrees on the shared server do not cross-trigger |
| `run-shell -b -c <dir> <cmd>` | honours the start-directory (verified via `pwd`) |
| 20-step drag, one size change per 50ms | **20** `window-resized` fires in ~1s |
| Header pinned to 1 row, window grows 50 → 51/52/55/60 | header becomes 2/3/4/6 rows — the M7 bug — and SIGWINCH reaches the header process every time |
| Header pinned to 1 row, window shrinks 60 → 59/58/55/45/30 | header stays 1 row, **no SIGWINCH**, while the bottom strand is squeezed from 15 rows to 2 |
| A hand-built layout string with no checksum prefix | `select-layout` rejects it (`invalid layout`) — layout strings carry a leading checksum |
| `select-layout` while **attached**, changing pane sizes (`even-vertical`) | exit 0, layout changes, **`window-resized` fires 0 times** |
| `select-layout` while attached, re-applying the identical current layout | exit 0, **0 fires** |
| `select-layout` while **detached**, changing pane sizes | exit 0, **0 fires** |
| `select-layout` while detached with an **over-budget** string (60-row layout into a 40-row window) | exit 0, window **grows** 120x40 → 120x60 and panes become 20/20/18 — and still **0 fires** |

**Logging.**

- `internal/logger` (`logger.go`) runs a dual handler: an stderr half gated by `levelVar`, defaulting to `slog.LevelWarn`, and a durable half whose `Enabled` is unconditional at `Info` and above.
  `logger.SetOutput` rebinds the stderr half only.
- The Live-Substrate Spawn Observability constraint applies to the hook install if it is treated as a spawn site;
  the `run-shell` command is executed by the tmux server, not by `lyx`, so the observable lyx-side event is the `set-hook` round trip, not a process spawn.
  The watch loop itself spawns no processes in steady state.

**Config.**

- `LoadConfig` (`config.go`) uses `configengine.LoadOrTemplate`, so reed is on the degrading side of the Config Strictness Invariant — an absent key resolves from the embedded template.
- The two templates (`template_posix.yaml`, `template_windows.yaml`) are separate embedded files selected by build tag (`template_posix.go` uses `!windows`, deliberately not a `_linux` suffix).
  A new key must be added to **both**, in the same `${env:LYX_REED_*:-default}` shape.

**Sandbox suite.**

- `tools/sandbox/SANDBOX-REED-SUITE.md` is an agent-driven scenario document, `M0`–`M25`, run via `sandbox/posix/reed-suite.sh` → `go run ./tools/sandbox reed-suite`.
  M7 (attach) and M14 (attach visual) are the existing operator-assisted visual scenarios and are the shape a new resize scenario should follow.
  `sandbox_coverage_test.go` enforces the Sandbox Suite Coverage constraint via `**Covers:**` tags.

## Constraints

From `CONSTRAINTS.md`:

- **CLI / Cobra Invariant** — no new command is added, so `Short` text, help-tree tests (`cmd/lyx/helptree_test.go`), and seam signatures (`seamsignature_test.go`) are untouched.
  `reed header --blocking` is already a registered interactive-handoff exception;
  its `Long` text should state that the pane also self-heals the layout.
  `reedcli` imports `reedengine`;
  the engine never imports cli or cobra.
- **Told-Geometry Invariant** — `reedengine` is a bound package.
  The watcher derives no coordinates;
  it uses the `Geometry` the engine was already told.
  No new `internal/lyxcwd` import.
- **Durable-vs-Ephemeral State Invariant** — the signal file is never-tracked state and belongs under `.lyx`, at the mirrored subpath of the `_lyx` content it relates to, reached via the existing `Engine.stateDir()` accessor.
  No module derives its own `.lyx` path.
- **Live-Substrate Spawn Observability** — the watch loop spawns no OS process in steady state;
  any tmux round trip it adds goes through `TmuxCmd`, which is already covered.
  A retry/backoff loop must cap attempt **count**, not only elapsed time — satisfied by the per-event attempt cap defined in `watch-loop-failures-are-never-fatal`, which bounds the retries for one debounced re-apply while leaving the watcher itself running.
  The constraint is treated as applying here rather than argued away, even though the loop spawns no process of its own.
- **Shell Mechanics Seam** — the hook's shell command string is built only via `internal/shell`, which is stdlib-only.
  The interface today is `Quote`/`Invoke`/`ReadFile`/`WithEnv` (`shell/shell.go:13`) and has no file-touch primitive, so **this task adds one** — a `Touch(path string) string` method with both a POSIX and a pwsh implementation, alongside the existing four.
  This is a commitment, not a conditional: the hook string needs it, and hand-rolling it at the call site is exactly what the seam forbids.
  Dialect selection: `run-shell` is executed by the **tmux server's** shell, not by the pane shell the seam otherwise models, so the two are not the same thing.
  tmux runs `run-shell` commands under `/bin/sh` on POSIX;
  psmux's equivalent is unverified.
  The rule is therefore to select by GOOS via `shell.ForGOOS()` as the closest available approximation, and to note that only the POSIX dialect is ever exercised in practice today — on Windows the hook cannot be verified to install at all, so that platform runs in poll mode and the string is never executed.
- **Config Strictness Invariant** — reed is a `LoadOrTemplate` (degrading) adopter;
  the new key must exist in both embedded templates.
- **Test Tier Purity Invariant** — no `exec.Command` and no `time.Sleep` ≥ 1s in untagged test files.
  Everything touching a live tmux is `integration`- or `smoke`-tagged.
- **Hermetic Git Test Environment Invariant** — any new test package spawning git calls `gitkit.HermeticGitEnv()` in `TestMain`.
- **Sandbox Suite Coverage** — a new scenario carrying a `**Covers:** reed` tag.
- **Documentation Lifecycle / task-completion rule** (`CLAUDE.md`) — `internal/reedengine/doc.go` is where this package records its load-bearing behavioural assumptions;
  the resize self-heal, the hook, and the SIGWINCH rejection belong there, in the same commit.
  `manifest/roadmap.md`: **no section move.**
  `reed: watchdog daemon` is a **Someday** entry (`roadmap.md:32`), not a Planned one, and CLAUDE.md restricts roadmap movement to completing or adding a *Planned* item;
  roadmap Maintenance documents Planned/Someday → Done on ship, which this is not, since only one of the entry's two halves ships.
  What changes is the entry's **prose, amended in place**: the resize-geometry half is done, the pane-reap half (and its reap-probe prerequisite) remains, and the description "a standalone per-worktree daemon" is corrected — the shape that shipped is a watch loop hosted in the existing header pane, which is the opposite of standalone, and leaving that wording would misdescribe the thing the remaining half must now be built on.

Discovered during discussion:

- **`applyLayoutLocked` moves input focus.** Its trailing `select-pane` targets the persisted-table focus strand, not the live-active pane — acceptable for an operator-invoked op, disruptive for an unattended one.
- **`withOpLock` blocks with no timeout**, but `lock.TryAcquireWriteLock` already exists.
- **`select-layout` does not fire `window-resized`** — verified in all four cases including the documented detached-grow path — so the self-trigger risk that disqualified `window-layout-changed` does not apply to reed's own applies.
- **`internal/shell` has no file-touch primitive**, so the seam must be extended rather than bypassed;
  and `run-shell` is executed by the tmux server's shell, which is not the pane shell the seam models.
- **The header pane's stdout/stderr is its screen.** Anything written there after `--blocking` paints the header text corrupts the operator console.
- **`testing.Testing()` gates the header launch line**, so no Go test can boot a real header-hosted watcher.
- **`window-layout-changed` is self-triggering** — hooking it would make reed's own `select-layout` re-enter the watcher.
- **tmux prefix-matches bare `-t` session names**, so every session target the hook uses must be the exact `=<name>:` form.
- **`run-shell` without `-b` blocks the tmux server.**

## Testing

**Tier 1 (untagged, no substrate).**

- The debounce/coalesce state machine, as a pure function of (signal events, clock) → (apply decisions).
  Drive a synthetic clock: a single signal fires one apply after the quiet period;
  20 signals at 50ms intervals fire exactly one apply;
  a signal arriving during the quiet period restarts it;
  a signal arriving during an in-flight apply schedules exactly one follow-up, not a queue.
  **TDD candidate** — pure decision logic, many edges, no substrate.
- The hook command string built for a given session, signal path, and shell dialect: assert the `=<session>:` exact-target form, the `-b` flag, and correct quoting for both `shell.Posix()` and `shell.Pwsh()`.
  **TDD candidate.**
- The signal-file consume step: existence detected, file removed before the apply, a re-touch during the apply detected on the next tick.
- The hook-availability decision: given scripted `show-hooks`-class output via `TmuxCmd.execHook`, assert the watcher selects signal-driven mode when the hook is present, poll mode when absent, and poll mode when the round trip errors.
- The `watchdog` config key: `on`/`off`/absent/empty/garbage against the `watchdogOption` validator, asserting a hard error for every non-`on`/`off` value exactly as `mouseOption`'s own test table does, plus the embedded-template default for both GOOS variants.
- `watchdog: off` scope: assert the loop does not start, that `pinGeometryOptionsLocked` issues an **unset** rather than an install and removes the signal file, and that the unset failing is non-fatal — all drivable through `TmuxCmd.execHook` with no live server.
- Per-consumer invalid-value behaviour, which is three different answers and therefore three assertions: the boot path returns an error naming the value;
  `pinGeometryOptionsLocked` unsets rather than installs and returns normally;
  the header tail declines to start the loop and does **not** return an error.
  **TDD candidate** — the header-tail case is the one where an obvious implementation propagates the error and kills the keepalive.
- The box-equality guard's degraded case: when `BoxIsLive` is false, assert the last-applied box is not updated and the comparison does not gate the next apply — neither a permanent skip nor a permanent re-apply.
- Signal-file lifecycle: a stale file present at watcher start is removed before the loop begins.
- The box-equality guard: a signal whose live box equals the last successfully-applied box issues no `select-layout`;
  a signal after a *failed* apply is not skipped even though the observed box is unchanged.
  **TDD candidate** — this is the loop-breaker, and the failed-apply case is the one an obvious implementation gets wrong.
- Focus preservation: assert the watcher's apply path issues `select-layout` and **no** `select-pane`, while every existing caller of the full path still issues both.
- The try-lock discipline: with the op lock already held, assert the watcher issues no tmux call, leaves the attempt counter and backoff delay untouched, and reconsiders on the next tick rather than blocking.
- Mode promotion: a watcher started in poll mode promotes itself to signal-driven on the cycle where the hook first appears;
  a signal-driven watcher never demotes.
- `shell.Touch` for both `shell.Posix()` and `shell.Pwsh()`, including quoting of a path containing spaces.
- `ReapplyLayout`'s guard inheritance: with fewer than two live panes, and with no strand owning a present pane, assert no `select-layout` is issued.
  This is the guard that keeps an unattended watcher from destroying a session's entire pane set.
  **TDD candidate.**
- The retry/backoff contract from `watch-loop-failures-are-never-fatal`, which is now a real contract to assert against: a re-apply that keeps failing is attempted exactly N times and no more;
  the delay between attempts escalates;
  the attempt counter and the delay both reset after a successful apply, and again on the arrival of a fresh resize signal;
  and — the load-bearing assertion — the loop is still running and still responsive to the next signal **after** a streak has been exhausted.
  **TDD candidate** — this is what separates the per-event cap from the rejected "stop the loop after N failures".

**Tier 2 (`//go:build integration && linux`, live tmux, real pty).**

The acceptance evidence is a live reproduction of M7, not source review.
`internal/reedengine/attachgeometry_integration_test.go` already carries a `/dev/ptmx`-based pty harness (`startInPTY`, built on `golang.org/x/sys/unix`'s `TIOCSPTLCK`/`TIOCGPTN`/`TIOCSWINSZ`) in this same package — reuse it rather than writing a second one.
The `testing.Testing()` obstacle is sidestepped by running the watch loop **in-process**, as a goroutine in the test against a real session, rather than by booting a header pane that would re-exec the test binary.

The test must:

1. Boot a real reed session with a header pane and at least two strands.
2. Attach a pty client of a known size and let the layout settle to the correct, planned string.
3. Start the watch loop against that session.
4. **Grow** the window via `TIOCSWINSZ` + `SIGWINCH` and assert the layout self-heals to the string planned for the new box within a bounded wait — specifically that the header returns to `header.height_rows`.
5. **Shrink** the window and assert the same.
   This case is non-negotiable: it is the one SIGWINCH misses entirely, and a watcher that passes only the grow case is the failure mode this task must not ship.
6. Drive a burst of size changes in rapid succession and assert the layout converges to the final size, with the apply count consistent with coalescing rather than one-per-event.
7. Cover the degraded path: with the hook uninstallable, the poll fallback still converges.
8. Assert the loop survives an induced tmux failure (e.g. a round trip against a killed session) without exiting.
9. Assert the operator's live-active pane is unchanged across a resize-driven re-apply — the focus-steal regression, which only a real session can demonstrate.
10. Assert no self-trigger loop: after the watcher's own apply settles, no further apply occurs without a new client resize (the live counterpart of the box-equality guard and of the `select-layout`-fires-nothing probe).

Platform gating follows the existing file's precedent — Linux-only, with a stated reason, since the pty harness has no portable equivalent and psmux's behaviour under a real pty is unverified anywhere in this repo.

**Sandbox suite.**

Add a scenario to `tools/sandbox/SANDBOX-REED-SUITE.md` in M7/M14's operator-assisted shape, carrying a `**Covers:** reed` tag: attach, confirm the layout, resize the terminal window by hand in both directions, and confirm the layout re-applies without running any `lyx` command.
This is the check that would have caught M7, and it is the one that keeps catching it.

**Operator acceptance (outside the suite).**

Attach with `lyx reed attach`, drag the terminal window larger and smaller, and confirm the header stays one row and the strand budgets hold, with no manual `lyx reed up`.

## Q&A log

- **Q:** Where does the watchdog live — inside the existing header-pane process, a separate detached daemon, or no long-lived process at all? **A:** Inside the existing header-pane process. **Why:** reed already boots exactly one permanent `lyx` process per session whose entire body is a sleep loop, with lifecycle, healing, and a resolved engine already in place;
  a supervised daemon would be an architectural first for this repo and would owe a PID file, liveness, restart policy, and `Down` teardown for no behavioural gain.
- **Q:** How does the watchdog learn about a resize — tmux hook, polling, or hooks made mandatory? **A:** A session-scoped `window-resized` hook, with polling as the fallback where the hook cannot be installed. **Why:** `client-resized` reports stale geometry and `window-layout-changed` is self-triggering (both verified live);
  making hooks mandatory would bet the entire Windows path on unverified psmux behaviour, since `set-hook`/`run-shell` are absent from `requiredSubcommands`.
- **Q:** Could the header process just use its own SIGWINCH and avoid tmux hooks entirely? **A:** No — rejected on live evidence. **Why:** with the header pinned to one row, growing the window delivers SIGWINCH every time but **shrinking never does**, while the strand budgets below are silently violated;
  a watcher that self-heals only on growth is worse than none.
- **Q:** Does the task ship both roadmap halves (resize + pane-reap), or resize only? **A:** Resize only, with the host built so pane-reap can land on it later. **Why:** `planReconcile` already reaps deterministically on every reed op, so the roadmap's added value there is an unwritten intent policy;
  and its stated prerequisite — cheapening the pwsh + WMI reap probe — is Windows-only work unverifiable from this Linux box.
- **Q:** What does the watchdog do on a resize — a new in-process engine op, shell out to `lyx reed up`, or a full reconcile? **A:** A new public engine op composing existing pieces. **Why:** verified that every part already exists and the op is structurally identical to `Status()`;
  `applyLayoutLocked` already resolves the live box and already carries both session-destruction guards, which matters most for a caller that fires unattended.
- **Q:** How is the resize burst handled? **A:** Trailing-edge debounce, ~150–250ms quiet period, coalescing. **Why:** a 20-step drag was measured to fire 20 hook events in one second;
  applying on each would serialise 20 immediately-invalidated layouts through `reed.lock`.
- **Q:** How does the hook reach the in-pane watcher? **A:** The hook touches a signal file in `.lyx/`; the watcher consumes it by removing it, on a cheap stat tick. **Why:** near-zero steady-state cost with no subprocess until an actual resize;
  a hook that invokes `lyx` directly would race `reed.lock` once per drag step, and a socket/FIFO owes a Windows named-pipe story for no measurable gain.
- **Q:** Where is the hook set? **A:** In `pinGeometryOptionsLocked`, alongside the two existing pins. **Why:** it is already the non-fatal geometry-option block, and it already runs both at boot and in the attach pre-flight — which is exactly the established migration path for the "session booted by an older lyx" case, since boot options never re-apply to an already-up session.
- **Q:** Config surface? **A:** A single `watchdog: on|off` key, default `on`, no tunables. **Why:** new always-on infrastructure touching every session warrants an operator kill-switch — a different risk profile from the status-line pin's YAGNI rejection — while `debounce_ms` and tick have no requirement behind them.
- **Q:** Failure policy in the loop? **A:** Log-and-continue, always non-fatal, with backoff. **Why:** matches the `geometry-tmux-failures-are-non-fatal-everywhere` Shared Decision, and the header pane exists to stay alive as a keepalive — a watchdog able to kill it would turn a cosmetic failure into a session-survival failure.
- **Q:** What is the acceptance evidence? **A:** Tier 1 for the pure parts, plus a live pty test reproducing M7 in both directions, plus a new sandbox scenario. **Why:** M7 was only ever caught by running the reed suite live, so operator-acceptance-only would repeat the exact gap that let it go unnoticed;
  the sandbox scenario also satisfies the Sandbox Suite Coverage constraint directly.
- **Q:** What does the "capped attempt count" actually bound, given the loop is also said never to exit? **A:** Bounded retries per debounced event — at most N attempts with escalating delay, counter and delay resetting on a successful apply or on the next resize signal — while the watcher itself never exits. **Why:** it satisfies the Live-Substrate Spawn Observability rule literally (a bounded count, not merely bounded elapsed time) without self-heal ever silently stopping;
  what ends after N is this event's retries, not the watcher, which is exactly what distinguishes it from the rejected "stop the loop after N consecutive failures".
- **Q:** The re-apply path ends in `select-pane` — does an unattended watcher steal the operator's cursor on every resize? **A:** Yes, so the watcher's apply skips the focus half entirely. **Why:** `select-pane` targets the persisted-table focus strand rather than the live-active pane, which is right when an operator asked for a render and wrong mid-keystroke;
  save-and-restore was rejected because it costs two extra round trips and still races the operator's own pane switches.
- **Q:** Does reed's own `select-layout` re-fire `window-resized`, giving the self-trigger loop that disqualified `window-layout-changed`? **A:** No — 0 fires in all four probed cases, including the detached over-budget apply that genuinely grew the window 40 → 60 rows. A box-equality guard is kept anyway. **Why:** the probe settles tmux 3.6 but not psmux, and a silent infinite loop inside the session keepalive is the worst available failure mode, so the guard is cheap insurance on a round trip the apply path already makes.
- **Q:** What does `watchdog: off` actually disable, and what happens to a hook already installed? **A:** Everything — loop, poll fallback, and hook install — and `pinGeometryOptionsLocked` actively unsets an existing hook on the next boot or attach pre-flight. **Why:** a kill-switch that leaves the hook behind keeps writing a signal file no one reads;
  the function is an `*Engine` method and already holds `e.cfg`, so it can see the toggle.
- **Q:** How does the watcher take the op lock, given `withOpLock` blocks with no timeout? **A:** Non-blocking try-lock via the existing `lock.TryAcquireWriteLock`; a held lock is a deferral, not an attempt and not a failure. **Why:** an 11-second block is recorded in this repo's own measurements, and a blocked watcher is in a state the retry contract cannot describe;
  deferring is also correct on the merits, since whatever holds the lock is a reed op that ends by re-applying the layout anyway.
- **Q:** Is the hook-availability mode fixed for the process's life? **A:** No — poll mode re-probes each cycle and promotes itself when the hook appears. **Why:** this design installs the hook at attach pre-flight, so a one-shot probe would strand a watcher started on an already-up session in poll mode permanently.
- **Q:** Is an invalid `watchdog` value a hard error or a silent degrade? **A:** Hard error, validated exactly like `mouse`. **Why:** two sibling keys in the same config file validated differently is the inconsistency, and a silently-ignored typo in a self-heal kill-switch surfaces only when self-heal has already stopped working.
- **Q:** Does the shell seam need extending for the hook string? **A:** Yes — commit to a `Touch` primitive with POSIX and pwsh implementations, dialect selected by `shell.ForGOOS()`. **Why:** the interface has no file-touch today and hand-rolling at the call site is what the Shell Mechanics Seam forbids;
  note that `run-shell` runs under the tmux server's shell, not the pane shell, and that only the POSIX dialect executes in practice since Windows runs in poll mode.
- **Q:** The `watchdog` key has three consumers but only one has an error channel — what does an invalid value do on the other two? **A:** Hard error on the boot path only;
  `pinGeometryOptionsLocked` treats it as `off` and unsets the hook;
  the header tail declines to start the loop and keeps blocking. **Why:** a hard error in the header tail would kill the keepalive the pane exists to provide, contradicting `watch-loop-failures-are-never-fatal`, and `pinGeometryOptionsLocked` returns nothing and cannot report one — so the loud path is the one the operator is watching, and the other two fail safe toward "no watchdog".
- **Q:** `liveBoxLocked` never reports failure — it returns the configured box on a degraded query. Does that break the box-equality guard? **A:** Yes, so the guard consumes the box from `ReapplyResult{Applied, Box, BoxIsLive}` and treats a fallback box as *not an observation*. **Why:** otherwise a fallback that happens to equal the last applied box would skip forever, and one that differs would re-apply forever;
  `liveBoxLocked` already computes the `ok` flag internally and only needs to expose it.
- **Q:** What is the signal file's lifecycle outside the consume step? **A:** Removed at watcher start, removed alongside the hook unset on `watchdog: off`, left alone at `Down`. **Why:** removing at start makes the loop's initial state deterministic and costs nothing, since the boot that starts the watcher applies the layout anyway;
  at `Down` the file is meaningless without a session and lives in the ephemeral tree, so adding teardown code for it would be ceremony.
- **Q:** Does `manifest/roadmap.md` move? **A:** No section move — the Someday entry's prose is amended in place. **Why:** CLAUDE.md limits roadmap movement to Planned items, this is a Someday entry, and only one of its two halves ships;
  the entry's "standalone per-worktree daemon" wording is corrected, since the shape that shipped is the opposite of standalone and the remaining half will be built on it.
- **Q:** Anything blocking the header pane from hosting the loop? **A:** Two things, both handled. **Why:** `testing.Testing()` gates the header launch line so no Go test can boot a real header-hosted watcher — the tier-2 test runs the loop in-process instead;
  and the pane's stderr is its screen, so the `--blocking` tail discards the logger's stderr half (the durable sink keeps everything) or the first degraded tmux round trip paints a slog line over the operator console.
