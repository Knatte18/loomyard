# Discussion: Replace reed's header pane with a status-line and Selvage

```yaml
task: Replace reed's header pane with a status-line and Selvage
slug: reed-header-selvage
status: discussing
parent: main
```

## Problem

reed's single header pane does three unrelated jobs at once.
It renders identity text ("hub: …") once at pane launch;
it keeps the tmux session from dying, because tmux destroys a session the moment its last window loses its last pane;
and it hosts the resize watchdog daemon, because `internal/reedcli/header.go`'s `--blocking` branch calls `eng.Watch(ctx)` on its way to blocking forever.
The third job has no home of its own — it runs only because the header pane's process happens to call it.

Conflating the three is why ~230 lines of pane-lifecycle machinery exist across `apply.go` (fixed top band), `reconcile.go` (reap exemption plus the `headerAlive` authorization), `spawn.go` (special split-target selection), and `lifecycle.go` (creation, corpse detection, up/resume healing, the even-vertical split retry).
It is also why the pane dies on a stray Ctrl-C: `lyx reed header --blocking` runs as the pane's own root process with no signal handling, and nothing brings it back until the next `up`/`resume`.

Why now: the design is already written and recorded (`manifest/designs/reed-header-selvage.md`, roadmap Planned item 1), the watchdog it has to rehome is already Done, and the Someday item `reed: born-as-strand for operator, watchdog, and orchestrator sessions` is blocked on reed's pane lifecycle being solid.

## Scope

**In:**

- Identity content moves from the header pane to tmux's own native status-line (`status on`, `status-position bottom`), rendered through the existing `tokenvocab` pipeline.
- A new `worktree` token in `internal/tokenvocab`, plus the `WorktreeName` field on `reedengine.Geometry` and `hubgeom.ReedGeometry` needed to feed it.
- The keepalive pane is replaced by **Selvage**: an ordinary shell pane pinned to the bottom, one row tall, in the same single window, launched with `cfg.Shell` and no lyx command of its own.
- `internal/reedengine/render`: the fixed band flips from top to bottom — `render.Header` becomes `render.Selvage`, `bandHeader` becomes a bottom splice, `planCells`' stack-box math is rewritten for a bottom band, and `FixedHeightPins`' band pin follows it.
- `ReedState.HeaderPaneID` becomes `SelvagePaneID`; `reconcile.go`'s exemption/authorization, `spawn.go`'s split-target choice, and `lifecycle.go`'s ensure/heal path all retarget onto it and onto a bottom split.
- `pinGeometryOptionsLocked` flips from `status off` to `status on` + `status-position bottom` + the rendered `status-left`, and the attach chain's reserved-row accounting follows (`readStatusRowsLocked` already returns 1 for `on`).
- The watchdog moves out of every pane into a detached, single-instance, **per-hub** background process, driven by a new blocking `lyx reed watchdog --hub-path <abs>` verb, spawned by `up`/`resume`/`attach` and watching every live session on the hub's socket.
- `internal/reedengine/geometry.go` gains `Geometry.WorktreeName`, and both tellers (`hubgeom.ReedGeometry`, `standalonegeom.ReedGeometry`) fill it;
  `geometry.go`'s "eight-field struct" doc comment and its `RepoName`/`HubPath` "header pane's … token" field comments are corrected in the same commit.
- `lyx reed header` is replaced by `lyx reed statusline` (envelope-returning preview only, no `--blocking`).
- reed.yaml: the `header:` block is replaced by `status_line:` (template) and `selvage:` (height_rows).
- Docs in the same commit: `manifest/designs/reed-header-selvage.md` rewritten from "Planned, not yet built" to what shipped, `manifest/roadmap.md` item moved to Done, `internal/reedengine/doc.go`'s package doc updated.

**Out:**

- Reed window support — Selvage stays in the same single window as every strand; the Someday `reed: own-window strand anchoring` item is untouched, and `render.AnchorOwnWindow` stays deferred and still rejected by `Rules`.
- A machine-wide watchdog, an OS-level service, or a launch-agent/systemd unit — the daemon stays a detached child process.
- Any change to the strand height policy (`height.go`'s shrink/collapse/clamp rules), to focus resolution, or to the layout checksum.
- Any change to `internal/shuttleengine`, webster, loom, or any consumer of reed's `Display` JSON contract — `Display`'s on-disk keys do not change.
- Making Selvage immortal against `reed down`: a session's life is bounded to its worktree's life, and `down` killing Selvage is correct.
- A migration path for old `reed.json` files carrying `headerPaneId` (see the decision below — the field is simply not read).

## Decisions

### three-way-split-lands-as-one-task

- Decision: all three splits — status-line, Selvage, per-hub daemon — land in this task.
- Rationale: they are one roadmap item, and they are not separable.
  Making Selvage "just an ordinary shell" deletes the only process that calls `eng.Watch(ctx)`;
  shipping Selvage without rehoming the daemon silently turns the watchdog off for every worktree.
  The status-line half is likewise coupled: flipping `status off` → `status on` changes the window's usable row count, which is exactly the budget the band layout math consumes.
- Rejected: status-line + Selvage now, daemon later — this is the precise trap the design doc records an earlier draft falling into.

### status-line-content-and-pins

- Decision: reed sets, per session, `status on`, `status-position bottom`, `status-left <rendered text>`, `status-right ""`, and `status-left-length` large enough for the rendered text (tmux's default is 10, which would truncate).
  Any `#` in the rendered text is escaped to `##` before it reaches `status-left`, since tmux expands `#{…}`/`#[…]` inside a status string.
  These are set in `pinGeometryOptionsLocked` (`windowsize.go`), session/window-targeted like today's pins, and each failure stays non-fatal per the existing `geometry-tmux-failures-are-non-fatal-everywhere` decision.
- Rationale: `pinGeometryOptionsLocked` already runs on both the paths that need it — boot (`lifecycle.go`) and the attach pre-flight (`attach.go`) — so a session booted by an older lyx picks the new status-line up on the operator's next attach, exactly as the hook install already does.
  The status-line is not a pane: it cannot take focus and cannot be typed into, which is what makes it the right home for text that was never interactive.
- Rejected: a global (`-g`) set — a session- or window-scoped value from the operator's own `~/.tmux.conf` silently wins over a global set while `set-option` still exits 0 (already live-verified for the existing pins).
  Also rejected: `status-format[0]`, which would bypass `status-left-length` but hands the whole format string's escaping to reed for no gain here.

### reserved-row-accounting-follows-the-pin

- Decision: the attach chain keeps computing its told box as `rows - reserved` from `readStatusRowsLocked`;
  no change beyond the pin flip.
  `reservedRowsFromStatus` already maps `"on"` → 1, and `AttachArgv` already floors `reserved` at `rows-1`.
- Rationale: the machinery for a status-line consuming rows is already written and tested;
  the only thing that changes is which value the readback returns.
- Rejected: hard-coding `reserved = 1` now that reed owns the option — the readback is what makes an operator's multi-line status config degrade gracefully instead of mis-sizing every layout.

### selvage-is-a-bottom-band-not-a-strand

- Decision: Selvage stays outside `ReedState.Strands`, exactly as the header is today (`header-is-not-a-strand`), and is injected at the `render.Params` seam as `render.Selvage{PaneID, HeightRows}`.
  `render.Header` is renamed to `render.Selvage` and the `Params.Header` field to `Params.Selvage`;
  the band cell is emitted **last** in the layout body rather than first, and the stack box starts at `box.Y` instead of below the band.
- Rationale: the band is a fixed row budget, not a participant in parent-chain ordering, height splitting, or focus resolution — everything that made `header-is-not-a-strand` right still holds.
  Renaming rather than adding a position enum keeps the policy layer honest: there is exactly one band and it is at the bottom.
- Rejected: a `Position` enum on the band type (YAGNI — nothing wants a top band any more);
  modelling Selvage as a `Strand` with a new anchor (it would then enter `orderStack`, `focusTarget`, `stackHeights`, and `isAncestor`, all of which must ignore it).

### band-clamp-and-degenerate-cases-carry-over-unchanged

- Decision: `clampHeaderHeight` is renamed with the band (`clampBandHeight`) and keeps its semantics verbatim — the band yields rows first so the strand stack keeps its `MinFullRows` floor, and the band is never starved below 1 row.
  The one-row divider between band and stack stays budgeted before the clamp.
  The sole-band branch (no strand placed → the band claims the whole box as a bracket-less single cell) stays, since it is what keeps tmux from being handed a zero-height cell inside a group.
- Rationale: those rules are about a fixed-height band coexisting with a stack, not about which edge the band sits on;
  the design doc's "single-pane degenerate case needs no special-casing" refers to tmux's own tiling, and does not remove reed's need for the bracket-less sole-cell string it already emits.
- Rejected: dropping the sole-band branch on the theory that tmux tiles it anyway — reed still has to emit a layout string, and a zero-height cell inside a group is mishandled by the real multiplexer.

### selvage-splits-at-the-bottom

- Decision: `splitHeaderPaneAtTopLocked` becomes `splitSelvagePaneAtBottomLocked`: it splits **below** the physically bottom-most pane (`paneIDsByTop`'s last entry), dropping tmux's `-b` flag, and keeps the existing even-vertical re-tile retry verbatim.
  The pane is launched with `e.cfg.Shell` as its trailing command argument, the same way `new-session` already launches the session's first pane.
- Rationale: the physical-position requirement is symmetric — `render.Rules` emits the band cell last and `paneIDsByTop` resequences by `pane_top`, so a Selvage pane that is not physically bottom-most would invert cell heights on the very first `select-layout`.
  The retry exists because a one-row pane cannot be split at all, which is just as true at the bottom as at the top.
- Rejected: splitting from a strand pane with `-v` and no position constraint (loses the physical-order guarantee);
  keeping the `-b` split and reordering cells instead (moves a live-verified constraint into arithmetic nobody can check by looking at the window).

### selvage-runs-reeds-configured-shell

- Decision: Selvage's shell is `reed.yaml`'s existing `shell:` key (`bash` on POSIX, `pwsh` on Windows by default, `LYX_REED_SHELL`-overridable) — not `$SHELL`, not a hard-coded `bash`.
  This closes the design doc's open item.
- Rationale: `new-session` already launches the session's first pane with `e.cfg.Shell`, so this is the existing precedent rather than a new axis;
  it is already cross-platform and already operator-overridable through one documented key.
- Rejected: `$SHELL` (absent or meaningless on Windows, and invisible to reed.yaml);
  a fixed `bash` (wrong on Windows, and silently ignores a key the operator already set).

### selvage-needs-no-signal-handling

- Decision: no signal-handling code is written for Selvage;
  it is a plain interactive shell and nothing else.
- Rationale: live-confirmed during design — an interactive shell ignores SIGINT at its idle prompt, and Ctrl-C to a foreground job inside it kills only that job.
  It still dies cleanly on `reed down`'s pty-close/SIGHUP path, which is the correct behaviour.
- Rejected: a small custom keepalive binary with signal traps (reintroduces exactly the "the pane is a lyx process" coupling this task removes, and makes the pane non-typeable again).

### selvage-is-deliberately-typeable

- Decision: Selvage is a real, usable terminal — the always-on control terminal for running `lyx`/`reed` commands (e.g. `lyx reed add`) against the worktree — and reed never sends keys to it, never clears its screen, and never writes to it.
- Rationale: being typeable is the point, not a residual flaw;
  the header pane's ED2/ED3 screen-clear payload exists only because it had to display text, which it no longer does.
- Rejected: keeping a screen-clear or a banner write (it would fight the operator's own prompt).

### state-field-rename-with-no-migration

- Decision: `ReedState.HeaderPaneID` (`json:"headerPaneId,omitempty"`) becomes `SelvagePaneID` (`json:"selvagePaneId,omitempty"`).
  An old state file's `headerPaneId` is simply not read — no compatibility shim, no dual-read.
- Rationale: the degrade is already correct and needs no code.
  On the first `up` after the upgrade `SelvagePaneID` is empty, so `ensureSelvagePaneLocked` creates Selvage at the bottom;
  the old header pane is then an untracked alive pane, and `planReconcile`'s reap — authorized by an alive Selvage — kills it on the same pass.
  The operator sees one stale pane for less than one op.
- Rejected: reading `headerPaneId` as a fallback (carries a dead field name through the codebase forever to save one reap);
  a migration step in `loadOrInitStateLocked` (same cost, plus a code path nothing will exercise again).

### statusline-verb-replaces-header-verb

- Decision: `lyx reed header` is removed and `lyx reed statusline` takes its place: envelope-returning preview of the rendered text, no `--blocking` flag, keeping `clihelp.SkipStencilSeedAnnotation`.
  `internal/reedcli/header.go` is renamed to `statusline.go`;
  `headerBlockingPayload`, `headerWatch`, `headerPark`, and `blockForever` are all deleted.
- Rationale: the verb's remaining job is previewing the template after an edit, which the default (non-blocking) mode already did.
  With the text now living in a tmux option that `pinGeometryOptionsLocked` rewrites on every boot and every attach, the preview stays useful and the "the running pane keeps its old text" caveat in today's help goes away.
  The name does not collide with `lyx reed status`, which reports session state.
- Rejected: keeping the `header` name (names the thing that no longer exists);
  deleting the verb outright (an operator editing `status_line.template` has no way to see the rendering before booting).
- Note for the plan: `--blocking` was reed's narrow entry in CONSTRAINTS.md's CLI/Cobra Invariant interactive-handoff exception list (`reedengine attach`/`header --blocking`).
  That list entry is replaced, not just deleted — see the daemon decision below.

### watchdog-is-one-detached-process-per-hub

- Decision: the watchdog becomes a detached background process, one per hub, hosted by a new blocking `lyx reed watchdog` verb.
  It is spawned by `reed up` via `exec.Cmd` + `proc.Detach`/`DetachBreakaway` (the pattern `internal/vscode` and `internal/fabricengine` already use), and it is never a tmux pane.
- Rationale: `eng.Watch(ctx)` needs no tty and no pane — it only issues tmux commands against a socket from outside.
  Per-hub matches a boundary that already exists: `ServerName`/`SocketKey` is keyed on hub path (`hubgeom.ReedGeometry`), so a per-hub daemon only ever talks to the one socket that hub already owns.
- Rejected: per-machine (must multiplex across every hub's socket, must discover hubs appearing and disappearing, needs its own login-time lifecycle, and makes one crash take reconcile down for every hub at once);
  per-worktree-session (that is what we have, minus the pane — it would just move the same N processes somewhere else).

### daemon-invocation-contract

- Decision: the verb is `lyx reed watchdog --hub-path <abs>`, and the hub path is **told**, never resolved from cwd.
  `watchdog` opts out of `internal/reedcli/cli.go`'s `PersistentPreRunE` by extending its existing early-return guard (`cmd.Name() == "reed"` becomes `cmd.Name() == "reed" || cmd.Name() == "watchdog"`);
  the verb builds nothing from `lyxcwd.Resolve` and never touches `c.eng`.
  The spawn sets `cmd.Dir` to the hub path explicitly, and leaves stdin/stdout/stderr nil so no parent handles are inherited — the same shape `boardengine.spawnSync` and `fabricengine`'s detached push already use.
- Rationale: the process is per-hub and outlives the worktree that spawned it — `errWorktreeRootGone` is already a live, handled case — so a cwd-bound, single-worktree `Geometry` would be exactly the wrong identity to hand it.
  Both existing detached-spawn precedents pass the path explicitly (`internal/boardengine/spawn.go:28` `--board-path`, `internal/fabricengine/spawn.go:66` `--weft-path`), so `--hub-path` is the established shape rather than a new one.
  Pinning `cmd.Dir` to the hub matters on Windows, where a held cwd handle on a worktree directory blocks that directory's deletion — a daemon squatting on the worktree it was spawned from would break `fabric` teardown.
- Rejected: letting `watchdog` run the normal `PersistentPreRunE` and reading the hub off the resolved `Location` (the daemon would then refuse to start outside a worktree, and would hold a geometry it must not use);
  a `clihelp` annotation for the opt-out (a new annotation for one command, where the guard already has exactly this shape for the group command);
  inheriting the spawning process's cwd (the Windows deletion block above, plus a daemon whose cwd is a directory it has no relationship to).
- Note for the plan: `watchdog` is a reed verb without an engine, so it must not be reached through `c.eng` anywhere;
  it builds its own per-worktree engines during discovery, as the next decision describes.

### daemon-single-instance-via-hub-lockfile

- Decision: single-instance is enforced by `lock.TryAcquireWriteLock` on `filepath.Join(fabricengine.HubScratchDir(hub), "reed-watchdog.lock")` — `HubScratchDir` is the already-declared accessor for `<hub>/_board/.lyx` (`internal/fabricengine/junctionnames.go:149`), the same hub-anchored ephemeral tree `HubLogsDir` is a sibling inside.
  The path is computed in `internal/reedcli` (which may import `fabricengine` directly, as `hubgeom` already does) and told to the daemon, never derived inside `reedengine`.
  A daemon that cannot take the lock logs at Info and exits 0, so a racing spawn costs one short-lived process and nothing else.
- Rationale: `HubScratchDir` already exists and is already the declared home for hub-level ephemeral state, so the Durable-vs-Ephemeral State Invariant's "no engine derives its own `.lyx` path" holds without a new accessor.
  The lock is itself the check, so there is no race between probing and spawning.
  `internal/lock` already owns advisory file locking in this repo, and `TryAcquireWriteLock`'s non-blocking half is exactly the "somebody else already has it" answer this needs.
- Rejected: a PID-file liveness check with `proc.IsAlive` as the primary gate (racy between check and spawn — the lock is the check);
  a tmux-hosted daemon in a hidden window (reintroduces the pane coupling, and a hidden window pops into view the instant the visible one loses its last pane, live-confirmed during design);
  a lock under the spawning worktree's own `.lyx` (it is per-worktree state naming a per-hub process, and it vanishes when that one worktree is torn down).

### daemon-spawn-attempted-by-up-resume-and-attach

- Decision: `up`, `resume`, and `attach` each attempt the spawn unconditionally, exactly as they each already call `pinGeometryOptionsLocked`.
  `down` never spawns.
  A spawn attempt against a live daemon costs one short-lived child that fails the lock and exits 0.
- Rationale: `up` alone is not enough.
  A daemon that crashes while sessions stay alive would otherwise leave those sessions unwatched until someone happened to run `up` again — and the status-line half of this very task relies on `attach`/`resume` back-filling through `pinGeometryOptionsLocked`, so making the daemon's coverage narrower than the pins' would be an unexplained asymmetry between two mechanisms that heal on the same three paths.
  `attach` is the path an operator takes when something looks wrong, which is exactly when a dead daemon should come back.
- Rejected: `up` as the sole spawn site (leaves a crash unrepaired for as long as the operator keeps attaching rather than re-upping);
  a supervisor process watching the watchdog (a second daemon to keep alive, for a process whose own restart is one lock try);
  respawning from the watch loop itself (it is the thing that died).

### daemon-idle-exit-timings-are-fixed-constants

- Decision: the daemon's discovery cycle and its idle-exit grace are two new fixed constants declared alongside the existing `watchdog*` timings in `internal/reedengine/watchdog.go`: `watchdogHubDiscoveryCycle = 5 * time.Second` and `watchdogHubIdleCycles = 3`.
  The daemon re-enumerates the hub socket's live sessions every discovery cycle, and exits after three consecutive enumerations that find none — a 15-second grace.
- Rationale: the existing watchdog timings are deliberately fixed and non-configurable, and these belong in the same block for the same reason.
  Five seconds is well below any human `down` + `up` gap while being an order of magnitude slower than the 100ms signal tick, so discovery costs one `list-sessions` round trip per hub every five seconds rather than riding the per-worktree tick.
  Three cycles covers a `down` immediately followed by an `up` without the daemon dying and respawning in between.
- Rejected: leaving the count to the plan writer (it interacts with the `down` + `up` case this decision exists to cover);
  reusing `watchdogPollCycle` for discovery (that constant means "the fallback platform's reconcile cadence" and would silently change meaning);
  a config key (the other watchdog timings are fixed, and an operator has no information with which to tune this one).

### daemon-exits-when-the-hub-server-is-gone-never-on-down

- Decision: `reed down` never kills the daemon.
  The daemon exits on its own once the hub's tmux server has no live sessions — checked each discovery cycle, after `watchdogHubIdleCycles` consecutive empty enumerations (see the timings decision below) — and releases its lock on the way out.
  This closes the design doc's lifecycle open item.
- Rationale: a per-hub daemon cannot be stopped by one worktree's `down`, since sibling worktrees on the same hub may still be alive, and reference-counting `up`/`down` calls is exactly the bookkeeping that goes wrong when a session dies without a `down`.
  "No sessions left on this socket" is directly observable, needs no counter, and is self-correcting;
  the grace period keeps a `down` + immediate `up` from killing and respawning the daemon.
- Rejected: reference-counted first-`up`/last-`down` (requires every teardown path to be well-behaved, including crashes);
  spawn-once-at-`fabric clone` and run forever (survives the hub having nothing to watch, and puts a long-lived process in a command whose job is repo wiring).

### daemon-discovers-worktrees-by-scanning-the-hub

- Decision: each cycle the daemon lists live sessions on the hub socket, and maps session name → worktree by scanning the hub directory's immediate subdirectories, calling `lyxcwd.ResolveWorktree(<subdir>)` and matching `reedengine.SessionName(location.WorktreePath())` against the listed names.
  For each match it builds a `reedengine.Engine` from `hubgeom.ReedGeometry(location)` + `reedengine.LoadConfig` and runs that worktree's existing watch work against it.
  The mapping is cached and rebuilt when the live session set changes.
- Rationale: `lyxcwd.ResolveWorktree` already resolves a `Location` from a worktree root without touching cwd, and `lyxcwd` stays the sole owner of that resolution (Cwd Resolution Invariant).
  `SessionName` is a one-way derivation from the worktree path, so forward-deriving every candidate and matching is the only mapping available without inventing a registry.
  The hub is `filepath.Dir(worktreeRoot)`, so its immediate subdirectories are exactly the candidate set.
- Rejected: persisting a session→worktree registry file (a second source of truth that goes stale precisely when a worktree is renamed or removed — the case it exists for);
  parsing the worktree back out of the session name (`SessionName` is lossy and deliberately sanitizes).

### per-worktree-signal-files-stay-per-worktree

- Decision: the resize signal file stays at `<worktree>/.lyx/reed-resize.signal`, one per worktree, and the `window-resized` hook keeps writing it exactly as today.
  The daemon watches every resolved worktree's file rather than a single hub-wide one.
- Rationale: the Durable-vs-Ephemeral State Invariant puts each worktree's ephemeral state under its own `.lyx`, and one file per worktree is what keeps sibling worktrees on the shared per-hub server from colliding.
  The hook is installed per session by `installResizePinsLocked` and already names the right file.
- Rejected: a hub-wide signal file carrying the session name (turns a file-exists check into a parse, and makes two simultaneous resizes lose one).

### watch-loop-internals-are-rehosted-not-rewritten

- Decision: `watchLoop`'s decision state (`watchState`, the debounce, the per-event retry cap, the poll/signal/dormant mode machine, `reapplyLayout`) is kept as-is and driven once per watched worktree.
  The daemon owns the outer loop: discovery, per-worktree scheduling, and process lifetime.
  `Engine.Watch(ctx)` stays the per-worktree entry point.
- Rationale: all of that logic is already written, tested, and live-verified;
  the task is rehosting, not redesigning.
  Keeping `Engine.Watch` intact also keeps the per-worktree unit tests (`watchloop_test.go`) meaningful.
- Rejected: one shared loop multiplexing all worktrees into a single debounce/retry state (couples unrelated worktrees' failure streaks);
  rewriting the signal mechanism around fsnotify (a new dependency and a new platform matrix for a poll that already works).

### watchdog-config-key-keeps-its-meaning-and-scope

- Decision: `reed.yaml`'s `watchdog:` key stays, still read per worktree, and still gates both the watch loop and the `window-resized` hook install for that worktree.
  The daemon reads each watched worktree's own config;
  a worktree with `watchdog: off` is discovered but not watched.
  Because config is now read by a long-lived process, the daemon re-reads a worktree's config when it (re)enters the watched set, rather than once per process.
  The key's help text is updated: it no longer takes effect "on the next header-pane rebuild".
- Rationale: the knob is per-worktree today and there is no reason for one worktree's kill-switch to disable a sibling's.
  `pinGeometryOptionsLocked`'s existing off-path (unset the hook, remove the signal file) is already per-worktree and is unchanged.
- Rejected: a hub-level watchdog key (silently changes the meaning of an existing operator-set value);
  reading config once at daemon start (a `down` + config edit + `up` would then need a daemon restart nobody would think to do).

### daemon-verb-is-the-new-interactive-handoff-exception

- Decision: `lyx reed watchdog` is a blocking, envelope-exempt command, and CONSTRAINTS.md's CLI/Cobra Invariant exception list is edited in the same commit: `reedengine attach`/`header --blocking` becomes `reedengine attach`/`watchdog`.
  It still carries a non-empty `Short`, still checks `clihelp.ShouldAbort`, and still fails loud on the envelope for everything fallible before it starts blocking.
  It rebinds `logger.SetOutput(io.Discard)` for the same reason `header --blocking` does — it is a detached process whose stderr nobody reads — while the durable handler keeps recording at Info and above.
- Rationale: the exception list is a closed enumeration of commands that legitimately never return an envelope;
  swapping one entry for its replacement keeps it closed and keeps the invariant enforceable.
- Rejected: hiding the verb from the command tree (`Short` and the help-tree tests are part of the same invariant, and an operator debugging a dead watchdog needs to be able to run it in the foreground).

### config-block-replaces-header-with-status_line-and-selvage

- Decision: `reed.yaml`'s `header: {template, height_rows}` block is replaced by two blocks:
  `status_line: {template}` and `selvage: {height_rows}` (default 1).
  `HeaderConfig` becomes `StatusLineConfig` and `SelvageConfig` on `reedengine.Config`.
  Both template YAML files (`template_posix.yaml`, `template_windows.yaml`) are updated in the same change.
- Rationale: the two settings now describe two different mechanisms with nothing in common — one is text in a tmux option, the other is a row budget for a pane — so one block holding both would be misleading.
  `height_rows` stays configurable rather than becoming a constant because the band math already reads it and a wider Selvage is a plausible operator preference.
- Rejected: keeping the `header:` block and reinterpreting its keys (an operator's existing `header.height_rows: 3` would silently become Selvage's height, which is a different pane in a different place);
  dropping `height_rows` to a hard-coded 1 (removes a knob the layout math already supports for free).
- Gotcha for the plan: reconcile **does** remove the stale keys, so no removal logic gets written — but it is not automatic.
  `configsync.ReconcileAll` calls `yamlengine.Reconcile(template, existing)`, which diffs leaf key-paths in both directions and merges only what the template still declares, returning `added` **and** `removed`;
  `internal/configsync/configsync_test.go`'s `TestReconcileAll_DropsStaleReedClaudeKey` pins exactly this for a prior stale reed key (`claude:`) the template stopped declaring.
  Once the template drops `header:`, `header.template` and `header.height_rows` are stale leaves and are stripped the same way.
  It only happens where `ReconcileAll` is actually called with `apply=true`: `lyx config reconcile --apply` (`internal/configcli/configcli.go:274`, where `apply` is flag-gated) and `fabric clone` (`internal/fabriccli/clone.go:100`, unconditionally `true`).
  No reed verb reconciles — `up`, `down`, `resume` and `attach` all leave an unreconciled `reed.yaml` exactly as it is.
  So the plan must handle both states: a reconciled file with only `status_line:`/`selvage:`, and an un-reconciled file still carrying `header:` alongside them.
  A leftover `header:` block is inert (nothing unmarshals it into `Config` any more), which is what makes the un-reconciled state safe.

### worktree-token-joins-the-vocabulary

- Decision: add a `worktree` token to `internal/tokenvocab`'s registry, resolving from a new `Ctx.WorktreeName` field.
  Feed it by adding `WorktreeName` to `reedengine.Geometry` and filling it in **both** tellers: `hubgeom.ReedGeometry` from `l.WorktreeName` (already exposed by `lyxcwd.Location`), and `standalonegeom.ReedGeometry` (`internal/standalonegeom/reedgeom.go:45`) from `filepath.Base(standalonestate.Normalize(target))` — the same normalized readable name that file already computes for `SessionName`'s readable half, reused rather than re-derived.
  The default template becomes `{{.repo}}/{{.worktree}} · {{.hub}}`, and the embedded asset `console-header.md` is renamed `status-line.md`.
  Also in scope, same commit: `internal/reedengine/geometry.go`'s own doc comment says "the eight-field struct" and must say nine, and the `RepoName`/`HubPath` field comments say "the header pane's … token" and must name the status-line instead.
- Standalone disposition, stated explicitly: standalone mode has no worktree — `WorktreeRoot` is the plain checkout the operator pointed at — so `{{.worktree}}` there renders the same directory name `{{.repo}}` does, and the default template reads `foo/foo · <stateDir>`.
  That redundancy is accepted deliberately: in standalone the target genuinely is both the repo and the working directory, and a mildly repetitive line is the right trade against a token that renders empty.
- Rationale: `tokenvocab.Build` resolves every registry token unconditionally (`internal/tokenvocab/tokenvocab.go:30`), so a token left unfilled by one teller does not degrade — it renders an empty segment (`foo/ · …`) in every template that names it.
  Filling both tellers is therefore the only option that keeps the default template honest in both modes.
  In hub mode the status-line is per-session and every session is a worktree, so the worktree name is the single most useful token the old header could not show.
  `tokenvocab`'s registry is the one declared source of the vocabulary, and adding a token there is a localized change.
- Rejected: deriving the worktree name inside `reedengine` with `filepath.Base(geom.WorktreeRoot)` (a per-module cwd/path derivation, which the Cwd Resolution Invariant reserves for `lyxcwd`);
  keeping only `repo` and `hub` (the design doc names showing the worktree as the reason the token is needed);
  leaving `standalonegeom` unfilled and accepting `foo/ · …` there (an empty segment reads as a bug, where a repeated name reads as what it is).

### no-new-cross-cutting-invariant

- Decision: this task adds no new entry to CONSTRAINTS.md beyond editing the CLI/Cobra Invariant's exception list (see above).
  Selvage's rules — bottom-most physical position, never a strand, never written to — are module-local and belong in `internal/reedengine/doc.go` and `manifest/designs/reed-header-selvage.md`.
- Rationale: CONSTRAINTS.md holds cross-cutting structural invariants;
  "reed's band pane sits at the bottom" binds exactly one package and is already enforced by that package's own tests.
- Rejected: adding a "Selvage Keepalive Invariant" (would restate a module doc in the authoritative cross-cutting list, which is what the file explicitly is not for).

## Technical context

**Where the header lives today.**

- `internal/reedcli/header.go` — the `header` verb, its `--blocking` branch, `headerBlockingPayload` (ED2/ED3/cursor-home), `headerWatch` (calls `eng.Watch`), `headerPark` (`blockForever`).
- `internal/reedengine/headerpane.go` — `headerLaunchCmd`/`headerLaunchLine`, the pure command-line composer (`shell.Invoke`/`shell.Quote`), with the `underTest` branch that leaves the pane a bare shell under `go test` (`Engine.suppressHeaderLaunch`).
  With Selvage launching `cfg.Shell` and nothing else, this whole file and the `suppressHeaderLaunch` flag become unnecessary.
- `internal/reedengine/header.go` — `HeaderText`/`ValidateHeader` over `tokenvocab.Render`.
- `internal/reedengine/headertemplate.go` + `console-header.md` — the embedded default template, deliberately outside the stencil mechanism.
- `internal/reedengine/lifecycle.go` — `ensureHeaderPaneLocked` (~line 448), `splitHeaderPaneAtTopLocked` (~line 555) with its even-vertical retry, the `-b` split argv (~line 627), the `ValidateHeader` pre-tmux call (~line 202), and the two `st.HeaderPaneID = ""` clears on server-respawn/heal paths (~lines 678, 727).
- `internal/reedengine/state.go` — `ReedState.HeaderPaneID` (~line 41).
- `internal/reedengine/reconcile.go` — `planReconcile(strands, live, headerPaneID)`: the corpse-kill exemption (~line 75), the `headerAlive` authorization for reaping untracked panes (~lines 105-127), and `clearConflictingPaneBindings` (~line 190).
- `internal/reedengine/spawn.go` — `planPaneTarget(live, headerPaneID)`: prefer tallest alive non-header pane, fall back to a non-header corpse, fall back to the header itself.
- `internal/reedengine/apply.go` — `toRenderInputs` blanks `HeaderPaneID` when the pane is absent and builds `render.Params`; `planLayout` and `fixedHeightPins` share it.
- `internal/reedengine/render/` — `types.go` (`Header`, `Params.Header`), `rules.go` (`planCells`, the sole-header branch, `Rules`, `FixedHeightPins`), `layout.go` (`bandHeader` splices the band cell first), `height.go` (`clampHeaderHeight`).

**Where the status-line pins live.**

- `internal/reedengine/windowsize.go` — `pinGeometryOptionsLocked` currently sets `status off` and `window-size latest`, and owns the watchdog hook's unset half;
  `readStatusRowsLocked`/`reservedRowsFromStatus` read `#{status}` back;
  `installResizePinsLocked`/`resizePinHookArgvs` rebuild the whole `window-resized` array, with the band always at pin index 0.
- `internal/reedengine/attach.go` — `AttachArgv` computes the told box as `rows - reserved` and reproduces `applyLayoutLocked`'s two skip guards exactly.
  Its comment "the told box is only correct once `status off` has landed" must be rewritten, not just left.

**Where the watchdog lives.**

- `internal/reedengine/watchdog.go` — `watchdogOption`, the fixed timings, `resizeSignalPath`, `resizeHookCommand`, `tmuxQuoteValue`.
- `internal/reedengine/watchloop.go` — `Engine.Watch`, `watchLoop`, `watchState`, the poll/signal/dormant mode machine, `tickerPeriodFor`.
- `internal/reedengine/reapply.go` — `reapplyLayout`, `hookInstalledLocked` (the hook array's read side).
- `internal/reedengine/server.go` — `errWorktreeRootGone`, matched with `errors.Is` by the dormancy decision;
  `ServerName(hubPath)` is the per-hub socket key.

**Helpers to reuse, not reinvent.**

- `lyxcwd.ResolveWorktree(worktreeRoot)` — resolves a `Location` from a worktree root with no cwd involvement.
  This is what makes per-hub discovery possible without violating the Cwd Resolution Invariant.
- `hubgeom.ReedGeometry(location)` + `reedengine.LoadConfig(baseDir, "reed")` — the exact pair `internal/reedcli/cli.go`'s `PersistentPreRunE` already uses to build an `Engine`.
- `internal/lock` — `TryAcquireWriteLock`/`Release` for the daemon's single-instance lock.
- `internal/proc` — `Detach`/`DetachBreakaway` (platform-split) for the detached spawn, `IsAlive`/`KillPID` for diagnostics.
- `internal/logger` — every real OS-process spawn must log it (Live-Substrate Spawn Observability);
  the daemon spawn is a lifecycle spawn, so `Info`, and it is a detached `Start` with no `Wait`, so there is no teardown to log.
- `internal/shell` — the only permitted builder of pane-shell command strings (Shell Mechanics Seam).
  Selvage's launch is a single trailing argument to `split-window`, exactly like `new-session`'s `e.cfg.Shell`, so it needs no quoting helper of its own — but anything more elaborate must go through `shell`.
- `internal/output` + `internal/clihelp` — the envelope and the `ShouldAbort` check every `RunE` opens with.

**Gotchas discovered while exploring.**

- A window_layout string naming a pane twice is accepted by tmux (exit 0) and answered by **destroying** every pane the short cell list no longer covers (live-verified, tmux 3.6).
  `removeDuplicatePaneCells` and `clearConflictingPaneBindings` both exist for this;
  both take the band's pane id and both must be retargeted onto Selvage's.
- A layout string enumerating **zero** panes is likewise accepted and answered by destroying the session's whole pane set.
  `anyPlacedStrand` guards this in `applyLayoutLockedOpts` and again in `AttachArgv`;
  neither guard may be dropped while the band moves.
- `select-layout` with dimensions that disagree with the live window exits 0 and silently rescales proportionally — which is why every apply path plans against the live box, and why the `status on` row change must flow through the box rather than being absorbed as a fudge factor.
- tmux cannot split a one-row pane at all.
  With the band at the bottom and `height_rows: 1`, the bottom-most pane is a one-row Selvage whenever state is stale, which is exactly the wedge the even-vertical re-tile retry was written for — it must survive the flip, targeting the bottom.
- `set-hook` array entries fire independently, while a single `;`-separated command list aborts at the first failure (live-verified, tmux 3.6).
  The array encoding and the "band pin is index 0, signal entry is last" ordering both depend on that;
  the band pin keeps index 0 even though the band is now at the bottom — index is fire order, not screen position.
- tmux's `status-left-length` defaults to 10.
  Setting `status-left` without raising it truncates the rendered text silently.
- `#` is tmux's format-expansion character inside a status string;
  a hub path containing `#` would otherwise be interpreted rather than displayed.
- `tokenvocab` is a leaf: stdlib + `internal/stencil` only, reverse import never allowed (Tokenvocab Leaf Invariant).
  Adding `WorktreeName` to `Ctx` is fine; making it resolve a path is not.
- Test-tier purity: anything spawning a real process or a real tmux belongs in an `integration`- or `smoke`-tagged file, and any package whose tests spawn git must call `gitkit.HermeticGitEnv()` in `TestMain`.
- `Engine.suppressHeaderLaunch` exists only because re-exec'ing the test binary as the header pane's command would run the test binary's own `main`.
  Selvage launching a plain shell removes that hazard entirely — but the daemon spawn reintroduces it in a new place, so the daemon must never re-exec `os.Executable()` under `go test` (already a written rule under Live-Substrate Spawn Observability).

## Constraints

From `CONSTRAINTS.md`, the ones this task can break:

- **Cwd Resolution Invariant** — the daemon must not derive worktree roots itself.
  Every resolution goes through `lyxcwd.ResolveWorktree`;
  no `os.Getwd`, no `git rev-parse`, no `filepath.Base` standing in for `WorktreeName`.
- **Told-Geometry Invariant** — `reedengine` never imports `lyxcwd`.
  The daemon's discovery therefore lives in `internal/reedcli` (which already imports `lyxcwd` and `hubgeom`), and the engine is told a fully-built `Geometry` exactly as `PersistentPreRunE` tells it today.
  This is the single most load-bearing structural constraint on the daemon's shape.
- **CLI / Cobra Invariant** — `Command()`/`RunCLI` seam unchanged, non-empty `Short` on `watchdog` and `statusline`, JSON errors via `internal/output`, `clihelp.ShouldAbort` first in every `RunE`, help-tree tests updated for the removed `header` verb and the two new ones.
  The interactive-handoff exception list is edited in the same commit.
- **Shell Mechanics Seam** — pane-shell command strings only via `internal/shell`.
- **Durable-vs-Ephemeral State Invariant** — the daemon's lock file and any runtime state it keeps live under a hub-anchored `.lyx` tree, never in `_lyx`, and never in a location the engine derives for itself.
- **Live-Substrate Spawn Observability** — the daemon spawn is logged at `Info` via `internal/logger`;
  a detached `Start` with no `Wait` logs the spawn alone.
  Never re-exec `os.Executable()` under `go test`.
- **Tokenvocab Leaf Invariant** — stdlib + `internal/stencil` only.
- **Config Strictness Invariant** — `reedengine` is on the degrading side (`LoadOrTemplate`);
  the daemon's per-worktree config reads must use the same call, so a worktree with no `_lyx` still resolves the embedded template rather than erroring.
- **Test Tier Purity Invariant** — no `exec.Command`, no real tmux, no `time.Sleep` ≥ 1s in untagged files.
- **Documentation Lifecycle** and this repo's CLAUDE.md task-completion rule — the module design doc, `manifest/roadmap.md`, and `CONSTRAINTS.md` all move in the same commit as the code.

Discovered during discussion:

- The `geometry-tmux-failures-are-non-fatal-everywhere` decision already governs `windowsize.go`: every new `set-option` this task adds is logged via `logger.Warn` and then ignored, never returned as an error.
- The `header-is-not-a-strand` decision carries over verbatim as "Selvage is not a strand";
  `len(st.Strands)` must keep excluding it in `Up`, `Status`, and `noSessionMessage`.

## Testing

**`internal/reedengine/render` (pure, untagged — strongest TDD candidate).**
Write the bottom-band tests before flipping `planCells`/`bandHeader`: the band cell is emitted last;
the stack box starts at `box.Y` and is `box.H - bandHeight - 1` tall;
cell offsets and heights sum to the box with the one-row divider accounted for;
the sole-band branch still emits a bracket-less single cell claiming the whole box;
`FixedHeightPins` reports the band at the height `Rules` actually placed it at;
`clampBandHeight` still yields rows to the stack's `MinFullRows` floor and still never starves the band below 1.
The existing `rules_test.go`/`height_test.go`/`pins_test.go` fixtures are the starting point — they encode the top-band geometry and must be rewritten, not merely renamed.

**`internal/reedengine` unit level (untagged).**
`planReconcile` with a Selvage id: corpse never killed, alive Selvage authorizes reaping untracked panes, dead-but-present Selvage does not.
`planPaneTarget`: prefers the tallest alive non-Selvage pane, falls back to a non-Selvage corpse, falls back to Selvage itself when nothing else exists.
`toRenderInputs`: blanks `SelvagePaneID` when the pane is absent.
`removeDuplicatePaneCells`/`clearConflictingPaneBindings` against Selvage's id.
`resizePinHookArgvs`: the band pin stays at array index 0 and the signal entry stays last.
`reservedRowsFromStatus` already covers `"on"` → 1;
add the status-line text escaping helper (`#` → `##`) as a pure, table-driven test.
Config: `status_line`/`selvage` unmarshal and defaults, plus an un-reconciled `reed.yaml` still carrying a stale `header:` block alongside the new ones — it must unmarshal cleanly with the stale block ignored.

**`internal/configsync` (untagged).**
Add a reconcile-removal case alongside `TestReconcileAll_DropsStaleReedClaudeKey`: a seeded `reed.yaml` carrying `header.template`/`header.height_rows` has both reported in `Removed` and stripped from the merged output once the template declares `status_line:`/`selvage:` instead, with the new keys reported in `Added`.
This is the test that pins the migration path the operator actually gets from `lyx config reconcile --apply`.

**`internal/tokenvocab` (untagged).**
`worktree` resolves from `Ctx.WorktreeName`;
`Build` returns all three tokens;
the leaf-enforcement test still passes.

**`internal/reedcli` (untagged).**
Help-tree tests updated: `header` gone, `statusline` and `watchdog` present with non-empty `Short`.
`statusline` returns the rendered text on the envelope.
Daemon discovery is the TDD candidate here: the session-name → worktree matching should be a pure function over (listed session names, candidate worktree roots) so it can be tested with no tmux and no filesystem spawning, with the `lyxcwd.ResolveWorktree` call injected or the candidate list told.
Also assert `watchdog` takes the `PersistentPreRunE` early return — it must run with no git repository present and must never populate `c.eng` — and that it refuses an absent or relative `--hub-path` on the envelope before blocking.

**`internal/standalonegeom` and `internal/hubgeom` (untagged).**
Both tellers fill `WorktreeName`: hub mode from `Location.WorktreeName`, standalone from the normalized target basename, with a symlinked and a real spelling of one target yielding the same value (the existing `SessionName` normalization tests are the model).

**Smoke/integration (tagged).**
`smoke_header_keepalive_test.go` becomes the Selvage keepalive smoke: kill every strand pane, assert the session survives and Selvage remains;
assert Selvage is physically bottom-most after a series of adds and removes;
assert Selvage survives a Ctrl-C sent to it at an idle prompt, and survives a Ctrl-C that kills a foreground job inside it.
A status-line smoke: after `up`, `#{status}` reads `on`, `status-position` reads `bottom`, and `status-left` contains the repo and worktree names.
An upgrade smoke: a `reed.json` carrying `headerPaneId` plus a live header-shaped pane — after `up`, Selvage exists at the bottom and the stale pane is gone.
A daemon integration test: spawn the daemon against a hub with two live worktree sessions, resize one window, assert only that worktree's layout is re-applied;
assert a second spawn attempt exits 0 without taking the lock;
assert the daemon exits after the last session goes away, within `watchdogHubIdleCycles * watchdogHubDiscoveryCycle` plus slack, and that it releases the lock so a later spawn takes it;
assert a `down` immediately followed by an `up` does not kill it;
assert `attach` and `resume` each attempt the spawn when no daemon holds the lock.
`smoke_headerscrollback_test.go`, `smoke_headerseed_test.go` and the dot-fill/header smokes need review one by one — some assert behaviour (ED3 scrollback clearing, stencil-seed suppression) that no longer has a subject and should be deleted rather than adapted.

**Whole-repo gates.**
`go build ./...` and `go test ./...` with `CGO_ENABLED=1`;
the sandbox suite still exercises the reed module;
the markdown link-integrity test over `manifest/`/`docs/` after the design doc and roadmap edits.

## Q&A log

- **Q:** Do all three splits (status-line, Selvage, per-hub daemon) land in this task, or is the daemon deferred? **A:** [auto-pick] All three in this task. **Why:** Selvage as a plain shell deletes the only caller of `eng.Watch(ctx)`, so deferring the daemon silently disables the watchdog for every worktree — the exact trap the design doc records.
- **Q:** How does reed render the status-line's text? **A:** [auto-pick] `status on` + `status-position bottom` + `status-left <rendered>` + a raised `status-left-length`, set per session in `pinGeometryOptionsLocked`. **Why:** that function already runs on both boot and the attach pre-flight, so an older session adopts the change on the next attach, matching how the resize hook already back-fills.
- **Q:** Does the attach chain's reserved-row math need changing? **A:** [auto-pick] No — keep reading `#{status}` back. **Why:** `reservedRowsFromStatus` already maps `"on"` → 1 and `AttachArgv` already floors `reserved` at `rows-1`; hard-coding 1 would break an operator's multi-line status config.
- **Q:** Is Selvage a strand or a band? **A:** [auto-pick] A band at the `render.Params` seam, exactly as the header is today. **Why:** it takes no part in parent-chain ordering, height splitting, or focus; modelling it as a strand means teaching four policy functions to ignore it.
- **Q:** Rename `render.Header` → `render.Selvage`, or keep the type and add a position enum? **A:** [auto-pick] Rename, bottom-only. **Why:** nothing wants a top band any more, and a position enum would be an untested branch from the day it ships.
- **Q:** What shell does Selvage run? **A:** [auto-pick] `reed.yaml`'s existing `shell:` key. **Why:** `new-session` already launches the session's first pane with `cfg.Shell`; `$SHELL` is meaningless on Windows and invisible to reed.yaml, and a fixed `bash` ignores a key the operator already set.
- **Q:** Does Selvage need signal handling? **A:** [auto-pick] No. **Why:** live-confirmed that an interactive shell ignores SIGINT at its prompt and that Ctrl-C to a foreground job kills only that job; it should still die on `reed down`.
- **Q:** How is `ReedState.HeaderPaneID` migrated? **A:** [auto-pick] Rename to `SelvagePaneID` and do not read the old field. **Why:** the first `up` creates Selvage and the same pass's reap kills the now-untracked old header pane; a shim would carry a dead name forever to save one reap.
- **Q:** What happens to `lyx reed header`? **A:** [auto-pick] Replaced by `lyx reed statusline`, preview-only, no `--blocking`. **Why:** the blocking mode's whole reason for existing is gone, and the preview is still needed after a template edit; `statusline` does not collide with `lyx reed status`.
- **Q:** How is the daemon hosted and started? **A:** [auto-pick] A blocking `lyx reed watchdog` verb, spawned detached by `reed up` via `proc.Detach`. **Why:** it needs no tty; detaching matches the existing spawn pattern in `internal/vscode`/`internal/fabricengine`, and a tmux-hosted daemon would reintroduce the pane coupling this task removes.
- **Q:** How is one-daemon-per-hub enforced? **A:** [auto-pick] `lock.TryAcquireWriteLock` on a hub-scoped lock file; a losing spawn logs and exits 0. **Why:** the lock is itself the check, so there is no race between probing and spawning, and an unconditional attempt on every `up` makes the daemon self-healing after a crash or logout.
- **Q:** When does the daemon stop? **A:** [auto-pick] It exits itself once the hub socket has had no live sessions for a short grace period; `reed down` never kills it. **Why:** one worktree's `down` cannot speak for its siblings, and reference-counting breaks the moment a session dies without a `down`.
- **Q:** How does the daemon map tmux sessions to worktrees? **A:** [auto-pick] Scan the hub's immediate subdirectories, `lyxcwd.ResolveWorktree` each, and match forward-derived `SessionName` values against the live session list. **Why:** `SessionName` is a lossy one-way derivation, and a persisted registry would go stale exactly when a worktree is renamed or removed.
- **Q:** One hub-wide resize signal file or one per worktree? **A:** [auto-pick] One per worktree, unchanged. **Why:** the Durable-vs-Ephemeral State Invariant puts each worktree's ephemeral state under its own `.lyx`, and a shared file loses one of two simultaneous resizes.
- **Q:** Is the watch loop rewritten for multi-worktree operation? **A:** [auto-pick] No — it is rehosted; `Engine.Watch` stays the per-worktree entry point and the daemon owns only discovery, scheduling, and process lifetime. **Why:** the debounce, retry cap, and mode machine are already live-verified, and sharing one state across worktrees would couple unrelated failure streaks.
- **Q:** Does `watchdog:` stay a per-worktree key? **A:** [auto-pick] Yes, and the daemon re-reads a worktree's config when it re-enters the watched set. **Why:** one worktree's kill-switch must not disable a sibling's, and a once-per-process read would make a config edit need a daemon restart nobody would think to do.
- **Q:** How does the new blocking verb sit with the CLI/Cobra Invariant? **A:** [auto-pick] Swap `header --blocking` for `watchdog` in the interactive-handoff exception list, same commit. **Why:** the list is a closed enumeration; replacing the entry keeps it closed and keeps the invariant enforceable.
- **Q:** What replaces the `header:` config block? **A:** [auto-pick] `status_line: {template}` plus `selvage: {height_rows}`. **Why:** the two settings now describe unrelated mechanisms; reinterpreting the old keys would silently apply an operator's `header.height_rows` to a different pane in a different place.
- **Q:** Should the plan write removal logic for a stale `header:` block in existing `reed.yaml` files? **A:** [auto-pick] No. **Why:** `lyx config reconcile --apply` already strips stale leaves once the template drops them (see the round-1-gap entry below); the plan only has to keep the un-reconciled state harmless, which it is — nothing unmarshals `header:` into `Config` any more.
- **Q:** How is the `worktree` token fed? **A:** [auto-pick] `Ctx.WorktreeName`, filled from `lyxcwd.Location.WorktreeName` via a new `Geometry.WorktreeName` field. **Why:** deriving it inside `reedengine` with `filepath.Base` would be a per-module path derivation the Cwd Resolution Invariant reserves for `lyxcwd`.
- **Q:** (review round 2 gap) How does the hub reach the detached daemon, and what is its cwd? **A:** [auto-pick] An explicit `--hub-path <abs>` flag, `watchdog` opted out of reed's `PersistentPreRunE`, and `cmd.Dir` pinned to the hub. **Why:** the process outlives the spawning worktree, both existing detached precedents pass the path explicitly, and a cwd handle on a worktree blocks that directory's deletion on Windows.
- **Q:** (review round 2 gap) Is `reed up` the only spawn site? **A:** [auto-pick] No — `up`, `resume` and `attach` all attempt it; `down` never does. **Why:** a crash while sessions stay alive would otherwise go unrepaired, and the status-line pins already heal on exactly those three paths, so narrower daemon coverage would be an unexplained asymmetry.
- **Q:** (review round 2 gap) What does `{{.worktree}}` render in standalone mode, which has no worktree? **A:** [auto-pick] The normalized target basename, so the default template reads `foo/foo · <stateDir>`. **Why:** `tokenvocab.Build` resolves every token unconditionally, so an unfilled token renders an empty segment; a repeated name reads as what it is, an empty one reads as a bug.
- **Q:** (review round 2 gap) Where exactly does the daemon's lock live? **A:** [auto-pick] `filepath.Join(fabricengine.HubScratchDir(hub), "reed-watchdog.lock")`, computed in `reedcli` and told to the daemon. **Why:** `HubScratchDir` is the already-declared accessor for `<hub>/_board/.lyx`, so no engine derives its own `.lyx` path.
- **Q:** (review round 2 gap) How long is the daemon's idle-exit grace? **A:** [auto-pick] Three consecutive empty enumerations on a five-second discovery cycle, as two new fixed constants beside the existing `watchdog*` timings. **Why:** those timings are deliberately fixed and non-configurable, and 15 seconds covers a `down` + immediate `up` without the daemon dying and respawning.
- **Q:** (review round 1 gap) Does `lyx config reconcile` remove a stale `header:` block, or leave it? **A:** [auto-pick] It removes it — but only on an explicit `lyx config reconcile --apply` or a `fabric clone`, never from a reed verb. **Why:** `yamlengine.Reconcile` diffs leaf key-paths both ways and `TestReconcileAll_DropsStaleReedClaudeKey` pins the identical stale-reed-key case; the original claim that reconcile is additive-only was wrong.
- **Q:** Does this task add a CONSTRAINTS.md invariant? **A:** [auto-pick] No new invariant — only the CLI/Cobra exception-list edit. **Why:** Selvage's rules bind one package and are already enforced by that package's own tests; CONSTRAINTS.md is for cross-cutting structure.
