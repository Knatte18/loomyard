# Discussion: Build internal/mux — the window to the world (overlay + strands + render)

```yaml
task: 'Build internal/mux: the window to the world (overlay + strands + render)'
slug: internal-mux
status: discussing
parent: main
```

## Problem

`mux` is the one module that owns the live psmux session for a worktree and decides what
the operator sees. It is the **hard prerequisite for the whole orchestration spine**
(`proc → mux → shuttle → review → loom`): shuttle cannot run an interactive agent in a pane,
and loom cannot show a status line + a stack of agents, until mux exists. Today only a
proof-of-concept exists (`internal/muxpoccli`) that proved the risky parts — the tmux layout
checksum, bottom-dominant layout, env hygiene, and native `--resume` — but it bakes in
Claude-specific `review` semantics and is not the clean, domain-free module the spine needs.

**Why now:** the orchestration spine is blocked on it. Everything above mux in the stack
(`shuttle`, `review`, `loom`) depends on a clean `AddStrand/UpdateStrand/RemoveStrand`
contract and a pure render function. mux is built **fresh**, informed by what muxpoc proved;
muxpoc stays on disk as a reference but is unwired ("parked") from the CLI.

mux is **three things in one module**:
1. an **overlay** over psmux (every psmux command: pane create/kill, send-keys, capture,
   layout apply, env hygiene, native `--resume`, one named server per hub);
2. **strand bookkeeping** (every managed process = a *strand*, persisted to `.lyx/mux.json`);
3. a **render** sub-package — a pure function `layout = rules(strands)` over a closed,
   generic display vocabulary.

## Scope

**In:**
- New package pair `internal/muxengine` (domain kernel) + `internal/muxcli` (cobra CLI),
  following the CLI/Cobra Invariant.
- **Engine API** (in-process, the seam shuttle/loom will call): `AddStrand`, `UpdateStrand`,
  `RemoveStrand`, plus session boot/teardown, reconcile-on-startup, and resume.
- **Overlay**: a thin psmux subprocess wrapper (`run`/`output` + typed helpers), env hygiene
  on the server spawn, one named psmux server **per hub** (`lyx-<hub-basename>-<short-hash>`),
  one psmux session per worktree inside it.
- **Strand bookkeeping**: the strand record + persistence to `.lyx/mux.json` via
  `internal/state`, and reconcile against live `list-panes` on startup (pane-id is ephemeral,
  re-derived; claude session-id is the durable key).
- **Render sub-package** `internal/muxengine/render`: pure `rules(strands) → window_layout`
  string, handling anchors `top`, `below-parent`, `hidden` (bottom-active-dominant stack),
  with the tmux checksum. own-window deferred.
- **CLI verbs**: `up`, `add`, `remove`, `status`, `attach`, `resume`, `down`.
- **`internal/logger`**: new thin `log/slog` wrapper (`Debug`/`Info`/`Warn`), a
  `slog.LevelVar` set once by a persistent `-v`/`-vv` flag on the `cmd/lyx` root; injectable
  `os.Stderr` sink; default level `Warn`.
- **`internal/hubgeometry`**: add ownership of the ephemeral `.lyx` dir (a `Layout` accessor).
- **mux config**: register `mux` in `internal/configreg` with a `mux.yaml` template (tool
  paths, dimensions, active-pane share).
- **Park muxpoc**: unwire `internal/muxpoccli` from the CLI (keep the code as reference).
- Docs + invariants: `docs/modules/mux.md` reconciled to the as-built design (its stale
  decision-3), `docs/overview.md` module table updated, any new cross-cutting invariant
  recorded in `CONSTRAINTS.md`, sandbox suite scenario added.

**Out:**
- **Anything Claude/completion-specific** — the `--settings`, `Stop`/`SessionStart`/
  `PreToolUse` hook wiring, marker-grammar/idle detection, `last_assistant_message`
  interpretation, resume-command *construction*, and the `Agent`/`AskUserQuestion`
  guardrails all belong to **shuttle**. mux runs the opaque command strings it is handed and
  never reads them; its only liveness signal is the generic `pane-died`.
- **The mux daemon** (out-of-psmux crash detection, `capture-pane` journal + poller, Slack
  relay) — deferred.
- **Cross-worktree columns / `mplex`** and the `own-window` anchor — deferred (no consumer
  yet).
- **The "no transcript → fresh launch" resume fallback** — deferred (needs pane reads /
  belongs to shuttle/daemon).
- **`UpdateStrand` as a CLI verb** — it exists in the engine API (used in-process by shuttle
  for focus changes) but gets no `lyx mux` verb in v1.
- **Session-file portability / `lyx session push/pull`** — deferred milestone.
- **Removing muxpoc's code** — parked, not deleted.

## Decisions

### Domain-free strand contract (opaque command strings, no `type`)

- **Decision:** `AddStrand{ name, worktree, parent?, cmd, resumeCmd, sessionId?, display }`
  where `display{ anchor, height, focus, shrinkWhenWaitingOnChild }`. mux **stores all fields
  and reads none semantically** — `cmd` and `resumeCmd` are opaque launch/resume strings
  (built by the caller/shuttle), `sessionId` is opaque metadata (for status/reconcile), and
  there is **no domain `type` field ever**. `UpdateStrand{ id, display }` and
  `RemoveStrand{ id }` re-render.
- **Rationale:** a `type` field would force mux to import its consumers' vocabulary
  (circular). The CSS model: the caller says `anchor: top`, never "I am a status line". Keeps
  mux provider- and domain-invariant. Matches `mux.md`'s closed-vocabulary contract and the
  task body ("no domain type crosses the boundary").
- **Rejected:** mux generating the `--session-id` / building the claude command itself
  (`mux.md` decision-3 style) — that makes mux read/construct the Claude command, breaking
  dumb-carrier; shuttle owns launch+resume construction incl. the id. (`mux.md` decision-3 is
  the stale pre-shuttle-split text and will be reconciled in the doc.)

### Store both launch and resume commands; native resume; journal deferred

- **Decision:** each strand persists **both** the opaque launch cmd and the opaque resume cmd.
  `lyx mux resume` re-runs the stored resume cmd per strand opaquely (native
  `claude --resume`, which works given env hygiene). The `capture-pane` journal + poll-and-diff
  are **deferred to the future daemon**.
- **Rationale:** `mux-exploration.md`'s final determination (Landed decision 7) is that native
  `--resume` works for programmatically-driven panes once `CLAUDE_CODE_*` env is stripped;
  the journal is optional belt-and-suspenders. Storing both commands is cheap and future-proofs
  the fresh-launch fallback without building it now.
- **Rejected:** building the "no conversation found → fresh launch" fallback now (needs pane
  reads → breaks dumb-carrier; shuttle/daemon's job); building the capture-pane journal now
  (likely wasted work per the exploration's final decision). **Note:** `mux-exploration.md`
  contains a *stale contradictory bullet* ("Design implication: cannot use native resume, keep
  journal") — it predates the env-hygiene resolution; the authoritative reading is native
  resume + optional/deferred journal.

### One named psmux server per hub (orphan firewall) + per-worktree session

- **Decision:** boot one named server per hub as `lyx-<hub-basename>-<short-hash>`, with one
  psmux session per worktree inside it. `<hub-basename>` = `filepath.Base(Layout.Hub)`;
  `<short-hash>` = first 8 hex chars of `sha256(abs-hub-path)`. Server-name construction lives
  in `muxengine` (psmux domain), computed from `Layout.Hub` (obtained via hubgeometry).
- **Rationale:** the name is the **orphan firewall** — any psmux process outside this server
  is provably stray, so `status` can flag it. The hash makes the name unique per absolute hub
  path (two hubs sharing a basename must not collide) and socket-safe (raw paths contain
  `:`/`\`/spaces). Matches `mux.md` decision-4 and the future mplex/columns direction.
- **Rejected:** muxpoc's server-per-worktree (`muxpoc-<basename>`, no hash) — loses the
  hub-level orphan firewall and diverges from `mux.md`. No hub-path hash exists in the repo
  today, so mux implements it.

### Render is a pure function; anchor→layout policy is explicit and legible

- **Decision:** `internal/muxengine/render` is `rules(strands) → window_layout` string —
  deterministic, no I/O. It handles **three anchors in v1**: `top` (pinned status line),
  `below-parent` (the bottom-active-dominant agent stack — muxpoc-proven), and `hidden`
  (tracked, excluded from the layout string). The anchor vocabulary stays the closed
  four-member set (`top | below-parent | own-window | hidden`); `own-window` is deferred until
  review clusters exist.
- **The anchor→layout logic must be clear and easy to change/extend:** keep two distinct
  layers — (1) **layout policy** (which anchor lands where: an explicit per-anchor
  rule structure — a legible dispatch/table from anchor → placement rule, not implicit/buried
  logic), separated from (2) **layout mechanics** (the `window_layout` string builder + the
  tmux checksum). Adding or changing an anchor is then a localized, obvious edit: a new
  `rules()` case + its render test in the same commit. Do **not** add anchors speculatively —
  each anchor multiplies the layout/test cases; the vocabulary grows only when a real consumer
  needs a new spatial relation. Deferred candidates (until a consumer exists): `own-window`,
  and later `bottom` (absolute bottom-pin) and `column`/left-right (mplex).
- **Rationale:** purity makes render the clean golden-file test surface (no psmux/agents
  needed). The mechanics/policy split keeps the checksum math stable while the domain-facing
  policy stays small and total over a closed set — exactly where future change happens.
- **Rejected:** all four anchors incl. `own-window` (no consumer, adds window management now);
  just `below-parent` (too lean — loom needs the top status line and hidden strands).

### Re-render on structural events, debounced

- **Decision:** recompute the layout on `AddStrand`/`UpdateStrand`/`RemoveStrand` and on
  `pane-died`. Active-bottom-dominance is derivable from the parent tree; focus is caller-set —
  **no runtime idle signal is needed** for layout. Debounce a burst of mutations into one
  `ApplyLayout`.
- **Rationale:** completion/idle is shuttle's concern (via the file contract), not a mux
  re-render trigger; matches `mux.md`.
- **Rejected:** timed re-render / idle-driven re-render (couples mux to Claude semantics).

### Env hygiene lives in muxengine (not proc)

- **Decision:** an exported `muxengine.CleanClaudeEnv(environ) (clean, strippedKeys []string)`
  strips `CLAUDECODE` and `CLAUDE_CODE_*`. Applied **once** on the `new-session` server-spawn
  command (`cmd.Env = clean`); all later panes inherit the server's clean env. `internal/proc`
  is **not** touched. muxpoc's private copy is retired with muxpoc.
- **Rationale:** `proc` is a provider-agnostic OS primitive ("spawn any OS process, cross-OS")
  — hardcoding Claude env-var names in it leaks Claude knowledge into the base layer. mux is
  the documented chokepoint that spawns the psmux server, so the responsibility already lives
  there, and the exported helper is importable by shuttle later (can relocate to shuttle's
  Claude engine when it lands). Minimal diff (one new surface). This env stripping is the
  single verified cause of "transcript doesn't persist → resume finds nothing".
- **Rejected:** promoting into `internal/proc` (leaks Claude specifics into the OS primitive);
  keeping a private copy in muxengine only (fine, but exporting costs nothing and helps
  shuttle).

### logger: stderr sink, root flag, default Warn

- **Decision:** `internal/logger` wraps `log/slog`: `logger.Debug/Info/Warn` over a
  `slog.TextHandler` bound to a package `slog.LevelVar`. A **persistent `-v`/`-vv` flag on the
  `cmd/lyx` root** sets the level once at startup (`-v` = Info, `-vv`/`--verbose` = Debug),
  default **`Warn`**. The sink is an **injectable `io.Writer` field defaulting to the real
  `os.Stderr`** — deliberately **not** routed through `clihelp`'s stdout/stderr seam.
- **Rationale:** two hard constraints — (1) the sink must be separate from the command's JSON
  output writer so stdout (JSON envelope) and stderr (logs) stay on separate streams in
  production, and in tests logs go to real `os.Stderr` rather than the merged seam buffer, so
  the JSON buffer tests parse stays clean; (2) default `Warn` is non-negotiable belt-and-
  suspenders — a normal run emits zero log lines, so no diagnostic line can ever leak into a
  JSON consumer regardless of stream wiring. Injectable sink lets a test capture logs into its
  own buffer to assert on them. Root flag = every module inherits verbosity (future-shared
  logger).
- **Rejected:** file sink `.lyx/mux.log` (couples a general logger to the mux domain, hides
  output during live runs); flag on the `mux` command only (other modules can't adopt it
  without rewiring).

### mux.json path via hubgeometry `.lyx` ownership

- **Decision:** add ownership of the **ephemeral `.lyx`** dir to `internal/hubgeometry` (a
  `Layout` accessor, e.g. `EphemeralDir()`/`DotLyxDir()` → `<Cwd>/.lyx`). mux resolves
  `.lyx/mux.json` through it. Note `.lyx` (dot, ephemeral, machine-bound, in
  `.git/info/exclude`) is **distinct** from hubgeometry's existing `_lyx` (underscore,
  durable/weft-synced).
- **Rationale:** the Hub Geometry Invariant makes hubgeometry the sole owner of cwd/geometry
  paths; adding `.lyx` there is the principled fix and avoids a second hardcoded `.lyx` literal
  now that muxpoc's is being parked. overview.md is explicit that mux.json is ephemeral and
  belongs in `.lyx/`.
- **Rejected:** hardcoding `.lyx/mux.json` in muxengine (muxpoc style) — scatters ephemeral-
  path knowledge, cuts against the invariant.

### mux config via configreg

- **Decision:** register `mux` in `internal/configreg` with a `mux.yaml` template holding
  machine-specific tool paths (`psmux`, `pwsh`, `claude`), dimensions (width/height), and the
  active-pane share. Loaded via `configengine.Load(baseDir, "mux", []byte(ConfigTemplate()))`.
- **Rationale:** tool paths are machine-specific and belong in config, not code defaults;
  matches the repo convention and makes sandbox/other-machine use clean. shuttle will likely
  reuse tool-path config.
- **Rejected:** cobra flags with hardcoded defaults (muxpoc style — bakes machine paths into
  code); flags-now-config-later (risks a churny migration).

### CLI verb set (minimal-but-functional)

- **Decision:** `up` (idempotent boot; cold-recovers a dead session's strands), `add`/`remove`
  (thin strand CRUD = the seam that exercises `AddStrand`/`RemoveStrand` + re-render without
  shuttle), `status` (reconcile vs live `list-panes` + orphans), `attach` (pop one maximized
  terminal), `resume` (replay stored resume cmds), `down` (teardown + delete state).
- **Rationale:** smallest set that is genuinely functional and independently sandbox-testable
  before shuttle exists; `add`/`remove` make the engine drivable and cover the load-bearing
  re-render behaviors (parent shrinks on add, grows on remove). Keeps `resume` explicit per
  `mux.md`.
- **Rejected:** folding `resume` into `up` (diverges from `mux.md`); even-leaner
  `up/add/status/attach/down` (can't exercise RemoveStrand re-render or crash recovery via
  CLI).

### Park muxpoc (keep as reference, unwire from CLI)

- **Decision:** keep `internal/muxpoccli` on disk as a reference, but **unregister it from the
  `lyx` CLI**: remove from `cmd/lyx/main.go` `newRoot()` `AddCommand` + import + `root.Long`
  module list; add `muxpoc` to `registration_test.go`'s allowlist with a reason (package still
  has `Command()` but is intentionally not wired); remove `muxpoc` from the pinned lists in
  `helptree_test.go`, `jsonhelp_test.go`, `unknown_subcommand_test.go`; remove the `muxpoc`
  entry from `excludedModules` in `sandbox_coverage_test.go` (the test rejects stale
  exclude entries for non-registered modules).
- **Rationale:** user directive — keep the proven reference while mux matures, but stop
  exposing a second mux-ish command. Smaller/safer than deleting.
- **Rejected:** leaving muxpoc registered (two mux-ish commands, confusing); deleting muxpoc
  now (loses the live reference before mux is proven).

## Technical context

**Layering (execution stack).** `proc` (OS spawn primitive) → `mux` (this task) → `shuttle`
(one LLM agent, next) → `review` → `loom`. Each layer knows only the one below. mux exists
because agents must run as **interactive** psmux sessions, not headless `claude -p` (economic
constraint). See `docs/overview.md` and `docs/modules/{mux,shuttle}.md`.

**Dependencies and their exact APIs (verified during exploration):**

- **`internal/proc`** — only `HideWindow(cmd)` and `Detach(cmd)` (SysProcAttr helpers). The
  background-spawn pattern is: build `*exec.Cmd`, set `cmd.Env`, `proc.Detach(cmd)`,
  `cmd.Start()` (never `Wait()`). The psmux server must be spawned this way so it survives CLI
  exit. proc has **no** env handling (that's why env hygiene lives in muxengine).
- **`internal/state`** — generic locked/atomic JSON: `WriteJSON[T](path, lockPath, v) error`
  and `ReadJSON[T](path, lockPath) (T, bool, error)` (returns `found=false` for absent file;
  surfaces corruption). Convention: `lockPath = dataPath + ".lock"`; atomic temp-file+rename;
  advisory read/write locks via `internal/lock`. Model the persisted `MuxState` struct and
  wrap these (see muxpoc's `state.go` as the closest template).
- **`internal/hubgeometry`** — `Getwd()` (only sanctioned `os.Getwd` outside main) and
  `Resolve(cwd) (*Layout, error)` (runs `git rev-parse --show-toplevel`; `ErrNotAGitRepo`).
  `Layout` fields: `Cwd`, `WorktreeRoot`, `Hub` (= `filepath.Dir(WorktreeRoot)`), `RelPath`,
  `Prime`. `LyxDir()` → `<Cwd>/_lyx`. Worktree slug = `filepath.Base(WorktreeRoot)`. **No
  hashing exists anywhere in the repo** — mux implements the hub-path hash. **This task adds a
  `.lyx` accessor here** (see decision).

**Proven muxpoc techniques to reuse (all in `internal/muxpoccli`):**

- **tmux layout checksum** (`cmd.go` `layoutChecksum`): 16-bit rotate-right-1 accumulate over
  the body bytes, 4 lowercase hex digits. **Reuse verbatim.** Pinned fixture: body
  `220x50,0,0[220x15,0,0,1,220x15,0,16,4,220x18,0,32,3]` → `acd7`.
- **layout string** format `csum,WxH,0,0[paneWxpaneH,x,y,paneNum,...]` where paneNum = pane id
  with leading `%` stripped; panes ordered top→bottom. **bottom-active-dominant**: bottom pane
  gets `activePaneShare=55`%, ancestors split the remainder equally (reuse the 55 constant).
  Applied atomically via `select-layout "<csum>,<body>"`, then `select-pane` the bottom pane.
- **psmux subprocess wrapper** (`PsmuxCmd`): `run(args...)` (discard I/O) and `output(args...)`
  (capture stdout) **always prepend `-L <socketName>`**. The **server-spawning `new-session`
  is NOT routed through it** — it's raw `exec.Command` so `cmd.Env = CleanClaudeEnv(...)` +
  `proc.Detach` + `cmd.Start()` can be attached.
- **Two distinct pane-id capture strategies** (both required): `split-window -P -F "#{pane_id}"`
  for a **new** pane; `display-message -p "#{pane_id}"` for the `new-session` pane
  (`display-message` is unreliable for freshly-split panes on a detached session).
- **Pane-id is ephemeral, claude session-id is durable.** psmux reassigns pane ids across a
  server restart; on reconcile/recover, re-derive pane ids and re-persist; the stored
  session-id is the stable key.
- **Launch/resume via `send-keys ... "Enter"`** into the pane shell (proven). The `[prompt]`
  positional/argv content, if any, is inside the opaque `cmd` string shuttle builds — mux just
  send-keys the whole string.
- **`has-session` semantics** (`hasSession`): exit 1 → absent (`false, nil`); other errors
  surface. After `new-session`, poll `has-session` a few times before proceeding.
- **All parsing is pure functions** (`parsePaneList`, `parseWindowSize`, `parsePaneOrder`,
  `buildColumnLayout`, `layoutChecksum`) taking strings → values, so layout/checksum/parse
  logic is unit-tested without a live psmux; only the thin I/O shells + `new-session` need one
  (guard live tests behind a build tag).

**Empirical psmux guardrails (from `docs/research/mux-exploration.md`):**
- `pane_current_command` is always `shell` on Windows → use `capture-pane`/`pane_pid`, never it.
- Launch with **explicit binary paths**, never PATH aliases (`pwsh` resolved to a 0-byte
  WindowsApps stub under ConPTY).
- `select-layout even-horizontal` **flattens** vertical sub-stacks → mux must emit the layout
  string directly (this is why render exists).
- `pane-died` fires via `run-shell -b` (needs `set-option -g remain-on-exit on`; fires
  detached) but **format vars don't expand in hook commands** → it's a bare trigger; the
  handler must scan `list-panes -F "#{pane_id} #{pane_dead}"`. `monitor-silence`/`alert-silence`
  are silently accepted but non-functional. `set-window-option` doesn't exist (use
  `set-option -w`).
- Env hygiene: strip `CLAUDE_CODE_CHILD_SESSION` (prime culprit), `CLAUDECODE`,
  `CLAUDE_CODE_SESSION_ID`, `CLAUDE_CODE_ENTRYPOINT`, `CLAUDE_CODE_SSE_PORT`.

**CLI/engine wiring checklist (from the convention study):**
- `internal/muxengine` — pure domain, funcs return `(T, error)`, no cobra/`io.Writer`/exit
  codes; package doc comment; `ConfigTemplate()` + config load via `configengine.Load`.
- `internal/muxcli` — package `muxcli` (no import alias — the registration AST guard matches
  `<pkgname>.Command()`); `func Command() *cobra.Command` (parent `Use:"mux"`, non-empty
  `Short`, `RunE: clihelp.GroupRunE`, `PersistentPreRunE` returning `nil` early when
  `cmd.Name()=="mux"`); `func RunCLI(out io.Writer, args []string) int { return
  clihelp.Execute(Command(), out, args) }`. Every subcommand: non-empty `Short` (+ `Long` with
  examples if user-facing); `RunE` begins with `if clihelp.ShouldAbort(ctx) { return nil }`,
  calls the engine, threads exit via `clihelp.SetExit(ctx, output.Ok/Err(out, ...))`, returns
  `nil` (never return the error to cobra).
- **Wiring in `cmd/lyx/main.go`**: add `internal/muxcli` import, `muxcli.Command()` to
  `root.AddCommand(...)`, append `mux` to `root.Long` "Available modules:" line.
- **Pinned test edits (same commit)**: `helptree_test.go` `requiredModules` + a
  `TestHelpTree_VerbModuleSubcommands` case listing mux's subcommands; `jsonhelp_test.go`
  `requiredModules`; `unknown_subcommand_test.go` group list (+ the bare-group-listing case if
  a `PersistentPreRunE` guard is used); `configreg_test.go` `want` list (add `mux`);
  `sandbox_coverage_test.go` `excludedModules` (remove `muxpoc`); the muxpoc-parking edits
  above. Auto-derived guards (no list edit, code must pass): `drift_test.go` (Short
  everywhere), `registration_test.go` (registered/unaliased — add muxpoc to its allowlist),
  `longlist_test.go`.

## Constraints

From `CONSTRAINTS.md` (authoritative) and this discussion:

- **Hub Geometry Invariant.** All cwd/worktree-root/geometry resolution goes through
  `internal/hubgeometry`. Raw `os.Getwd`/`git rev-parse --show-toplevel` banned outside
  hubgeometry + `cmd/lyx/main.go`. Geometry tokens (`_lyx`, `_board`, `-weft`, `-HUB`,
  `_portals`, `_launchers`, `_codeguide`) owned solely by hubgeometry — no other package may
  use them in path construction (production files; enforced by
  `hubgeometry/enforcement_test.go`). **Consequence:** the new `.lyx` accessor is added *in*
  hubgeometry; the mux.json path resolves through it, not a hardcoded literal. (If `.lyx`
  becomes an enforced geometry token, register it in the enforcement set in the same commit.)
- **CLI / Cobra Invariant.** `Command()`/`RunCLI` seam; `Short` on every command; JSON via
  `internal/output` envelope (`output.Ok`/`output.Err`), one JSON object per line;
  `RunE = clihelp.GroupRunE` on the parent; `<module>cli`/`<module>engine` split (cli imports
  engine; engine never imports cobra/cli/`io.Writer`); registration + help-tree + longlist +
  drift tests updated in the same commit. Help prose accuracy is a review obligation.
- **Sandbox Suite Coverage.** Every registered module is exercised by a
  `tools/sandbox/SANDBOX-SUITE.md` scenario tagged `**Covers:** mux`, or excluded with a
  reason. Add a mux scenario (parking muxpoc removes its exclude entry).
- **lyxtest Leaf Invariant.** `internal/lyxtest` imports only stdlib + hubgeometry; tests
  needing real config call `lyxtest.SeedConfig(tb, dir, map[string]string{...})` with the
  `configreg`→map conversion at the test site.
- **Documentation Lifecycle.** This task lands a module → update `docs/modules/mux.md`
  (reconcile the stale decision-3 to the dumb-carrier design; the durable design also folds
  into the package header), `docs/overview.md` (module table: mux 🚧→✅, muxpoc parked), and
  record any new cross-cutting invariant in `CONSTRAINTS.md` — all in the same commit(s).
  `docs/roadmap.md`: mark the mux milestone ✅ Done (it is a planned milestone).
- **Design constraint (this discussion):** the render anchor→layout **policy** is explicit and
  legible, separated from the layout **mechanics** (checksum/string builder), so extending the
  anchor vocabulary is a localized change (`rules()` case + render test in the same commit).
- **fslink / geometry:** not directly touched, but any cross-OS links use `internal/fslink`
  (directory junctions on Windows) — not expected here.
- **Windows-first reality:** psmux 3.3.4 at `C:\Code\tools\bin\psmux.exe`, pwsh 7 at
  `C:\Code\tools\powershell7\pwsh.exe`; launch with explicit paths; drive `send-keys` from Go
  `exec` (no MSYS slash-arg mangling).

## Testing

Follow `mill:testing` + `golang:golang-testing`. Per-file unit tests next to source;
`//go:build integration` for tests needing real fixtures; live-psmux tests behind a build tag
(e.g. `smoke`). Drive the CLI through the `RunCLI(&out, args)` seam and assert on the parsed
JSON envelope (`ok` true/false).

- **render sub-package (primary TDD candidate).** Pure `rules(strands) → layout-string`.
  Golden-file / table tests over strand sets — no psmux, no agents:
  - checksum matches the pinned `acd7` fixture; checksum prefix always equals
    `layoutChecksum(body)`.
  - bottom-active-dominant invariants (reuse muxpoc's): heights + 1-row dividers exactly fill
    window height; bottom pane strictly tallest and ≥50%; ancestors equal; cumulative y-offsets.
  - **anchor policy** cases: `top` pinned as a fixed-height pane above the stack;
    `below-parent` forms the bottom-dominant stack ordered by parent chain; `hidden` strands
    are **excluded** from the layout string entirely; mixed sets (top + stack + hidden);
    empty/single-strand edge cases. Each anchor's rule is independently asserted so adding an
    anchor adds an isolated test.
- **muxengine strand bookkeeping (TDD candidate).** `AddStrand`/`UpdateStrand`/`RemoveStrand`
  mutate the table and persist; round-trip through `state.ReadJSON/WriteJSON` (absent file →
  empty table; corruption surfaced). Reconcile: given a saved table + a fake `list-panes`
  result, drop dead strands, keep live, re-derive pane ids. Debounce: a burst of mutations →
  one `ApplyLayout`. `CleanClaudeEnv`: strips exactly `CLAUDECODE` + `CLAUDE_CODE_*`, returns
  the stripped keys, leaves the rest untouched.
- **server naming.** `lyx-<hub-basename>-<short-hash>` is deterministic, socket-safe (no
  `:`/`\`/space), and distinct for two hubs sharing a basename on different absolute paths.
- **hub-path hash.** `sha256(abs-hub-path)`-first-8-hex is stable and case/path-normalized as
  intended.
- **hubgeometry `.lyx` accessor.** Returns `<Cwd>/.lyx`; distinct from `_lyx`; add to
  hubgeometry's own tests (config-layout tests use hubgeometry helpers even in test code).
- **logger.** Default `Warn` emits **zero** lines for `Info`/`Debug` calls; `-v`→Info,
  `-vv`→Debug thresholds; sink is injectable and captured into a test buffer to assert; the
  JSON-output buffer stays clean (no log leakage) under the `RunCLI` seam.
- **muxcli (integration).** No-arg `lyx mux` lists subcommands, exit 0; unknown subcommand exit
  1 with `ok=false`; a real round-trip (e.g. `up` then `status` then `down`) on a fixture hub
  using `lyxtest.CopyPaired`/`SeedConfig`. Overlay I/O and `new-session` behind the live-psmux
  build tag (smoke).
- **cmd/lyx guard tests.** Updated pinned lists pass; drift/registration/longlist/help-tree/
  sandbox-coverage all green with mux registered and muxpoc parked.
- **sandbox scenario.** A `**Covers:** mux` scenario exercising the real deployed binary
  (`up`/`add`/`status`/`attach`/`resume`/`down` lifecycle) — realistically behind the same
  live-psmux caveat; ensure the coverage guard is satisfied by the tag regardless.

## Q&A log

- **Q:** v1 CLI surface? **A:** Minimal-but-functional → `up, add, remove, status, attach, resume, down` (UpdateStrand engine-API-only).
- **Q:** psmux server topology? **A:** One server per hub, `lyx-<hub-basename>-<short-hash>` (session per worktree inside); mux implements the hub-path hash.
- **Q:** What to do with muxpoc? **A:** Keep the code as reference but **park it** — unwire from the `lyx` CLI (unregister + registration-test allowlist + drop from pinned help/sandbox lists).
- **Q:** Where does env hygiene live? **A:** In `muxengine` (exported `CleanClaudeEnv`), not `proc` — `proc` stays a provider-agnostic OS primitive; muxengine is the server-spawn chokepoint; relocatable to shuttle later. Must leave muxpoc.
- **Q:** `.lyx/mux.json` path resolution? **A:** Add `.lyx` ownership to `hubgeometry` (a `Layout` accessor); do not hardcode. `.lyx` (ephemeral) ≠ `_lyx` (durable).
- **Q:** logger design? **A:** `os.Stderr` sink (injectable `io.Writer`, deliberately not through clihelp's seam), persistent `-v`/`-vv` flag on the `cmd/lyx` root, default `Warn` (non-negotiable — zero lines on a normal run), `slog.LevelVar` + `slog.TextHandler`, `Debug/Info/Warn`. No file sink.
- **Q:** Resume model / how much now? **A:** Native `claude --resume` via the stored opaque resume cmd; store **both** launch + resume cmds; capture-pane journal **deferred** to the daemon; "no-transcript → fresh launch" fallback deferred (shuttle/daemon). Note the stale contradictory bullet in `mux-exploration.md` — native-resume is authoritative.
- **Q:** Strand contract? **A:** `AddStrand{ name, worktree, parent?, cmd, resumeCmd, sessionId?, display{anchor,height,focus,shrinkWhenWaitingOnChild} }`; mux stores all, reads none semantically; **no domain `type`**. mux does NOT assign the session-id (shuttle owns launch+resume construction).
- **Q:** mux config? **A:** Config file via `configreg` (`mux.yaml`: tool paths, dims, active-pane share), not flags-with-defaults.
- **Q:** Render anchor scope? **A:** `top` + `below-parent` + `hidden` in v1; `own-window` deferred (no consumer). Keep the closed 4-member vocabulary; grow only when a real consumer needs it (new `rules()` case + test same commit).
- **Q:** Render code structure? **A:** Anchor→layout **policy** must be explicit/legible and **separated** from layout **mechanics** (checksum/string builder), so changing/adding an anchor is a localized, obvious edit.
