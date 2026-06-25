# Module: mux (design)

> **Status:** design, nothing implemented yet. Every claim about psmux here is **grounded in
> hands-on testing** — see the evidence logs [`mux-exploration.md`](../research/mux-exploration.md)
> and [`mux-hooks-exploration.md`](../research/mux-hooks-exploration.md), and the TUI-behavior
> reference [`../research/psmux-tui-behavior.md`](../research/psmux-tui-behavior.md). The daemon + Slack relay are in
> [Deferred](#deferred). See [roadmap.md](../roadmap.md) for sequencing.
>
> **Note:** A working proof-of-concept already exists in `internal/muxpoc` — see
> [overview.md#modules](../overview.md#modules). muxpoc is a POC; it stays as a reference and is
> **not** extracted into mux. mux is built fresh, informed by what muxpoc proved.

mux is **the window to the world** — the one module that owns the live psmux session for a
worktree and decides what the operator sees. It is three things in one:

1. **Overlay over psmux** — the shell that holds *every* psmux command: pane create/kill,
   send-keys, capture, the layout primitives, env hygiene, native `--resume`, CC-hook wiring, and
   one named server per hub.
2. **Strand bookkeeping** — the record of every managed process (a **strand**), persisted to
   `.lyx/mux.json`.
3. **Render** (an internal sub-package, [`internal/mux/render`](#render--a-pure-function-over-strands))
   — a pure function `layout = rules(strands)` over a **closed, generic display vocabulary**.

It is the Go reimplementation of millpy's `_psmux.py` (which "doesn't work" and is not a
reference). Driven by `lyx mux <subcommand>`; shells out to `psmux.exe` via Go `exec` (no MSYS
layer, so no slash-arg mangling — a hazard the probe harness hit from git-bash).

## The strand model

Everything mux manages is a **strand**: one tracked process. Most strands are backed by a visible
pane, but a strand may be **`hidden`** — tracked by mux but not shown in the current layout
(optionally parked in a dedicated background window — see [Hidden strands](#hidden-strands-and-background-work)).
A strand is just a metadata record:

```
strand {
  id             // stable internal handle (mux-assigned)
  name           // human-readable label, caller-supplied (e.g. "feature-x:plan-handler")
  sessionId?     // claude session id, if it has one (for --resume)
  worktree       // owning worktree slug — generic grouping; mux does not know what the worktree does
  parent?        // the strand-id that spawned this one — forms the spawn tree
  display {      // drawn ONLY from mux's closed vocabulary (below)
    anchor:  top | below-parent | own-window | hidden
    height:  fixed(n) | grow | share
    focus:   bool
    shrinkWhenWaitingOnChild: bool
  }
}
```

`name` vs `id`: `id` is mux's stable internal handle; `name` is the human label the caller picks,
used for the psmux pane/window title, the dashboard, and `lyx mux status`. Like `display`, the name
is **opaque to mux** — it is a label, not a domain `type` mux branches on — so it does not
reintroduce the type-circularity.

### The contract: callers hand mux `{cmd, name, display}`

Anything that wants to be shown calls into mux with a command to run and a **generic display
spec** — never a domain type. mux runs the command in a pane, records the strand, and re-renders:

```
AddStrand{ cmd, name, worktree, parent?, display }  →  pane created, strand recorded, layout recomputed
UpdateStrand{ id, display }                          →  display changes over a strand's life → re-render
RemoveStrand{ id }                                   →  pane killed, strand dropped → re-render
```

Examples — the **caller** owns the domain→display mapping; mux never sees "loom-watcher":

```
loom:     AddStrand{ name:"loom-status", cmd:"lyx loom status --watch",
                     display:{ anchor:top, height:fixed(1) } }
shuttle:  AddStrand{ name:"feature-x:plan-handler", cmd:"claude …",
                     display:{ anchor:below-parent, height:grow, focus:true, shrinkWhenWaitingOnChild:true } }
review:   AddStrand{ name:"cluster-rev-3", cmd:"claude …", display:{ anchor:own-window } }
```

### Why a closed, generic vocabulary (not a `type` field)

A domain `type` field (`loom-watcher`, `cluster`, …) would force mux to **know every type its
consumers might invent** — mux would import its own consumers' vocabulary. Circular. Instead mux
exposes a **closed, generic** set of display behaviors (anchors, sizing modes, `focus`,
`shrinkWhenWaitingOnChild`); callers compose from it. The analogy is CSS: an element says
`position: sticky; top: 0`, never "I am a navbar" — the layout engine knows the generic
primitives, the author owns the domain→primitive mapping. Consequences:

- **A new domain thing needs zero mux change** — express it in the existing vocabulary.
- Only a genuinely **new geometric behavior** extends the (still domain-free) vocabulary. Rare.
- The domain (`type`) stays on the caller side; the presentation stays generic in mux.

### Hidden strands and background work

`anchor: hidden` is a tracked strand with **no place in the current layout** — mux knows about it
(session, lifecycle, resume) but does not show it. Two uses:

- **Surface on demand.** A strand can flip `hidden ⇄ visible` (via `UpdateStrand`) — run something
  out of sight, then pull it into view when you want to watch it.
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
collapse to compact strips via `shrinkWhenWaitingOnChild`. This is muxpoc's proven bottom-dominant
layout (2 panes → 56% bottom; 3 → 60% with 9+9-row ancestors), now expressed declaratively over the
`parent` tree rather than hand-coded.

### Render — a pure function over strands

The render sub-package is `layout = rules(strands)` — deterministic, no I/O. It reads the strand
set, applies the rules to the generic `display` fields + the `parent` tree, and emits a psmux
`window_layout` string that the overlay applies. Because it is pure, it is the clean **test
surface**: feed a set of strand records + dummy commands, assert the layout string — golden-file
tests, no psmux and no agents needed. Keep it an **internal sub-package** so it stays modular and
can be split back out later if mux ever bloats (it is, in effect, the absorbed "viewer").

**Re-render is event-driven, not timed.** Recompute on the **structural** events that change the
strand set or a strand's display: `AddStrand` / `UpdateStrand` / `RemoveStrand`, or a `pane_dead`.
The active-bottom-dominant arrangement is derivable from the parent tree, so **no runtime idle
signal is needed** — completion is [shuttle's concern](#completion-and-hooks-live-in-shuttle-not-mux)
(via the file contract), not a mux re-render trigger. Debounce a burst into one `ApplyLayout`.
**(Future:** re-render *within one column* without touching the others — a per-column-independent
render — once cross-worktree columns arrive.)

### Persistence is load-bearing

The `display` spec, `parent` ref, and the **opaque launch + resume command strings** (built by
shuttle — `claude …` and `claude --resume …`, which mux re-runs without knowing they are Claude)
are all **caller-supplied** — they cannot be reconstructed from psmux alone (psmux knows where panes
are, not that one "should be 1 line at top", is a child of another, or how to relaunch). So mux
**persists the full strand table** to `.lyx/mux.json` (local, untracked, via `internal/state` —
see [overview.md](../overview.md#durable-vs-ephemeral-state-_lyx-vs-lyx)). On startup it **reloads
the strands and reconciles** against live `list-panes` (and, generically, `pane_dead`): drop dead
strands, keep the live ones, re-apply the layout, and re-run the stored resume command per recovered
strand. Without this, a crash loses the display intent, the spawn tree, and how to relaunch.

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
2. **Loomyard never owns OS window management.** Popping ONE maximized terminal attached to a
   session is reliable (`lyx mux attach`); precise multi-window docking and WT multi-tab launches
   are brittle → best-effort, not core. psmux auto-resizes to whatever client attaches.
3. **Crash recovery via native `claude --resume`, given env hygiene** — mux assigns each pane a
   `--session-id` at launch; recovery relaunches `claude --resume <id>` per strand. The one
   requirement: strip the inherited Claude-Code parent-session env (see [Resume](#resume-after-crash--native---resume-with-env-hygiene)).
4. **One named psmux server per hub — the orphan firewall.** mux boots its server as
   `psmux -L lyx-<hub-hash>`, a name derived deterministically from the hub via `internal/paths`.
   Ownership is then unambiguous: that one server holds lyx's panes, so any other psmux process is
   provably stray and `lyx mux status` flags it. This fixes the **orphaned-process problem seen
   during exploration**, where anonymous per-probe servers left panes no one could attribute.

## Subcommands (v1)

| Command | Does |
|---|---|
| `lyx mux status` | Reconcile `.lyx/mux.json` strands against the named server's live `list-panes` + `claude agents --json`: report tracked strands, dead sessions, and **orphans** (psmux processes outside `lyx-<hub-hash>`). Cleanup is confirm-gated. |
| `lyx mux attach` | Pop / attach one maximized terminal to the worktree's psmux session. The popped terminal has a real TTY so claude renders there. |
| `lyx mux resume` | Rebuild the session from `.lyx/mux.json` and relaunch `claude --resume <id>` per strand (env stripped). |

Callers (`shuttle`, `loom`, `review`) drive `AddStrand`/`UpdateStrand`/`RemoveStrand` through the
package API, not the CLI.

### Naming

Go packages carry **no prefix** — `internal/mux`, matching `internal/board` (an early `gomux`
draft was dropped). mux absorbs what earlier drafts split into separate `shed`/`glance` modules:
with one terminal per worktree and a closed generic display vocabulary, the model (the strand
bookkeeping) and the view (the render sub-package) sit cleanly inside mux without dragging domain
knowledge in. The render half is the internal sub-package
[`internal/mux/render`](#render--a-pure-function-over-strands).

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
  started clean. See [Resume](#resume-after-crash--native---resume-with-env-hygiene).

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
carried opaque by mux: a `PreToolUse` deny on `Agent` (steer to `lyx mux spawn` so nested work
stays a visible strand) and on `AskUserQuestion` (steer to the file contract). mux never sees them.

## Liveness and orphans — generic, not Claude

mux's own "is this strand's process still alive / is that an orphan" needs are met **generically**,
with no Claude knowledge: the [named server](#load-bearing-psmux-decisions-verified) (any psmux
process outside `lyx-<hub-hash>` is provably stray) plus psmux `pane_dead`. A richer cross-check —
`claude agents --json` joined on `sessionId` (`state ∈ {working, blocked, done, failed}`) — is
*Claude-specific*, so it belongs with shuttle, surfaced to mux only as a generic "this session is
gone" signal if at all. mux's core liveness stays provider-invariant.

## Resume after crash — native `--resume` with env hygiene

`lyx mux resume` rebuilds the session from `.lyx/mux.json` and re-runs each strand's **stored
resume command** (shuttle built it as `claude --resume <session-id>`; mux runs it opaquely, not
knowing it is Claude). **This works for programmatically-driven panes** — verified end-to-end twice
(independent thread + in-session): a full transcript persisted (~14 KB, real `user`/`assistant`
records) and after a `kill-server` crash the resumed pane recalled its codeword.

```
lyx mux resume:
  read .lyx/mux.json → reload strands → render layout string → apply
  per strand: <spawn with clean server env>            (mux owns the server env)
              <re-run the strand's stored resume command>   # opaque; shuttle built it
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

> **Robustness gap:** cold-recover must not relaunch `claude --resume <id>` unconditionally. If a
> strand crashed *before any conversation* (no transcript), `--resume` errors with "No conversation
> found"; recover must detect that and fall back to a fresh `--session-id` launch. (Note: for an
> unfinished step, [`loom`](loom.md#crash-recovery--resume-on-output-files-not-live-processes)
> respawns rather than resumes — mux's `--resume` restores the *visible* session, not loom's
> correctness.)

> The `capture-pane` journal (see daemon) is an **optional** belt-and-suspenders log — useful for
> streaming and recap, but not required for resume.

---

## Deferred

Post-v1, kept so the design isn't lost. Each maps to a later [roadmap](../roadmap.md) milestone; do
not build until it is reached.

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
(layout untouched); it respawns the default shell, into which mux launches `claude --resume <id>`
(env stripped). **Mutual watchdog:** psmux crash → daemon relaunches it and reloads strands;
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
