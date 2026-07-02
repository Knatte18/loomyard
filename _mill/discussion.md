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
  re-derived; the mux-generated **`guid` is the durable key**, `sessionId` is opaque metadata).
  **Reconcile clears the pane binding of dead strands but keeps their record** (so `resume` can
  rebuild them); only `remove` deletes a record.
- **Render sub-package** `internal/muxengine/render`: pure `rules(strands) → window_layout`
  string, handling anchors `top`, `below-parent`, `hidden` (bottom-active-dominant stack),
  with the tmux checksum. own-window deferred.
- **CLI verbs**: `up`, `add`, `remove`, `status`, `attach`, `resume`, `down`.
- **`internal/logger`**: new thin `log/slog` wrapper (`Debug`/`Info`/`Warn`), a
  `slog.LevelVar` set once by a persistent `-v`/`-vv` flag on the `cmd/lyx` root; injectable
  `os.Stderr` sink; default level `Warn`.
- **`internal/hubgeometry`**: add ownership of the ephemeral `.lyx` dir (a `Layout` accessor).
- **mux config**: register `mux` in `internal/configreg` with a `mux.yaml` template (tool
  paths, dimensions, layout knobs `collapsedStripRows`/`topBandRows`/`minFullRows`, and the
  `strand-name` template).
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
- **Automatic `pane-died`-driven re-render** (the psmux hook + a hidden `on-pane-died` handler
  verb + low-latency nudge) — deferred with the daemon. v1 re-renders **on-demand** (in-process
  per mutation + reconcile on each CLI verb); dead panes are detectable via `remain-on-exit on`
  but noticed only when a verb next runs.
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

- **Decision — the stored strand record:** `{ guid, name, worktree, parent?, cmd, resumeCmd?,
  sessionId?, paneId, display{ anchor, focus, shrinkWhenWaitingOnChild } }`. mux **stores all
  fields and reads none semantically** — `cmd`/`resumeCmd` are opaque launch/resume strings
  (built by the caller/shuttle), `sessionId` is opaque metadata, `paneId` is the ephemeral
  psmux id (re-derived on reconcile), and there is **no domain `type` field ever**.
  `UpdateStrand{ guid, display }` and `RemoveStrand{ guid }` re-render. `UpdateStrand` may flip
  `anchor: hidden → visible` (surface a pending strand: create pane + run `cmd`) but **rejects
  `visible → hidden`** in v1 (see the render decision's hidden-strand rule). (No `display.height`
  — height is fully derived; see the render decision.)
- **Strand identity — GUID is the canonical key, `name` is the descriptive label (orch #1, feedback_02):**
  - **`guid`** — mux-generated at `AddStrand` (128-bit via `crypto/rand`, hex), **100% unique**,
    the **durable key**: parent links are stored as the parent's `guid`, reconcile keys on it,
    and it is the identity `UpdateStrand`/`RemoveStrand` mutate. This **replaces `sessionId` as
    the durable key** — `sessionId` (claude's, optional/absent for a shell or status strand)
    stays pure opaque metadata, never an identity. **In v1 mux neither writes nor reads it**: the
    CLI `add` exposes no flag for it, and mux never consumes it (its only reader — a join against
    claude's agent list in `status` — is Claude-specific and deferred to shuttle/daemon). It
    exists in the record only so shuttle can set it later; **do not derive it from the `cmd`
    string** (that would break dumb-carrier). (internalmux #D)
  - **`name`** — caller-supplied descriptive label, stored **verbatim** (mux never parses it —
    dumb-carrier). Used **only for display**: the pane title and `status` output. It is **not** a
    selector and carries no uniqueness requirement.
  - **Referential integrity — `guid`-only selectors (internalmux #C2):** `--parent <guid>` and
    `remove <guid>` take the **`guid`** (always unique, printed in the `add` JSON so a later
    reference is trivial). Keeping selectors guid-only removes an ambiguity-resolution path + its
    failure mode for marginal hand-CLI convenience; `name` stays purely cosmetic.
  - **How `name` is set — a domain-free config template (feedback_02):** the name is composed
    from a `mux.yaml` `strand-name` template (see the config decision), default
    `<ROLE>:<ROUND>:<SHORT_GUID>`. `--role`/`--round` are **formatting-only inputs consumed at
    add-time** to fill the template — they are **never persisted as strand fields and mux never
    branches on them** (the sharp difference from a forbidden `type` field: they feed one string
    substitution, then are discarded). Substitution is a pure `FormatStrandName(template, parts)`
    helper in the cli/caller layer (reusable by shuttle), not engine semantics. `--name`
    overrides the template verbatim; if neither `--name` nor `--role` is given, the name falls
    back to `<SHORT_GUID>` alone (always present, never empty).
- **`resumeCmd` is optional (nullable).** A stateless strand (e.g. a `lyx loom status --watch`
  status line, or a plain shell pane) has no meaningful `--resume`. On `resume`, a strand
  **with** a `resumeCmd` runs it; a strand **without** one re-runs its `cmd` (a fresh launch —
  appropriate for stateless/idempotent strands). (orch minor)
- **`display.focus` and `display.shrinkWhenWaitingOnChild` have precise v1 render semantics**
  (they are NOT undefined-but-present fields — see the render decision below for the rules and
  tests). `shrinkWhenWaitingOnChild` defaults **true**; `focus` defaults to *unset* (render
  then focuses the bottom-most/active strand). (orch #1)
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
- **Rationale:** the name is the **orphan firewall** — any psmux process outside this server is
  provably stray. The named server *enables* orphan detection, but **v1 `status` does not actively
  enumerate stray servers** (internalmux NOTE3): a reliable psmux-server listing on Windows is
  unverified, so active listing is deferred; v1 `status` reports only **this** session (tracked
  strands + live/dead reconcile). The hash makes the name unique per absolute hub path (two hubs
  sharing a basename must not collide) and socket-safe (raw paths contain `:`/`\`/spaces). Matches
  `mux.md` decision-4 and the future mplex/columns direction.
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
- **Hidden strand realization — no pane exists until surfaced (internalmux GAP1 A + regel 1/3):**
  a `hidden` strand is a **record with no psmux pane**, and its `cmd` is **not run at `add` time**.
  The pane is created and `cmd` launched only when the strand is **surfaced** — `UpdateStrand`
  flips its `anchor` off `hidden` (to `top`/`below-parent`). So render's exclusion of `hidden` is
  **by construction** (there is no live pane to enumerate), never a live-but-filtered pane, which
  keeps the layout string consistent with the window's live pane set. **In v1 `anchor: hidden` is
  valid only at `add` time** (pre-registration / pending surface): `UpdateStrand` may flip
  `hidden → visible` but **rejects `visible → hidden`** (`cannot hide a live strand in v1`) —
  hiding a *running* strand is the deferred "observable background work". This holds the invariant
  *a hidden strand never has a live pane* across all three paths (add / update / resume).
- **The anchor→layout logic must be clear and easy to change/extend:** keep two distinct
  layers — (1) **layout policy** (which strand lands where: an explicit per-anchor
  rule structure — a legible dispatch/table from anchor → placement rule, not implicit/buried
  logic), separated from (2) **layout mechanics** (the `window_layout` string builder + the
  tmux checksum). Adding or changing an anchor is then a localized, obvious edit: a new
  `rules()` case + its render test in the same commit. Do **not** add anchors speculatively —
  each anchor multiplies the layout/test cases; the vocabulary grows only when a real consumer
  needs a new spatial relation. Deferred candidates (until a consumer exists): `own-window`,
  and later `bottom` (absolute bottom-pin) and `column`/left-right (mplex).
- **`focus` and `shrinkWhenWaitingOnChild` are part of the layout policy, with defined v1
  rules (orch #1):**
  - `shrinkWhenWaitingOnChild` (bool, default **true**): a `below-parent` strand that is an
    **ancestor** (has a visible child below it in the parent chain) collapses to a **single
    compact strip** of `collapsedStripRows` rows (from `mux.yaml` — see the config decision).
    A strand with it **false** opts out of collapsing and is treated as a **co-equal full pane**
    (keeps a full share alongside the active strand — for "keep watching this parent while the
    child runs"). Default true reproduces muxpoc's bottom-dominant behavior.
  - `focus` (bool, default unset): the render's `select-pane` target. **Exactly one** pane is
    focused per session; if no strand sets `focus:true`, render defaults to the bottom-most
    (active) strand (muxpoc's "always select the bottom pane"). If a caller sets `focus:true`
    on a specific strand (e.g. loom parking focus on a parent at an input gate), render
    focuses that strand instead. Ties (multiple focus:true) resolve to the bottom-most such
    strand.
  - Both are asserted by isolated render tests (see Testing), so they are defined behavior,
    not latent rot.
- **Derived height policy — no per-strand `height` field (orch #1b, Q3):** heights are fully
  derived, so `display` carries no `height`. Given usable height `H_u` = window height − top
  band(s) − 1-row dividers: each `top` strand is a fixed band of `topBandRows` rows (config,
  default 1 — a status line); in the `below-parent` stack, each **shrink:true ancestor** is a
  strip of `collapsedStripRows`, and the **active/focused strand plus every shrink:false strand**
  are "full" panes that split the remaining height **equally**. This replaces muxpoc's fixed
  `activePaneShare=55%` — strip thickness and band height are operator-tunable config, dominance
  is *derived* as the remainder. **Integer-division remainder is deterministic (internalmux
  review r2, NOTE1):** when the remaining height does not divide evenly among ≥2 co-equal full
  panes, the **leftover rows are assigned to the active/bottom pane** (the others get the floor).
  This mirrors muxpoc's single-bottom-pane absorbing the leftover (`bottomH = usable −
  ancestorH*(n−1)`), so "heights + dividers exactly fill the window height" stays a **total,
  deterministic** golden invariant even with multiple full panes. The layout **mechanics**
  (checksum + `window_layout` string) are reused from muxpoc **verbatim**; only the height
  *policy* changes.
- **Clamp rule — render is total, never emits a non-positive pane (orch #B):** when fixed demand
  (top bands + all strips + dividers + a `minFullRows` floor, default 3, per full pane) exceeds
  the window, reduce in strict priority: (1) shrink the strips below nominal down to 1 row, then
  (2) reduce full panes equally down to `minFullRows`, then (3) as a last resort keep the active
  pane at whatever remains and clamp earlier panes to 1 row. Render **always** yields a valid,
  non-negative layout string — a torn/negative pane height would make psmux reject the layout.
- **Sibling ordering is deterministic (orch #E):** two strands sharing a parent (same depth in
  the chain) are ordered by **insertion order** (their position in the persisted strand table,
  which is GUID-stable), so the layout string is deterministic and golden-file tests are stable.
- **Rationale:** purity makes render the clean golden-file test surface (no psmux/agents
  needed). The mechanics/policy split keeps the checksum math stable while the domain-facing
  policy stays small and total over a closed set — exactly where future change happens.
- **Rejected:** all four anchors incl. `own-window` (no consumer, adds window management now);
  just `below-parent` (too lean — loom needs the top status line and hidden strands).

### Re-render is on-demand in v1 (daemonless); pane-died auto-trigger deferred (orch #2)

- **Decision:** the layout is recomputed **(a) in-process on each mutation** —
  `AddStrand`/`UpdateStrand`/`RemoveStrand` recompute + apply within the same invocation (under
  the mux operation lock; a burst debounced into one `ApplyLayout`) — **and (b) on-demand on
  every CLI verb**: `status`, `resume`, and the next `add`/`remove` read live `list-panes`,
  reconcile (clear the pane binding of dead strands — **keep the record** — and re-derive pane
  ids), and re-apply the layout. There is **no
  live `pane-died` listener in v1.** So dead panes are noticed the next time a verb runs, not
  instantly.
- **Dead-pane detectability:** v1 sets `set-option -g remain-on-exit on` so a pane whose
  process exits **persists as `pane_dead=1`** (rather than vanishing — which would also kill
  the session if it were the last pane), letting the on-demand reconcile detect it via
  `list-panes -F "#{pane_id} #{pane_dead}"`. Because `select-layout` must enumerate exactly the
  window's live panes, reconcile then **kills the dead pane before re-applying the layout**
  — *except* a **sole-remaining** dead pane (killing it would end the session), which is kept and
  rendered until `resume`/`remove`. `remain-on-exit on` thus gives reconcile the chance to *see*
  the death deliberately, then reap it (internalmux GAP2). Order inside the mux lock: **kill dead
  → re-enumerate live → compute layout → apply** (the kill mutates the pane set the next
  `select-layout` enumerates, so enumeration must follow the kill).
- **The psmux `pane-died` hook is deferred with the daemon.** An automatic, low-latency
  re-render on pane death would require the psmux hook (`run-shell -b`, needs `remain-on-exit`,
  fires detached) calling back into a **hidden lyx handler verb** — but the hook can't expand
  format vars (bare trigger), and a daemonless one-shot has nothing listening. That whole path
  (hook + handler verb + poller) belongs to the deferred **daemon**; v1 deliberately does not
  add a hidden `on-pane-died` verb. This resolves the earlier contradiction (a "recompute on
  pane-died" trigger with no daemon to run it).
- **Rationale:** completion/idle is shuttle's concern (file contract), not a mux re-render
  trigger; and a daemonless module can only act when invoked. On-demand reconcile + in-process
  mutation re-render covers every v1 consumer path without a background process.
- **Rejected:** timed/idle-driven re-render (couples mux to Claude semantics); a hidden
  `on-pane-died` handler verb in v1 (daemon-era; no listener without the daemon).

### Cross-process concurrency: a mux operation lock around the whole mutate+apply cycle (orch #6)

- **Decision:** guard the **entire** `read → mutate → persist → render → apply(select-layout)`
  cycle with a single **mux operation lock** at `.lyx/mux.lock` (via `internal/lock`). This is
  distinct from, and coarser than, the `internal/state` per-write lock on `mux.json`.
- **Acquired at exactly ONE layer — the engine operation boundary (internalmux review r2,
  NOTE2):** each public engine op (`AddStrand`/`UpdateStrand`/`RemoveStrand` + the `up`/`resume`/
  `status`-reconcile-apply ops) acquires `mux.lock` **once at entry** and holds it for its whole
  cycle, composing from **unexported, unlocked** helpers. **CLI verbs never take the lock
  themselves** — they just call the engine op, which locks. This is mandatory because
  `internal/lock` (gofrs/flock) is **non-reentrant across separate handles even in-process** on
  Windows: if a CLI verb locked and then called an engine op that also locked, it would
  **self-deadlock**. One acquisition point (engine entry) removes that risk while still
  serializing the full cycle across both separate CLI processes and a long-lived in-process
  driver (shuttle/loom). Engine ops must not call each other while holding the lock (use the
  unlocked helpers), for the same reentrancy reason.
- **Why:** each CLI verb is its own process doing read→render→`select-layout`, and shuttle
  drives `AddStrand` in-process concurrently. Locking only the JSON write (as `state` does)
  still lets two mutations both read, both render, and **clobber each other's layout**. The
  concurrent scenario is real (the loom driver adding a strand while an operator runs
  `lyx mux add`), so v1 serializes the full cycle rather than assuming a single driver.
- **Lock ordering (deadlock-free):** the **outer `mux.lock` is always acquired BEFORE**
  `state.WriteJSON` internally takes its `mux.json.lock`. Strict **outer → inner**, never the
  reverse — so there is no lock-ordering cycle and no deadlock.
- **Scope is per-worktree:** `mux.lock` lives in the worktree's `.lyx/`, so two *different*
  worktrees never contend on it (each has its own). The OS file handle is **released
  automatically if a holding process dies** (the handle closes on exit) — so v1 needs **no**
  stale-lock detection / lock-stealing machinery; do not build any.
- **Rejected:** "assume one driver at a time" + document it (a torn/clobbered layout the moment
  loom and an operator both touch the session — a latent correctness bug); reusing the `state`
  JSON lock as the cycle lock (wrong granularity — it does not cover the `select-layout` apply).

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
  `slog.TextHandler` bound to a package `slog.LevelVar`. A single **persistent count flag on the
  `cmd/lyx` root** — cobra `CountVarP("verbose", "v", …)` — sets the level once at startup: count
  **0 → `Warn`** (default), **1 → Info**, **≥2 → Debug**. `--verbose` is the long form of the same
  count flag, so one `-v`/`--verbose` = Info and `-vv` (= `--verbose --verbose`) = Debug
  (internalmux NOTE2 — the earlier "`--verbose`=Debug" phrasing was imprecise). The sink is an
  **injectable `io.Writer` field defaulting to the real `os.Stderr`** — deliberately **not**
  routed through `clihelp`'s stdout/stderr seam.
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
  machine-specific tool paths (`psmux`, `pwsh`, `claude`), dimensions (width/height), the layout
  knobs **`collapsedStripRows`** (rows of a collapsed ancestor strip — replaces muxpoc's fixed
  55% active-pane share), **`topBandRows`** (default 1 — height of a `top` status band), and
  **`minFullRows`** (default 3 — the clamp floor for a full pane; see the render decision), plus
  the **`strand-name`** template (default `<ROLE>:<ROUND>:<SHORT_GUID>`; tokens
  `<WORKTREE> <ROLE> <ROUND> <SHORT_GUID>`). Loaded via
  `configengine.Load(baseDir, "mux", []byte(ConfigTemplate()))`.
- **`strand-name` is a config field, NOT hardcoded (feedback_02).** The default is
  `<ROLE>:<ROUND>:<SHORT_GUID>` but the operator can reorder/replace it. `<WORKTREE>` is a token
  but **omitted from the default** — inside a per-worktree session the worktree is redundant (it
  *is* the session), and long worktree names hurt readability; it earns its place only in the
  deferred cross-worktree/mplex view (add it then). Substitution stays domain-free: `<ROLE>`/
  `<ROUND>` are formatting-only inputs (see the strand-identity decision), never persisted or
  branched on.
- **Rationale:** tool paths are machine-specific and belong in config, not code defaults;
  strip thickness is a layout-tuning knob the operator should control; matches the repo
  convention and makes sandbox use clean. shuttle will likely reuse tool-path config.
- **Portability note (orch #5) — v1 chooses correctness over cross-machine portability, on
  purpose.** `_lyx/config/` is **weft-synced**, so an absolute tool path committed into
  `mux.yaml` will be wrong on machine #2. But the empirical finding is that psmux/claude/pwsh
  **must** be launched with **explicit absolute paths** (bare `pwsh` resolves to a 0-byte
  WindowsApps ConPTY stub that renders nothing). v1 deliberately prioritizes correctness
  (explicit paths that actually work here) over cross-machine portability — which is **deferred
  anyway** (session-file portability is a later milestone). **Do NOT "fix" this by making paths
  PATH-relative** (it reintroduces the ConPTY-stub failure). The future cross-machine path is
  the existing gitignored per-machine `.env` (weft-local, never synced — see overview.md), which
  can override the synced defaults per machine; that is a later refinement, not v1.
- **Rejected:** cobra flags with hardcoded defaults (muxpoc style — bakes machine paths into
  code); flags-now-config-later (risks a churny migration); PATH-relative tool names (breaks on
  the ConPTY stub).

### CLI verb set (minimal-but-functional)

- **Decision:** `up`, `add`, `remove`, `status`, `attach`, `resume`, `down`. (`UpdateStrand` is
  engine-API-only — no CLI verb in v1.)
- **Rationale:** smallest set that is genuinely functional and independently sandbox-testable
  before shuttle exists; `add`/`remove` make the engine drivable and cover the load-bearing
  re-render behaviors (parent shrinks on add, grows on remove).
- **Rejected:** folding `resume` into `up` (diverges from `mux.md`); even-leaner
  `up/add/status/attach/down` (can't exercise RemoveStrand re-render or crash recovery via
  CLI).

**Sharp `up` vs `resume` boundary (orch #3) — `up` = substrate only, `resume` = replay
content. `up` NEVER launches/relaunches a strand command; `resume` is the only replayer:**

| Verb | Does |
|---|---|
| `up` | Ensure the server (clean env) + this worktree's session **exist** (boot if absent; no-op if up). Apply the layout from the current strand table. Reconcile (clear the pane binding of dead strands, **keep the record**, re-derive pane ids). **Runs no strand command.** |
| `resume` | For each persisted strand that is **not live AND not `hidden`** (no pane / `pane_dead=1`, `anchor != hidden`): (re)create its pane, then run its stored `resumeCmd` (or `cmd` if no `resumeCmd` — every strand has at least a `cmd`, so all such strands are rebuildable). **`hidden` strands are skipped** — they are "pending first surface", not dead, so resume does not surface them (internalmux GAP1 regel 2). Boots server+session first if absent. **Strands already live are left untouched** (no double send-keys). Apply layout; re-persist pane ids. |
| reconcile | Shared by **every** verb (read table + live `list-panes`). For a dead/absent pane it **clears the pane binding and marks the strand not-live, but keeps the record** (so `resume` can rebuild it) and excludes it from render; re-derives ids for live panes. To keep the layout string consistent with psmux's live pane set it **kills the physical `pane_dead=1` pane before re-applying the layout** — *except* a **sole-remaining** dead pane (kept + rendered until `resume`/`remove`). Order: **kill dead → re-enumerate → layout → apply**. **Only `remove` deletes a record.** Not a separate command. |

Behavior in the three states:
- **Server dead (reboot):** `resume` rebuilds — boots server+session, recreates a pane per
  strand, replays each strand's resume/launch cmd, re-persists new pane ids. `up` alone on a
  dead server just boots an **empty** session (a fresh workspace) — it does not resurrect
  strand content.
- **Server up, CLI restarted (the normal one-shot case):** any verb reconciles against live
  `list-panes` and re-applies the layout; **no relaunch** (panes are alive).
- **Single strand's pane died (server alive):** on-demand reconcile detects `pane_dead=1`, clears
  that strand's pane binding (marks it not-live) but **keeps its record**, and re-renders (parent
  grows back). Bringing it back = `resume` (recreates its pane + replays it; already-live strands
  untouched). The record persists until an explicit `remove`, so it is always resume-able.

**`add` flag spec (orch #4) — exposes `--anchor` so top/hidden get a real CLI + sandbox path,
not render-unit-tests only:**

```
lyx mux add --cmd <launch-cmd> [--role <role>] [--round <n>] [--name <override>]
            [--resume-cmd <cmd>] [--parent <guid>]
            [--anchor top|below-parent|hidden] [--focus]
```

- `--anchor` defaults to **`below-parent`**. Exposing it means the sandbox scenario can
  integration-test all three v1 anchors (top status line, below-parent stack, hidden), not
  just the default. `own-window` is rejected by `add` in v1 (deferred anchor).
- **Name:** `--role`/`--round` fill the `strand-name` template; `--name` overrides it verbatim;
  neither given → name = `<SHORT_GUID>`. mux generates the strand's `guid` and prints it (with
  the resolved name) in the `add` JSON so a later `--parent` can reference it.
- **No `--height`** — height is fully derived (see the render decision).
- `--resume-cmd` optional (see strand contract). `--focus` sets `display.focus:true`.
- `--parent` and `remove` take the **`guid`** (`lyx mux remove <guid>`; `--recursive` required to
  remove a non-leaf). `name` is display-only, never a selector.

**`attach` is session-level, in-place (orch minor + orch #F):** `lyx mux attach` runs
`psmux attach` to this worktree's psmux **session** **in the operator's current terminal** — no
strand argument (you see every strand's pane, then `Ctrl+b z` to zoom one). v1 attaches in-place
rather than popping a new window, so `mux.yaml` needs **no** terminal-emulator path; you run
`lyx mux attach` from a terminal you already have. (Popping a dedicated terminal window — needed
if the driver is a headless programmatic session with no TTY to attach into — is deferred and
would add a terminal-emulator config value.)

`attach` is a **documented, narrow exception to the JSON-envelope invariant (internalmux NOTE1).**
An in-place `psmux attach` inherits the operator's stdio and blocks — it cannot emit the
`output.Ok`/`Err` envelope the CLI/Cobra Invariant otherwise requires. The exception is scoped
tightly: everything that can fail (session missing, lock contention, reconcile) runs **pre-flight
and stays on the envelope** (emits `output.Err`, returns non-zero); only the **terminal-handover
tail** — after stdio is inherited — is exempt, and on success it emits **no** JSON. This follows
the existing interactive-`ide` precedent and is registered in `CONSTRAINTS.md` (CLI/Cobra
Invariant) in the same commit. The test asserts the **built `psmux attach` invocation** (target =
worktree session), not a JSON round-trip.

**`remove` of a non-leaf, and parent integrity (orch #D + internalmux #B):** removing a strand
with descendants **cascades** (removes the subtree, never orphaning children into a broken chain),
then re-renders. Two guards on the operator footgun:
- **CLI:** `lyx mux remove <guid>` on a **non-leaf** requires `--recursive`; without it the command
  fails with `strand has children, use --recursive` (so `remove <parent>` never silently kills N
  descendants). With `--recursive` it cascades.
- **Engine API:** `RemoveStrand(guid)` **always cascades** — the in-process caller (shuttle/loom)
  owns the spawn tree and manages it deliberately.
- **Observability:** the `remove` result JSON **lists every removed strand** (`guid` + `name`), so
  a cascade is always visible.

On `add`, a `--parent` naming no existing strand is rejected; a parent link that would form a
**cycle** is rejected at add-time, and render additionally breaks any cycle defensively (walk with
a visited-set, treat a repeat as a root) so the pure function stays **total** even on a corrupt
table.

**Session name, first pane, canvas, debounce (orch #G/H/I/J):**
- **Session name** = the worktree slug (`filepath.Base(WorktreeRoot)`), inside the per-hub
  server. Sibling worktrees can't share a basename, so it is collision-free. (G)
- **First pane:** psmux `new-session` always creates one initial shell pane. The **first strand
  adopts** that pane (captured via `display-message -p "#{pane_id}"`); every subsequent strand
  is a fresh `split-window -P -F "#{pane_id}"`. (H)
- **Dimensions** (`mux.yaml` width/height, muxpoc's 220×50) are the **assumed virtual canvas**
  for the detached session — the layout math targets them; on `attach` psmux re-flows to the
  real terminal size. v1 accepts that the dominance math was computed for the virtual canvas. (I)
- **Debounce** (coalescing a burst of mutations into one `ApplyLayout`) is an **in-process
  driver** concern only (shuttle/loom making rapid `AddStrand` calls); a one-shot CLI verb is a
  single mutation → single apply, so there is nothing to debounce there. (J)

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

## Forward-compatibility (v2: session-per-repo, column-per-worktree)

v2 (deferred) inverts the topology: **one session per repo/hub**, and **each worktree becomes a
column** within that session's window (mplex), with the current per-worktree strand stack living
*inside* each column. v1 must not bake in the false assumption "a session = one worktree's whole
world." Three near-zero-cost seams keep the v2 migration cheap — they are also just good v1
hygiene (avoiding a false assumption), **not** speculative v2 machinery:

1. **Every strand carries `worktree`** (already in the record). It is the v2 **column-grouping
   key**: v2 groups strands into columns by `worktree`. In v1 all strands share one worktree, but
   the field stays — it is the whole bridge. Do not drop it as "redundant in v1".
2. **The stack render is region-relative.** The pure stack builder takes a bounding box
   `(x, y, w, h)` and returns a **sub-layout** for that region, even though v1 always passes the
   full window `(0, 0, W, H)`. The tmux layout string already nests (`csum,WxH,0,0[…[…]…]`), so
   v2 columns are an outer horizontal split of these vertical stacks — a wrapping layer, no render
   rewrite. (This is only a region parameter; do **not** build the column arrangement now.)
3. **`MuxState` is a flat, GUID-keyed strand list**, each strand self-describing its `worktree`
   and `parent` — not "the strands of *this* worktree". So v2's hub-level state is a **union**
   (concatenation of the per-worktree files), not a reshape.

Explicitly **out of v1** (would be premature): hub-level state, hub-level lock, the column
arrangement/render, session-per-repo, and a `column` anchor. The lock stays per-worktree
(`.lyx/mux.lock`) and is promoted to hub-level in v2 (a natural change). Columns are an *outer*
grouping on `worktree`, orthogonal to the within-column anchor (`top`/`below-parent`/`hidden`),
so the anchor vocabulary is unchanged (`column`/left-right remains a deferred vocab candidate).

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
  with leading `%` stripped; panes ordered top→bottom. **bottom-active-dominant** (v1 height
  policy, see the render decision): each collapsed ancestor strip = `collapsedStripRows`
  (config), the bottom/active pane = the remainder; a pinned `top` strand is a fixed-height
  band above the stack; `hidden` strands are excluded. (This replaces muxpoc's fixed
  `activePaneShare=55%` — the *mechanics* below are reused verbatim, only the height policy
  differs.) Applied atomically via `select-layout "<csum>,<body>"`, then `select-pane` the
  focused strand (default bottom).
- **psmux subprocess wrapper** (`PsmuxCmd`): `run(args...)` (discard I/O) and `output(args...)`
  (capture stdout) **always prepend `-L <socketName>`**. The **server-spawning `new-session`
  is NOT routed through it** — it's raw `exec.Command` so `cmd.Env = CleanClaudeEnv(...)` +
  `proc.Detach` + `cmd.Start()` can be attached.
- **Two distinct pane-id capture strategies** (both required): `split-window -P -F "#{pane_id}"`
  for a **new** pane; `display-message -p "#{pane_id}"` for the `new-session` pane
  (`display-message` is unreliable for freshly-split panes on a detached session).
- **Pane-id is ephemeral, the strand `guid` is durable.** psmux reassigns pane ids across a
  server restart; on reconcile/recover, re-derive pane ids and re-persist; the mux-generated
  `guid` is the stable key (claude's `sessionId` is opaque metadata, not the identity — see the
  strand-identity decision).
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
  **`attach` is a registered exception** (like the interactive `ide` commands): its
  terminal-handover tail emits no JSON envelope — only its pre-flight errors do. Record this
  exception in `CONSTRAINTS.md` when the module lands (internalmux NOTE1).
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
  - bottom-active-dominant invariants: heights + 1-row dividers exactly fill window height;
    each collapsed ancestor strip = `collapsedStripRows`; the bottom/active pane = the
    remainder and is strictly tallest; cumulative y-offsets. Parameterize over
    `collapsedStripRows` (the config knob) and assert the remainder-height math holds for
    several values + degenerate cases (window too short for N strips → clamp rule).
  - **remainder determinism with ≥2 full panes (internalmux r2 NOTE1):** with the active strand
    + one or more `shrink:false` full panes, an integer-division remainder is assigned to the
    **active/bottom** pane (others get the floor); assert heights still exactly fill and the
    layout is deterministic (no ambiguous leftover owner).
  - **anchor policy** cases: `top` pinned as a fixed-height band above the stack;
    `below-parent` forms the bottom-dominant stack ordered by parent chain; `hidden` strands
    are **excluded** from the layout string entirely; mixed sets (top + stack + hidden);
    empty/single-strand edge cases. Each anchor's rule is independently asserted so adding an
    anchor adds an isolated test.
  - **`shrinkWhenWaitingOnChild`** (orch #1): an ancestor with it true collapses to a
    `collapsedStripRows` strip; with it false keeps its `height` while still an ancestor
    (assert the bottom pane's remainder shrinks accordingly).
  - **`focus`** (orch #1): with no strand focused, the select-pane target = bottom-most; with
    one strand `focus:true`, that strand is the target; exactly one focused pane; ties resolve
    to the bottom-most focused strand.
  - **shrink:false + clamp priority** (orch #1b/#B): a `shrink:false` ancestor becomes a co-equal
    full pane (splits the remainder equally with the active); the clamp reduces in strict order
    (strips → full panes → active-last) so a too-small window still yields only positive pane
    heights.
  - **sibling order** (orch #E): two strands sharing a parent render in insertion order.
  - **parent cycle** (orch #D): a cyclic parent table renders total (cycle broken via a
    visited-set), never loops.
- **muxengine strand bookkeeping (TDD candidate).** `AddStrand`/`UpdateStrand`/`RemoveStrand`
  mutate the table and persist; round-trip through `state.ReadJSON/WriteJSON` (absent file →
  empty table; corruption surfaced). Reconcile: given a saved table + a fake `list-panes`
  result (incl. `pane_dead=1` rows, which `remain-on-exit on` produces), **clear the pane binding
  of dead strands but keep their record** (assert the record survives and is resume-able; only
  `remove` deletes), keep live, re-derive pane ids, and **kill the physical `pane_dead=1` pane
  before re-apply — except a sole-remaining dead pane, which is kept and rendered** (GAP2).
  Debounce: a burst of mutations → one `ApplyLayout`.
  `CleanClaudeEnv`: strips exactly `CLAUDECODE` + `CLAUDE_CODE_*`, returns the stripped keys,
  leaves the rest untouched. `resumeCmd` optional: a strand without one falls back to re-running
  `cmd` on resume.
- **strand identity + naming (feedback_02).** `AddStrand` generates a unique `guid`; `name` is
  formatted from the `strand-name` template with `--role`/`--round` consumed (not persisted),
  `--name` overriding verbatim, and `<SHORT_GUID>` fallback when neither is given.
  `FormatStrandName` is a pure table test over templates/tokens. `--parent`/`remove` take the
  `guid` (guid-only). CLI `remove` on a non-leaf without `--recursive` errors; `RemoveStrand`
  (engine) always **cascades** and the result lists every removed strand; an `add` with a
  non-existent or cyclic parent is rejected.
- **up vs resume boundary (orch #3).** Assert `up` never emits a strand launch/resume command
  (substrate only), only server/session bring-up + layout + reconcile; assert `resume` replays
  the stored `resumeCmd` (or `cmd` when absent) per strand and re-persists pane ids; assert
  `resume` **skips `hidden` strands** (`not-live AND anchor != hidden` — a hidden strand is
  pending-surface, not dead, so resume does not surface it, GAP1 regel 2). Cover the
  three states (server dead / server-up-CLI-restarted / single pane died) at the seam that can
  be driven without a live psmux (pure planning of "what commands would run"), with the live
  psmux round-trip behind the smoke tag.
- **Concurrency lock (orch #6 + internalmux r2 NOTE2).** Assert the lock is acquired at **exactly
  one layer** — the engine op — and that a CLI verb does **not** re-take it (a test that a CLI
  verb → engine-op path never double-acquires, i.e. no self-deadlock from the non-reentrant
  flock); the outer `mux.lock` is acquired before `state`'s inner `mux.json.lock` (ordering
  test); two concurrent mutations serialize (no interleaved render/apply); the lock is
  per-worktree (different `.lyx/` dirs don't contend); a process dying while holding it leaves no
  stale lock (handle auto-released — assert a subsequent acquire succeeds).
- **`add --anchor` integration (orch #4).** Drive `lyx mux add --anchor top|below-parent|hidden`
  through `RunCLI` and assert the strand lands with the right anchor (and the sandbox scenario
  exercises all three end-to-end), so top/hidden have real CLI/integration coverage, not only
  render unit tests. Assert a `hidden` strand lands **with no pane and `cmd` un-run** (launch
  deferred to surface); `UpdateStrand` flipping `hidden → visible` creates the pane + runs `cmd`;
  `UpdateStrand` `visible → hidden` is **rejected** (GAP1 regel 1/3).
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
- **Q:** Strand contract? **A:** `AddStrand{ name, worktree, parent?, cmd, resumeCmd, sessionId?, display{anchor,height,focus,shrinkWhenWaitingOnChild} }`; mux stores all, reads none semantically; **no domain `type`**. mux does NOT assign the session-id (shuttle owns launch+resume construction). _(Superseded by feedback_02: record adds a mux-generated `guid` (the durable key) and drops `display.height`; see the strand-identity decision + the feedback_02 identity Q&A.)_
- **Q:** mux config? **A:** Config file via `configreg` (`mux.yaml`: tool paths, dims, active-pane share), not flags-with-defaults. _(Superseded by feedback_02: `active-pane share` → layout knobs `collapsedStripRows`/`topBandRows`/`minFullRows` + the `strand-name` template; see the config decision.)_
- **Q:** Render anchor scope? **A:** `top` + `below-parent` + `hidden` in v1; `own-window` deferred (no consumer). Keep the closed 4-member vocabulary; grow only when a real consumer needs it (new `rules()` case + test same commit).
- **Q:** Render code structure? **A:** Anchor→layout **policy** must be explicit/legible and **separated** from layout **mechanics** (checksum/string builder), so changing/adding an anchor is a localized, obvious edit.

_Orch review round (feedback_01):_

- **Q (orch #1):** `focus`/`shrinkWhenWaitingOnChild` undefined in v1? **A:** Define both with precise render rules + isolated tests (not trim). `shrink` default true → ancestor collapses to a `collapsedStripRows` strip; `focus` default unset → bottom-most focused, caller may override.
- **Q (orch #1b):** Collapsed strip thickness? **A:** Configurable via `mux.yaml` `collapsedStripRows` (replaces muxpoc's fixed 55% active-share); active pane = the remainder. Mechanics (checksum/string) unchanged.
- **Q (orch #2):** What triggers `pane-died` re-render in a daemonless model? **A:** Nothing live in v1 — re-render is on-demand (in-process per mutation + reconcile on each verb); `remain-on-exit on` makes dead panes detectable. The psmux hook + hidden handler verb are deferred with the daemon (removes the earlier contradiction).
- **Q (orch #3):** `up` vs `resume` boundary? **A:** `up` = substrate only (boot/ensure session + layout + reconcile, **never** replays a command); `resume` = the only replayer (recreate panes + run stored resume/launch cmds). Defined across three states.
- **Q (orch #4):** Does `add` expose `--anchor`? **A:** Yes — full flag spec (`--name --cmd --resume-cmd --parent --anchor[=below-parent] --height --focus`); `--anchor` gives top/hidden a CLI + sandbox integration path.
- **Q (orch #5):** Tool paths in synced `mux.yaml` vs portability? **A:** Keep in `mux.yaml`; document that v1 chooses correctness (explicit absolute paths, required by the ConPTY-stub finding) over cross-machine portability (deferred anyway). Do not PATH-relativize; future per-machine override rides the gitignored `.env`.
- **Q (orch #6):** Concurrency across separate CLI processes + in-process driver? **A:** One coarse mux operation lock (`.lyx/mux.lock`) around the whole read→mutate→persist→render→apply cycle. Ordering: outer `mux.lock` before `state`'s inner `mux.json.lock` (deadlock-free). Per-worktree scope; OS handle auto-releases on process death → no stale-lock-stealing in v1.
- **Q (orch minor):** `attach` target? **A:** Session-level, **in-place** (`psmux attach` in the current terminal — no terminal-emulator config; popping a window is deferred); no strand arg. **`resumeCmd`?** Optional/nullable; absent → resume re-runs `cmd`.

_Orch review round (feedback_02):_

- **Q (identity):** How is a strand identified? **A:** mux-generated **GUID** = canonical durable key (replaces `sessionId`, demoted to opaque metadata); caller-set **`name`** = descriptive display-only label; `--parent`/`remove` are **GUID-only** (internalmux #C2 — no name-selector/ambiguity path).
- **Q (naming):** How is `name` set? **A:** From a **config** `strand-name` template (default `<ROLE>:<ROUND>:<SHORT_GUID>`, NOT hardcoded); `--role`/`--round` are domain-free formatting-only inputs; `--name` overrides; `<SHORT_GUID>` fallback. `<WORKTREE>` token exists but is omitted from the default (redundant inside a per-worktree session).
- **Q (height):** Keep `--height`? **A:** No — dropped; height fully derived (`topBandRows` band, `collapsedStripRows` strips, active + `shrink:false` = full panes splitting the remainder) with a clamp rule so render stays total.
- **Q (defaults B/D/E):** clamp / remove-non-leaf / sibling order? **A:** clamp = strips → full → active priority (never a non-positive pane); `remove` cascades to descendants + reject cyclic/unknown parent (render breaks cycles defensively); siblings ordered by insertion.
- **Q (F–J):** attach / session name / first pane / dimensions / debounce? **A:** attach in-place (`psmux attach`, no terminal-emulator config); session name = worktree slug; first strand adopts the `new-session` pane; dimensions = assumed virtual canvas (re-flows on attach); debounce is in-process-driver-only.
- **Q (v2):** Forward-compat for session-per-repo / column-per-worktree? **A:** Three cheap seams — strand carries `worktree` (column key), region-relative stack render, flat GUID-keyed `MuxState` (union not reshape); hub state/lock + column render stay out of v1.

_Orch review round (first discussion review):_

- **Q (GAP1):** How is a `hidden` strand realized in psmux? **A:** No pane until surfaced — a record whose `cmd` is not run at `add`; the pane is created + `cmd` run only when `UpdateStrand` flips `anchor` off `hidden`. Exclusion from the layout is **by construction** (no live pane exists). `anchor: hidden` is add-time-only; `visible → hidden` is **rejected** in v1 (regel 3). `resume` **skips** hidden strands (`not-live AND anchor != hidden`) — hidden = pending-surface, not dead (regel 2). Same bug-class as reconcile-drop-vs-resume: *died* strands rebuild, *hidden* strands stay put.
- **Q (GAP2):** Dead pane vs `select-layout` consistency? **A:** reconcile **kills** the physical `pane_dead=1` pane before re-applying the layout (so the string matches the live pane set), *except* a **sole-remaining** dead pane (killing ends the session) which is kept + rendered until `resume`/`remove`. Order: kill → re-enumerate → layout → apply, under the mux lock.
- **Q (NOTE1):** `attach` in-place vs the JSON-envelope invariant? **A:** Documented **narrow exception** — pre-flight failures stay on the envelope; only the terminal-handover tail is exempt (no success JSON). Registered in `CONSTRAINTS.md` (CLI/Cobra), interactive-`ide` precedent; test asserts the built `attach` invocation.
- **Q (NOTE2):** `-v`/`-vv` mechanics? **A:** One cobra `CountVarP("verbose","v")`: 0 → Warn, 1 → Info, ≥2 → Debug; `--verbose` is the long form of the same count.
- **Q (NOTE3):** Does v1 `status` do orphan detection? **A:** No — the named server *enables* the firewall, but active stray-server enumeration (unverified on Windows) is deferred; v1 `status` = this session only.
