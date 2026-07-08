# Discussion: Build perch - the review gate loop

```yaml
task: Build perch - the review gate loop
slug: internal-perch
status: discussing
parent: main
```

## Problem

The spine is `shuttle ✅ → burler ✅ → perch → loom`. burler runs exactly one review+fix
round and deliberately knows nothing about loops, caps, convergence, or progress. Nothing
today can take an artifact and drive burler rounds **until the artifact is actually good** —
that iterative gate is perch, and it is the last spine layer before loom. Design was
authoritative in `docs/modules/perch.md`; this discussion amends parts of it (recorded
below) and is the authoritative source for the plan.

perch (`lyx perch`) is the gate engine: a generic, profile-driven loop that spawns a fresh
burler each round, reads its verdict, and exits `APPROVED` (a clean round with zero
blocking findings on top of the prior round's fixes — loop-until-dry) or `STUCK`
(milestone-gated round caps + a progress/circularity judge) or `PAUSED` (operational,
resumable). One engine serves all text review — discussion, plan, code, ad-hoc — because
the per-profile difference is data (rubric, fasit, gate mode, caps), never code.

## Scope

**In:**

- `internal/perchengine` — the deterministic Go round loop: Burler seam, milestone-capped
  loop, progress-judge + asking-triage (ephemeral shuttle runs), pluggable convergence
  gate (`llm-verdict` / `command` / `both`), round state via `internal/state`, automatic
  resume from on-disk state, pause seam.
- `internal/perchcli` — `lyx perch run` + `lyx perch pause`; registered module (CLI/Cobra
  Invariant + Sandbox Suite Coverage apply). Owns claudeengine wiring (mirrors burlercli)
  and the standalone weft commit at block exit.
- perch config module: `ConfigTemplate()` / `LoadConfig` registered in `internal/configreg`
  (config file `perch.yaml` via `hubgeometry.ConfigFile`).
- A `hubgeometry` accessor for the perch runs dir under `_lyx` (Hub Geometry Invariant —
  perch never builds `_lyx` paths itself).
- Progress-judge and asking-triage prompt templates (embedded, stencil-filled) + strict
  fail-loud parsers for their output files.
- Deterministic loop tests (fake burler, fake judge), opt-in judge smoke test, sandbox
  scenario tagged `**Covers:** perch`.
- Docs: package headers carry the design; `docs/modules/perch.md` is deleted on landing
  per the Documentation Lifecycle; `docs/overview.md` module table + roadmap milestone
  updated; `CONSTRAINTS.md` pinned test sets updated (helptree/registration/longlist,
  sandbox coverage).

**Out:**

- loom (the phase machine) — perch only exposes the engine API loom will call.
- Cluster fan-out (`cluster-n > 0`) — burler rejects it (gated on mux own-window
  anchoring); perch passes the profile value through and inherits the rejection.
- Session-level resume of a crashed mid-round burler (`claude --resume` re-entry) — the
  round is the atom in v1; `ResumeCmd` is already persisted by shuttle, so this is a
  later shuttle-surface addition, not perch scope.
- Per-rung model/effort escalation schedules — v1 applies uniform `model`/`effort` from
  the profile to every round.
- LLM-triage of died/timeout outcomes — v1 handles those deterministically; triage over
  `events.jsonl` is a later addition behind the same judge-model config.
- Non-Claude engines — provider-agnosticism rides shuttle's existing seam; nothing
  perch-side is provider-specific (judge model is an opaque string).
- Hardener, and any behavior-driving review beyond the light `command` gate.

## Decisions

### K-milestone ladder replaces scalar K + K_max

- Decision: the round cap is an array of strictly increasing ints, `round-caps: [5, 8, 10]`
  (config vocabulary; Go field `RoundCaps []int`). Intermediate entries are **milestone
  rungs**: reaching rung r with the block still BLOCKING triggers a mandatory continuation
  gate (see judge decision). The final entry is the **hard cap**: reaching it still
  BLOCKING → `STUCK` (reason `hard-cap`), no evaluation, unconditional. A one-element
  array degenerates to the plain hard cap. Built-in default `[5, 8, 10]`.
- Rationale: the design doc's "cap escalation with ceiling K_max" pre-declared as data —
  the operator writes the escalation ladder instead of perch inventing raises. Guaranteed
  termination is preserved by the last element.
- Rejected: separate `K` + `K_max` keys (two keys describing one ladder); v1 hard-cap-only
  with inert milestones (the ladder logic is deterministic Go, cheap to ship now).

### Full milestone behavior ships in v1

- Decision: array, milestone gates, judge, early-stuck all ship now. This supersedes the
  design doc's "v1 may ship the plain hard cap; cap-review is a v-later refinement".
- Rationale: the loop logic is deterministic Go tested with fake burler + fake judge; the
  only LLM surface is the judge prompt, which costs the same whenever it ships.

### Verdict-judge model (amends the design doc's key-canonicalization split)

- Decision: drop finding-key canonicalization and Go cycle detection entirely. The
  progress judge is an ephemeral LLM (default Haiku) spawned via shuttle that **reads the
  prior rounds' review files** (already on disk in the run dir) and writes a verdict file:
  strict YAML frontmatter (verdict + rationale citing concrete finding evidence) over
  unconstrained prose. The prose carries a human-facing **themes overview** — what kinds
  of findings keep appearing — so an operator can eyeball overlap across rounds. Parsing
  is fail-loud like `burlerengine.ParseReview`.
- Two framings, same judge, same template with a mode value:
  - **Per-round circling check** — after every BLOCKING round that has a prior round
    (never round 1, never after APPROVED): verdict `PROGRESSING | CIRCLING | UNCERTAIN`.
    `CIRCLING` (which the rubric requires clear evidence for) → `STUCK` immediately
    (reason `circling`), any round. `PROGRESSING`/`UNCERTAIN` → continue.
  - **Milestone continuation gate** — at a non-final rung still BLOCKING: verdict
    `CONTINUE | STOP | UNCERTAIN` ("round 5 of soft cap — does the trajectory justify
    continuing?"). `STOP` → `STUCK` (reason `milestone-stop`). `UNCERTAIN` → continue.
    The milestone gate replaces (not stacks on) that round's circling check.
- Fail-safe: `UNCERTAIN` or any judge infrastructure failure (spawn error, non-done
  outcome, unparseable verdict file) → treat as progressing, `logger.Warn` with round,
  rung, and cause, continue. The hard cap bounds the damage. A judge failure is never a
  perch error and never `STUCK`.
- Rationale: the doc's key machinery (key-file contract, key stability across separate
  judge calls, strict parser, Go recurrence code) bought a deterministic stuck decision at
  real complexity cost, while the hard cap already bounds the damage of a wrong LLM
  verdict. Loop tests stay fully deterministic either way because tests fake the judge
  with scripted verdicts. Economically, a Haiku judge call is negligible next to a full
  burler round, so per-round judging is cheap insurance against wasted burler rounds.
- Rejected: key canonicalization + Go cycle detection (over-engineered for the value);
  milestone-only judging (a perfect A/B oscillation burns up to a full rung window of
  expensive burler rounds); deterministic-only milestone gate (blind to reworded
  same-issue findings).

### Judge/triage model is config with profile override

- Decision: `perch.yaml` keys `judge-model` (default `haiku`) + `judge-effort` (default
  empty = provider default). Resolution order: **profile > perch.yaml > built-in
  default**. The same model pair serves all of perch's ephemeral utility calls (progress
  judge, asking-triage). The value is an opaque string passed to `shuttleengine.Spec.Model`
  — provider vocabulary; when a Gemini engine exists behind shuttle the same key holds
  e.g. `flash` with no perch change. How model families will eventually be described is
  deliberately unsettled; do not build vocabulary around it.
- Rationale: user requirement — everything configurable, and phase profiles
  (discussion-reviewer, plan-reviewer, …) must be able to override engine defaults.

### round-caps is per-profile, perch.yaml holds only the default

- Decision: `round-caps` is a profile key. `perch.yaml` holds the engine-wide default;
  built-in default `[5, 8, 10]` when neither sets it. Per-phase ladders (discussion
  `[3,5]`, code `[5,8,10]`, …) will live in loom's per-phase config and arrive via the
  profile — perch never knows phase names.
- Rationale: the recurring config rule — phase knowledge collects in loom; perch.yaml is
  engine-general rails only.

### Non-done burler outcomes: deterministic for died/timeout, LLM-triage for asking

- Decision:
  - `died` / `timeout`: retry the round once with a fresh burler (same hydration; round
    number NOT advanced — no review was produced). A second consecutive non-done →
    perch returns an **error** (not `STUCK`), its message carrying run-id, `SessionID`,
    and the kept shuttle run-dir path (shuttle deliberately keeps died/timeout run dirs
    and strands for inspection).
  - `asking`: spawn an ephemeral triage call (judge-model config) that reads
    `LastAssistantMessage` and returns `RETRY | GIVE_UP` + reason (strict file contract
    like the judge). `RETRY` → retry once (then the second-consecutive rule applies).
    `GIVE_UP` → error surfacing the agent's question. Triage infrastructure failure →
    fail-safe `RETRY`.
- Rationale: `stuck` means "the artifact won't converge" (semantic); a dead pane is
  machinery failure — loom must escalate the two differently. Deaths are nearly always
  environmental, so the first response is a cheap deterministic retry; `asking` carries
  text worth interpreting before burning a retry on a structurally-broken profile.
- Rejected: all-deterministic (operator sees the agent's question only after two burns);
  triage for died/timeout too (no text to interpret; revisit when real death patterns
  emerge).

### Result contract: three-way outcome + stuck reason

- Decision: `perchengine` returns `Result{Outcome: APPROVED | STUCK | PAUSED, StuckReason:
  "" | hard-cap | milestone-stop | circling, RoundsRun, per-round summaries (round number,
  verdict, blocking count, review/fixer/judge/gate artifact paths)}` plus `error` for hard
  failures (invalid profile/config, double non-done, triage GIVE_UP). `PAUSED` is an
  operational exit — resumable, not judged; loom treats it entirely differently from
  `STUCK`.
- Rationale: the doc's "two exits" holds for the *judgment* (`APPROVED | STUCK`); pause
  needed a home that is neither a verdict nor an error.
- Rejected: modeling pause as an error value (a pause is not a failure).

### Run identity derived, continuation automatic from state

- Decision: `perchengine.Run` operates on a **run dir**. Unfinished `state.json` there →
  continue at the recorded round; empty dir → fresh block; terminal state → error ("this
  block already finished"). The engine is re-entrant — exactly the contract `lyx loom run`
  needs (loom knows the block identity and calls Run on that block's dir). Standalone:
  default run-id = stable slug derived from the profile (path + content hash);
  `--run-id <name>` overrides for parallel or deliberately-fresh runs.
- Rationale: user requirement — perch must figure out where it stopped from what is on
  disk (delivered reviews, state), same model loom will use.
- Rejected: mandatory `--run-id` flag (typo forks a block); fuzzy profile-matching
  auto-resume without a stable identity.

### Crash mid-round: round-level restart

- Decision: the round is the atom. On resume, a round that started but never reached done
  is re-run from scratch with a fresh burler (round number not advanced). Stale partial
  output files are moved aside first (shuttle rejects pre-existing output files). Safe by
  construction: source-scope fixes are committed per-fix so partial B-work survives in
  git, and the fresh round's A reviews the tree as it now stands.
- Rationale: session-level resume (`claude --resume` re-entry into the wait loop) is a
  missing shuttle surface, not perch scope; `ResumeCmd` is already persisted so it stays
  cheap to add later. Cost of restart: one partial round's tokens.

### Pluggable gate: argv command + feed-forward

- Decision: profile declares `gate: {mode: llm-verdict | command | both, command: [argv]}`.
  `llm-verdict`: clean = fresh burler A returns `APPROVED`. `command`: after each round's
  B completes, Go itself runs the argv (no shell — portable, quoting-safe) with cwd =
  worktree root; exit 0 = clean; the burler verdict does not decide convergence in this
  mode (its review still drives B's fixes). `both`: burler `APPROVED` AND exit 0. On
  command failure, perch writes stdout/stderr to `round-N-gate.md` in the run dir and
  includes it in the next round's burler hydration (alongside prior reviews) so the next
  round starts knowing exactly what failed. The command gets a timeout (profile key
  `gate.timeout`, sensible default, e.g. 10m) so a hung command cannot hang the loop.
  Mode validation fail-loud: `command`/`both` with an empty argv is a profile error;
  `llm-verdict` with a non-empty argv is a profile error.
- Rationale: the decider does not trust the worker — the command runs in perch, never
  inside the burler's A. Feed-forward stops the next round wasting attention
  rediscovering a known failure.
- Rejected: shell-string commands (which shell on Windows?); no feed-forward; running the
  command before B too (more signal, more moving parts — later if needed).

### Profile: one YAML file, burler content + perch keys

- Decision: `lyx perch run --profile p.yaml` takes ONE file: the burler content keys
  (`target`, `fasit`, `rubric`, `fix-scope`, `tool-use`, `cluster-n` — same kebab-case
  vocabulary as `lyx burler run`'s profile) plus perch keys (`gate`, `round-caps`,
  optional `judge-model`/`judge-effort`, optional `model`/`effort` for the burler rounds).
  Strict decode (`KnownFields(true)`, like burlercli). Perch constructs each round's
  `burlerengine.Profile` from it, adding the loop-owned fields itself: per-round
  `ReviewPath`/`FixerReportPath` (numbered, in the run dir) and
  `PriorReviews`/`PriorFixerReports` hydration (all prior rounds' artifacts + any
  `round-N-gate.md`). The operator never writes those. Loom later supplies the same data
  as a Go struct with no file.
- Rationale: a gate block is one thing, described in one place.
- Rejected: two files (always needed together, meaningless apart).

### Run-tuning v1: uniform per profile

- Decision: profile `model`/`effort` map onto every round's `burlerengine.RunOpts`
  (plus `RunOpts.Round` = the round number, for the strand display name). CLI flags
  `--model`/`--effort`/`--timeout` override the profile (burlercli precedent). Per-rung
  escalation schedules are a later refinement with an obvious insertion point.

### Weft commit: once at block exit, owned by the CLI

- Decision: `perchengine` stays weft-blind (like burler). `perchcli` — the standalone
  loop owner — calls `weftengine.Commit` on the run dir ONCE when the block exits
  (`APPROVED`/`STUCK`/`PAUSED`), message identifying run-id + outcome. Composed under
  loom, loom's weft-sync owns the commit instead. Never raw git, never an agent (Weft
  Git Invariant).
- Rationale: task body pins "at block boundaries"; crash-safety does not need more —
  artifacts live in the weft working tree the whole time, so resume reads them
  uncommitted.
- Rejected: per-round commits (finer history, deviates from the pinned wording).

### Pause: callback seam + flag file + pause verb

- Decision: `perchengine` takes a `PauseRequested func() bool`, checked **between rounds
  only** (the round is the atom — never mid-round, never mid-aggregation). True → exit
  `PAUSED`. Standalone: `lyx perch pause --run-id <id>` writes a flag file in the run
  dir; the CLI wires the seam to check it. Loom later wires its own status-file check
  into the same seam. Resume = re-run `lyx perch run` with the same profile (identity
  derivation finds the state; the flag file is cleared on resume).
- Rejected: no pause in v1 (undiscoverable hand-made flag files); mid-round interrupts
  (loom's design: boundary pause, nothing killed).

### Command tree v1: run + pause

- Decision: `lyx perch run --profile <f> [--run-id <id>] [--model] [--effort] [--timeout]`
  and `lyx perch pause --run-id <id>`. Inspection = reading the run dir files. Full
  CLI/Cobra Invariant applies (see Constraints).

### Run dir layout and state

- Decision: run dirs live under a perch area in `_lyx` (weft-side, reached via the
  junction), resolved through a NEW `hubgeometry` accessor (e.g. perch-runs dir under
  the existing `_lyx` ownership) — perch never joins `_lyx` itself. Layout:
  `<perch-runs>/<run-id>/` containing `state.json` (+ `state.json.lock` via
  `internal/state`'s caller-supplied lock path), `round-N-review.md`,
  `round-N-fixer-report.md`, `round-N-judge.md` (when the judge ran), `round-N-gate.md`
  (when the command gate failed), `round-N-triage.md` (when asking-triage ran), and the
  `pause` flag file. `state.json` records: profile identity (hash), resolved
  `round-caps`, per-round entries (round number, outcome, verdict, blocking count,
  retry count, judge verdict, gate result, artifact paths), and the terminal outcome
  when finished. Retried rounds use suffixed artifact paths (shuttle rejects
  pre-existing output files).
- Rationale: Hub Geometry Invariant; `internal/state` is the pinned persistence
  mechanism; everything needed for resume and for the judge's reading is in one dir.

## Technical context

- **Burler seam:** `internal/burlerengine.Engine.Run(Profile, RunOpts) (Result, error)`
  is the whole surface — perch's seam is `type Burler interface { Run(burlerengine.Profile,
  burlerengine.RunOpts) (burlerengine.Result, error) }`, satisfied by `*burlerengine.Engine`
  as-is (compile-time proof like burlerengine's own `var _ Shuttle = ...`), faked in tests
  with scripted Results. `Result` carries `Outcome` (done/asking/died/timeout),
  `Verdict`/`Findings` (only on done), paths, `SessionID`, `LastAssistantMessage`.
- **Round validity:** `burlerengine.ParseReview` already enforces verdict/findings
  consistency fail-loud; a done round with an invalid review file comes back as an error
  from burler — perch treats it as a hard error (it is a reviewer-agent defect, not a
  loop event).
- **Judge/triage transport:** perch composes `shuttleengine.Spec{Prompt, OutputFiles:
  [verdict file], Model: judge-model, Effort: judge-effort, Role: "judge"/"triage",
  Round: N}` and drives it through its own package-local Shuttle seam (mirroring
  burlerengine's), satisfied by `*shuttleengine.Runner`, faked in tests. Spec's
  OutputFiles must not pre-exist. Prompt templates embedded via `go:embed` +
  `internal/stencil.Fill` (burlerengine/template.go is the pattern).
- **Wiring (perchcli):** copy burlercli's `PersistentPreRunE` chain — `hubgeometry.Getwd`
  → `Resolve` → `shuttleengine.LoadConfig(layout.Cwd, "shuttle")` +
  `muxengine.LoadConfig` → `muxengine.New` → `shuttleengine.NewRunner(muxEngine,
  claudeengine.New(), layout, shuttleCfg)` → `burlerengine.New(runner, layout)` →
  `perchengine.New(burlerEngine, runner, perchCfg, layout)`; skip resolution for the bare
  group command. perchcli is the module's claudeengine wiring point (Provider-Seam
  Invariant).
- **Config module:** follow weftengine/shuttleengine's `ConfigTemplate()` + `LoadConfig`
  pattern; register in `internal/configreg.Modules()` (alphabetical: board, mux, perch,
  shuttle, warp, weft). Config file resolves via `hubgeometry.ConfigFile(base, "perch")`.
  perch.yaml keys: `judge-model`, `judge-effort`, `round-caps` (default ladder). No
  `cluster-n` key in v1 (burler rejects >0 anyway; add when clusters land).
- **State:** `internal/state.WriteJSON/ReadJSON[T]` with caller-supplied lock path —
  generic, already atomic + locked.
- **Weft:** `weftengine.Commit(weftPath, pathspec, message, opts)` +
  `weftengine.EnvSyncOptions()`; `ScopedPathspec` exists for scoping.
- **Logger:** `internal/logger` (slog wrapper: Debug/Info/Warn, SetVerbosity) — judge
  fail-safe continues log via `logger.Warn`.
- **JSON output:** all CLI results/errors via `internal/output` (`output.Ok`/`output.Err`)
  + `clihelp.SetExit`/`ShouldAbort`/`GroupRunE` — burlercli/run.go is the exact pattern,
  including manual required-flag validation (not `MarkFlagRequired`).
- **Stale doc note:** `burlerengine` comments call Finding IDs "perch's future
  cycle-detection keys" — superseded by the verdict-judge decision; correct those
  comments in this task (verdict.go, doc.go) since perch is the referent.
- **docs/modules/perch.md** is deleted when the module lands (Documentation Lifecycle);
  its durable parts move to the perchengine/perchcli package headers and
  `docs/overview.md` (module table + execution stack). Roadmap: mark the perch milestone
  ✅ Done. The design amendments (verdict-judge, K-ladder) need no doc edit beyond that —
  the package header is written fresh from this discussion.

## Constraints

From `CONSTRAINTS.md`, all apply:

- **Hub Geometry Invariant** — the run-dir base under `_lyx` goes through a new
  `hubgeometry` accessor; no geometry tokens in perch code or tests
  (`TestEnforcement_GeometryLiterals` enforces).
- **lyxtest Leaf Invariant** — if tests need config seeding, use `lyxtest.SeedConfig`.
- **CLI / Cobra Invariant** — `Command()`/`RunCLI(out, args)` seam via `clihelp.Execute`;
  `Short` on every command (parent + `run` + `pause`), `Long` with concrete examples;
  register in `cmd/lyx/main.go` `newRoot()` (import, AddCommand, root `Long` module
  list); update pinned sets in `helptree_test.go`, `registration_test.go`,
  `longlist_test.go` in the same commit; JSON envelope errors; `RunE = clihelp.GroupRunE`
  on the parent; package naming `perchcli`/`perchengine` (cli imports engine; engine
  never imports cobra).
- **Shuttle Provider-Seam Invariant** — perchengine references no Claude specifics;
  claudeengine appears only in perchcli's wiring.
- **Weft Git Invariant** — weft commit only via `weftengine`, only from the CLI loop
  owner, never from the engine, never an agent.
- **Review Round Invariant** — perch preserves it structurally: fresh burler per round,
  round N's fix judged by round N+1's fresh A, never self-graded.
- **Sandbox Suite Coverage** — a `lyx perch` scenario in a `tools/sandbox/*SUITE.md`
  tagged `**Covers:** perch`, or the coverage test fails.
- **Documentation Lifecycle** — see Technical context.

## Testing

perch's strong test surface is deterministic Go — everything below the LLM seams:

- **Fake burler + fake judge loop tests (TDD candidates, `internal/perchengine`):**
  - loop-until-dry: BLOCKING, BLOCKING, APPROVED → `APPROVED` after 3 rounds; hydration
    paths accumulate correctly per round.
  - hard cap: still BLOCKING at final rung → `STUCK`/`hard-cap`, no judge call at the
    final rung.
  - milestone gate: judge `CONTINUE` at rung → loop proceeds past it; `STOP` →
    `STUCK`/`milestone-stop`; `UNCERTAIN` → continues + Warn logged; milestone replaces
    that round's circling check.
  - per-round circling: judge `CIRCLING` mid-window → `STUCK`/`circling` immediately;
    judge never spawned on round 1 or after APPROVED.
  - judge fail-safe: judge spawn error / non-done / unparseable verdict file → continue +
    Warn, never an error, never STUCK.
  - round-caps validation: non-increasing, empty, non-positive entries → fail-loud
    profile error; one-element array = plain hard cap; default resolution chain
    (profile > perch.yaml > `[5,8,10]`).
  - gate modes: `llm-verdict`; `command` (fake command runner: burler APPROVED but
    command fails → loop continues and `round-N-gate.md` is hydrated forward; command
    passes → clean even with a BLOCKING verdict — convergence is the command result,
    though the review must still parse); `both` (each alone insufficient);
    argv/mode mismatch validation; command timeout.
  - non-done outcomes: died → retry once, round not advanced, suffixed artifact paths;
    second consecutive → error carrying SessionID/run-dir; asking → triage `RETRY` /
    `GIVE_UP` branches; triage failure → fail-safe RETRY.
  - resume: state.json mid-block → continues at recorded round; terminal state → error;
    partial round artifacts moved aside; fresh dir → round 1.
  - pause: flag between rounds → `PAUSED` at the next boundary, never mid-round; resume
    clears the flag and continues.
- **Parsers (table-driven, fail-loud):** judge verdict file (both framings' vocabularies,
  case-sensitivity, missing rationale, empty/garbled frontmatter), triage file, profile
  YAML strict decode (unknown keys, gate/argv mismatches).
- **perchcli tests:** mirror burlercli/cli_test.go — help tree, missing `--profile`,
  JSON envelope on every error path, pause verb writes the flag file. Root-level pinned
  sets (helptree/registration/longlist/sandbox-coverage/drift) updated.
- **Judge smoke test (opt-in, the only LLM touch):** real Haiku judge over two small
  fixture reviews, asserts a parseable verdict file — same opt-in pattern as
  burlerengine's `smoke_round_test.go`.
- **Sandbox scenario:** operator-run llm-verdict block over a deliberately-flawed short
  fixture doc: watch convergence to `APPROVED`, run-dir contents, weft commit; plus a
  pause mid-block + re-run continuation step. Tagged `**Covers:** perch`.

## Q&A log

- **Q:** Cap parameter K — shape? **A:** An array of milestones, e.g. `[5, 8, 10]`:
  extra scrutiny at each rung, last element is the hard cap (user's opening requirement).
- **Q:** What runs at a milestone? **A:** Judge-gated continuation (the doc's cap-review
  made concrete by the pre-declared ladder).
- **Q:** Does round-caps apply globally? **A:** No — per-profile; each gated thing
  (discussion, plan, code) gets its own ladder, typically from loom's config;
  perch.yaml holds only the default.
- **Q:** Stuck detection between milestones too? **A:** Yes — per-round check, early
  `STUCK` on clear circularity.
- **Q:** v1 scope of the ladder? **A:** Full behavior in v1; supersedes the doc's
  "plain hard cap in v1".
- **Q:** Keys + Go cycle detection, or holistic judge? **A:** Holistic verdict judge —
  user challenged the key machinery's utility ("a Haiku that reads all prior reviews and
  decides is quick"); keys dropped.
- **Q:** Judge verdict extras? **A:** A human-facing themes/keywords overview in the
  verdict file prose — nice-to-have for operator overlap-spotting.
- **Q:** Is a spawned Haiku OK as judge, and is the model fixed? **A:** Yes via shuttle;
  model is config (`judge-model`), overridable per profile, opaque string (model-family
  vocabulary deliberately unsettled — could be Gemini Flash once a Gemini shuttle
  engine exists).
- **Q:** Judge fail-safe direction? **A:** Uncertain/failure → continue, but it MUST be
  logged (`internal/logger`).
- **Q:** Non-done outcomes — how to know WHY a pane died? **A:** v1 doesn't diagnose:
  retry once, then error surfacing SessionID + kept run-dir; shuttle keeps died/timeout
  state for inspection. LLM-triage over events.jsonl is v-later. `asking` DOES get LLM
  triage in v1 (same model config as judge) since it carries interpretable text.
- **Q:** Resume UX? **A:** "You figure it out — as long as it works": continuation
  automatic from on-disk state; identity = derived run-id with `--run-id` override;
  loom will drive the same re-entrant contract.
- **Q:** Mid-round crash — `claude --resume`? **A:** Round-level restart in v1;
  session-level resume is possible later (ResumeCmd already persisted) but out of scope.
- **Q:** Command-gate failure visibility? **A:** Feed forward: `round-N-gate.md`
  hydrated into the next round.
- **Q:** Profile file shape? **A:** One YAML (burler content + perch keys).
- **Q:** perch.yaml a real config module? **A:** Yes — defaults live there, profiles
  override.
- **Q:** Weft commit timing? **A:** Once at block exit, from the CLI, per the task
  body's "block boundaries".
- **Q:** Per-round model escalation? **A:** v1 uniform; later refinement.
- **Q:** CLI verbs? **A:** `run` + `pause`; "husk også Cobra CLI" — full CLI/Cobra
  Invariant checklist explicitly in scope.
- **Q:** Default ladder? **A:** `[5, 8, 10]`.
- **Q:** Sandbox scenario? **A:** Small llm-verdict block with pause/resume step.
