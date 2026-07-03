# Module: mux (as-built)

> **Status:** ✅ built (`internal/muxengine` + `internal/muxengine/render` + `internal/muxcli`,
> wired into `lyx mux`). This doc is reconciled to the **as-built** design; earlier drafts of
> this file described mux constructing the Claude `--session-id` / launch command itself (a
> "decision-3" that predated the shuttle split) — **that decision is superseded**, see
> [Load-bearing psmux decisions](#load-bearing-psmux-decisions-verified) item 3 and
> [Resume](#resume-native---resume-via-the-stored-opaque-resumecmd) below. Every claim about
> psmux here is **grounded in hands-on testing** — see the evidence logs
> [`mux-exploration.md`](../research/mux-exploration.md) and
> [`mux-hooks-exploration.md`](../research/mux-hooks-exploration.md), and the TUI-behavior
> reference [`../research/psmux-tui-behavior.md`](../research/psmux-tui-behavior.md). The daemon,
> the `pane-died` auto-trigger, the `own-window` anchor, cross-worktree columns/mplex, and session
> portability are all **deferred** — see [Deferred](#deferred). See [roadmap.md](../roadmap.md)
> for sequencing.
>
> **Note:** A working proof-of-concept, `internal/muxpoccli`, informed this design (the tmux
> layout checksum, the `window_layout` string format, and the pane-id/parse plumbing are reused
> **verbatim** from it — see [Load-bearing psmux decisions](#load-bearing-psmux-decisions-verified)).
> muxpoc stays on disk as a reference and is **parked** — unwired from the `lyx` CLI, not deleted.
> See [overview.md#modules](../overview.md#modules).

mux is **the window to the world** — the one module that owns the live psmux session for a
worktree and decides what the operator sees. It is three things in one, split across three
packages (`muxcli -> muxengine -> render`, the only import direction — render never imports
the engine):

1. **Overlay over psmux** (`internal/muxengine`) — the shell that holds *every* psmux command:
   pane create/kill, send-keys, capture, the layout primitives, env hygiene, native `--resume`,
   and one named server per hub.
2. **Strand bookkeeping** (`internal/muxengine`) — the record of every managed process (a
   **strand**), persisted to `.lyx/mux.json`.
3. **Render** (`internal/muxengine/render`, a pure leaf package —
   [see below](#render--a-pure-function-over-strands)) — a pure function `Rules(strands, box) ->
   (layout, focus)` over a **closed, generic display vocabulary**. `muxengine` imports `render`
   and maps its own persisted records down to `render.Strand`; render never imports muxengine, so
   the graph stays acyclic.

It is the Go reimplementation of millpy's `_psmux.py` (which "doesn't work" and is not a
reference). Driven by `lyx mux <subcommand>` (`internal/muxcli`, the cobra CLI over `muxengine`);
shells out to `psmux.exe` via Go `exec` (no MSYS layer, so no slash-arg mangling — a hazard the
probe harness hit from git-bash).

## The strand model

Everything mux manages is a **strand**: one tracked process. Most strands are backed by a visible
pane, but a strand may be **`hidden`** — tracked by mux but not shown in the current layout (see
[Hidden strands](#hidden-strands-and-background-work)). A strand is a metadata record — mux
**stores every field a caller writes and reads none of them semantically**, and there is
deliberately **no domain `type` field**:

```
strand {
  guid           // mux-generated (128-bit crypto/rand, hex) — the durable identity/selector
  name           // caller-supplied descriptive label, stored verbatim — display-only, never a selector
  worktree       // owning worktree slug — generic grouping; mux does not know what the worktree does
  parent?        // the parent strand's guid — forms the spawn tree
  cmd            // opaque launch command string mux never parses
  resumeCmd?     // opaque resume command string, optional — see Resume
  sessionId?     // opaque metadata (e.g. claude's session id); mux neither writes nor reads it in v1
  paneId         // the live psmux pane id — ephemeral, re-derived on reconcile
  display {      // drawn ONLY from mux's closed vocabulary (below) — no height field, it is derived
    anchor:  top | below-parent | own-window | hidden
    focus:   bool
    shrinkWhenWaitingOnChild: bool
  }
}
```

**`guid` is the durable key; `name` is display-only.** `guid` is mux-generated at `AddStrand` and
is the identity every selector uses — `--parent <guid>`, `remove <guid>`, `UpdateStrand(guid,
…)`/`RemoveStrand(guid, …)` all key on it, and parent links store the parent's `guid`. `name` is a
caller-supplied label surfaced in `add`/`status`/`remove` output (v1 does not yet set it as the
psmux pane title); it is **not** a selector and carries no uniqueness requirement. It is composed at `add` time from a `mux.yaml`
`strand-name` template (default `<ROLE>:<ROUND>:<SHORT_GUID>`) — `--role`/`--round` are
formatting-only inputs consumed once to fill the template, never persisted or branched on (the
sharp difference from a forbidden `type` field).

### The contract: callers hand mux `{cmd, name, display, …}`, mux never reads it semantically

Anything that wants to be shown calls into mux with a command to run and a **generic display
spec** — never a domain type. mux runs the command in a pane, records the strand, and re-renders:

```
AddStrand{ cmd, name, worktree, parent?, resumeCmd?, display }  →  guid assigned, pane created
                                                                    (unless anchor:hidden), layout recomputed
UpdateStrand{ guid, display }                                   →  display changes over a strand's
                                                                    life → re-render (may surface a
                                                                    hidden strand; may not hide a live one)
RemoveStrand{ guid, recursive }                                 →  pane(s) killed, strand(s) dropped,
                                                                    cascades over descendants → re-render
```

Examples — the **caller** owns the domain→display mapping; mux never sees "loom-watcher":

```
loom:     AddStrand{ name:"loom-status", cmd:"lyx loom status --watch",
                     display:{ anchor:top } }
shuttle:  AddStrand{ name:"feature-x:plan-handler", cmd:"claude …", resumeCmd:"claude --resume …",
                     display:{ anchor:below-parent, focus:true, shrinkWhenWaitingOnChild:true } }
review:   AddStrand{ name:"cluster-rev-3", cmd:"claude …", display:{ anchor:own-window } }  // deferred anchor
```

### Why a closed, generic vocabulary (not a `type` field)

A domain `type` field (`loom-watcher`, `cluster`, …) would force mux to **know every type its
consumers might invent** — mux would import its own consumers' vocabulary. Circular. Instead mux
exposes a **closed, generic** set of display behaviors (anchors, `focus`,
`shrinkWhenWaitingOnChild`; heights are fully derived, not caller-set — see
[the height policy](#render--a-pure-function-over-strands)); callers compose from it. The analogy
is CSS: an element says
`position: sticky; top: 0`, never "I am a navbar" — the layout engine knows the generic
primitives, the author owns the domain→primitive mapping. Consequences:

- **A new domain thing needs zero mux change** — express it in the existing vocabulary.
- Only a genuinely **new geometric behavior** extends the (still domain-free) vocabulary. Rare.
- The domain (`type`) stays on the caller side; the presentation stays generic in mux.

### Hidden strands and background work

`anchor: hidden` is a tracked strand with **no place in the current layout** — mux knows about it
(session, lifecycle, resume) but does not show it. **In v1 a hidden strand has no live pane at
all**: `cmd` is not run at `add` time, so `anchor: hidden` is only valid at `add` (pending a
future surface). Two uses:

- **Surface on demand, one-directional in v1.** `UpdateStrand` may flip a strand's anchor off
  `hidden` (to `top`/`below-parent`) — creating its pane and running `cmd` — but **rejects
  `visible → hidden`** (`cannot hide a live strand in v1`); hiding a *running* strand is deferred
  background work. This keeps the invariant "a hidden strand never has a live pane" true across
  `add`/`update`/`resume` — `resume` also skips `hidden` strands (they are pending, not dead).
- **Background work via mux instead of `proc`.** A process that today would be a plain
  [`internal/proc`](README.md) background spawn (invisible) could instead run as a `hidden` strand,
  so it is **observable** — mux can gather all such strands into a dedicated psmux **background
  window** you switch to (`Ctrl+b`) when you want to see them. This is an optional future extension:
  mux gains the ability to *host* background processes, trading `proc`'s total invisibility for
  "hidden but surfaceable." Plain `proc` remains for truly fire-and-forget infra that never needs a
  pane; `hidden` strands are for background work you might want to inspect.

### The active-pane rule

Spawns nest (orchestrator → child → grandchild, ≤3 deep) and **only the deepest/bottom child is
active** — every ancestor is blocked waiting on its child. So the rule is: **the bottom-most child
is always the largest and sits at the bottom** (the active pane, where a human types). Ancestors
collapse to compact strips via `shrinkWhenWaitingOnChild`. This bottom-dominant layout — active/
bottom pane largest, ancestors collapsed to strips — is expressed declaratively over the `parent`
tree via a **derived** height policy (each shrink:true ancestor collapses to a fixed
`collapsedStripRows` strip and the active/bottom pane absorbs all remaining rows, so with the
default `collapsed_strip_rows: 3` in a 50-row window a 2-pane stack gives the bottom pane ~46 rows)
rather than a hand-coded **fixed** `activePaneShare` — see the height policy below.

### Render — a pure function over strands

`internal/muxengine/render` is `Rules(strands, box, params) -> (layout, focus, err)` —
deterministic, no I/O, and **total**: it always returns a valid, non-negative layout string (see
the clamp rule below), never errors on a well-formed strand set. `muxengine` maps its persisted
records down to `render.Strand` (only `guid`, `parent`, `display`, `paneId`, `live` — render never
sees `cmd`/`resumeCmd`/`sessionId`/`worktree`/`name`) and calls `Rules`. Because it is pure, it is
the clean **test surface**: feed a set of strand records, assert the layout string — golden-file
tests, no psmux and no agents needed.

**Two distinct layers inside render**, kept separate so an anchor is a localized, obvious edit:

- **Layout policy** (`policy.go`, `height.go`) — an explicit per-anchor dispatch (`top` /
  `below-parent` / `hidden` in v1; `own-window` is a closed vocabulary member but rejected —
  deferred until a consumer exists) and the **derived height policy**: given usable height
  `H_u` = window height − top band(s) − 1-row dividers, each `top` strand is a fixed
  `topBandRows` band (config, default 1); in the `below-parent` stack, a **shrink:true ancestor**
  collapses to a `collapsedStripRows` strip and the **active/focused strand plus every
  shrink:false strand** split the remainder equally, with any integer-division leftover going to
  the active/bottom pane (deterministic — the single bottom pane absorbs the
  remainder). A **clamp rule** keeps render total when fixed demand exceeds the window: shrink
  strips to 1 row, then reduce full panes to a `minFullRows` floor (config, default 3), then clamp
  earlier panes to 1 row as a last resort — a torn/negative height would make psmux reject the
  layout, so render never emits one. Sibling ordering (same-parent strands) is **insertion order**
  (position in the persisted strand table), so the layout string is deterministic.
- **Layout mechanics** (`checksum.go`, `layout.go`) — the `window_layout` string builder and the
  **tmux layout checksum** (see
  [Load-bearing psmux decisions](#load-bearing-psmux-decisions-verified) item 1); only the height
  *policy* feeding it changed.

**Re-render is on-demand, not event-driven or timed.** v1 is daemonless: the layout recomputes
**in-process on each mutation** (`AddStrand`/`UpdateStrand`/`RemoveStrand` recompute + apply within
the same call) and **on-demand on the mutating verbs** (`up`/`resume` and the next `add`/`remove`
reconcile against live `list-panes` and re-apply). `status` is the exception — it is **read-only**:
it cross-references `list-panes` to report live/dead but never reconciles (kills dead panes /
clears bindings) or re-applies the layout, so a query never moves focus or mutates state. There is
**no live `pane-died` listener** — a
dead pane is noticed the next time a verb runs, not instantly (the listener + a hidden handler
verb are deferred with [the daemon](#mux-daemon-deferred)). The whole
`read -> mutate -> persist -> render -> apply` cycle is guarded by one **mux operation lock** at
`.lyx/mux.lock`, acquired once at each engine operation's entry (never by a CLI verb directly —
see [Cross-process concurrency](#cross-process-concurrency-one-mux-operation-lock) below), so two
concurrent mutators (an operator's CLI verb and shuttle/loom driving `AddStrand` in-process) never
clobber each other's layout. Completion/idle detection is
[shuttle's concern](#completion-and-hooks-live-in-shuttle-not-mux) (via the file contract), never a
mux re-render trigger.

### Cross-process concurrency: one mux operation lock

Each public engine op (`AddStrand`/`UpdateStrand`/`RemoveStrand`, and the `up`/`resume`/
`status`-reconcile-apply ops) acquires `.lyx/mux.lock` (via `internal/lock`) **once at its own
entry** and holds it for its whole read→mutate→persist→render→apply cycle, composing internally
from unexported, unlocked helpers that never re-acquire the lock. **CLI verbs never take the lock
themselves** — they only call the engine op. This single-acquisition-point rule is mandatory, not
stylistic: `internal/lock` (gofrs/flock) is **non-reentrant across separate handles even
in-process** on Windows, so a CLI verb locking and then calling a lock-taking engine op would
self-deadlock. Lock ordering is strict **outer → inner**: `mux.lock` is always acquired before
`internal/state`'s own `mux.json.lock`. The lock is scoped per-worktree (`.lyx/mux.lock` lives in
the worktree's `.lyx/`), and the OS file handle releases automatically if a holding process dies —
v1 needs no stale-lock detection.

**(Future:** re-render *within one column* without touching the others — a per-column-independent
render — once cross-worktree columns arrive.)

### Persistence is load-bearing

The `display` spec, `parent` ref, and the **opaque launch + resume command strings** (`cmd` /
`resumeCmd`, built by the caller — shuttle's `claude …` and `claude --resume …`, which mux re-runs
without knowing they are Claude) are all **caller-supplied** — they cannot be reconstructed from
psmux alone (psmux knows where panes are, not that one "should be 1 line at top", is a child of
another, or how to relaunch). So mux **persists the full strand table** to `.lyx/mux.json` (local,
untracked, via `internal/state` — see
[overview.md](../overview.md#durable-vs-ephemeral-state-_lyx-vs-lyx)), keyed by `guid`. On every
**mutating** verb it **reconciles against live `list-panes`** (`status` reports without
reconciling): a strand whose pane is gone/`pane_dead=1` has its pane binding **cleared but its
record kept** (so `resume` can rebuild it) — only an explicit `remove` deletes a record. `resume` then recreates a pane for each not-live, non-`hidden` strand
and re-runs its stored `resumeCmd` (or `cmd` if it has none — see
[Resume](#resume-native---resume-via-the-stored-opaque-resumecmd)). Without this, a crash loses the
display intent, the spawn tree, and how to relaunch.

## Scope: one terminal per worktree (now); cross-worktree columns (later)

For now, **one terminal per worktree** — each worktree has its own psmux session, and mux arranges
that session's strands (the loom-status line on top, agents stacked below, active pane dominant).
The cross-worktree multi-column view (all worktrees in one window) is **deferred** and is *cheap*
when it comes: it is just the existing `worktree` strand field plus a rule that groups strands into
columns by slug. No architectural change — a metadata field and a rule.

## Load-bearing psmux decisions (verified)

1. **mux renders the layout string itself.** `select-layout even-horizontal` is fine for a flat
   row but **flattens** vertical sub-stacks, so once a column owns a stack the `window_layout`
   string must be emitted directly. The tmux checksum (rotate-right-1 accumulate, 16-bit) is
   verified and reproducible in Go; `select-layout "<csum>,<body>"` applies it atomically and
   honors sizing. The render sub-package computes the body; the overlay applies it.
2. **Loomyard never owns OS window management.** `lyx mux attach` attaches **in-place, in the
   operator's current terminal** — no popped/dedicated window in v1, so `mux.yaml` needs no
   terminal-emulator path. Popping a dedicated maximized terminal (needed if the driver is a
   headless programmatic session with no TTY to attach into) is deferred; precise multi-window
   docking and WT multi-tab launches remain brittle → best-effort, not core. psmux auto-resizes to
   whatever client attaches.
3. **Crash recovery via native `--resume`, given env hygiene — mux stores, never constructs, the
   resume command (supersedes an earlier "decision-3").** An earlier draft of this doc had mux
   itself assigning each pane a `--session-id` and building `claude --resume <id>`; that would make
   mux read/construct a Claude-specific command, breaking the dumb-carrier contract. **As built,**
   the caller (shuttle) builds both the launch `cmd` and the opaque `resumeCmd` and hands them to
   `AddStrand`; mux stores them verbatim and `resume` replays the stored `resumeCmd` per strand
   without parsing it. The one requirement mux itself owns: strip the inherited Claude-Code
   parent-session env from the psmux **server** spawn (see
   [Resume](#resume-native---resume-via-the-stored-opaque-resumecmd)).
4. **One named psmux server per hub — the orphan firewall — with one psmux session per
   worktree inside it.** mux boots its server as `psmux -L lyx-<hub-basename>-<short-hash>` — a
   legible hub basename plus a short hash of the hub's **absolute path**, derived deterministically
   via `internal/hubgeometry`; the session name is the worktree slug
   (`filepath.Base(WorktreeRoot)`), so sibling worktrees under the same hub share one server but
   never collide on a session. The hash is required
   for two reasons: the name must be unique per absolute hub path (two hubs sharing a basename on
   different paths must not collide onto one server), and a raw path is not a valid `-L` name
   (`:` / `\` / spaces). The basename keeps it human-legible in `psmux ls` and `lyx mux status`;
   the hash keeps it unique and socket-safe.
   Ownership is then unambiguous: that one server holds lyx's panes, so any other psmux process is
   provably stray and `lyx mux status` flags it. This fixes the **orphaned-process problem seen
   during exploration**, where anonymous per-probe servers left panes no one could attribute.

## Subcommands (v1, as built)

`up`/`resume` have a sharp boundary: **`up` never launches or relaunches a strand command — it is
substrate-only; `resume` is the only replayer.**

| Command | Does |
|---|---|
| `lyx mux up` | Ensure the server (clean env) + this worktree's session exist (boot if absent, no-op if up). Reconcile + apply the layout from the current strand table. **Runs no strand command.** |
| `lyx mux add` | `AddStrand` — `--cmd`, optional `--role`/`--round`/`--name`/`--resume-cmd`/`--parent <guid>`/`--anchor top\|below-parent\|hidden`/`--focus`. Prints the assigned `guid` + resolved `name`. A `hidden` strand gets no pane until surfaced. |
| `lyx mux remove <guid>` | `RemoveStrand` — requires `--recursive` on a non-leaf (fails otherwise: `strand has children, use --recursive`); the engine API itself always cascades. Result JSON lists every removed strand. |
| `lyx mux status` | **Read-only** cross-reference of `.lyx/mux.json` strands against the named server's live `list-panes`: report **this session's** tracked strands and their live/dead state, where **live means present *and* not `pane_dead`** (a crashed strand reads `live:false`). Unlike the mutating verbs, `status` does **not** reconcile (it never kills dead panes or rewrites bindings) and does **not** re-apply the layout — a query must not move focus or mutate state; the next mutating verb persists any correction. v1 does **not** actively enumerate stray/orphan psmux servers (a reliable listing on Windows is unverified) — the named server still provides the orphan-firewall property, `status` just doesn't scan for it yet. |
| `lyx mux attach` | `psmux attach` to this worktree's session **in the operator's current terminal, in place** (no popped window) — see the [envelope exception](#attach-is-a-documented-envelope-exception) below. |
| `lyx mux resume` | For every persisted strand that is **not live and not `hidden`**, (re)create its pane and run its stored `resumeCmd` (or `cmd` if it has none). Already-live strands are left untouched (no double send-keys); `hidden` strands are skipped (pending, not dead). Boots the server+session first if absent; after a **server rebirth** it clears every stale pane binding first, so a reborn session's reused pane ids are never mistaken for live strands. |
| `lyx mux down` | Kill **this worktree's session** (`kill-session`, never the shared per-hub server — sibling worktrees keep running) and clear this worktree's strand state. When this was the server's last session, the now-empty server is cleaned up too. |

`UpdateStrand` is engine-API-only — there is no `lyx mux update` verb in v1. Callers (`shuttle`,
`loom`, `review`) drive `AddStrand`/`UpdateStrand`/`RemoveStrand` in-process through the
`internal/muxengine` package API for anything the CLI's flag surface doesn't cover (e.g. focus
changes).

### `attach` is a documented envelope exception

`lyx mux attach` hands off the operator's stdio to `psmux attach` and blocks — the terminal-handover
tail cannot emit the CLI/Cobra Invariant's `output.Ok`/`Err` JSON envelope. Everything that can
fail (session missing, lock contention, reconcile) runs **pre-flight and stays on the envelope**;
only the post-handoff tail is exempt, and on success it emits **no** JSON. This follows the
existing interactive-`ide` precedent and is registered in
[CONSTRAINTS.md](../../CONSTRAINTS.md#cli--cobra-invariant).

### Naming

Under the cli/engine naming convention (see the "Package naming" rule in
[CONSTRAINTS.md](../../CONSTRAINTS.md#cli--cobra-invariant)), mux is split into `internal/muxcli` (the
cobra CLI) + `internal/muxengine` (the domain kernel) + `internal/muxengine/render` (the pure
display-vocabulary leaf). muxengine absorbs what earlier drafts split into separate
`shed`/`glance` modules: with one terminal per worktree and a closed generic display vocabulary,
the model (strand bookkeeping) and the view (render) sit cleanly inside mux without dragging
domain knowledge in. The render half is
[`internal/muxengine/render`](#render--a-pure-function-over-strands); `muxcli -> muxengine ->
render` is the only import direction.

## Environment assumptions (verified)

- psmux **3.3.4** at `C:\Code\tools\bin\psmux.exe`; default pane shell = PowerShell 7.
- **Launch with explicit binary paths, never PATH aliases.** Bare `pwsh` resolved to a
  0-byte WindowsApps execution-alias stub that renders nothing under ConPTY; the explicit
  `C:\Code\tools\powershell7\pwsh.exe` works. Same discipline for `claude`.
- No Node/npm needed (claude is a native binary; Loomyard is Go, psmux is Rust).
- **Loomyard MUST sanitize the psmux child env — this is mandatory, not defensive.** The *primary*
  use case is **claude itself running `lyx` to spawn reviewers/implementers**, so Loomyard is normally
  launched from inside a Claude Code session and its env carries `CLAUDE_CODE_CHILD_SESSION=1`
  (+ `CLAUDECODE`, `CLAUDE_CODE_SESSION_ID`, `CLAUDE_CODE_ENTRYPOINT`, `CLAUDE_CODE_SSE_PORT`). If
  these bleed into psmux, the pane's claude treats itself as a nested child and **silently stops
  persisting its transcript** — breaking resume. Since Loomyard (Go) is the chokepoint, it builds the
  **psmux server's `exec.Cmd.Env` without those vars** → server + all panes + all claude inherit a
  clean env. Crucially, strands spawned later inherit the *server's* env, so they stay clean even
  when the spawning `lyx` call was itself launched by a poisoned claude — as long as the server was
  started clean. See [Resume](#resume-native---resume-via-the-stored-opaque-resumecmd).

## What actually works (empirical guardrails)

These are the tested facts any implementation must respect. Full evidence in
[`mux-exploration.md`](../research/mux-exploration.md).

- **Scripting contract: `send-keys` + `capture-pane` is reliable** by pane name or `%id`.
  Use plain `capture-pane -p` (primary == alternate buffer on this build). Captured text is
  *rendered* — long lines come back wrapped at pane width.
- **`pane_current_command` is always `shell`** on Windows psmux — useless for "what's running
  in this pane". Use `capture-pane` content or `pane_pid`.
- **Claude's TUI renders and is fully drivable** in a pane. Marker grammar for a parser:
  `❯ ` = input line (echo or empty), `● ` = an assistant response, `✻ Verb for Ns` =
  completion, `✽`/`·` = spinner. `❯` is present in *every* state → never an idle signal.
  **Idle vs busy keys on the status bar: `shortcuts` (idle) / `interrupt` (busy)** as ASCII
  tokens (robust across the non-ASCII-space quirk seen on older claude builds).
- **Give claude its task at launch as the `[prompt]` arg.** Multi-line prompts cannot be
  typed into a running TUI: `paste-buffer` silently drops content and bracketed paste submits
  on every `\n`. Reuse turns are single-line and must send `Esc` first to clear leaked
  auto-suggest.
- **Teammate-mode does not auto-spawn panes here** (a delegated agent ran in-process). mux
  owns pane creation (`split-window` + launch); it does not rely on claude populating panes.
- **`pipe-pane` does not work on Windows psmux** (exit 0, no data) → no streaming-to-file.
  The only read mechanism is `capture-pane` on demand; a monitor must **poll-and-diff**
  (~500 ms; capture latency ≈ 23 ms, so ~4–5 % wall-clock).
- **psmux hooks are a convenience, not a foundation.** (These are psmux's *own* hooks — not
  Claude Code's; the CC hooks belong to [`shuttle`](shuttle.md), see
  [Completion and hooks](#completion-and-hooks-live-in-shuttle-not-mux).) `pane-died` fires via `run-shell -b` (needs
  `set-option -g remain-on-exit on`; fires with no client attached) — but **format vars do
  not expand in hook commands**, so the hook is a bare trigger that can't say *which* pane
  died (the handler must scan `list-panes -F "#{pane_id} #{pane_dead}"`). `monitor-silence` /
  `alert-silence` are **silently accepted but non-functional** → no built-in "agent went idle"
  signal from psmux. (`set-window-option` doesn't exist; use `set-option -w`.)
- **Smallest client wins.** Two clients on one session shrink the window to the smaller; a
  pop-up helper must pop **maximized** or be the sole client. `detach-client` by name is
  unsupported.

## Completion and hooks live in shuttle, not mux

mux **does not wire or interpret Claude hooks** — that would make it care what runs in a strand.
The Claude completion mechanism is entirely [`shuttle`](shuttle.md)'s:

- shuttle builds the full `claude` launch command **and** its `--settings` (the `Stop` /
  `SessionStart` hooks, the `PreToolUse` guardrails). The `Stop` hook routes to **shuttle's own
  channel — a file** (it writes the turn-end payload), which fits the file contract. `shuttle.Run`
  waits on that file + the output file and interprets `last_assistant_message` (done vs. asking).
  This is the event-driven idle/needs-input edge proven in
  [`mux-hooks-exploration.md`](../research/mux-hooks-exploration.md) — but it terminates in shuttle.
- mux receives this as an **opaque command string** via `AddStrand` and spawns it (in a pane, clean
  server env). It never reads the `--settings`; it does not know a `Stop` hook exists. The Claude
  marker grammar, the hook semantics, the completion interpretation — all of it (the research) lives
  in shuttle's Claude engine. mux is the dumb carrier.

(Platform gotcha shuttle must respect: hook commands run under git-bash on Windows, so a backslash
in a hook command is silently destroyed — POSIX paths only.)

**mux re-renders on structure, not on hooks.** The layout reacts to the strand set + the parent
tree, which change via the explicit `AddStrand` / `UpdateStrand` / `RemoveStrand` calls mux already
receives: a new child shrinks its parent, a removed child grows it back, and `focus` is set by the
caller (loom at an input gate, via `UpdateStrand`). The active-bottom-dominant arrangement is
**derivable from the tree** — no runtime idle signal needed.

**The `Agent` / `AskUserQuestion` guardrails are also shuttle's `--settings`** (Claude-specific),
carried opaque by mux: a `PreToolUse` deny on `Agent` (steer to `lyx mux add` so nested work
stays a visible strand) and on `AskUserQuestion` (steer to the file contract). mux never sees them.

## Liveness and orphans — generic, not Claude

mux's own "is this strand's process still alive" need is met **generically**, with no Claude
knowledge: reconcile against live `list-panes` and psmux `pane_dead`. The
[named server](#load-bearing-psmux-decisions-verified) (any psmux process outside
`lyx-<hub-basename>-<short-hash>` is provably stray) *enables* orphan detection, but **`lyx mux
status` does not actively enumerate stray servers in v1** — a reliable psmux-server listing on
Windows is unverified, so active listing is deferred and `status` reports only **this session**
(its tracked strands + their live/dead state). A richer cross-check — joining on Claude's own
session-state API — is *Claude-specific*, so it belongs with shuttle, surfaced to mux only as a
generic "this session is gone" signal if at all. mux's core liveness stays provider-invariant.

## Pause is not a mux concern

A graceful [pause](loom.md#graceful-pause) needs nothing special from mux. Pause is a *driver*
property — the Go loop stops spawning the next step; the strands stay in the table and their panes
stay **alive and idle**. mux keeps hosting them exactly as before; it has no "paused" state. (An
optional [in-agent interrupt](shuttle.md#in-agent-interrupt-optional) is just shuttle sending `ESC`
via mux send-keys — still no mux-side state.) The crash recovery below is the fallback for
*involuntary* death; pause deliberately keeps strands warm so that path is not needed.

## Resume: native `--resume` via the stored, opaque `resumeCmd`

`lyx mux resume` rebuilds a strand's pane for every persisted strand that is **not live and not
`hidden`**, then re-runs that strand's **stored `resumeCmd`** (falling back to `cmd` if the strand
has none) — mux never constructs a `--session-id` or a `claude --resume …` string itself, it only
replays what the caller (shuttle) built and handed to `AddStrand` opaquely. Already-live strands
are left untouched (no double send-keys). **Native resume works for programmatically-driven
panes** — verified end-to-end twice (independent thread + in-session): a full transcript persisted
(~14 KB, real `user`/`assistant` records) and after a `kill-server` crash the resumed pane recalled
its codeword.

```
lyx mux resume:
  read .lyx/mux.json → reconcile against live list-panes → boot server+session if absent
  for each strand that is not live and not hidden:
    <(re)create its pane with clean server env>     (mux owns the server env)
    <re-run resumeCmd, or cmd if resumeCmd is unset>   # opaque; caller built it, mux just replays it
  apply layout → re-persist pane ids
```

**The one requirement — lyx must sanitize the psmux child env (mandatory):**
`CLAUDE_CODE_CHILD_SESSION=1` (prime culprit), plus `CLAUDECODE`, `CLAUDE_CODE_SESSION_ID`,
`CLAUDE_CODE_ENTRYPOINT`, `CLAUDE_CODE_SSE_PORT`, must not reach the pane. If they do, claude treats
the pane as a **nested child session and suppresses transcript writing** — leaving only a ~100-byte
`ai-title` stub, so `--resume` finds nothing. This single inherited-env effect caused every
"doesn't persist" result during exploration; it is **not** send-keys, the visible window, or the
model. Because lyx (Go) spawns psmux, it is the natural chokepoint: build the **psmux server's
`exec.Cmd.Env` without** these vars (verified-fallback: clear them in the pane right before the
`claude` launch). Strands spawned later inherit the server's clean env.

> **Robustness gap (deferred — the "no transcript → fresh launch" fallback):** cold-recover must
> not replay a `resumeCmd` unconditionally. If a strand crashed *before any conversation* (no
> transcript), native `--resume` errors with "No conversation found"; a full recover would need to
> detect that and fall back to a fresh launch (re-running `cmd` instead). Detecting this needs pane
> reads, so it is deferred to shuttle/the daemon — v1 mux just replays the stored `resumeCmd`
> opaquely and does not inspect the result. (Note: for an unfinished step,
> [`loom`](loom.md#crash-recovery--resume-on-output-files-not-live-processes) respawns rather than
> resumes — mux's `resume` restores the *visible* session, not loom's correctness.)

> The `capture-pane` journal (see daemon) is an **optional** belt-and-suspenders log — useful for
> streaming and recap, but not required for resume.

---

## Deferred

Post-v1, kept so the design isn't lost. Each maps to a later [roadmap](../roadmap.md) milestone; do
not build until it is reached.

### `pane-died` auto-trigger (deferred with the daemon)

v1 is daemonless and re-renders **on-demand** (see [Render](#render--a-pure-function-over-strands))
— a dead pane is noticed the next time a verb runs, not instantly. An automatic, low-latency
re-render on pane death needs the psmux `pane-died` hook (`run-shell -b`, needs
`remain-on-exit on`, fires detached) calling back into a **hidden lyx handler verb** — but the hook
can't expand format vars (it is a bare trigger), and a daemonless one-shot process has nothing
listening for it. That whole path (hook + hidden handler verb + poller) belongs to the
[mux daemon](#mux-daemon-deferred); v1 deliberately does not add a hidden `on-pane-died` verb.

### `own-window` anchor (deferred until a consumer exists)

The anchor vocabulary is the closed four-member set `top | below-parent | own-window | hidden`,
but `render.Rules` **rejects** `own-window` in v1 (`lyx mux add --anchor own-window` is likewise
rejected) — it needs real window-management plumbing (review clusters spawning their own psmux
window) that has no consumer yet. Adding it later is a localized `render` change (a new policy
case + its golden test), not a redesign.

### Cross-worktree columns

All worktrees in one window, a column per worktree. As noted [above](#scope-one-terminal-per-worktree-now-cross-worktree-columns-later),
this is just the `worktree` strand field + a grouping rule — a metadata addition, not new
architecture. Deferred only because one-terminal-per-worktree is the right starting scope.

### mux daemon (deferred)

A process running **outside** psmux is required to detect that psmux itself died (nothing inside
psmux can). `lyx mux start` launches it as a standalone Windows process (not in a pane), running
until `lyx mux stop`.

```
mux daemon
  ├── psmux watcher  — cmd.Wait() blocks until psmux exits; recovers on crash
  ├── capture poller — per strand: capture-pane diff ~500ms → append to journal;
  │                    derive state from markers (shortcuts=idle, interrupt=busy);
  │                    detect death via pane_dead.
  ├── pane-died hook — optional low-latency nudge: run-shell -b → lyx mux event
  ├── Slack inbound  — Socket Mode listener; routes Slack messages → send-keys
  └── Slack outbound — journal/state events → filter → post to Slack
```

**Recovery uses `respawn-pane`** — it reuses the same pane id and revives a dead pane in place
(layout untouched); it respawns the default shell, into which mux runs the strand's stored
`resumeCmd` opaquely (env stripped). **Mutual watchdog:** psmux crash → daemon relaunches it and reloads strands;
daemon crash → next `pane-died` → `lyx mux ensure-daemon` relaunches it. IPC is a Windows named
pipe (`\\.\pipe\lyx`).

**Push alternative — control-mode `-CC`:** a single `psmux -CC attach` control client works on
Windows and pushes live `%output %<pane> <data>` for all panes (the real-time channel `pipe-pane`
fails to provide), as a raw VT100 stream (needs ANSI-strip). Use `capture-pane` polling for the
journal; reserve `-CC` for true streaming (Slack) or to replace N pollers with one control client.

### Slack relay (deferred)

One channel per worktree; the daemon bridges both directions. Outbound filter stays conservative —
needs-human-input 🔴, recovery-failed 🔴, crash-loop 🔴, recovered ℹ️, task-completed ℹ️. Channel↔
strand mapping lives in `.lyx/mux.json`.

### Session files and portability (the session-sync milestone)

Claude stores transcripts at `%USERPROFILE%\.claude\projects\<project>\<session-id>.jsonl` (the
project segment encodes the cwd with `:`, `\`, and `.` all replaced by `-`). With env hygiene
mux's panes populate these normally, but sessions are **not** portable across machines — `claude
--resume` only works where the JSONL exists. `lyx session push/pull` would copy them.
`CLAUDE_CONFIG_DIR` redirects *all* config (incl. credentials), so it is unsuitable for selective
session sync.
