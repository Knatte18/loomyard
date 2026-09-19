# reed: replace the header pane with a native status-line plus "Selvage"

> **Status: Implemented.** The design below describes what shipped, not a proposal.

## The problem

Today's single header pane (`internal/reedcli/header.go`'s `--blocking` branch, kept alive by `internal/reedengine`'s `ensureHeaderPaneLocked`) does **three** unrelated jobs at once — a second look during this design found a third job beyond the two originally identified:

1. Renders identity text ("hub: {{.hub}}") once at launch.
2. Keeps the tmux session from dying when every other pane dies — tmux destroys a session once its last window loses its last pane, so *something* must always be alive.
3. Hosts the watchdog daemon: `headerCmd`'s `--blocking` branch calls `eng.Watch(ctx)` (the resize-reconcile-and-reap loop, the already-Done `reed: watchdog daemon` roadmap item) before blocking forever. The daemon has no home of its own today — it only runs because the header pane's own process happens to call it on the way to blocking.

Conflating all three is why the pane needs ~230 lines of pane-lifecycle machinery across `apply.go` (fixed-band layout), `reconcile.go` (reap-exemption), `spawn.go` (special split-target), and `lifecycle.go` (creation, corpse-detection, up/resume healing) — and why it currently dies on a stray Ctrl-C, since `lyx reed header --blocking` runs as the pane's own root process with no signal handling. A naive split of only the first two jobs (an earlier draft of this doc) would have silently dropped the third: making Selvage "just an ordinary shell" removes the process that calls `eng.Watch()`, and nothing replaces it.

## The design

Split all three jobs onto three different mechanisms, instead of hardening the one pane that does all of them.

### Content → tmux's native status-line

tmux's own status-line (`status on`, `status-position bottom`) is not a pane — it cannot receive focus and cannot be typed into.

`internal/reedengine/windowsize.go`'s `pinGeometryOptionsLocked` renders the status-line text via `Engine.StatusLineText()` and issues seven `set-option` calls to pin it: `status on`, `status-position bottom`, `status-left <escaped text>`, `status-right ""`, `status-left-length <statusLeftLength(escaped)>`, and, window-targeted, `window-status-format ""` and `window-status-current-format ""`. `escapeStatusText` doubles every `#` in the rendered text before it reaches `status-left`, since tmux expands `#{…}`/`#[…]` inside a status string. `statusLeftLength` floors the length at tmux's own default of 10 and otherwise measures the escaped string in runes. `pinGeometryOptionsLocked` runs at boot (`lifecycle.go`) and again in the attach pre-flight (`attach.go`), and every call it issues is non-fatal — logged via `logger.Warn` and ignored, per the `geometry-tmux-failures-are-non-fatal-everywhere` decision — because a `set-option` failing loudly changes nothing and is answered by the `#{status}` readback (`readStatusRowsLocked`) rather than trusted from `set-option`'s own exit status.

- The status-line's three tokens, `{{.repo}}`, `{{.hub}}` and `{{.worktree}}` (`tokenvocab.Ctx`), are static and never need a live update. `StatusLineText()` makes no tmux round trip and reads only `cfg`+`geom`.
- A status-line alone does **not** keep a session alive when every real pane dies — killing all panes in a session with the status-line on still kills the whole tmux server. The status-line only ever covers content, never keepalive.
- Showing both repo and worktree needs the `worktree` token this task added to `tokenvocab` alongside the pre-existing `repo` and `hub` tokens.

### Keepalive → a permanent pane named "Selvage"

The pane that must always exist is named **Selvage** — the self-finished edge of a woven fabric that keeps it from fraying, chosen because it sits at an edge (the bottom of the screen) and exists to keep the whole session from unraveling. (`Anchor` was the first idea; rejected because it collides with the unrelated, already-shipped `AnchorPath` in `hubgeom`/`lyxcwd`.)

Selvage is a deliberately ordinary shell — **not** a custom binary, and it needs no signal-handling code:

- A plain shell already survives an accidental Ctrl-C at its idle prompt (interactive bash ignores SIGINT while waiting at the prompt).
- Ctrl-C to a foreground job running inside it kills only that job, not the shell — the pane survives.
- It still dies cleanly when `reed down` deliberately tears the session down (pty close / SIGHUP path, distinct from SIGINT) — a session's life is bounded to its worktree's life, not immortal. Killing it is a normal part of worktree housekeeping, not something to defend against.

Being a real, typeable shell is intentional, not a residual flaw: it is the always-on control terminal for running `lyx`/`reed` commands directly against the worktree — e.g. `lyx reed add` to spawn a new strand (a new Claude instance) — without needing a spare pane first.

`internal/reedengine/selvagepane.go`'s `ensureSelvagePaneLocked` ensures Selvage exists and is alive on both Up and Resume, (re)creating it when missing, dead, or gone via `splitSelvagePaneAtBottomLocked`, which splits a new pane in below the physically bottom-most live pane and retries once behind an even-vertical re-tile when the first attempt has no room — the retry is what keeps a lost or stale `ReedState.SelvagePaneID` from wedging a worktree permanently. `ReedState.SelvagePaneID` (`json:"selvagePaneId,omitempty"`) is the persisted binding; `reed.yaml`'s `selvage.height_rows` config key configures the band's height (the render side is `render.Selvage`/`Params.Selvage`).

### Placement

Selvage is pinned to the bottom, one row tall by default, with every strand pane scaled to fill the remainder above it — mirroring the former fixed-band header layout in `apply.go`, just from the opposite edge.

It stays in the **same single window** as every strand, never a second window:

- Reed has no window support today (see `reed: own-window strand anchoring`).
- *(confirmed live)* tmux auto-switches the attached client to a window the instant its previously-active window loses its last pane — a "hidden" second window holding Selvage would pop into view at exactly the moment it needs to stay out of the way, defeating the point.

### The single-pane degenerate case

When Selvage is the only pane left (every strand has died), it automatically fills the entire window — tmux always tiles existing panes to 100% of the window, so there is no "shrink to minimum, leave blank space" option. This needs no special-casing: it is tmux's own default tiling behavior, not something reed's layout math has to detect or handle.

### Watchdog daemon → a detached, per-hub background process

`eng.Watch(ctx)` needs no tty and no pane at all — it only ever issues tmux commands against a socket from the outside. Once it is no longer riding along inside the header pane's process, its hosting is chosen freely; it does not have to live on Selvage (or on any pane).

Granularity: **per hub, not per worktree-session and not per machine.**

- **Per machine** was considered and rejected. The tempting argument (a reconcile daemon is cheap, so why not have just one) optimizes for CPU, which was never the actual cost. The real cost is that a single machine-wide daemon must multiplex across every hub's own tmux socket at once (today's daemon only ever touches the one socket it was started against), discover hubs appearing and disappearing over time, and — since it no longer maps to any single `reed up`/`down` call — needs its own independent lifecycle (e.g. autostart at login) rather than starting and stopping with worktree activity. It also turns every hub on the machine into one shared blast radius: a crash takes down reconcile for all of them at once, not just one.
- **Per hub** matches a boundary that already exists — `SocketKey`/`ServerName` is already keyed on hub path (`hubgeom.ReedGeometry`), so a per-hub daemon only ever talks to the one tmux socket that hub already owns, no multiplexing needed.

`internal/reedcli/watchdog.go` implements the daemon as `lyx reed watchdog --hub-path <abs> --tmux <path>`: told its hub path and tmux binary on the command line, opting out of reed's normal cwd/location/config resolution. `runWatchdogLoop` polls `reedengine.ListSessions` (the one new exported, engine-less function `internal/reedengine` gained for this — see the `told-geometry-keeps-the-daemon-out-of-reedengine` decision below) against the hub's socket every `watchdogHubDiscoveryCycle` (5s), enters newly-appeared sessions by building a `*reedengine.Engine` for each and starting its `Engine.Watch` goroutine (unless that worktree's own config says `watchdog: off`), and tears down departed ones. Single-instance per hub is enforced by a `reed-watchdog.lock` file under `fabricengine.HubScratchDir(hub)`: a spawn that finds the lock already held exits 0 immediately, since a racing spawn costing one short-lived process is the expected, harmless outcome. The daemon's own stdio is discarded and its diagnostics land in `fabricengine.HubLogsDir(hub)` instead.

Lifecycle: the daemon exits itself rather than being killed by any reed verb. `watchdogHubDiscoveryCycle`/`watchdogHubIdleCycles` govern this — the idle counter increments on anything other than an affirmative listing (exit 0 with at least one session name), so an exit-0 empty listing, a "no server running" error, and any other `list-sessions` failure all count as idle alike; a non-empty listing resets the counter to zero, and `watchdogHubIdleCycles` (3) consecutive idle cycles exit the daemon. `reed down` never kills it — the daemon notices the hub going quiet on its own next discovery cycle instead.

It is spawned as a **detached child**, never an OS-level service, launch agent, or systemd unit — that broader pattern is explicitly out of scope for this task. Three spawn sites attempt it: `up`, `resume`, and `attach`, each after its own engine op returns without error, via `os.Executable()` + `exec.Command` + `proc.Detach`.

Each discovery cycle also decides whether every live session name still names a worktree that exists on disk. For a session name in the cycle's listing, `filepath.Join(hub, name)` is the exact worktree root that name maps to — hub-mode `SessionName(worktreeRoot)` is `filepath.Base(worktreeRoot)` verbatim, so the join is exact rather than a scan — and the gone verdict for that path is only-proven-gone: an `os.Stat` returning a not-exist error, or succeeding against something other than a directory, counts as gone, while a successful stat of a directory, or any other stat outcome including a permission or I/O error, counts as not gone. The hub directory itself is probed the same way once per cycle, first: when it does not stat live, the cycle skips the reap pass entirely, without advancing, resetting, or pruning a single counter, because an unmounted or moved hub makes every name's join look gone at once while the socket keeps listing every session exactly as before. A name must be observed gone on `watchdogOrphanGoneCycles` (3) consecutive affirmative cycles before it is reaped, and a cycle whose listing was not affirmative leaves every counter exactly where it was rather than resetting or advancing it — the three cycles the rule counts are three consecutive *affirmative* ones, which may span a tmux outage.

Once a name crosses the threshold, the reap captures the session's pane process closure before issuing an exact-match `kill-session`, and only reaps that closure afterwards — the same ordering `Engine.Down` already uses, and for the same reason: `kill-session` reparents the pane children, so a closure computed after the kill collapses to the root pids alone and silently loses the detached agent descendants the reap exists to kill. The reap runs in its own goroutine, off the discovery loop's own tick, with the loop keeping an in-flight set that excludes the name being reaped from both re-selection and the appeared set until that goroutine finishes. Both the tmux half and the process-tree half of the reap reach `reedengine.ReapSession`, the second engine-less exported function that package carries alongside `ListSessions` — it holds no lock and persists nothing, since the worktree it reaps no longer exists to lock or persist against. Its process-tree half, on Windows, walks descendant pids through a told shell path: `--shell` is told to the daemon by the same spawn construction that tells it `--tmux`, accepted on every platform but never required, so an empty value degrades the reap to killing pane root pids without their descendants rather than refusing to start the daemon at all.

Two limitations are accepted rather than closed, since both are operator-visible: an orphan on a hub with no running daemon survives until the next lyx command against that hub re-spawns one, and a hand-made session on the hub's socket with no matching worktree directory is reaped ~15s later, same as any other.

## Open items

- **Windows/psmux verification of the status-line options is unverified, not verified.** The exact check: run `lyx reed up` under psmux, then read back `#{status}`, `#{status-position}`, `#{status-left}` and `#{window-status-format}`, recording which of the options survived. The named degrade accepted in the meantime: if psmux refuses them, Windows loses the identity text, while Selvage, the layout, the reap rules and the watchdog are all unaffected, and `status-position` falling back to `top` is acceptable on its own since the status-line is not a pane and its edge is independent of Selvage's. reed does not branch on Windows here — unlike `hookInstalledLocked`, the one place it does branch — because a refused `set-option` fails loudly into the log, changes nothing, and is answered by the `#{status}` readback, whereas `hookInstalledLocked`'s consequence would be silent and unrecoverable. This decision is to be revisited against the verification's result, not treated as settled.

## Module-local Selvage rules

Kept here rather than in `CONSTRAINTS.md`, because they describe this package's own design rather than a cross-cutting invariant another module could violate:

- Selvage is always physically bottom-most in the window.
- Selvage is never a strand.
- Selvage is never written to, cleared, or sent keys by reed — it is a real, usable terminal an operator can run lyx/reed commands in.

## Standalone disposition

Standalone reed runs `Engine.Watch` as an in-process goroutine off the `reedUp` seam, never the daemon — the per-hub watchdog daemon is a hub-mode-only mechanism, since standalone mode has no hub to key a shared socket off of.

## Related

- [loom-step.md](loom-step.md) and the `ly-drive` skill — the shipped `reed: born-as-strand for the operator's loom start attach` depended on reed's pane lifecycle being solid, which this item was a prerequisite for.
- `reed: per-hub daemon reaps orphaned sessions` extends this daemon, once it exists, to also check whether each live session's worktree still exists on disk and tear down any that don't.
