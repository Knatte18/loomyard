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
7. **Crash recovery via native `claude --resume` — works, given env hygiene.** mux assigns each
   pane `--session-id <uuid>` at launch, records it (+ worktree + layout) in local-state, and after
   a crash `mhgo mux resume` rebuilds the layout and runs `claude --resume <session-id>` per pane.
   **Verified end-to-end twice** (this session + an independent thread): full transcript persisted
   (~14 KB, real `user`/`assistant` records), and after `kill-server` the resumed pane recalled the
   codeword. **The one requirement:** strip the inherited Claude-Code parent-session env before
   launching claude in a pane — `CLAUDE_CODE_CHILD_SESSION=1` (prime culprit), plus `CLAUDECODE`,
   `CLAUDE_CODE_SESSION_ID`, `CLAUDE_CODE_ENTRYPOINT`, `CLAUDE_CODE_SSE_PORT`. When inherited (i.e.
   mhgo invoked from inside a Claude Code session), claude treats the pane as a nested child and
   **suppresses transcript writing** → empty resume. A standalone-CLI mhgo has a clean env already;
   mux should strip them defensively regardless. mux's `capture-pane` journal is then an **optional**
   belt-and-suspenders / higher-availability log, not the primary resume mechanism.

Open sub-decisions: window naming; whether the orchestrator is always isolated or only on
overflow; reflow on worktree add/remove (re-render); stable column ordering across syncs;
exact journal format + cadence + how much scrollback to re-inject on resume (full vs summary).

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

### Real interactive `claude` in a pane (verified 2026-06-11)
Cross-reference: [`../psmux-tui-behavior.md`](../psmux-tui-behavior.md) (prior millhouse findings,
claude 2.1.158/159). This session re-verified on claude **2.1.173**.
- **Renders + drivable.** A real interactive `claude` TUI launched via `send-keys` in a pane
  renders fully in `capture-pane`; `send-keys -l "<text>"` + `send-keys Enter` submits a prompt;
  the response is read back via `capture-pane`. Round-trip confirmed repeatedly.
- **Primary == alternate here.** `capture-pane -p` and `capture-pane -a -p` returned identical
  55-line output → mux can use plain `capture-pane -p`. (millhouse's `-a` insistence was
  version-specific.)
- **Marker grammar** (for a parser): `❯ ` = input line (echo of sent text, or empty = idle);
  `● ` = an assistant response; `✻ Verb for Ns` = completion marker; `✽`/`·` = spinner.
  `❯` is present in ALL states → never an idle signal. Idle vs processing keys on status-bar
  ASCII tokens: **`shortcuts`** (idle) / **`interrupt`** (processing).
- **Status-bar spaces are ASCII on 2.1.173** (`20 3f 20 66 6f 72 20 73 68 6f 72 74 63 75 74 73`
  = `? for shortcuts`). millhouse's non-ASCII-space bug (2.1.158) did NOT reproduce. Still match
  the single token (`shortcuts`/`interrupt`) to stay version-agnostic.
- **Multi-line prompts cannot be typed into a running pane** (paste-buffer drops content;
  bracketed paste submits on each `\n` — see psmux-tui-behavior.md). → mux gives each claude its
  task **at launch** (positional `[prompt]` arg / `Get-Content -Raw` script), not by typing into
  a live TUI. Reuse = single-line only, and must send **Esc** first to clear leaked auto-suggest.
- **Teammate-mode does NOT auto-spawn panes here.** With `CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS=1`
  + `--teammate-mode tmux` + interactive + attached, asking haiku to delegate produced an
  **in-process** `Agent(...)` (no new pane; pane count stayed 1). → **mux owns pane creation**
  (`split-window` + launch); it must not rely on claude populating panes via its teammate
  integration. (Confirms the "Column owns its subtree, mux renders layout" decision.)

### Session persistence & `--resume` — RESOLVED: native resume works; root cause was inherited env
**Final determination (after a long, flip-flopping investigation, confirmed twice):** native
`claude --resume` **works for programmatically-driven psmux panes.** The persistence failures were
an artifact of the test harness: probes ran from **inside a Claude Code session**, whose
environment exports `CLAUDE_CODE_CHILD_SESSION=1` (+ `CLAUDECODE`, `CLAUDE_CODE_SESSION_ID`,
`CLAUDE_CODE_ENTRYPOINT`, `CLAUDE_CODE_SSE_PORT`); psmux passed these into the panes, and claude
treated each pane as a **nested child session and suppressed transcript writing** → only an
`ai-title` stub. **Strip those vars before launching claude in the pane and persistence + native
resume work perfectly** — verified independently twice (14.3 KB transcript with real
`user`/`assistant` records; `claude --resume <id>` after `kill-server` recalled the codeword). Not
send-keys, not the visible window, not the model — inherited parent-session env. See Landed
decision 7. The evidence trail (note: bullets below marked "did NOT persist" were all run with the
poisoning env present — they are the symptom, not a limitation):
- claude stores transcripts at `~/.claude/projects/<cwd-encoded>/<session-id>.jsonl`
  (path encoding replaces `:`, `\`, AND `.` with `-`).
- **A real keyboard-typed interactive `claude` in a psmux pane persists the FULL transcript,
  and `--resume` restores it — VERIFIED end-to-end.** Operator launched `claude --session-id
  <id>` in an attached psmux pane, typed "Husk kodeordet appelsin001" by hand, stopped it → the
  `.jsonl` was **18.6 KB / 17 records** with real `user`/`assistant`/`last-prompt` entries. Then
  `claude --resume <id>` in a fresh psmux pane (same cwd) **reopened the prior conversation and
  recalled the codeword** ("Kodeordet du ga meg var appelsin001."). So native resume DOES work
  **when the transcript exists** — i.e. for a human-typed session. There is no psmux limitation
  on the *resume* step itself; the gap is purely whether the transcript got written.
- **Programmatically-driven sessions do NOT persist — this is the case that matters for mux.**
  Every probe where input was injected (`send-keys` burst, `send-keys` char-by-char, or the task
  passed as the launch `[prompt]` arg) wrote **only an `ai-title` stub** (~100 B), never
  `user`/`assistant` records — so `--resume` finds nothing. Controls ruled out, one at a time,
  all still failing: model (haiku *and* default), cwd (collision, dot-dir, clean), **attach**
  (detached *and* a real maximized WT client, 210×56), concurrency, teammate-mode (wrapper *and*
  raw `claude.exe` env-cleared), exit method, **flush-timing** (+60 s / +150 s while running),
  input cadence (burst vs char-by-char), launch shape (positional `[prompt]` vs send-keys), and
  — decisively — **an autonomous tool-using agent** (`--dangerously-skip-permissions`, task as
  positional arg): it created `proof.txt` (tools ran) yet the transcript stayed **116 B /
  ai-title only.** The *only* surviving correlate is **human keystrokes through an attached
  client vs server-side injection**; the latter (which is what any programmatic driver does)
  never persists. **Caveat on the explanation:**
  `send-keys` writes the same bytes to the pty as real typing, so claude cannot distinguish them
  at the input level — the mechanism is therefore unexplained, not "send-keys is special". What
  is solid is the *correlation* and the design consequence below; the root cause is unproven.
  - **Web research (cited):** the `ai-title`-only `.jsonl` is an independently **documented
    Claude Code regression** — GitHub **#60984** (Windows, plain interactive mode, 2.1.144/145;
    2.1.143 last good) — caused by a **flush/shutdown race**: the `ai-title` is written by an
    early async step and survives, while buffered `user`/`assistant` records are lost if the
    process is killed before flushing (SDK **#625**, **#21751**). Ink processes pty-injected
    keystrokes identically to typing (so send-keys is "not supposed to" be detectable). The
    transcript is officially append-only/incremental (https://code.claude.com/docs/en/claude-directory).
    NOTE: the flush-race / teardown-timing explanation was later **falsified** here — the probes
    failed even while running (+150 s), even as an autonomous tool-using agent, and even char-by-
    char. The robust empirical statement is the correlate (human-keystrokes-through-client vs
    server-side injection); the mechanism remains unexplained, but the design no longer depends on
    resolving it.
  - **False-positive warning:** content-searching the `.jsonl` for a codeword matches the
    `aiTitle` string (which echoes the prompt), NOT a persisted turn. Check file **size / record
    count** (a real transcript is KB+ with `user`/`assistant` records), not a substring hit. The
    earlier "it persisted" readings here were this exact false positive.
- **Headless `claude -p` also persists + resumes** (21 KB; `--resume` recalled the codeword),
  even while another claude session runs → **concurrency was never the blocker.**
- **Reference point:** a real-terminal interactive claude writes its transcript **incrementally
  while running** (verified on a live 2.1 MB / 997-record session with seconds-old `user`/
  `assistant` records) — that is why a human's reboot → `/resume` works. The psmux-pane programmatic
  case does NOT reach this state (init records never written), so this reference does not transfer
  to mux's use.
- **Design implication (Landed decision 7):** `mhgo mux resume` **cannot** use native
  `claude --resume` for the programmatically-driven panes mux runs. mux keeps its **own** durable
  per-pane journal (poll `capture-pane`, append to local-state, keyed by the mux-assigned
  `--session-id` stored from t0) and on resume rebuilds the layout, relaunches a real interactive
  claude per pane (full TUI, not `-p`), and **re-injects the journal as opening context.** Fidelity
  cost: rendered conversation text, not exact tool-call/internal state.

### Driving send-keys from git-bash mangles slash-args
`send-keys -l "/exit"` (or any leading-`/` arg: `/model`, `/resume`, absolute POSIX paths) run
**from the Bash tool (git-bash/MSYS)** is path-converted to e.g. `C:/Program Files/Git/exit` and
never reaches claude. Drive `send-keys` from **pwsh** (or Go `exec`, which has no MSYS layer).
mhgo is unaffected; the probe harness was.

### Hooks & event-driven monitoring (tested 2026-06-12)
psmux documents tmux-style hooks (`set-hook -g <event> "<cmd>"`, `show-hooks`, `run-shell -b`).
What actually works on this build:
- **`pane-died` fires reliably**, and runs a background command via `run-shell -b` (verified: a
  `pwsh -File logger.ps1` hook wrote to disk). **Requires `set-option -g remain-on-exit on`** —
  otherwise a pane whose process exits just vanishes (and if it's the last pane, the session and
  server exit) and the hook does not fire. **Fires with NO client attached** → usable by a
  daemonless mux.
- **Format variables do NOT expand in hook commands.** `#{pane_id}` / `#{hook_pane}` came through
  literally/empty (`info=paneid_#`). So a `pane-died` hook is a **bare trigger** — it cannot tell
  mux *which* pane died. The handler must then scan `list-panes -F "#{pane_id} #{pane_dead}"` to
  find the dead pane(s). (Format vars DO expand in normal `display-message`/`capture-pane -F`,
  just not inside hook-invoked `run-shell`.)
- **Silence/activity monitoring is NON-functional here.** `set-window-option` is an *unknown
  command*; `set-option -w monitor-silence|monitor-activity|monitor-bell <n>` is **silently
  accepted but never stored** (`show-window-options` keeps showing only `monitor-activity off`),
  and the `alert-silence` hook never fired across an 8 s idle. → **no built-in "agent went idle"
  signal.** mux must detect idle itself via the capture-pane poller (the `shortcuts` status-bar
  marker = idle, `interrupt` = busy).
- **Implication for mux:** hooks are a *minor convenience*, not a monitoring foundation. mux needs
  the `capture-pane` poller anyway (for the resume journal — decision 7 — and for idle detection,
  since `pipe-pane` and silence-hooks don't work). That poller already sees `pane_dead`, so it can
  detect death too; `pane-died` is at best a low-latency nudge to wake the poller / trigger a
  cleanup-or-relaunch. Don't build core behavior on hooks.

### respawn-pane & control-mode `-CC` (tested 2026-06-12)
- **`respawn-pane` reuses the SAME pane id and revives a dead pane in place** (verified: a dead
  `%3` went `dead=1 → dead=0`, id unchanged). This is exactly what daemon recovery wants — the
  layout/column stays intact, no re-render needed. In the test the respawn launched the **default
  shell** (not the custom command string I passed — the `[shell-command]` arg form needs care
  through the quoting layers), so the working pattern is: `respawn-pane` → then launch claude into
  the revived pane. `-k` kill-and-respawn on a live pane also works (id reused).
- **Control-mode `-CC` works on Windows psmux — including live `%output` push.** `psmux -CC attach`
  speaks the tmux control protocol: `%begin/%end` framing around command responses (`list-windows`
  → `0: pwsh* (1 panes) [100x30]`), commands accepted on stdin. Crucially, **`%output %<pane>
  <data>` notifications fire in real time** as a pane produces output (verified: a marker echoed in
  the pane appeared as `%output %1 …MARKER…` in the control stream). → **this is the working push
  channel that `pipe-pane` is not.** Cost: the `%output` payload is the **raw VT100 byte stream**
  (escape sequences, `\134` = `\`, `\015\012` = CRLF) — a consumer must ANSI-strip it, vs
  `capture-pane` which returns already-rendered text. **Design fit:** `capture-pane` (rendered) is
  simpler for the resume journal + idle detection; `-CC %output` (raw, real-time, no scroll-off) is
  the better basis for true streaming (e.g. the Slack relay) and for a single control client that
  watches all panes at once instead of N pollers.

---

## Open / TODO

- [x] Real interactive `claude` in a pane: renders, send-keys drives it, capture reads it. ✓
- [x] `claude --resume <id>`: **works for programmatically-driven panes** once the inherited
  `CLAUDE_CODE_CHILD_SESSION=1` (+ `CLAUDECODE`/`CLAUDE_CODE_*`) env is stripped before launch.
  Verified twice (14 KB transcript, recall after `kill-server`). The earlier "doesn't persist"
  results were all caused by that inherited env (probes ran inside a Claude Code session) — NOT by
  send-keys/visible-window/model. → native resume IS the mechanism (decision 7); env hygiene is the
  one requirement; mux's journal is optional.
- [x] Teammate-mode does NOT auto-spawn panes (in-process Agent) → mux owns panes.
- [x] Hooks: `pane-died` fires via `run-shell -b` (needs `remain-on-exit on`; no format-var
  expansion → bare trigger; fires detached). `monitor-silence`/`alert-silence` NON-functional
  (silently accepted, never fires). → hooks are a convenience nudge; poller is the foundation.
- [x] `respawn-pane` reuses the same pane id and revives a dead pane in place (layout stays
  intact); respawns the default shell → then launch claude into it. `-k` works on live panes.
- [x] control-mode `-CC` works on Windows incl. live `%output` push (raw VT100; needs ANSI
  strip) — the working push channel `pipe-pane` is not; good for streaming/Slack.

All brief questions are now answered; remaining items are implementation-time, not exploration.
