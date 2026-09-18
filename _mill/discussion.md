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
- Docs in the same commit: `manifest/designs/reed-header-selvage.md` rewritten from "Planned, not yet built" to what shipped;
  `manifest/roadmap.md` item moved to Done;
  `internal/reedengine/doc.go`'s package doc updated;
  `manifest/designs/reed-fabric-standalone-api.md`'s `*Engine` method inventory (line 124) updated for the two renamed methods;
  and `docs/overview.md` corrected in three places — line 301 (reed's verb list still names `header`, and calls `reed header --blocking` one of reed's two registered interactive-handoff exceptions, which is now `reed watchdog`), line 408 and line 462 (both describe `tokenvocab` as the `repo`/`hub` registry consumed by "reed's header pipeline", now a three-token registry feeding the status-line).

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

- Decision: reed sets, per session, `status on`, `status-position bottom`, `status-left <rendered text>`, `status-right ""`, and `status-left-length` set to `max(10, utf8.RuneCountInString(escaped))` — measured over the **escaped** string that is actually handed to `status-left`, in runes, since tmux's length limit counts characters rather than bytes and 10 is its own default (which would truncate).
  Any `#` in the rendered text is escaped to `##` before it reaches `status-left` and before the length is measured, since tmux expands `#{…}`/`#[…]` inside a status string.
  The escape-then-measure order is load-bearing: a hub path carrying `#` grows by one character per occurrence, and measuring the pre-escape string would truncate exactly those lines.
  The **window-status segment** tmux renders between `status-left` and `status-right` is suppressed, not left at its default: `window-status-format ""` and `window-status-current-format ""`, window-targeted like the other pins.
  reed's session has exactly one window, so tmux's default `0:bash*` segment beside the identity text names nothing the operator can act on, and it would shift position as the window's active pane name changes.
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
  The engine methods behind it are renamed and kept, not retired: `Engine.HeaderText` → `Engine.StatusLineText`, `Engine.ValidateHeader` → `Engine.ValidateStatusLine`, with `internal/reedengine/header.go` renamed `statusline.go`.
  `ValidateStatusLine` stays the eager pre-tmux boot gate `ensureServerAndSessionLocked` already calls (`lifecycle.go:202`) — it is now more load-bearing, not less, since a template that fails to render would otherwise reach `set-option status-left` where every failure is non-fatal and merely logged.
  `manifest/designs/reed-fabric-standalone-api.md`'s public-surface inventory (line 124: 17 exported `*Engine` methods, naming `HeaderText` and `ValidateHeader`) is updated in the same commit.
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

### windows-status-line-is-an-accepted-named-degrade

- Decision: reed issues the same four new `set-option` calls on every platform — no `runtime.GOOS == "windows"` branch for the status-line — and accepts that psmux may reject some or all of them.
  Each failure stays Warn-only, as every other geometry option already is.
- Rationale: the layout stays correct either way, and that is the part that must not break.
  `readStatusRowsLocked` reads `#{status}` back rather than assuming what was set (`internal/reedengine/windowsize.go`), so a `status on` that psmux refused reads back as `off`, yields `reserved = 0`, and the attach chain's told box is right for the window that actually exists.
  The mechanism is self-correcting by construction, which is exactly why the readback was kept in the round-2 decision instead of hard-coding 1.
- The degrade, named rather than left to be discovered: if psmux rejects these options, **Windows loses the identity text** that today's header pane shows.
  Selvage, the layout, the reap rules, and the watchdog are all unaffected.
  `status-position` falling back to `top` is acceptable on its own — the status-line is not a pane, so its edge is independent of Selvage's, and the band pane stays bottom-most regardless.
- Rationale for not branching: reed branches on Windows where the consequence is silent and unrecoverable — `hookInstalledLocked` returns `(false, false)` with no round trip because a hook that installs but never fires would pin the watcher in signal mode forever with zero self-heal (`internal/reedengine/reapply.go:53-56`).
  A refused `set-option` has the opposite shape: it fails loudly into the log, changes nothing, and is answered by a readback.
  Skipping the attempt would guarantee the regression on Windows;
  attempting it costs four Warn lines in the worst case and delivers the feature if psmux supports it.
- Note for the plan: this is unverified, not verified.
  The plan carries a Windows verification item — run `lyx reed up` under psmux and record which of the four options survive — and this decision is revisited against that result rather than being treated as settled.
- Rejected: skipping the status-line on Windows entirely (guarantees the regression rather than risking it);
  gating on a psmux capability probe (`requiredSubcommands` covers subcommands, not option names, so the probe would be new machinery for a question the readback already answers);
  keeping the header pane on Windows only (two pane-lifecycle designs in one module, which is the thing this task exists to stop).

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

### standalone-runs-the-watch-loop-in-process

- Decision: the detached per-hub daemon is a **hub-mode** mechanism only.
  Standalone reed sessions instead run `Engine.Watch(ctx)` as an in-process goroutine, started by the same seam that already boots them.
  No lock, no discovery, no second process, no `--hub-path`.
- Seam signature, stated because today's cannot carry this: `reedUp func() error` (`internal/burlercli/cli.go:41`, `internal/webstercli/cli.go:69`) takes no context, and it is assigned in `wireStandalone`, where no context exists.
  It becomes `reedUp func(ctx context.Context, watch bool) error`.
  The closure boots the engine as it does today and, when `watch` is true and the boot succeeded, starts `go eng.Watch(ctx)`;
  the caller's context is what stops it.
- `recover-batch` disposition: **no watcher**.
  `internal/webstercli/run.go:104` passes `(cmd.Context(), true)`;
  `internal/webstercli/recoverbatch.go:129` passes `(cmd.Context(), false)`, and `internal/burlercli/run.go:167` passes `(cmd.Context(), true)`.
  `recover-batch` is a short-lived verb that spawns a cold recovery strand and returns, so a watcher bound to its context would be dead before it observed anything, while one detached from that context would be an unowned goroutine in an exiting process.
  The `watch bool` is an explicit parameter rather than an implicit rule precisely so this asymmetry is visible at both call sites instead of being rediscovered from a comment.
- Rationale: standalone reed is never booted by `lyx reed up` (that verb is hub-only, as `wiring.go:181`'s comment states);
  it is booted in-process by a long-lived supervising run that exists for exactly the session's working lifetime.
  That supervising process is precisely what hub mode lacks and why hub mode needs a daemon at all, so reusing it is the smaller mechanism, not a special case.
  It also sidesteps two things that are simply meaningless in standalone: `fabricengine.HubScratchDir(hub)` (standalone's `Geometry.HubPath` is a derived `stateDir`, not a hub), and worktree discovery (standalone has exactly one target and one session, known at wiring time).
- Accepted behaviour change, stated rather than discovered later: today the standalone header pane keeps watching after the run that booted it finishes;
  with this decision the watcher's life is bounded to that run.
  A standalone session with no run in progress has no strands being added or removed, so it keeps its last layout and self-heals on the next reed op — the same degrade a hub session takes when the watchdog is off.
- Rejected: watchdog off in standalone (a silent capability regression for a mode that has it today);
  a per-target detached daemon with a `standalonestate`-anchored lock (a whole second lifecycle, lock, and spawn path for a mode that already has a supervising process to hang the loop on);
  generalizing the daemon over "socket root" to cover both modes (the two modes differ in discovery, lock home, and lifetime — the generalization would be three conditionals wearing one name).

### daemon-spawn-is-owned-by-reedcli

- Decision: `internal/reedcli` owns the spawn outright.
  `reedengine` never spawns it and never learns it exists.
  `PersistentPreRunE` stores `location.HubPath` as a `hubPath string` field on the `reedCLI` receiver beside `c.eng` — the hub path alone, not the whole `*lyxcwd.Location`, since the spawn needs nothing else from it and a stored `Location` would invite other code to re-read geometry the engine was already told — and `upCmd`, `resumeCmd` and `attachCmd` each call one shared `c.ensureWatchdogSpawned()` helper immediately after their engine op returns without error.
  "A boot happened" is therefore just "the verb's own engine call succeeded" — there is no signal to plumb back out of the engine.
  For `attach`, which has three engine touches and ends in a blocking handover, the call site is named exactly: after the `Status()` pre-flight and **before** `attach.Run()` hands the operator's stdio over (`internal/reedcli/attach.go:55-83`).
  A spawn placed after the handover would fire only once the operator detaches, which is the one moment it is useless.
- Rationale: the engine is told its geometry and derives nothing;
  making it spawn would require it to import `fabricengine` for the lock path and to be told an executable path, both of which the Told-Geometry Invariant and the engine's own contract exclude.
  `reedcli` already holds the `Location`, already imports `hubgeom`, and can import `fabricengine` directly (as `hubgeom` does).
- Correction to the earlier wording: the round-2 decision said the spawn fires "exactly as they each already call `pinGeometryOptionsLocked`".
  That named the right three verbs but the wrong layer — `pinGeometryOptionsLocked` is engine-internal (`internal/reedengine/windowsize.go:117`).
  The three verbs are the same; the call site is the CLI's, after the engine returns.
- Rejected: spawning from inside `Engine.Up`/`Resume`/`AttachArgv` (drags `fabricengine` and an exe path into the engine);
  a new engine return field saying "I booted" (nothing would consume it except the spawn the CLI can already gate on the error being nil).

### daemon-spawn-uses-os-executable-with-a-test-suppression-field

- Decision: the spawn resolves its binary with `os.Executable()`, matching `internal/boardengine/spawn.go:18` and `internal/fabricengine/spawn.go` exactly.
  Test-time suppression moves, rather than disappearing: `reedCLI` carries a `suppressWatchdogSpawn bool` field initialised from `testing.Testing()`, with an in-package enabler so a test can flip it back on and drive the real spawn path — the same shape, and the same rationale, as today's `Engine.suppressHeaderLaunch` (`internal/reedengine/lock.go:38-55`).
  `gitkit.refuseCLIReexec` stays what it is: a backstop that aborts a test binary re-exec'd as a CLI, never the gate.
- Rationale: re-exec'ing `os.Executable()` from a test binary runs the whole suite recursively, which is exactly the hazard `suppressHeaderLaunch` was written for.
  Deleting the header pane's re-exec removes that call site but not the hazard — the daemon spawn is a new one — so the mechanism is relocated to the layer that now owns the spawn.
- Correction to the earlier wording: the Technical context said `suppressHeaderLaunch` "becomes unnecessary".
  The field on `Engine` does go away with the pane launch, but its pattern is reproduced on `reedCLI` for the daemon spawn;
  it is moved, not dropped.
- Rejected: a configured binary path (a second way to be wrong about which lyx is running);
  relying on `refuseCLIReexec` alone (it aborts the child after the spawn has already happened, so the suite still pays the process and the test still sees a spawn it did not want);
  skipping suppression and marking every test that boots reed as integration-tagged (Test Tier Purity already bars the spawn from untagged files, but the suppression is what makes an untagged test of the surrounding logic possible at all).

### daemon-needs-one-new-engine-less-seam

- Decision: add exactly one exported, engine-less function to `reedengine` — `ListSessions(tmuxPath, socketKey string) ([]string, error)` in `overlay.go` — built on `NewTmuxCmd(tmuxPath, socketKey).output("list-sessions", "-F", "#{session_name}")`, the identical invocation five existing call sites already use (`lifecycle.go:384`, `:862`, `:926`, `generation.go:207`).
  The daemon's two inputs come from opposite directions: the **socket key** is `reedengine.ServerName(hubPath)`, already exported and a pure function of the told hub path;
  the **tmux binary path** is told on the command line, so the verb is `lyx reed watchdog --hub-path <abs> --tmux <path>`, filled by the spawning CLI from the `cfg.Tmux` it already resolved.
  Once a worktree is discovered, that worktree's own `reedengine.LoadConfig` supplies its engine's `cfg.Tmux` as it does today.
- Rationale: `TmuxCmd.run`/`output` are unexported (`internal/reedengine/overlay.go:47,64`) and every exported `*Engine` method is bound to one session, so the daemon — which has no engine before discovery — has no way to enumerate.
  Telling it the binary rather than having it load a config keeps the Told-Geometry Invariant intact: the daemon derives nothing, including which tmux to run.
  It also avoids the false choice of picking one worktree's config to speak for the hub — the sessions share one socket, so one binary is the only coherent answer for enumeration anyway.
- `manifest/designs/reed-fabric-standalone-api.md`'s public-surface inventory gains `ListSessions` alongside the two renamed `*Engine` methods, same commit.
- Rejected: exporting `TmuxCmd.run`/`output` (opens the whole tmux surface to every caller to solve one enumeration);
  a `LoadConfig` at the hub's board dir (degrades to the embedded template and silently ignores an operator's `LYX_REED_TMUX`/`tmux:` setting);
  constructing a throwaway `Engine` with a fake session name just to reach `e.tmux` (a geometry that names nothing, built to be lied to).

### daemon-idle-signal-is-anything-but-a-non-empty-listing

- Decision: the idle counter is driven by one rule — **anything other than "exit 0 with at least one session name" counts as idle.**
  A non-empty listing resets it to zero.
  An exit-0 empty listing, a "no server running" error, and any other `list-sessions` failure all increment it;
  three consecutive increments exit the daemon.
- Rationale: the normal last-`down` case is not an empty list at all — the tmux server is gone, so `list-sessions` *fails*, which the earlier "three enumerations that find none" wording left undefined as the daemon's own main exit path.
  Folding every non-affirmative outcome into one bucket makes the rule total, and the cost of folding a transient failure in with it is bounded and already paid for: three consecutive failures exit a daemon that any subsequent `up`/`resume`/`attach` respawns, which is the self-healing property the spawn-sites decision already rests on.
  Requiring an affirmative answer to keep running is also the conservative direction — a daemon that wrongly exits is repaired by the next reed op, while one that wrongly persists holds the hub lock against the daemon that should replace it.
- Windows/psmux, which forces this shape rather than merely tolerating it: `internal/reedengine/proctree_windows.go:4-5` records that `list-sessions`, `has-session` and `kill-server` "exit identically with and without a server on the socket", so error-vs-no-error carries no information there.
  A rule that distinguished "no-server error" from "other error" would be unimplementable on Windows;
  this one needs only the output, which stays meaningful on both platforms.
- Rejected: counting only exit-0-empty as idle (leaves the POSIX last-`down` path undefined — the daemon would outlive its server indefinitely);
  distinguishing a no-server error from a transient one by matching tmux's message text (operator-facing prose, and unavailable on Windows regardless);
  probing the socket file's existence instead (tmux can hold a socket it is no longer reachable on — `lifecycle.go:852-862` already treats an errored or empty listing as the same "unreachable" answer).

### daemon-points-its-durable-sink-at-the-hub-before-discarding-stderr

- Decision: the daemon calls `logger.SetDurableSinkDir(fabricengine.HubLogsDir(hub))` as its **first** action, before `logger.SetOutput(io.Discard)` and before any lock attempt.
- Rationale: the design's lock-failure Error, spawn Info, and per-session Debug lines are its only diagnostics, and as written they would all have gone nowhere.
  The durable sink's cwd-anchored fallback arms only inside a lyx-owned worktree — `armDurableSinkLocked` returns false when `lyxcwd.Resolve(cwd)` fails or `isLyxWorktree` is false (`internal/logger/sink.go:125-142,191-193`), and `sink.go:189-190` states outright that commands run from the hub never arm it.
  Since `cmd.Dir` is deliberately pinned to the hub (see the invocation contract), the fallback can never arm for this process, so the sink must be pointed explicitly.
  `HubLogsDir` is the right target: it is hub-anchored, it already exists as the shared per-hub reed server's own log directory, and it sits inside the same `HubScratchDir` tree the daemon's lock lives in.
- This makes the earlier "logs at Error to the durable sink" and "the durable handler keeps recording at Info and above" claims true rather than aspirational, and it is what makes the foreground `lyx reed watchdog --hub-path <abs>` diagnosis path meaningful for a daemon that was spawned detached.
- Rejected: leaving `cmd.Dir` at the spawning worktree so the fallback arms (reintroduces the Windows directory-deletion block the invocation contract exists to avoid, and ties the daemon's log location to whichever worktree happened to spawn it);
  not discarding stderr (a detached process's stderr goes to a closed handle, so the lines are lost either way, and the discard is what keeps `logger`'s stderr half from attempting it).

### daemon-single-instance-via-hub-lockfile

- Decision: single-instance is enforced by `lock.TryAcquireWriteLock` on `filepath.Join(fabricengine.HubScratchDir(hub), "reed-watchdog.lock")` — `HubScratchDir` is the already-declared accessor for `<hub>/_board/.lyx` (`internal/fabricengine/junctionnames.go:149`), the same hub-anchored ephemeral tree `HubLogsDir` is a sibling inside.
  The path is computed in `internal/reedcli` (which may import `fabricengine` directly, as `hubgeom` already does) and told to the daemon, never derived inside `reedengine`.
  The two ways "I did not get the lock" can happen are answered differently, because `lock.TryAcquireWriteLock` reports them differently (`internal/lock/lock.go:31-41`):
  `(nil, false, nil)` is **contention** — another daemon holds it — which logs at Info and exits 0, so a racing spawn costs one short-lived process and nothing else;
  a non-nil error is a **failure** (the lock path is unusable — absent parent directory, permissions, a read-only filesystem), which logs at Error to the durable sink and exits non-zero, never silently.
  `lock.TryAcquireWriteLock` does not create the lock file's parent, so `c.ensureWatchdogSpawned()` calls `os.MkdirAll(fabricengine.HubScratchDir(hub), 0o755)` before it spawns;
  a `MkdirAll` failure is logged at Warn, naming the path, and the spawn is skipped rather than failing the operator's `up` (geometry-adjacent failures stay non-fatal in this module).
- Rationale: `HubScratchDir` already exists and is already the declared home for hub-level ephemeral state, so the Durable-vs-Ephemeral State Invariant's "no engine derives its own `.lyx` path" holds without a new accessor.
  The lock is itself the check, so there is no race between probing and spawning.
  Splitting contention from failure is what keeps a hub whose `_board/.lyx` is missing or unwritable from becoming permanently watchdog-less behind an Info line that reads exactly like the normal, expected losing race.
  `<hub>/_board/.lyx` normally exists already — `HubLogsDir` lives inside it — but a hub that has never booted a reed server is not guaranteed to have it, which is precisely the first-`up` case.
- Diagnosis path, worth stating because the daemon is detached and nobody reads its stdio: running `lyx reed watchdog --hub-path <abs>` in the foreground reproduces both answers directly.
  `internal/lock` already owns advisory file locking in this repo, and `TryAcquireWriteLock`'s non-blocking half is exactly the "somebody else already has it" answer this needs.
- Rejected: a PID-file liveness check with `proc.IsAlive` as the primary gate (racy between check and spawn — the lock is the check);
  a tmux-hosted daemon in a hidden window (reintroduces the pane coupling, and a hidden window pops into view the instant the visible one loses its last pane, live-confirmed during design);
  a lock under the spawning worktree's own `.lyx` (it is per-worktree state naming a per-hub process, and it vanishes when that one worktree is torn down).

### daemon-spawn-attempted-by-up-resume-and-attach

- Decision: `up`, `resume`, and `attach` each attempt the spawn unconditionally — the same three paths `pinGeometryOptionsLocked` already heals on, but from the CLI layer after the engine op returns, not from inside it (see `daemon-spawn-is-owned-by-reedcli`).
  `down` never spawns.
  A spawn attempt against a live daemon costs one short-lived child that fails the lock and exits 0.
- Rationale: `up` alone is not enough.
  A daemon that crashes while sessions stay alive would otherwise leave those sessions unwatched until someone happened to run `up` again — and the status-line half of this very task relies on `attach`/`resume` back-filling through `pinGeometryOptionsLocked`, so making the daemon's coverage narrower than the pins' would be an unexplained asymmetry between two mechanisms that heal on the same three paths.
  `attach` is the path an operator takes when something looks wrong, which is exactly when a dead daemon should come back.
- Rejected: `up` as the sole spawn site (leaves a crash unrepaired for as long as the operator keeps attaching rather than re-upping);
  a supervisor process watching the watchdog (a second daemon to keep alive, for a process whose own restart is one lock try);
  respawning from the watch loop itself (it is the thing that died).

### daemon-idle-exit-timings-are-fixed-constants

- Decision: the daemon's discovery cycle and its idle-exit grace are two new fixed constants declared in `internal/reedcli`, beside the discovery loop that consumes them: `watchdogHubDiscoveryCycle = 5 * time.Second` and `watchdogHubIdleCycles = 3`.
  `internal/reedengine/watchdog.go`'s existing `watchdog*` constants stay exactly where they are and stay unexported — they govern `Engine.Watch`'s own internals, which `reedengine` still owns.
  The daemon re-enumerates the hub socket's live sessions every discovery cycle, and exits after three consecutive enumerations that find none — a 15-second grace.
- Rationale: the existing watchdog timings are deliberately fixed and non-configurable, and these two are fixed for the same reason — but they are not the same package's business.
  The discovery loop and the process lifetime they govern are `reedcli`'s (see the ownership decision above), and exporting two constants from `reedengine` purely so another package can read them would put the timings somewhere the code that uses them is not.
  Five seconds is well below any human `down` + `up` gap while being an order of magnitude slower than the 100ms signal tick, so discovery costs one `list-sessions` round trip per hub every five seconds rather than riding the per-worktree tick.
  Three cycles covers a `down` immediately followed by an `up` without the daemon dying and respawning in between.
- Rejected: declaring them beside the existing `watchdog*` block in `reedengine` (they would have to be exported to be readable by their only consumer, widening that package's surface for nothing);
  leaving the count to the plan writer (it interacts with the `down` + `up` case this decision exists to cover);
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

- Decision: the mapping is a **direct join**, not a scan.
  Every discovery cycle the daemon runs **one** `reedengine.ListSessions(tmuxPath, socketKey)` round trip against the hub socket — cheap, no process spawns (see the engine-less-seam decision for where that function and its two arguments come from).
  When that live session-name set differs from the cached one, it resolves each newly-appeared name by joining it onto the hub (`filepath.Join(hub, sessionName)`) and calling `lyxcwd.ResolveWorktree` on **that one path**, then builds a `reedengine.Engine` from `hubgeom.ReedGeometry(location)` + `reedengine.LoadConfig` and runs that worktree's existing watch work against it.
  A name that does not resolve — a foreign session, a renamed directory — is skipped and logged at `logger.Debug`, not retried until it next re-appears in the set.
- Teardown is the other half, and is not optional: the daemon holds a `map[sessionName]struct{engine, cancel context.CancelFunc}`, and on each cycle every name that has **dropped out** of the live set has its context cancelled and its entry deleted.
  `Engine.Watch` never returns while its context is live (`internal/reedengine/watchloop.go:147-154`), so without this a worktree whose session goes away while siblings remain leaves a goroutine polling a dead session for the daemon's whole lifetime.
  Cancelling on departure is also what makes the config-re-read claim true: `watchLoop` reads `cfg.Watchdog` exactly once at start (`watchloop.go:168-171`), so "re-read on re-entry" only means anything if entries can leave.
  The `ResolveWorktree` git spawns are logged at `logger.Debug`, as spawns inside a polling probe (Live-Substrate Spawn Observability).
- Rationale: in **hub mode** the derivation is exactly invertible.
  `reedengine.SessionName(worktreeRoot)` is `filepath.Base(worktreeRoot)` verbatim (`internal/reedengine/server.go:105-107`) — hub mode deliberately does **not** sanitize, and `validateToldTmuxIdentity` refuses an unusable name instead (`server.go:126-133`, whose own comment says rewriting `.` there would collapse sibling worktrees `svc.v2` and `svc_v2` onto one session).
  Since the hub is `filepath.Dir(worktreeRoot)`, `filepath.Join(hub, sessionName)` recovers the worktree root exactly, with no scan and no candidate set.
  `ResolveWorktree` is still required on top of that join — the join yields a path, while `hubgeom.ReedGeometry` needs a resolved `*lyxcwd.Location` (its `RepoName`, `HubPath`, `WorktreeName`, and `AnchorPath()`), and `lyxcwd` stays the sole owner of that resolution (Cwd Resolution Invariant).
  It is also the gate that rejects a session name that is not a worktree at all.
  Gating on the session-set diff still matters because `ResolveWorktree` shells out to `git rev-parse --show-toplevel` (`internal/lyxcwd/lyxcwd.go:149`);
  re-resolving unconditionally every five seconds would mean one real process spawn per live session per cycle, forever, to learn nothing.
- Correction to the earlier wording: rounds 1-3 justified a full hub-subdirectory scan with "`SessionName` is a lossy one-way derivation and deliberately sanitizes".
  That is true of **standalone** (`SanitizeSessionName` plus a `-<hash8>` suffix, `internal/standalonegeom/reedgeom.go`), and false of hub mode, which is the only mode this daemon serves.
  The scan was solving a problem that does not exist here;
  the direct join replaces it, and costs one git spawn per live session rather than one per hub subdirectory.
- Rejected: persisting a session→worktree registry file (a second source of truth that goes stale precisely when a worktree is renamed or removed — the case it exists for);
  scanning every hub subdirectory and forward-matching (strictly more work than the join, and it resolves worktrees that have no session);
  skipping `ResolveWorktree` and hand-building a `Geometry` from the joined path (it would re-derive `RepoName`/`AnchorPath` per module, which the Cwd Resolution Invariant forbids).

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
  a worktree with `watchdog: off` is discovered, entered in the map with a **nil** cancel func, and no `Engine.Watch` goroutine is started for it at all.
  It is never "started and parked": `watchLoop` answers a disabled watchdog by blocking on `<-ctx.Done()` rather than returning (`internal/reedengine/watchloop.go:179-185`), so starting one would cost a goroutine per disabled worktree to do nothing.
  The map entry still exists so the session counts as known and is not re-resolved every cycle, and so departure bookkeeping is uniform — a nil cancel is simply skipped when the name leaves the live set.
  Re-entry then re-resolves and re-reads config exactly as for a watched worktree, which is what lets an operator flip the key with a `down` + `up` and have it take effect.
  Because config is now read by a long-lived process, the daemon re-reads a worktree's config when it (re)enters the watched set, rather than once per process.
  The key's help text is updated: it no longer takes effect "on the next header-pane rebuild".
- Rationale: the knob is per-worktree today and there is no reason for one worktree's kill-switch to disable a sibling's.
  `pinGeometryOptionsLocked`'s existing off-path (unset the hook, remove the signal file) is already per-worktree and is unchanged.
- Rejected: a hub-level watchdog key (silently changes the meaning of an existing operator-set value);
  reading config once at daemon start (a `down` + config edit + `up` would then need a daemon restart nobody would think to do).

### daemon-verb-is-the-new-interactive-handoff-exception

- Decision: `lyx reed watchdog` is a blocking, envelope-exempt command, and CONSTRAINTS.md's CLI/Cobra Invariant exception list is edited in the same commit: `reedengine attach`/`header --blocking` becomes `reedengine attach`/`watchdog`.
  It still carries a non-empty `Short`, still checks `clihelp.ShouldAbort`, and still fails loud on the envelope for everything fallible before it starts blocking.
  It rebinds `logger.SetOutput(io.Discard)` for the same reason `header --blocking` does — it is a detached process whose stderr nobody reads — but only after pointing the durable sink at the hub explicitly, since the cwd-anchored fallback never arms from a hub cwd (see the durable-sink decision).
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
  Feed it by adding `WorktreeName` to `reedengine.Geometry` and filling it in **both** tellers: `hubgeom.ReedGeometry` from `l.WorktreeName` (already exposed by `lyxcwd.Location`), and `standalonegeom.ReedGeometry` (`internal/standalonegeom/reedgeom.go:45`) from the **raw** `filepath.Base(target)` — the identical expression that file already uses for `RepoName` (`reedgeom.go:57`), not the normalized spelling `SessionName`'s readable half takes.
  The default template becomes `{{.repo}}/{{.worktree}} · {{.hub}}`, and the embedded asset `console-header.md` is renamed `status-line.md`.
  Also in scope, same commit: `internal/reedengine/geometry.go`'s own doc comment says "the eight-field struct" and must say nine, and the `RepoName`/`HubPath` field comments say "the header pane's … token" and must name the status-line instead.
- Standalone disposition, stated explicitly: standalone mode has no worktree — `WorktreeRoot` is the plain checkout the operator pointed at — so `{{.worktree}}` and `{{.repo}}` there render the **same string**, byte for byte, and the default template reads `foo/foo · <stateDir>`.
  Taking the raw spelling for both is what makes that exact rather than approximate: `RepoName` is deliberately raw ("the told spelling is the one the operator typed and will recognise", per `reedgeom.go`'s own doc comment), and normalizing only the new token would make a symlinked target render two different names on one line.
  Both tokens are display values that never reach a tmux target, so neither needs `SessionName`'s normalization, which exists to keep one repository on one socket key.
  The redundancy is accepted deliberately: in standalone the target genuinely is both the repo and the working directory, and a mildly repetitive line is the right trade against a token that renders empty.
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
  With Selvage launching `cfg.Shell` and nothing else, this whole file goes away and `Engine.suppressHeaderLaunch` leaves the engine — but the suppression itself reappears on `reedCLI` for the daemon spawn (see the `daemon-spawn-uses-os-executable-with-a-test-suppression-field` decision).
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
  Selvage launching a plain shell removes that call site — but the daemon spawn is a new one, so the field is relocated rather than deleted;
  see the `daemon-spawn-uses-os-executable-with-a-test-suppression-field` decision for where it lands and why `gitkit.refuseCLIReexec` is only a backstop.

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
`reedengine.ListSessions` is its own TDD candidate on the engine side: it parses a `#{session_name}` listing into names, and reports the three outcomes the idle rule distinguishes (non-empty, empty, error) — table-driven against a fake tmux, no real server.
Also assert `watchdog` takes the `PersistentPreRunE` early return — it must run with no git repository present and must never populate `c.eng` — and that it refuses an absent or relative `--hub-path` on the envelope before blocking.

**`internal/burlercli` / `internal/webstercli` (untagged).**
The standalone `reedUp(ctx, watch)` seam: the watcher goroutine starts on a successful boot with `watch: true`, does not start when the boot fails, does not start when `watch: false`, and stops when the context is cancelled.
Assert the three call sites pass what this discussion decides — `burlercli/run.go` and `webstercli/run.go` true, `webstercli/recoverbatch.go` false — since that asymmetry is the whole reason the parameter exists.
Assert too that standalone never computes a hub lock path and never spawns a daemon.

**`internal/standalonegeom` and `internal/hubgeom` (untagged).**
Both tellers fill `WorktreeName`: hub mode from `Location.WorktreeName`, standalone from the raw target basename — assert standalone's `WorktreeName` and `RepoName` are byte-identical for both a symlinked and a real spelling of one target, which is what pins the "same string, not merely the same directory" claim.

**Smoke/integration (tagged).**
`smoke_header_keepalive_test.go` becomes the Selvage keepalive smoke: kill every strand pane, assert the session survives and Selvage remains;
assert Selvage is physically bottom-most after a series of adds and removes;
assert Selvage survives a Ctrl-C sent to it at an idle prompt, and survives a Ctrl-C that kills a foreground job inside it.
A status-line smoke: after `up`, `#{status}` reads `on`, `status-position` reads `bottom`, and `status-left` contains the repo and worktree names.
On Windows the same smoke asserts the self-correcting half instead of the value — whatever `#{status}` reads back, the reserved-row count derived from it matches the window the layout was planned against — so a psmux that refuses the options fails the identity assertion loudly rather than the layout silently (see the Windows degrade decision).
An upgrade smoke: a `reed.json` carrying `headerPaneId` plus a live header-shaped pane — after `up`, Selvage exists at the bottom and the stale pane is gone.
A daemon integration test: spawn the daemon against a hub with two live worktree sessions, resize one window, assert only that worktree's layout is re-applied;
assert a second spawn attempt exits 0 without taking the lock, and that an unusable lock path exits non-zero instead;
assert the idle counter's rule directly — a non-empty listing resets it, while an empty listing and a killed server (an errored listing) both increment it;
assert the daemon writes its diagnostics to `HubLogsDir` rather than nowhere, which is the only observable proof the durable sink was pointed before stderr was discarded;
assert a `watchdog: off` worktree is entered as known but starts no watcher goroutine;
assert the daemon exits after the last session goes away, within `watchdogHubIdleCycles * watchdogHubDiscoveryCycle` plus slack, and that it releases the lock so a later spawn takes it;
assert a `down` immediately followed by an `up` does not kill it;
assert a departing session's watcher is cancelled — bring two sessions up, `down` one, and assert the daemon drops that entry and stops touching that session, while the sibling keeps being watched;
assert re-entry re-reads config: `down` a worktree, flip its `watchdog:` key, `up` it again, and assert the new value takes effect without restarting the daemon;
assert `attach` and `resume` each attempt the spawn when no daemon holds the lock.
Disposition per header-adjacent site, re-derived by a stated method rather than enumerated from the reed packages alone: a repo-wide grep for `reed header`, `HeaderPaneID`, `console-header`, and `headerpane`, excluding `.git/` and `_mill/`.
The plan re-runs that grep and treats any site it names that is not listed below as an unhandled case, not as out of scope.

Outside the reed packages:

- `cmd/lyx/tiersleep_test.go:27` — two `allowedLongSleepers` entries whose justification cites "stand-in for the real `lyx reed header --blocking` keepalive".
  Both entries go away with the re-exec they excuse;
  removing them is part of this task, not a follow-up.
- `cmd/lyx/stencilseedgate_test.go:2,84` — pins `SkipStencilSeedAnnotation` on `reed header` **by name**.
  Retarget to `reed statusline`;
  the gate itself is unchanged, only the command it names.
- `cmd/lyx/stencilseed.go` and `internal/clihelp/annotations_test.go` — check whether either names the verb;
  retarget if so, leave alone if the reference is to the annotation rather than to `header`.
- `internal/reedcli/testmain_test.go` and `internal/reedengine/testmain_test.go` — both carry an `os.Args[1] == "reed"` stand-in that blocks forever, existing solely so a re-exec'd test binary impersonates the header keepalive.
  With no pane re-exec they have no subject: delete both.
  Note this does **not** conflict with `suppressWatchdogSpawn` — that field prevents the daemon spawn from ever re-exec'ing under test, which is why no replacement stand-in is needed.
- `.gitattributes:29` — names `console-header.md`;
  update to `status-line.md` with the rename.
- `internal/loomcli/bootstrap.go:183` — a comment pointing at `headerLaunchCmd`/`headerpane.go` as the model for composing an exe command line.
  `headerpane.go` is deleted, so the comment must be repointed (the daemon spawn in `reedcli` is the surviving analog) or dropped.
- `docs/overview.md`, `manifest/designs/reed-fabric-standalone-api.md`, `manifest/designs/reed-header-selvage.md` — already in the Scope doc list above.

Inside `internal/reedcli`, the smoke files:

- `smoke_lifecycle_test.go` — **adapt**, and it is the largest single test edit in the task: four `st.HeaderPaneID` assertions retarget to `SelvagePaneID`, and its `pollPaneContains(..., "hub: "+HubPath)` assertion against the header pane's rendered text becomes unassertable once the text lives in a tmux option rather than a pane's screen.
  Replace that one with a `display-message -p '#{status-left}'` readback, not a pane-content poll.

- `smoke_header_keepalive_test.go` — **adapt** and rename to Selvage: the keepalive subject survives, only its mechanism changes.
- `smoke_headerscrollback_test.go` — **delete**: its subject is `headerBlockingPayload`'s ED2/ED3 screen-and-scrollback clear, which this task deletes outright.
  Selvage is never written to or cleared by reed, so there is nothing left to assert.
- `smoke_headerseed_test.go` — **adapt** and retarget to `lyx reed statusline`: `clihelp.SkipStencilSeedAnnotation` survives on the replacement verb, and the thing the test protects — a preview command leaving no stencilstore warnings and no git commits in the hub — is still true and still worth pinning.
- `smoke_dotfill_test.go`, `smoke_dotfill_measure_test.go` — **keep**, retargeting any assertion that names the header band onto the Selvage band.
  The dot-fill artifact is a resize-render property of the strand stack and is not header-specific;
  `internal/reedengine/doc.go`'s "Measurement record (repaint candidates)" block still governs it unchanged.

Inside `internal/reedengine`, the unit and integration test files that name the header — `headerpane_test.go` (delete with its subject), `header_test.go`, `apply_test.go`, `reconcile_test.go`, `spawn_test.go`, `lifecycle_test.go`, `attach_test.go`, `strand_test.go`, `generation_test.go`, `contract_integration_test.go`, `attachgeometry_integration_test.go`, `watchdog_integration_test.go` — all **adapt** with the rename and the band flip.
They are listed rather than summarized so the plan's batching sees the real spread: the `HeaderPaneID` → `SelvagePaneID` rename alone reaches a dozen test files across two packages.

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
- **Q:** How does the daemon map tmux sessions to worktrees? **A:** [auto-pick] ~~Scan the hub's immediate subdirectories and forward-match~~ — superseded in round 4: `filepath.Join(hub, sessionName)` recovers the worktree root directly, then `lyxcwd.ResolveWorktree` on that one path. **Why:** hub-mode `SessionName` is `filepath.Base(worktreeRoot)` verbatim and is refused rather than sanitized, so the derivation is exactly invertible; `ResolveWorktree` is still needed for the `Location` the geometry requires, and a registry file would go stale exactly when a worktree is renamed.
- **Q:** One hub-wide resize signal file or one per worktree? **A:** [auto-pick] One per worktree, unchanged. **Why:** the Durable-vs-Ephemeral State Invariant puts each worktree's ephemeral state under its own `.lyx`, and a shared file loses one of two simultaneous resizes.
- **Q:** Is the watch loop rewritten for multi-worktree operation? **A:** [auto-pick] No — it is rehosted; `Engine.Watch` stays the per-worktree entry point and the daemon owns only discovery, scheduling, and process lifetime. **Why:** the debounce, retry cap, and mode machine are already live-verified, and sharing one state across worktrees would couple unrelated failure streaks.
- **Q:** Does `watchdog:` stay a per-worktree key? **A:** [auto-pick] Yes, and the daemon re-reads a worktree's config when it re-enters the watched set. **Why:** one worktree's kill-switch must not disable a sibling's, and a once-per-process read would make a config edit need a daemon restart nobody would think to do.
- **Q:** How does the new blocking verb sit with the CLI/Cobra Invariant? **A:** [auto-pick] Swap `header --blocking` for `watchdog` in the interactive-handoff exception list, same commit. **Why:** the list is a closed enumeration; replacing the entry keeps it closed and keeps the invariant enforceable.
- **Q:** What replaces the `header:` config block? **A:** [auto-pick] `status_line: {template}` plus `selvage: {height_rows}`. **Why:** the two settings now describe unrelated mechanisms; reinterpreting the old keys would silently apply an operator's `header.height_rows` to a different pane in a different place.
- **Q:** Should the plan write removal logic for a stale `header:` block in existing `reed.yaml` files? **A:** [auto-pick] No. **Why:** `lyx config reconcile --apply` already strips stale leaves once the template drops them (see the round-1-gap entry below); the plan only has to keep the un-reconciled state harmless, which it is — nothing unmarshals `header:` into `Config` any more.
- **Q:** How is the `worktree` token fed? **A:** [auto-pick] `Ctx.WorktreeName`, filled from `lyxcwd.Location.WorktreeName` via a new `Geometry.WorktreeName` field. **Why:** deriving it inside `reedengine` with `filepath.Base` would be a per-module path derivation the Cwd Resolution Invariant reserves for `lyxcwd`.
- **Q:** (review round 6 gap) How does the daemon enumerate sessions with no engine in hand? **A:** [auto-pick] One new exported `reedengine.ListSessions(tmuxPath, socketKey)`; socket key from `ServerName(hubPath)`, tmux binary told via a new `--tmux` flag. **Why:** `TmuxCmd.run`/`output` are unexported and every exported `*Engine` method is session-bound, and telling the binary keeps the daemon deriving nothing.
- **Q:** (review round 6 gap) Which `list-sessions` outcomes count toward the idle-exit counter? **A:** [auto-pick] Anything but "exit 0 with at least one session name" — empty listing, no-server error, and any other failure alike. **Why:** the normal last-`down` case is an *error*, not an empty list, so the earlier wording left the main exit path undefined; and psmux exits identically with and without a server, so any rule that read the error would be unimplementable on Windows.
- **Q:** (review round 6 gap) Where do the daemon's log lines actually go? **A:** [auto-pick] `logger.SetDurableSinkDir(fabricengine.HubLogsDir(hub))` as its first action, before discarding stderr. **Why:** the cwd-anchored fallback arms only inside a lyx-owned worktree and never from a hub cwd — which is exactly where `cmd.Dir` pins this process — so every diagnostic the design leans on would otherwise go nowhere.
- **Q:** (review round 6 gap) Where in `attach` does the spawn go? **A:** [auto-pick] After the `Status()` pre-flight, before the blocking handover. **Why:** after the handover it would fire only on detach.
- **Q:** (review round 6 gap) Is a `watchdog: off` worktree watched-and-parked or never started? **A:** [auto-pick] Never started — map entry with a nil cancel, no goroutine. **Why:** `watchLoop` parks on `<-ctx.Done()` rather than returning when disabled, so starting one costs a goroutine per disabled worktree to do nothing; the entry still keeps the session known and departure bookkeeping uniform.
- **Q:** (review round 5 gap) The standalone `reedUp` seam takes no context — how does the in-process watcher get one, and what about `recover-batch`? **A:** [auto-pick] The seam becomes `reedUp(ctx context.Context, watch bool) error`; `run.go` in both CLIs passes true, `webstercli/recoverbatch.go` passes false. **Why:** `recover-batch` returns right after spawning a cold strand, so a watcher bound to its context would die before observing anything and one detached from it would be an unowned goroutine in an exiting process; an explicit parameter keeps that asymmetry visible at the call sites.
- **Q:** (review round 5 gap) What cancels a watcher when its session goes away but siblings remain? **A:** [auto-pick] The daemon holds a cancel func per watched session and cancels on departure from the live set. **Why:** `Engine.Watch` never returns while its context is live, so the goroutine would otherwise poll a dead session forever — and `watchLoop` reads config once at start, so "re-read on re-entry" is meaningless unless entries can leave.
- **Q:** (review round 5 gap) What do the four new status-line options do on Windows/psmux? **A:** [auto-pick] Attempt them unbranched and accept a named degrade: the layout self-corrects via the `#{status}` readback, but Windows may lose the identity text. **Why:** reed branches on Windows only where the consequence is silent and unrecoverable; a refused `set-option` fails loudly, changes nothing, and is already answered by a readback — and the plan carries a psmux verification item rather than treating this as settled.
- **Q:** (review round 4 gap) Where do the two new daemon timing constants live? **A:** [auto-pick] In `internal/reedcli`, beside the discovery loop that reads them; `reedengine`'s existing `watchdog*` constants stay unexported and untouched. **Why:** they would otherwise have to be exported from `reedengine` purely for a consumer in another package.
- **Q:** (review round 4 gap) Is the session → worktree mapping really a scan? **A:** [auto-pick] No — `filepath.Join(hub, sessionName)` recovers it directly. **Why:** hub-mode `SessionName` is `filepath.Base(worktreeRoot)` verbatim and is refused rather than sanitized; the "lossy derivation" premise behind the scan was true of standalone only, which this daemon never serves.
- **Q:** (review round 4 gap) What happens when the daemon cannot take its lock? **A:** [auto-pick] Contention (`nil` error, not locked) → Info, exit 0; a non-nil error → Error to the durable sink, exit non-zero; and the spawning helper `MkdirAll`s `HubScratchDir(hub)` first. **Why:** `TryAcquireWriteLock` does not create the parent, so a first-`up` hub could otherwise become permanently watchdog-less behind an Info line that reads like a normal losing race.
- **Q:** (review round 4 gap) What happens to tmux's window-status segment beside the identity text? **A:** [auto-pick] Suppressed via empty `window-status-format`/`window-status-current-format`. **Why:** reed's session has exactly one window, so the default `0:bash*` segment names nothing actionable and moves as the active pane's name changes.
- **Q:** (review round 4 gap) What happens to each existing header smoke file? **A:** [auto-pick] Adapt the keepalive and seed smokes, delete the scrollback smoke, keep both dot-fill smokes with their band assertions retargeted. **Why:** the scrollback smoke's only subject is the ED2/ED3 payload this task deletes; every other subject survives the mechanism change.
- **Q:** (review round 3 gap) Standalone reed sessions boot in-process, never through `lyx reed up` — what watches them once the header pane is gone? **A:** [auto-pick] `Engine.Watch(ctx)` as an in-process goroutine off the same `c.reedUp` seam, cancelled by the run's context. **Why:** standalone already has a long-lived supervising run for exactly the session's working lifetime, which is the thing hub mode lacks and the reason hub mode needs a daemon; a hub-shaped lock path is meaningless there anyway.
- **Q:** (review round 3 gap) Which layer spawns the daemon? **A:** [auto-pick] `internal/reedcli`, via one shared helper each of `up`/`resume`/`attach` calls after its engine op returns nil. **Why:** an engine-owned spawn would need `fabricengine` and an executable path the Told-Geometry Invariant keeps out of it; the CLI already holds the `Location` and can gate on the error being nil.
- **Q:** (review round 3 gap) Which binary does the spawn run, and what stops it under `go test`? **A:** [auto-pick] `os.Executable()`, suppressed by a `testing.Testing()`-initialised `suppressWatchdogSpawn` field on `reedCLI`. **Why:** re-exec'ing the test binary runs the suite recursively — the exact hazard `Engine.suppressHeaderLaunch` exists for, so the pattern moves to the layer that now owns the spawn rather than disappearing with the pane.
- **Q:** (review round 3 gap) Does the daemon rescan worktrees every cycle? **A:** [auto-pick] No — one `list-sessions` per cycle, and the worktree mapping is rebuilt only when the live session-name set changes; the per-candidate git spawns log at Debug. **Why:** `ResolveWorktree` shells out per candidate, so an unconditional five-second rescan would be N process spawns per cycle to learn nothing.
- **Q:** (review round 3 gap) What happens to `HeaderText`/`ValidateHeader`? **A:** [auto-pick] Renamed `StatusLineText`/`ValidateStatusLine` and kept, boot gate included, with the standalone-API doc's method inventory updated in the same commit. **Why:** the gate matters more now, not less — every `set-option` on the new path is non-fatal and merely logged, so a broken template would otherwise fail silently.
- **Q:** (review round 2 gap) How does the hub reach the detached daemon, and what is its cwd? **A:** [auto-pick] An explicit `--hub-path <abs>` flag, `watchdog` opted out of reed's `PersistentPreRunE`, and `cmd.Dir` pinned to the hub. **Why:** the process outlives the spawning worktree, both existing detached precedents pass the path explicitly, and a cwd handle on a worktree blocks that directory's deletion on Windows.
- **Q:** (review round 2 gap) Is `reed up` the only spawn site? **A:** [auto-pick] No — `up`, `resume` and `attach` all attempt it; `down` never does. **Why:** a crash while sessions stay alive would otherwise go unrepaired, and the status-line pins already heal on exactly those three paths, so narrower daemon coverage would be an unexplained asymmetry.
- **Q:** (review round 2 gap) What does `{{.worktree}}` render in standalone mode, which has no worktree? **A:** [auto-pick] ~~The normalized target basename~~ — corrected in round 4: the **raw** `filepath.Base(target)`, the identical expression `RepoName` already uses, so the default template reads `foo/foo · <stateDir>` with both tokens byte-identical even for a symlinked spelling. **Why:** `tokenvocab.Build` resolves every token unconditionally, so an unfilled token renders an empty segment; a repeated name reads as what it is, an empty one reads as a bug.
- **Q:** (review round 2 gap) Where exactly does the daemon's lock live? **A:** [auto-pick] `filepath.Join(fabricengine.HubScratchDir(hub), "reed-watchdog.lock")`, computed in `reedcli` and told to the daemon. **Why:** `HubScratchDir` is the already-declared accessor for `<hub>/_board/.lyx`, so no engine derives its own `.lyx` path.
- **Q:** (review round 2 gap) How long is the daemon's idle-exit grace? **A:** [auto-pick] Three consecutive empty enumerations on a five-second discovery cycle, as two new fixed constants beside the existing `watchdog*` timings. **Why:** those timings are deliberately fixed and non-configurable, and 15 seconds covers a `down` + immediate `up` without the daemon dying and respawning.
- **Q:** (review round 1 gap) Does `lyx config reconcile` remove a stale `header:` block, or leave it? **A:** [auto-pick] It removes it — but only on an explicit `lyx config reconcile --apply` or a `fabric clone`, never from a reed verb. **Why:** `yamlengine.Reconcile` diffs leaf key-paths both ways and `TestReconcileAll_DropsStaleReedClaudeKey` pins the identical stale-reed-key case; the original claim that reconcile is additive-only was wrong.
- **Q:** Does this task add a CONSTRAINTS.md invariant? **A:** [auto-pick] No new invariant — only the CLI/Cobra exception-list edit. **Why:** Selvage's rules bind one package and are already enforced by that package's own tests; CONSTRAINTS.md is for cross-cutting structure.
