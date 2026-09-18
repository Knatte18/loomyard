# reed: replace the header pane with a native status-line plus "Selvage"

> **Status: Planned, not yet built.** Every claim below marked *(confirmed live)* was tested in a throwaway tmux session during design; everything else is reasoned but unverified.

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

- The header's only two tokens today, `{{.repo}}` and `{{.hub}}` (`tokenvocab.Ctx`), are static and never need a live update *(confirmed live: `HeaderText()` makes no tmux round trip and reads only `cfg`+`geom`)*.
- *(confirmed live)* A status-line alone does **not** keep a session alive when every real pane dies — killing all panes in a throwaway session with the status-line on still killed the whole tmux server. The status-line only ever covers content, never keepalive.
- Showing both repo and worktree (not just hub) needs a new `worktree` token added to `tokenvocab` alongside the existing two — this token does not exist today.

### Keepalive → a permanent pane named "Selvage"

The pane that must always exist is named **Selvage** — the self-finished edge of a woven fabric that keeps it from fraying, chosen because it sits at an edge (the bottom of the screen) and exists to keep the whole session from unraveling. (`Anchor` was the first idea; rejected because it collides with the unrelated, already-shipped `AnchorPath` in `hubgeom`/`lyxcwd`.)

Selvage is a deliberately ordinary shell — **not** a custom binary, and it needs no signal-handling code:

- *(confirmed live)* A plain shell already survives an accidental Ctrl-C at its idle prompt (interactive bash ignores SIGINT while waiting at the prompt).
- *(confirmed live)* Ctrl-C to a foreground job running inside it kills only that job, not the shell — the pane survives.
- It still dies cleanly when `reed down` deliberately tears the session down (pty close / SIGHUP path, distinct from SIGINT) — a session's life is bounded to its worktree's life, not immortal. Killing it is a normal part of worktree housekeeping, not something to defend against.

Being a real, typeable shell is intentional, not a residual flaw: it is the always-on control terminal for running `lyx`/`reed` commands directly against the worktree — e.g. `lyx reed add` to spawn a new strand (a new Claude instance) — without needing a spare pane first.

### Placement

Selvage is pinned to the bottom, one row tall, with every strand pane scaled to fill the remainder above it — mirroring today's fixed-band header layout in `apply.go`, just from the opposite edge. *(Not yet confirmed live, unlike everything above.)*

It stays in the **same single window** as every strand, never a second window:

- Reed has no window support today (see the Someday `reed: own-window strand anchoring` item).
- *(confirmed live)* tmux auto-switches the attached client to a window the instant its previously-active window loses its last pane — a "hidden" second window holding Selvage would pop into view at exactly the moment it needs to stay out of the way, defeating the point.

### The single-pane degenerate case

*(confirmed live)* When Selvage is the only pane left (every strand has died), it automatically fills the entire window — tmux always tiles existing panes to 100% of the window, so there is no "shrink to minimum, leave blank space" option. This needs no special-casing: it is tmux's own default tiling behavior, not something reed's layout math has to detect or handle.

### Watchdog daemon → a detached, per-hub background process

`eng.Watch(ctx)` needs no tty and no pane at all — it only ever issues tmux commands against a socket from the outside. Once it is no longer riding along inside the header pane's process, its hosting can be chosen freely; it does not have to move to Selvage (or to any pane).

Granularity: **per hub, not per worktree-session and not per machine.**

- **Per machine** was considered and rejected. The tempting argument (a reconcile daemon is cheap, so why not have just one) optimizes for CPU, which was never the actual cost. The real cost is that a single machine-wide daemon must multiplex across every hub's own tmux socket at once (today's daemon only ever touches the one socket it was started against), discover hubs appearing and disappearing over time, and — since it no longer maps to any single `reed up`/`down` call — needs its own independent lifecycle (e.g. autostart at login) rather than starting and stopping with worktree activity. It also turns every hub on the machine into one shared blast radius: a crash takes down reconcile for all of them at once, not just one.
- **Per hub** matches a boundary that already exists — `SocketKey`/`ServerName` is already keyed on hub path (`hubgeom.ReedGeometry`), so a per-hub daemon only ever talks to the one tmux socket that hub already owns, no multiplexing needed.
- Lifecycle is a genuine open question at this granularity (see below): unlike a per-session daemon, it can no longer simply start with one worktree's `reed up` and stop with that same worktree's `reed down`, since other worktree sessions under the same hub may still be alive.

## Open items

- The `worktree` token for the status-line template does not exist yet — needs adding to `tokenvocab`.
- Exact layout-math change in `apply.go` to flip the fixed band from top to bottom is reasoned by analogy, not yet implemented or tested.
- Whether Selvage's shell should be the user's own `$SHELL` or a fixed `bash` is not yet decided.
- The per-hub daemon's exact lifecycle is undecided: started by the first `reed up` in a hub and stopped by the last `reed down` (reference-counted), or something else entirely (e.g. spawned once at `fabric clone` time and left running for the hub's whole existence)?
- Whether the daemon should keep being spawned as a detached child of whichever `reed up` starts it, or move to a proper OS-level service/supervisor pattern, is not yet decided.

## Related

- [loom-step.md](loom-step.md) and the `ly-supervise` skill — a longer-term Someday item (`reed: born-as-strand for operator, watchdog, and orchestrator sessions`, in `manifest/roadmap.md`) depends on reed's pane lifecycle being solid, which this item is a prerequisite for.
