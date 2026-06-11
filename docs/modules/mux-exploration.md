# mux — hands-on psmux exploration log

Empirical evidence for redesigning [`mux.md`](mux.md). The brief: design the mux
module, but **first** find out what psmux/tmux actually supports in practice with
Claude Code on Windows — what works reliably, how Claude attaches/resumes inside
panes, what the harness already owns vs. what mux must own, and what a minimal v1 is.
This file is the running, committed log of that exploration; the final `mux.md`
rewrite draws from it.

All probes use an isolated psmux server (`psmux -L mhgoprobe …`) so the operator's real
psmux is never touched. Scratch scripts live in `.scratch/mux-probe/` (gitignored).

Environment (verified 2026-06-11):
- psmux **3.3.4** (`C:\Code\tools\bin\psmux.exe`)
- pwsh **7.6.2** (`C:\Code\tools\powershell7\pwsh.exe`; a WindowsApps alias stub also exists)
- claude **2.1.173**, native install first on PATH (`C:\Users\hanf\.local\bin\claude.exe`)
- node **not required** (claude runs from the native Bun binary; mhgo is Go, psmux is Rust)
- psmux's default shell on this box = PowerShell 7

---

## Landed decisions (current)

1. **Layout grunnform: columns.** One full-height vertical column per worktree. Rows
   rejected (4 rows @ ~16 high = unusable). 4 columns @ ~69 wide acceptable; 3 @ ~91
   comfortable.
2. **A `Column` is a self-owned subtree object** (worktree + x-offset + width + ordered
   panes). v1: one pane per column. v2: the same column gets extra panes stacked
   downward (dispatched agents). No architectural change between v1/v2 — just more panes.
3. **mux computes the layout itself (layout-string renderer), not presets.**
   `even-horizontal` flattens vertical sub-stacks, so once a column owns an internal stack
   mux must render the `window_layout` string directly. The tmux layout checksum is
   verified and reproducible in Go.
4. **Orchestrator/hub = its own psmux *window*, not a column** — keeps the worktree
   overview at fewer, wider columns.
5. **Overflow / orchestrator-switch via psmux *windows* inside ONE attached client** — not
   WT tabs, not multiple psmux clients. `Ctrl+b` switches. This is the only "tab" mechanism
   mux can drive without client-mirroring, smallest-wins, or WT-quoting fragility.
6. **mhgo never owns OS window management.** Popping ONE maximized window attached to a
   session is fine and reliable (`mhgo mux attach`). Precise multi-window docking and WT
   multi-tab launching are brittle → best-effort, not core. mux is host-agnostic; psmux
   auto-resizes to the attached client.

Open sub-decisions: window naming; whether the orchestrator is always isolated or only on
overflow; reflow on worktree add/remove (re-render); stable column ordering across syncs.

---

## Verified findings

### Core scripting contract
- **send-keys + capture-pane: reliable.** Clean round-trip to a detached pane targeted by
  name and by `%id`, with the default shell and explicit pwsh.
- **Default shell = pwsh 7.** `new-session -d` with no `-- cmd` gives a PowerShell 7 prompt.
- **`pane_current_command` always = `shell`.** psmux/Windows never reports the real
  foreground process name → a daemon cannot use it to know what runs in a pane; must use
  `capture-pane` content or `pane_pid`.
- **Bare `pwsh` fails inside a pane; explicit path works.** `new-session -- pwsh` rendered
  nothing (WindowsApps execution-alias stub under ConPTY); explicit
  `C:\Code\tools\powershell7\pwsh.exe` rendered a prompt → launch with explicit binary
  paths, never PATH aliases.
- **`capture-pane` returns *rendered* text** → long lines come back wrapped at pane width
  (a parser must account for wrapping), not the raw scrollback line.

### Layout & the "Column owns its subtree" model
- Naive repeated `split-window -h` gives uneven columns (`99|49|24|25`) — split halves the
  active pane.
- `select-layout even-horizontal` rebalances a flat row to equal columns (`49|49|49|50`).
  Sufficient for v1 (one pane per worktree) — no math needed.
- **psmux natively models a column with a vertical sub-stack.** `dump-layout` for "3
  columns, 3rd split vertically" =
  `{65x50,0,0,2, 65x50,66,0,3, 68x50,132,0[68x24,132,0,4, 68x25,132,25,5]}`. `{…}` =
  left-right container (columns); `[…]` = top-bottom container (the stack inside a column).
- **`even-horizontal` FLATTENS sub-stacks** — re-applying it pulls a stacked child out into
  its own top-level column. So presets are v1-only; once a column owns a stack mux must
  compute layout itself.
- **Hand-built layout strings work.** Format `<csum>,<body>`; tmux checksum = rotate-right-1
  accumulate over body bytes (16-bit). Verified against a real dump (`723c == 723c`).
  `select-layout "<csum>,<body>"` is accepted (rc=0), **preserves column+sub-stack
  structure**, and **honors sizing** (asked `120|39|39` → got `118|37|43`; psmux normalizes
  a few cells for constraints). → mux owns a `render(columns) → layout-string` function,
  applied atomically via `select-layout`, recomputed on each mutation.

### Windowing & the width/height trade-off
On one 1440p/27" screen (~280×70 cells) you cannot get all three at once: (a) all worktrees
visible, (b) comfortable width per WT, (c) vertical height to stack agents. Measured: 4
columns = 69 wide (narrow); 2×2 grid = 139×35 (grid kills the height v2 stacks need); rows =
280×16 (too short). **Full-height columns are the right form**; the width crunch is the
operator's screen limit, mitigated by zoom and (if needed) psmux-window overflow.
- `Ctrl+b z` zoom (per-pane, not per-column-subtree) = the read/type grip.
- Multi-window pagination across psmux windows verified (`pag` session: 3 windows, per-window
  `split`+`even-horizontal`, `select-window`, `list-windows -F`, `list-panes -s` all work).
  3 cols/window = comfortable width + full height.

### psmux windows are TABS, not OS windows (critical boundary)
- psmux "windows" = tabs inside one attached terminal; `select-window` flips the tab the
  single viewport shows. psmux never opens, positions, or docks an OS window.
- **No clean simultaneous multi-window view.** psmux does NOT support tmux grouped sessions
  (`new-session -t pag` gave a session with 1 window, not shared; `session_group` empty).
  Two clients on one session mirror each other → the design must not assume seeing two pages
  at once.
- **Smallest client wins.** Two clients on one session → psmux sizes the window to the
  SMALLEST. A 120×29 pop-up shrank a 210×56 view to 120×29 (4 columns → 29×29). A pop-up
  helper must pop **maximized** or be the sole client.
- **`detach-client` by name is NOT supported** (`detach` is self-detach only) → a harness
  cannot remotely kick a client to fix sizing; pop maximized instead.

### Controlling Windows Terminal
- **Pop ONE window attached to a session: reliable** (done repeatedly):
  `wt -w new [--maximized] --title … pwsh -NoExit -File <attach.ps1>` via `Start-Process`.
  The popped terminal has a real TTY, so `attach` renders there (the agent's Bash tool has
  no TTY — `attach` there just prints the version banner and returns).
- **Driving WT's own multi-tab layout: brittle.** Two-tab launches (separate sessions per
  tab) repeatedly failed to attach the first tab; the `;`-delimited multi-tab commandline
  through the pwsh quoting/escaping layers + `--title` placement + implicit-first-tab rules
  is finicky and version/machine-dependent. Solvable, but the fragility is why it must not be
  load-bearing.
- WT tab title "Command Prompt" is cosmetic (default-profile name); panes run pwsh 7.

### End-to-end external-process proof
An external process drove the **entire** lifecycle headless, zero human attach: built a
3-column window, sent real `git log`/`git status` into panes, read output back via
`capture-pane`, split a column vertically to add an `agent-reviewer` pane (v2) and drove+read
it, then added a worktree paginated to a new window. **Only the human "watch" act (attach)
needs a TTY; the orchestrator never attaches — it sees via `capture-pane`.**

### From the psmux repo docs (`C:\Code\psmux\docs`)
- **claude-code.md**: psmux has first-class Claude Code agent-team support. Inside a pane it
  auto-sets `TMUX`, `CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS=1`, `PSMUX_CLAUDE_TEAMMATE_MODE=tmux`
  and defines a `claude` wrapper injecting `--teammate-mode tmux`; teammate agents then spawn
  into panes via split-window/send-keys. **Requires pwsh 7+** (present). Caveat: **Opus
  prefers `isolation:"worktree"`** (in-process, invisible; tmux integration hardcoded-disabled
  on Windows), so Opus agents won't appear in panes regardless. Teammate panes spawn only in
  **interactive** mode, not `-p`.
- **control-mode.md**: `psmux -CC` = tmux-compatible control protocol over stdin/stdout, with
  live `%output %<pid>` notifications, `dump-state` (JSON), and pane events — an alternative
  to capture-polling for a future daemon. ConPTY caveats: alternate-screen flag always false;
  `capture-pane` only sees the primary buffer; Ctrl+C hits ALL console processes (prefer app
  quit-keys); allow 4–6s for TUI-exit screen settle.

---

## Open / TODO

- [ ] Real interactive `claude` in a pane: render? send-keys reaches the TUI? capture reads it?
- [ ] `claude --resume <id>` in a respawned pane (does the session restore?)
- [ ] Teammate-mode: does the env injection + wrapper actually spawn teammate panes here?
- [ ] Hooks: do `pane-died`, `alert-silence`, `monitor-silence` fire via `run-shell -b`?
- [ ] `respawn-pane` behavior (with/without `-k`).
- [ ] control-mode `-CC` live `%output` smoke test from Go.
