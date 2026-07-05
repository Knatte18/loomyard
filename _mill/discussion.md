# Discussion: Build internal/shuttle: one LLM agent via a swappable engine

```yaml
task: 'Build internal/shuttle: one LLM agent via a swappable engine'
slug: internal-shuttle
status: discussing
parent: main
```

## Problem

The orchestration spine (`proc → mux → shuttle → review → loom`) is the designed path to
replacing mill/millhouse with Go orchestration. `proc` and `mux` are done; **shuttle is the
next layer and its only dependency (mux) is complete**. Nothing above it (review, loom) can
be built until shuttle exists.

shuttle runs **one LLM agent as an interactive psmux session** and returns its result over
the file contract. Interactive-never-headless is an economic constraint (headless `claude -p`
is losing subscription coverage; see `CLAUDE.md` "Agent execution"), so the run mechanism is:
build a `claude` launch command, hand it to `mux.AddStrand` as an opaque string, wait on a
Stop-hook file + the agent's output file, interpret, return. The provider specifics live
behind an **engine** seam (Claude engine now; the seam exists to isolate the dependency, not
because a second engine is imminent).

Source of truth for the design intent: roadmap milestone 10 and `docs/modules/shuttle.md`
(the design doc; deleted when this lands per the documentation lifecycle). For **mux facts,
the code is authoritative, not `docs/modules/mux.md`** — that doc is already drifting and is
deleted in this task.

## Scope

**In:**

- New module: `internal/shuttleengine` (domain kernel) + `internal/shuttlecli` (`lyx shuttle`
  cobra subtree), registered in `cmd/lyx/main.go` per the CLI/Cobra Invariant.
- Engine seam: a Go interface in `shuttleengine`; one implementation,
  `internal/shuttleengine/claudeengine`. Claude only for now.
- One-shot run: `Start(spec)/Wait()` (with a blocking `Run` convenience) — launch via
  `mux.AddStrand`, wait on the Stop-event file + caller-named output files, classify outcome
  (`done | asking | died | timeout`), clean up.
- **In-agent interrupt is in scope (v1):** `Interrupt` (ESC — agent stops, session stays
  warm) + `Send` (one-line follow-up, e.g. updated instructions) — both as Go API and as
  CLI verbs. Rationale: today there is no way to stop an agent, give it updated
  information, and let it continue.
- Claude Code `--settings` composition: Stop hook (writes turn-end payload to a file) +
  PreToolUse guardrails (deny in-process `Agent` and `AskUserQuestion`), guardrails
  config-toggleable.
- New config module `shuttle.yaml` (registered in `internal/configreg`).
- **mux extensions (same task, all provider-agnostic):** `AddSpec.SessionID` (persisted into
  the existing `Strand.SessionID` field), an exported `SendKeys` op (generic key/text
  transport to a strand's pane, needed by Interrupt/Send), and an exported `CapturePane` op
  (needed for startup probing / trust-dialog detection). Remove the unused `claude:` key
  from mux — the `template.yaml` line, the `Config.Claude` struct field
  (`internal/muxengine/config.go:24`), and the empty-default assertion in
  `config_test.go:53-54`.
- Run-artifact lifecycle: per-run directory (prompt file, settings.json, events.jsonl,
  run.json), deleted on clean completion, kept on failure, orphan-swept later.
- Docs: delete `docs/modules/shuttle.md` **and** `docs/modules/mux.md` (documentation
  lifecycle — mux is built and its doc is already stale); fold durable parts into package
  headers + `overview.md`; retarget links in `loom.md`/`review.md`/`roadmap.md`/`overview.md`
  that point at the deleted files; mark roadmap milestone 10 ✅ Done; add shuttle to the
  `overview.md` module table and execution stack; update `CONSTRAINTS.md` pinned CLI test
  sets; sandbox suite coverage for the `shuttle` module.

**Out:**

- Non-Claude engines (Gemini etc.) — the seam exists; no second implementation.
- `review` and `loom` — the callers come later; shuttle ships with the CLI + tests as its
  only consumers.
- Follow-up *turn orchestration* (driving a multi-step conversation) — `Send` exists as a
  primitive for interrupt-with-new-info, but shuttle does not manage multi-turn protocols.
- Respawn-after-death / escalation — caller policy (loom respawns on the file contract;
  escalation is a fresh spawn, never in-session `/model`).
- mux's deferred items stay deferred: `own-window` anchor (cluster reviewers), hidden-strand
  surfacing changes, daemon, `pane-died` auto-trigger, cross-worktree columns.
- The mux "no transcript → fresh launch" resume gap — stays deferred.
- Pause/`pause_requested` machinery — loom's concern; shuttle only provides Interrupt.

## Decisions

### Package layout: engine/cli split with a registered CLI

- Decision: `internal/shuttleengine` + `internal/shuttlecli` (`lyx shuttle`), registered in
  `newRoot()`. Verbs: `run` (blocks until outcome, prints result JSON envelope),
  `interrupt <guid>`, `send <guid> <text>`. Parent group uses `clihelp.GroupRunE`.
- Rationale: standard CONSTRAINTS naming split; a CLI makes shuttle drivable standalone
  (sandbox suite, manual testing, and mill can spawn agents through it before loom exists).
  review/loom later call the Go API directly.
- Rejected: engine-only `internal/shuttle` (no black-box test surface, invisible to the
  sandbox-coverage invariant); CLI as a separate later task.

### Engine seam: interface + claudeengine leaf package

- Decision: a provider interface defined in `shuttleengine`; `internal/shuttleengine/claudeengine`
  implements it. The interface owns everything provider-specific: composing the launch
  command line + `resumeCmd` + session id, writing the settings file (hooks + guardrails),
  interpreting Stop-event payloads (done vs asking), startup babysitting (TUI-appeared probe,
  trust-dialog dismissal), and the key sequences for Interrupt/Send (ESC, Esc-before-text).
  `shuttleengine` owns the provider-invariant machinery: run directory, mux calls, the
  wait/poll loop, outcome classification, cleanup. Import direction:
  `shuttlecli → shuttleengine → claudeengine`-as-plugged-implementation (claudeengine never
  imports shuttlecli).
- Rationale: the verdict/output contract is provider-invariant by design; the interface makes
  the seam visible and gives hermetic tests a fake engine. Claude specifics stay in one leaf.
- Rejected: no interface (concrete Claude type only) — the seam is the design point;
  interface + impl in one package — blurs what is provider-specific.

### Config: new `shuttle.yaml`, flat keys, engine-prefixed; mux loses its dead `claude:` key

- Decision: register a `shuttle` config module in `internal/configreg` with a flat template
  (configengine templates are flat `key: value` with `${env:...}` defaults). Indicative keys
  (plan finalizes names/defaults): `run_dir` (empty ⇒ `<worktree>/.lyx/shuttle/` via
  `hubgeometry.Layout.DotLyxDir()`), `poll_interval_ms` (~500), `liveness_every_n_polls`
  (~10), `run_timeout_min` (default 30), `startup_timeout_s`, and Claude-engine keys
  prefixed `claude_`: `claude` (binary path, empty = PATH), `claude_deny_agent_tool`
  (default true), `claude_deny_ask_user_question` (default true). The unused `claude:` key
  is removed from mux in the same change — `internal/muxengine/template.yaml`, the
  `Config.Claude` struct field (`config.go:24`), and its config-test assertion
  (`config_test.go:53-54`) — with config reconcile handling the schema change (verify
  `lyx config` reconcile behaviour for removed keys).
- Rationale: shuttle owns its provider config; mux stays a dumb carrier. Guardrails are
  config-toggleable per operator request ("modulert"). Engine-specific keys grouped by
  prefix because templates are flat.
- Rationale (geometry note): CONSTRAINTS says geometry is structural/never config-overridable,
  but that governs hub-geometry tokens; a module-local working-dir key is ordinary module
  config (like mux `width`/`height`) as long as the default resolves through hubgeometry.
- Rejected: reusing mux.yaml's `claude:` (wrong ownership); a separate `claude-engine.yaml`
  config module (over-modular for one engine); nested YAML sections (templates are flat).

### mux API additions (generic, dumb-carrier-preserving)

- Decision: three additions to `internal/muxengine`, all provider-agnostic:
  1. `AddSpec.SessionID` — stamped into the existing `Strand.SessionID` field (documented as
     caller-written opaque metadata; today nothing writes it).
  2. Exported `SendKeys(guid, ...)` engine op — literal-text and named-key transport to a
     strand's pane (reusing `sendKeysLiteralArg` hazard handling), under the op lock.
  3. Exported `CapturePane(guid)` engine op — on-demand pane content read (read-only, like
     `Status`: no reconcile, no layout apply).
- Rationale: Interrupt/Send and the startup probe need pane I/O, and psmux access is mux's
  monopoly. All three carry zero domain knowledge — mux still never parses what it transports.
- Rejected: shuttle shelling to `psmux.exe` directly (breaks mux's overlay monopoly and env
  hygiene); shuttle keeping session ids in its own state only (two sources of truth — the
  field already exists on Strand).

### Prompt delivery: always buffered to a run-scoped file

- Decision: `spec.Prompt` is a plain Go string; the Claude engine writes it to
  `<run_dir>/<runID>/prompt.md` and builds the launch line
  `& '<claude>' (Get-Content -Raw '<prompt-file>') --session-id <id> --settings '<settings>'
  [--model <m>] [--dangerously-skip-permissions]`. One code path for all prompts.
- Rationale: mux types the launch command into the pane's pwsh via `send-keys -l` + `Enter`
  (`spawn.go:110-146`) — everything must survive as **one line typed into an interactive
  shell**. Multiline prompts inline would submit on every `\n` (the proven paste hazard);
  quoting arbitrary text is fragile. The file also gives an audit trail and deterministic
  respawn.
- Rejected: single-line-only contract (landmine for callers); dual path (inline when
  single-line) — two code paths, no gain.

### Run directory and cleanup

- Decision: per-run directory `<run_dir>/<runID>/` holding `prompt.md`, `settings.json`,
  `events.jsonl`, `run.json` (run state: strand guid, session id, spec echo — what the CLI
  `interrupt`/`send` verbs and diagnosis need). Files must live **as long as the strand
  lives** (resumeCmd and respawn reference them). Cleanup: on `done` → `RemoveStrand` +
  `os.RemoveAll(runDir)`; on `asking`/`died`/`timeout` → keep strand and directory (operator
  can attach/inspect; it is the diagnosis material). Orphan sweep: a later shuttle run
  removes run dirs whose strand guid no longer exists in `mux.json` — **guarded by an age
  threshold**: a dir younger than the startup window (`startup_timeout_s`, with margin) is
  never swept, because a concurrently starting run creates its dir and `run.json` *before*
  `AddStrand` persists the strand, and an unguarded sweep in that window would delete an
  in-flight run. `.lyx/` is untracked.
- Rejected: never clean (accumulates over long worktree lives); always delete at return
  (loses diagnosis + respawn inputs).

### Stop hook: plain shell append, POSIX paths

- Decision: the Stop hook command is a plain append of stdin to the run's event log —
  `cat >> '<posix-path>/events.jsonl'` (with a newline separator) — no hidden lyx verb.
  Hook commands run under **git-bash on Windows**, so the engine converts the run-dir path
  to POSIX form (`C:\…` → `/c/…`); a backslash path would be silently destroyed.
- Rationale: zero dependencies, daemonless, fits the file contract. shuttle polls and parses
  the JSONL (multiple Stop events accumulate across turns; shuttle tracks its read offset).
- Rejected: hidden `lyx shuttle on-stop` verb (CLI surface + PATH dependency for nothing v1
  needs).

### Outcome classification (the verdict contract)

- Decision: four outcomes. `done` = Stop event received **and** all expected output files
  exist. `asking` = Stop received, output missing — the agent ended its turn asking;
  `LastAssistantMessage` is returned so the caller sees the question. `died` = pane dead (or
  startup probe failed) without a qualifying Stop. `timeout` = deadline passed. Result:
  `{Outcome, SessionID, StrandGUID, LastAssistantMessage, RunDir}`.
- Decision: the **caller** names the expected output files (`spec.OutputFiles []string`) and
  writes its own prompt instructing the agent where to write. shuttle never templates prompt
  content — dumb transport, like mux.
- Decision (path contract): `OutputFiles` entries are **absolute, or relative to the
  worktree root** — the engine resolves relative entries against `layout.WorktreeRoot` at
  spec validation and stores/polls absolute paths from then on. One convention, pinned,
  because the caller's prompt and shuttle's existence check must agree on the base (the Go
  process cwd and the pane cwd are both the worktree in practice, but the contract must not
  depend on that coincidence).
- Decision: **`OutputFiles` is mandatory — at least one path.** `Run` (and `lyx shuttle run`)
  rejects an empty spec with an error. The file hand-off is what makes a run "return" to its
  caller; with zero expected files "all files exist" is vacuously true and every Stop would
  silently classify as `done`, making `asking` unreachable and turning a misconfigured spec
  into silent success. Even tree-mutating agents (future builder workers) end with a small
  report file — the same discipline mill's implementer JSON report uses. A run with no
  result contract belongs one layer down (`lyx mux add`), not in shuttle.
- Rejected: output-file-existence alone (cannot distinguish asking from working — loom needs
  that distinction for human gates); shuttle-side prompt placeholders (prompt opinions belong
  in review/loom profiles); empty-OutputFiles-allowed variants (Stop ⇒ `done` degenerates
  the verdict; Stop ⇒ `asking` makes `done` unreachable instead).

### Waiting: file polling + periodic liveness + deadline

- Decision: poll `events.jsonl` + output files at `poll_interval_ms`; every Nth poll, check
  strand liveness via mux (read-only). Total deadline: `spec.Timeout`, 0 ⇒ config
  `run_timeout_min`. Startup window: within `startup_timeout_s` the engine's probe must see
  the provider TUI in `CapturePane` output (and dismisses the trust dialog if shown); a pane
  whose claude exited immediately (bad flag, not logged in) leaves a live shell that would
  otherwise wait out the full timeout — the startup probe converts that to a fast `died`
  with the captured pane content as diagnostics.
- Rejected: no overall deadline (a hung-but-alive agent blocks a review round forever).

### Guardrails: PreToolUse deny — Agent always, AskUserQuestion mode-coupled

- Decision: settings.json includes `PreToolUse` deny rules, composed per run:
  - **`Agent` deny — always on, in both modes** (config-toggleable via `claude_deny_agent_tool`).
    Steer: do the work in-session / request a visible strand. Invisible nested work is
    equally wrong attended and unattended.
  - **`AskUserQuestion` deny — coupled to `spec.Autonomous`** (config-toggleable via
    `claude_deny_ask_user_question`). `Autonomous: true` (default; unattended): denied,
    because the dialog blocks silently with nobody watching and shuttle cannot even see that
    the agent is asking (no Stop fires). `Autonomous: false` (attended — the caller knows the
    operator will interact, e.g. a discussion phase): **allowed** — in a psmux pane the agent
    is a CLI TUI where AskUserQuestion is a keyboard-navigable picker that works well; the
    operator is attached and answers in place, the turn continues, and shuttle correctly sees
    nothing. (mill's blanket AskUserQuestion ban came from the VS Code extension context; it
    does not transfer to CLI panes.)
  - `spec.Autonomous` also controls `--dangerously-skip-permissions` (on when autonomous).
- Decision: **the deny-steer must preserve escalation.** Steer text (autonomous mode): "You
  cannot open an interactive dialog. If you are blocked or need operator input, state the
  question/blocker as your final message and end your turn WITHOUT writing the result file."
  Never "decide yourself" — that instruction belongs (if at all) to the caller's prompt, not
  shuttle's guardrail. `asking` is thus the **escalation channel**: an agent that is truly
  stuck stops cheaply and the Go caller must handle it; shuttle never auto-answers or
  auto-continues. (Operator incident motivating this: an auto-mode agent that found the
  review machinery broken, was effectively forbidden to ask, and "chose" to ship everything
  unreviewed — the correct behaviour was to halt and escalate "reviewer is broken". What a
  future loom `--auto` does with a blocker-level `asking` — answer vs halt to human — is
  loom's policy design, out of scope here; shuttle guarantees the channel.)
- Rationale: holds the everything-visible-in-mux invariant, prevents silent blocking, and
  keeps the stop-and-ask path always available. The deny-and-steer path is **unprobed**
  (open item in `docs/research/mux-hooks-exploration.md`) — this task must verify it live
  (smoke test).
- Rejected: unconditional AskUserQuestion deny (forces plain-text questions even when the
  operator sits attached and the CLI picker is the better UX); no deny at all (autonomous
  runs can hang invisibly on a dialog).

### Trust dialog: engine-side defensive dismissal

- Decision: during the startup window the Claude engine detects the "trust this folder"
  screen in `CapturePane` output and sends Enter (the proven pattern from
  `TestSmokeClaudeResumeRecallsCodeword`). Defensive robustness: the operator reports not
  seeing it for worktrees in practice (VS Code's workspace-trust prompt is a separate,
  unrelated thing), but claude does show it for genuinely fresh directories (smoke fixtures
  hit it), so the handler stays — cheap, engine-scoped, screen-scraping-as-liveness only
  (never the result channel).
- Rejected: pre-seeding `~/.claude.json` (undocumented schema, brittle); documented
  precondition (kills autonomy for spawned worktrees if it ever does trigger).

### In-agent interrupt: Interrupt + Send pair (API + CLI)

- Decision: `Interrupt(handle|guid)` sends ESC via mux `SendKeys` — the agent stops
  mid-turn, the session stays warm and idle (nothing killed). `Send(handle|guid, text)`
  types a follow-up: Esc first (clears leaked auto-suggest — the empirically required
  sequence), then the text as one literal line, then Enter; the wait loop continues on the
  same `events.jsonl` (next Stop event). `Send` rejects text containing newlines; multiline
  updates ride the file contract (write a file, send the one-line pointer "read <file> —
  updated instructions — and continue"). Go API: `Start(spec) (*Run, error)` returning a
  handle with `Wait/Interrupt/Send`; `Run(spec)` = Start+Wait convenience. CLI:
  `lyx shuttle interrupt <guid>` / `lyx shuttle send <guid> <text>` resolve the run via
  `run.json` (guid ↔ run dir), so an operator can do it from another terminal.
- Decision (v1 limitation, explicit): Interrupt/Send are only meaningful **against a live,
  blocking Run** — the in-process handle, or a still-blocking `lyx shuttle run` poked from
  another terminal (its wait loop keeps reading the same `events.jsonl`, so the next Stop is
  classified normally). Once `Run` has returned (e.g. `asking`), no process re-enters the
  wait loop: a later `send` injects keys but nothing classifies the next outcome. There is
  deliberately **no re-wait path in v1** (`lyx shuttle wait <guid>` is not built); the
  operator handles an already-returned `asking` by attaching (`lyx mux attach`) and typing,
  or the caller starts a fresh run. Revisit when review/loom show a concrete need.
- Rationale (operator): there is no way today to stop an agent with updated information —
  you must interrupt, give new info, and let it continue. Key sequences are Claude-specific
  → engine; transport is generic → mux `SendKeys`.
- Rejected: interrupt-only hold (doesn't cover the use case); Go-API-only (operator locked
  out from other terminals).

### Spec surface (consolidated)

- Decision: the run spec is minimal and closed — no `ExtraArgs` escape hatch; extend the
  struct when a real need arrives. Fields:
  - `Prompt string` (required) — buffered to the run's prompt file by the engine.
  - `OutputFiles []string` (required, ≥1) — the result contract; empty spec is rejected.
    Absolute, or relative to the worktree root (resolved to absolute at validation).
  - `Model string` (optional → `--model`).
  - `Autonomous bool` (default true) — controls `--dangerously-skip-permissions` and the
    AskUserQuestion guardrail (see Guardrails decision).
  - `Role, Round string` (optional) — strand-name formatting inputs, passed to `AddSpec`.
  - `Parent string` (optional) — parent strand guid.
  - `Display render.Display` — passed through to mux verbatim.
  - `Timeout time.Duration` (0 ⇒ config `run_timeout_min`).
  - `KeepPane bool` (default false) — when true, `done` skips both `RemoveStrand` and the
    run-dir deletion (debugging aid: inspect the finished session and artifacts). Outcomes
    other than `done` always keep pane + dir regardless.
- Rationale: one visible, documented surface instead of fields scattered across decisions;
  `KeepPane`'s semantics pinned (it modifies only the `done` cleanup path).
- Rejected: `ExtraArgs []string` (launch line becomes an undeclared contract, undermines the
  engine seam).

### Session identity and resume command

- Decision: the engine generates a UUID per run, passes `--session-id <uuid>` at launch,
  hands mux `resumeCmd = & '<claude>' --resume <uuid> --settings '<settings-file>'`, and
  stamps the id into `AddSpec.SessionID`. Deterministic per strand (never `--continue`,
  which grabs the directory's most recent session and misfires with concurrent runs).
- Rejected: `--continue` (ambiguous under concurrency); mux constructing any of this
  (violates the stored-never-constructed resume contract).

### Documentation lifecycle for this landing

- Decision: delete `docs/modules/shuttle.md` and `docs/modules/mux.md`; durable design goes
  in the `shuttleengine`/`claudeengine`/`muxengine` package headers and `overview.md`
  (module table + execution stack). Retarget inbound links (`loom.md`, `review.md`,
  `roadmap.md`, `overview.md`, `CLAUDE.md` if any) to `overview.md#modules` or godoc.
  Roadmap milestone 10 → ✅ Done. The research logs under `docs/research/` stay (they are
  evidence, not module docs).
- Rationale: operator directive — module docs of built modules always rot; mux.md is already
  superseded by code in several places.

## Technical context

- **mux engine API pattern** (what shuttle follows and calls):
  `hubgeometry.Getwd() → hubgeometry.Resolve(cwd) → LoadConfig(layout.Cwd, "<module>") →
  muxengine.New(cfg, layout)` (`internal/muxcli/cli.go:64-91`). All ephemeral state under
  `layout.DotLyxDir()`; never hardcode `.lyx`/`_lyx` (Hub Geometry Invariant).
- **`AddStrand`** (`internal/muxengine/strand.go:305`): takes `AddSpec{Role, Round,
  NameOverride, Parent, Cmd, ResumeCmd, Display}`; engine generates GUID, stamps Worktree,
  launches the pane (adopt-or-split), sends the command via `send-keys -l` + separate
  `Enter` (`spawn.go:110-146`). `Display` uses `render.Display{Anchor, Focus,
  ShrinkWhenWaitingOnChild}`; valid anchors in v1: `top | below-parent | hidden`
  (`own-window` rejected). shuttle passes the caller's Display straight through
  (typical agent: `{anchor: below-parent, focus: true, shrinkWhenWaitingOnChild: true}`).
- **`Strand.SessionID` exists but nothing writes it** (`state.go:25-35` vs `strand.go:24`) —
  the AddSpec extension closes that gap.
- **Send-keys hazards already solved in mux**: `-l` literal mode so `;`/key-names are never
  reinterpreted; leading-`-` text needs the space prefix (`sendKeysLiteralArg`,
  `spawn.go:75-88`). `SendKeys` reuses this.
- **Pane liveness**: `Status()` is read-only (no reconcile, no layout apply) and reports
  live = present ∧ ¬`pane_dead`. `CapturePane` must follow the same read-only discipline.
- **Env hygiene is mux's job and already done**: the psmux server is spawned without
  `CLAUDE_CODE_*` vars (`env.go:CleanClaudeEnv`), so panes launched later inherit a clean
  env; without this, a claude launched from inside a Claude Code session silently stops
  persisting its transcript (breaks `--resume`). shuttle relies on it, adds nothing.
- **Claude launch precedent**: `TestSmokeClaudeResumeRecallsCodeword`
  (`internal/muxcli/smoke_resume_test.go:142`) — pwsh call-operator launch
  (`& '<claude>' '<prompt>'`), trust-dialog dismissal via capture+Enter polling,
  transcript-stability waiting, `deferHubRelease` for the orphaned-conhost teardown hazard.
  The smoke-test helpers there are the starting point for shuttle's smoke layer.
- **Hooks facts** (verified in `docs/research/mux-hooks-exploration.md`): hook commands run
  under git-bash on Windows (POSIX paths only); every payload carries `session_id`,
  `transcript_path`, `cwd`; `Stop` carries `last_assistant_message` + `background_tasks[]`
  and is the immediate idle/needs-input edge; prompts must be injected at launch (paste into
  a live TUI is unreliable); reuse turns are single-line and need Esc first.
- **Claude CLI version note**: hook payloads/flags verified on claude 2.1.177; the engine
  should treat unknown/extra JSON fields leniently (ignore, don't fail).
- **CLI plumbing to reuse**: `internal/clihelp` (Execute/GroupRunE/Abort), `internal/output`
  (Ok/Err JSON envelope), `internal/configengine` + `internal/configreg` (template
  registration), `internal/lyxtest` (SeedConfig, paired fixtures — leaf invariant: shuttle
  tests seed config via `lyxtest.SeedConfig` with `shuttleengine.ConfigTemplate()` at the
  call site).
- **Registration ripples**: `cmd/lyx/main.go` `newRoot()` (+ root `Long` module list),
  pinned sets in `drift_test.go`/`helptree_test.go`/`registration_test.go`/
  `longlist_test.go`, sandbox coverage guard (`sandbox_coverage_test.go`).

## Constraints

From `CONSTRAINTS.md` (all apply; the file must be read before implementing/reviewing):

- **Hub Geometry Invariant** — all cwd/geometry via `hubgeometry`; no geometry tokens in
  path construction; config paths via `ConfigDir`/`ConfigFile` (in tests too). shuttle's
  run dir resolves through `layout.DotLyxDir()` when `run_dir` is unset.
- **lyxtest Leaf Invariant** — lyxtest imports nothing from shuttle; tests seed shuttle
  config at the test site.
- **CLI / Cobra Invariant** — `Command()`/`RunCLI` seam, `Short` on every command, JSON
  envelope for results/errors (no envelope exception needed: `run` blocks but prints JSON at
  the end — it is not a terminal handover), `GroupRunE` on the parent, registration + pinned
  help-tree test updates in the same commit. Package naming `shuttlecli`/`shuttleengine`.
- **Sandbox Suite Coverage** — a scenario tagged `**Covers:** shuttle` in a
  `tools/sandbox/*SUITE.md` file (new `SANDBOX-SHUTTLE-SUITE.md` or a section in the mux
  suite — plan decides), or an explicit allowlist entry. A scenario consumes a real
  subscription claude session; that is accepted (the mux suite already does).
- **Documentation Lifecycle** — design docs deleted on landing (see the decision above);
  module doc/overview/roadmap updates in the same commit as the behaviour.
- New cross-cutting invariants discovered during implementation go into `CONSTRAINTS.md` in
  the same commit (candidate: "provider specifics live only under `claudeengine`; shuttle
  core and mux stay provider-invariant" — plan/implementation judges whether to enshrine).

## Testing

Three layers plus process hardening:

1. **Hermetic unit tests** (run on every `go test`):
   - claudeengine pure functions: launch/resume command composition (quoting, model flag,
     autonomous flag, session id), settings.json content (Stop hook wiring, guardrail
     toggles on/off from config), Windows→POSIX path conversion for hook commands,
     Stop-payload interpretation (done/asking off `last_assistant_message` + output
     presence), trust-screen and TUI-appeared detection off captured text fixtures.
   - shuttleengine run loop against a **fake mux seam** (small interface over
     AddStrand/RemoveStrand/Status/SendKeys/CapturePane) and a fake engine: outcome
     classification for all four outcomes, event-offset tracking across multiple Stop
     events, startup-probe failure ⇒ fast `died`, timeout ⇒ strand kept, cleanup on `done`,
     orphan sweep, Interrupt/Send sequencing, Send newline rejection.
   - mux additions: AddSpec.SessionID persisted round-trip; SendKeys literal-arg hazard;
     CapturePane read-only discipline (no reconcile/apply) — following existing muxengine
     test patterns.
   - CLI: help-tree/registration/drift/longlist pinned-set updates; envelope-shaped errors.
   - TDD candidates: outcome classification, command composition, settings composition,
     POSIX conversion, event-log offset parsing.
2. **Opt-in smoke tests** (`-tags smoke`, real claude, subscription-consuming; muxcli smoke
   patterns incl. `deferHubRelease`): (a) one full `Run` end-to-end — prompt in, Stop hook
   fires, output file written, outcome `done`, run dir cleaned; (b) guardrail verification —
   agent instructed to use the `Agent` tool, deny fires, run still completes (this closes
   the unprobed deny-and-steer item); (c) Interrupt+Send — interrupt a running agent, send
   updated one-line instruction, agent continues and completes.
3. **Sandbox suite scenario(s)** driving `lyx shuttle run` (and interrupt/send) black-box,
   tagged `**Covers:** shuttle`.

Process: before merge, harden via the hand-executed review loop documented in
`docs/reviews/` (A-review → B-fix, fresh reviewer per round, no self-grading) — the method
that hardened mux over seven rounds. The operator explicitly wants this rigor repeated;
psmux/claude behavioural uncertainty is exactly where it pays.

## Q&A log

- **Q:** CLI or engine-only? **A:** Engine/cli split with registered `lyx shuttle` — standalone drivability and sandbox coverage; mill can use it before loom exists.
- **Q:** Engine seam shape? **A:** Interface in shuttleengine + `claudeengine` sub-package; Claude only for now.
- **Q:** Config ownership? **A:** New `shuttle.yaml` config module; remove mux's unused `claude:` key in the same task; engine keys `claude_*`-prefixed (flat templates).
- **Q:** Where does the Claude session id live? **A:** Extend `muxengine.AddSpec` with `SessionID`; the Strand field already exists as caller-written metadata.
- **Q:** Must prompts be buffered to a file? **A:** Yes — the launch line is typed into an interactive pwsh via send-keys, so multiline/quoted prompts cannot ride inline; shuttle always writes `prompt.md` and uses `(Get-Content -Raw …)`. Operator accepted after the send-keys mechanics were explained.
- **Q:** Won't run files pile up? **A:** Run-scoped dirs; delete on clean completion, keep on failure for diagnosis, sweep orphans later. Files must outlive launch (resume/respawn reference them). Run-dir *location* is a config key (`run_dir`), default under `.lyx/` via hubgeometry.
- **Q:** Stop-hook transport? **A:** Plain `cat >>` append to `events.jsonl` with POSIX path; no hidden lyx verb.
- **Q:** Completion semantics? **A:** Four outcomes (`done`/`asking`/`died`/`timeout`); Stop event + output-file existence; `asking` carries `last_assistant_message`.
- **Q:** Who names output files? **A:** The caller (spec + its own prompt text); shuttle never templates prompts.
- **Q:** Strand at run end? **A:** Remove + clean on `done`; keep on `asking`/`died`/`timeout`; `KeepPane` override.
- **Q:** What are guardrails? **A:** PreToolUse denies on `Agent` (nested work must stay visible) and `AskUserQuestion` (never block on a hidden dialog → surfaces as `asking`). Operator: implement, but config-toggleable ("modulert"), and Claude gets its own tailored config keys.
- **Q:** Trust dialog — real problem? **A:** Operator has not seen it for worktrees (VS Code workspace-trust is a separate mechanism); smoke fixtures do hit claude's own dialog. Keep the cheap engine-side dismissal as defensive robustness ("gjør det du må").
- **Q:** In-agent interrupt in v1? **A:** Yes — operator's core need: stop an agent, give it updated info, let it continue. `Interrupt` + `Send` pair, Go API + CLI verbs; multiline updates via file + one-line pointer.
- **Q:** Testing rigor? **A:** Three layers (hermetic / smoke / sandbox) plus the `docs/reviews/` hand-executed review loop before merge, as with mux.
- **Q:** mux.md authority? **A:** Operator: module docs of built modules always rot — use the mux *code* as the source of truth; delete `docs/modules/mux.md` in this task.
- **Q:** (review r1 gap) Empty `OutputFiles`? **A:** Mandatory, ≥1 — a run without a result file has no defined "done"; reject empty spec loudly. Operator convinced by "the file hand-off is what makes the run return"; no-result runs belong in `lyx mux add`.
- **Q:** (review r1 notes) Orphan-sweep race and send-after-return? **A:** Sweep gets an age guard (never touch dirs younger than the startup window); Interrupt/Send documented as live-run-only, no re-wait path in v1.
- **Q:** (review r2 gap) AskUserQuestion steer contradicted `asking` (write-to-output-file would classify as `done`)? **A:** Resolved two ways: the deny is now **mode-coupled** — allowed when `Autonomous: false` (operator attached; CLI picker works well; the old mill ban was a VS Code-extension artifact), denied when autonomous; and the autonomous steer is "ask as your final message, end the turn WITHOUT writing the result file" ⇒ `asking` with the question in `LastAssistantMessage`.
- **Q:** What about an agent that is HARD stuck in auto mode and must escalate? **A:** `asking` IS the escalation channel and must never be closed: the steer never says "decide yourself" (operator incident: an auto agent forbidden to ask found the review machinery broken and shipped everything unreviewed instead of halting). Caller-side auto-policy for blocker-level asks is loom's later design; shuttle guarantees the stop-and-ask path.
- **Q:** (review r2 notes) mux `claude:` removal ripple; `KeepPane` ambiguity? **A:** Removal includes `Config.Claude` field + config-test assertion; spec surface consolidated into its own decision with `KeepPane` semantics pinned (modifies only the `done` cleanup path).
