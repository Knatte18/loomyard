# Discussion: Build builder - the batch-implementation loop

```yaml
task: Build builder - the batch-implementation loop
slug: internal-builder
status: discussing
parent: main
```

## Problem

Loom's Builder phase — the loop that takes a pinned plan and drives implementers through
it, batch by batch, until the plan is built — has no module. The plan format it consumes
(plan-format v1) and the model-spec notation its config resolves (`internal/modelspec`)
are already pinned and merged; the design of the loop itself is written across
`docs/modules/loom.md` (Builder section), `docs/modules/plan-format.md`, and
`docs/reference/model-spec.md`. This task builds `internal/builderengine` +
`internal/buildercli`: the fat `lyx builder` verbs, the report distillation behind them,
and the orchestrator/implementer prompt templates. It is the next spine milestone —
builder branches off `shuttle` (it spawns implementers directly) and does not need
`perch`; `loom` comes last and needs both.

**Why now:** modelspec merged 2026-07-11 (the carved-out dependency); plan-format v1 is
pinned so builder can be built and tested against a hand-written plan fixture before any
Planner exists.

## Scope

**In:**

- `internal/builderengine` — the domain kernel: plan parsing + the six validation checks,
  batch/run state (`state.json`), spawn/poll/distillation logic, digest computation,
  chain rollback, pause flag, weft commits of builder artifacts, embedded stencil prompt
  templates (orchestrator + implementer).
- `internal/buildercli` — the cobra subtree: `lyx builder run | spawn-batch | poll |
  status | validate | pause`, registered in `cmd/lyx/main.go` per the CLI/Cobra
  Invariant.
- New `internal/hubgeometry` helpers: `PlanDir`, `BuilderDir`, `BuilderReportsDir`
  (mirroring `PerchRunsDir`).
- `builder.yaml` config (via configreg, template.yaml pattern): four model-spec roles +
  numeric knobs (see Decisions).
- A hand-written plan fixture under `testdata/` and a sandbox scenario
  (**Covers:** builder).
- Doc updates in the same commit: loom.md module table + Builder-section correction
  (holistic review is the gate's job, not the orchestrator's — see Decisions),
  overview.md module table, model-spec.md + plan-format.md role rename `fixer` →
  `recovery`, new module doc `docs/modules/` entry per the Documentation Lifecycle,
  roadmap milestone flip.

**Out:**

- `modelspec` — consumed as a dependency (`Parse`/`LoadRegistry`/`Resolve`), not built
  or modified here.
- The Planner — no plan-producing agent exists; builder is tested against a fixture.
- The terminal holistic builder-review — that is the Builder-review **gate** (perch),
  driven by loom or the operator running `lyx perch run`. `builder run` ends at
  batches-built.
- `loom` itself — builder is standalone-runnable; loom later drives the same
  `builder run` seam.
- Non-Claude engines, intra-plan parallelism (no DAG), per-batch design review, a
  mid-batch kill verb, cluster/window scaling.
- Skills — lyx has no skill layer yet; prompts are embedded templates (see Decisions).

## Decisions

### LLM orchestrator over Go verbs (pinned upstream — not re-opened)

- Decision: a long-lived orchestrator session (model config-chosen, Sonnet default)
  holds the batch loop and drives fat `lyx builder` verbs; Go provides only the verbs +
  distillation, never the loop. Batches are a strictly ordered list (no DAG), sized to
  implementer context. The orchestrator reads only distilled digests, never raw session
  prose. Recovery is orchestrator judgment. These were settled in the proposal and the
  authoritative docs; the discussion treated them as fixed.
- Rationale: mill-go's context bloat came from an LLM orchestrator swallowing verbose
  sub-agent output, not from the loop being LLM-held; a pure-Go loop is dumb at spotting
  an in-flight implementer circling.
- Rejected: pure-Go batch loop; raw-report ingestion; DAG.

### Verb surface

- Decision: `lyx builder` has six subcommands. `run` — the product verb: validates the
  plan, ensures the mux session, spawns the orchestrator via shuttle (stencil-filled
  prompt, OutputFiles = the outcome file), blocks until done/stuck/paused, backstop
  weft-commit at exit. `spawn-batch <NN>` — validates the plan (gate), checks the pause
  flag, records the batch start-SHA in `state.json`, spawns one implementer via shuttle.
  `poll` — long-poll for the in-flight batch's terminal state; distills the batch-report
  into the digest; weft-commits report + state on terminal classification. `status` —
  instant snapshot of `state.json` + reports (human- and loom-facing). `validate` — the
  six plan-format checks as a standalone pre-flight. `pause` — writes the pause flag
  file. All results/errors ride the `internal/output` JSON envelope.
- Rationale: run/spawn-batch/poll/status pinned in the proposal; validate and pause per
  Q&A below.
- Rejected: a `kill`/mid-batch interrupt verb (v1 has no actionable use); a
  `rollback-chain` standalone verb (folded into `spawn-batch --restart-chain`).

### Orchestrator and implementer prompts: embedded stencil templates

- Decision: both prompts are `.md` templates embedded (`//go:embed`) in
  `internal/builderengine`, filled via `internal/stencil`, co-versioned with the
  verb/digest parser in the same package.
- Rationale: the orchestrator prompt is one half of a Go-parsed contract — it names the
  exact verbs and reads the exact digest fields `poll` emits; independent versioning
  drifts silently (prompt keys off a field Go no longer produces, no compile error).
  Embedding enforces co-versioning; stencil fails loud on any unfilled marker. Direct
  precedent: perchengine's `judge-*-template.md`, burler's templates. `//go:embed`
  already keeps the prompt an editable file in the source tree — "editable without
  recompiling" buys nothing and costs the coupling. mill-go's length is not a
  counter-argument: it is long because the whole machine lives in prose; here the
  machine lives in the verbs, so the prompt collapses to the judgment core. lyx also has
  no skill layer yet — skills belong to the future ly plugin layer, never inside a core
  engine module.
- Rejected: a skill the session loads (independent versioning = the exact coupling that
  must not break; adds a "session must load the right skill" failure mode).

### Plan validation: standalone verb AND automatic gate

- Decision: the six machine checks from plan-format.md ("Validation checks" section) run
  both as `lyx builder validate` (cheap pre-flight for a Planner/human) and as a hard
  automatic gate inside `run` and `spawn-batch`. Checks: (1) `format` recognized +
  `approved: true`; (2) Batch Index ↔ batch files consistent; (3) per-batch `verify:`
  present or `deferred` with valid `chain-end`; (4) no dangling `chain-end`; (5) context
  estimate (bytes/4 over Scope + Where files) + card-count cap, over-cap without
  `oversized: true` fails; (6) scope paths exist or are creatable (well-formed prefix
  list).
- Rationale: refusal is not optional — the gate makes "fail loud, never misread"
  structural; the verb gives lint-without-run.
- Rejected: gate-only (no lint path); verb-only (trusts callers, violates fail-loud).

### `poll` semantics: long-poll; the long-poll IS the notification

- Decision: `poll --wait <duration>` blocks inside Go watching for the batch-report
  (default wait from `builder.yaml` `poll_wait`, ~8 minutes — safely under the ~10-minute
  tool-call cap). Returns the instant the batch reaches a terminal state; at the deadline
  it returns a `running` snapshot and the orchestrator re-polls.
- Terminal classification (nobody holds the shuttle `Run` handle — `spawn-batch` exits
  after `Start` — so `poll` re-derives cross-process): (1) batch-report present →
  `done`/`stuck` per the report; (2) no report but the implementer's turn has ended
  (Stop event observed in the run dir's `events.jsonl`, path via `run.json`) → `dead`
  with `dead_reason: asking` — an implementer that stopped without fulfilling the file
  contract is respawn/recover material, same as a crash; (3) no report and elapsed since
  spawn > `batch_timeout_min` → `dead` with `dead_reason: timeout`; (4) no report, turn
  in progress, strand pane gone → `dead` with `dead_reason: died`. On any `dead`, the
  pane/run dir is kept for diagnosis (shuttle's died/timeout discipline).
- Rationale: a true push notification cannot reach the orchestrator — it is a Claude
  session that is either mid-turn (only a tool-call result reaches it) or has ended its
  turn, and turn-end without the outcome file classifies the run `asking` under
  shuttle's file contract. Nudging an idle session via `shuttle.Send` fights the
  substrate (Send's documented no-re-wait limitation). Go-side file polling costs no
  tokens; tool calls per batch = ceil(duration/wait), i.e. 1 for most batches. A
  blocking `spawn-batch` was rejected because long batches exceed the tool-call cap.
- Rejected: instant-return-only poll (orchestrator busy-loops tool calls); Send-based
  notification; blocking spawn.

### Digest contract — fixed terse fields, no prose, ever

- Decision: `poll`'s terminal digest is one JSON envelope with exactly: `batch`,
  `status` (`running | done | stuck | dead`), `tests` (`green | red | skipped`),
  `stuck_reason` (one line, verbatim from the report), `out_of_scope` (path + one-line
  why, verbatim), `drift_unreported` (paths only — changed files outside declared scope
  with no `out_of_scope` entry; the rot signal), `files_changed` (count, not list), and
  `dirty` (bool — uncommitted/untracked changes left in the worktree at terminal state,
  a half-done-work signal), and `dead_reason` (`asking | timeout | died` — set only
  when `status: dead`; see the poll terminal-classification rule). A `running` snapshot
  carries only `{batch, status, elapsed}`. The field set is pinned in the module doc as
  the digest contract the orchestrator template co-versions with.
- Rationale: the orchestrator must stay lean (mill-go bloat lesson); the user explicitly
  capped clutter. Count-not-list for `files_changed` because the list scales with batch
  size — the exact bloat vector.
- Rejected: full changed-file list; free-form prose fields.

### Drift computation: terminal classification only

- Decision: the `git diff --name-only <batch-start-SHA>..HEAD` + scope-prefix comparison
  runs once, when the report lands (terminal classification). `running` snapshots never
  compute drift.
- Rationale: drift judgment on a half-done batch is noise; the orchestrator cannot act
  mid-batch anyway (no kill verb in v1).
- Rejected: per-poll computation (unactionable clutter).

### Role selection: Go picks from the batch; orchestrator overrides only for recovery

- Decision: `spawn-batch <NN>` reads the batch's `oversized:` frontmatter and
  auto-selects `implementer` vs `implementer_oversized`; recovery is explicit:
  `spawn-batch <NN> --role recovery`. The orchestrator never names models, only the
  recovery role; model-specs live in `builder.yaml`, resolved via
  `modelspec.LoadRegistry` + `Resolve`, mapped onto `shuttleengine.Spec`
  (Model/Effort/Version) per modelspec's documented consumer mapping.
- Rationale: Go enforcing the oversized flag is correctness-by-tool-design (the LLM
  cannot forget it); recovery timing/judgment stays with the orchestrator as pinned.
- Rejected: orchestrator always passes `--role` (can forget oversized); Go auto-escalates
  to recovery after N stucks (recovery judgment is pinned as the orchestrator's).

### Role rename: `fixer` → `recovery`

- Decision: builder.yaml's four roles are `orchestrator`, `implementer` (Sonnet
  default), `implementer_oversized`, `recovery`. model-spec.md and plan-format.md role
  lists updated in the same commit.
- Rationale: `recovery` says what it is — the model-spec for the fresh escalated
  recovery spawn after a batch reports stuck. `fixer` collides with burler's fixer (the
  B-phase of a review round), a different mechanism one module over; the operator's own
  "what is a fixer?" during discussion is evidence the name doesn't self-explain.
- Rejected: keeping `fixer` (doc-pinned but confusing).

### Builder run state: durable `_lyx/builder/state.json` + hubgeometry helpers

- Decision: run state — run GUID, current batch, per-batch start-SHAs, chain anchors
  (chain-start SHA per chain), in-flight shuttle run identity, pause flag path — lives
  in `_lyx/builder/state.json`. New hubgeometry helpers `PlanDir(baseDir)` →
  `_lyx/plan`, `BuilderDir(baseDir)` → `_lyx/builder`, `BuilderReportsDir(baseDir)` →
  `_lyx/builder/reports`, mirroring `PerchRunsDir`; no other package constructs these
  paths (Hub Geometry Invariant — enforcement test's token ownership applies).
- Rationale: weft-synced like perch's run dirs; survives crash and cross-machine resume;
  chain-start SHAs and in-flight identity are too fragile to reconstruct from git alone.
- Rejected: no state file (fragile reconstruction).

### Weft commits: Go commits at batch boundaries; orchestrator never touches weft git

- Decision: `poll` weft-commits the batch-report + `state.json` via `weftengine` when a
  batch reaches terminal classification; `spawn-batch` commits the updated `state.json`
  (start-SHA recorded) at spawn; `run` does a final backstop commit at exit. The
  orchestrator and implementers only ever write files via the junction (Weft Git
  Invariant); implementers commit their own code to the **host** repo per card
  (`NN.C: <short what>` subjects — the asymmetry).
- Rationale: batch boundary is the natural persist point; "commit when it makes sense"
  (operator). Crash between boundaries loses nothing durable — resume recomputes from
  host git + reports.
- Rejected: run-exit-only commit (crash loses weft-synced progress for cross-machine
  resume).

### Holistic review is out — builder ends at batches-built

- Decision: `builder run` terminates `done` when the last batch is green (or `stuck` /
  `paused`); the holistic builder-review is the Builder-review gate (perch), driven by
  loom or the operator. loom.md's Builder-section sentence "…and runs a holistic
  builder-review at the end" is corrected in the same commit to state the review is the
  gate's job.
- Rationale: keeps the advance (builder) / converge (perch) split clean; keeps an LLM
  agent from driving perch's block-exit weft-committing loop.
- Rejected: orchestrator invokes `lyx perch run` as its final verb.

### Chain rollback: Go performs the reset behind `spawn-batch --restart-chain`

- Decision: `spawn-batch <NN> --restart-chain` verifies NN is a chain member (lowest
  batch whose `chain-end` group matches), resets the host repo to the chain-start SHA
  recorded in `state.json`, clears the chain members' reports, then spawns. The
  orchestrator decides *when* (its pinned recovery judgment); Go performs the
  destructive act with the recorded SHA.
- Rationale: correctness-by-tool-design — no LLM typing `git reset --hard` with a
  hallucinatable SHA. Chain-start SHA = host commit immediately before the
  lowest-numbered member's first card commit, recorded at that member's spawn.
- Rejected: orchestrator runs raw host git (allowed by the invariant's host asymmetry,
  but destructive resets are exactly the class of act to move into a verb).

### Pause: mirror perch's flag-file seam

- Decision: `lyx builder pause` writes a flag file in the builder state dir;
  `spawn-batch` checks it at the batch boundary and refuses with a `paused` envelope;
  the orchestrator sees the refusal, writes its outcome file with `outcome: paused`
  (operational, not judged), and `run` exits cleanly. Resume = re-run
  `lyx builder run`. The flag is cleared at `run` entry (perch's
  never-instantly-re-pause rule) and at terminal outcomes.
- Rationale: loom.md already pins that builder's verbs check the pause flag at the batch
  boundary even though the loop is LLM-held; perch's PauseFlagPath discipline is the
  precedent, including the flag-clearing rules.
- Rejected: no pause in v1 (loom needs the uniform seam; the mechanism is cheap).

### Orchestrator outcome contract: `_lyx/builder/outcome.yaml`

- Decision: the orchestrator's final action is writing `_lyx/builder/outcome.yaml`:
  `outcome: done | stuck | paused`, `stuck_reason` (null or one line), `batches_done`
  (int). `run`'s shuttle Spec lists it as the OutputFile; Go parses it fail-loud
  (burler-verdict discipline: unparseable → hard error, never guessed) and renders
  `run`'s own JSON envelope from it.
- Stale file: shuttle's Spec.validate rejects a pre-existing OutputFile, and resume is
  re-running `lyx builder run` — so `run` **archives** (never refuses) a stale
  `outcome.yaml` before spawning, renaming it to
  `_lyx/builder/outcome-<UTC-compact-timestamp>.yaml` in place. Refusing would block
  the decided crash/resume path; archiving keeps the prior run's judgment auditable.
- Non-done shuttle outcomes: when `run`'s shuttle Result is `asking`, `died`, or
  `timeout`, no outcome.yaml exists — `run` maps each to its own distinct `output.Err`
  envelope (outcome + SessionID + kept run dir; for `asking`, the
  LastAssistantMessage), never entering the outcome-file parse. The fail-loud parse
  error is reserved for a `done` outcome whose file is present but malformed — the two
  failure classes are never conflated.
- Rationale: shuttle needs an output file anyway (the file contract is the only done
  signal); the orchestrator's own judgment ("why I stopped") needs a durable home.
- Rejected: deriving terminal state from state.json + reports alone; refusing on a
  stale outcome.yaml (blocks resume).

### Crash/resume: resume-on-files, always a fresh orchestrator

- Decision: re-running `lyx builder run` cold-starts from `state.json` + reports: batch
  report present → batch done, advance; no report but the implementer's mux strand live
  → the fresh orchestrator simply `poll`s it; dead + no report → the orchestrator
  respawns that batch fresh. The orchestrator session itself is always spawned fresh on
  resume (never `claude --resume`), hydrated by the stencil from on-disk state — the
  prompt template includes current progress so a resumed orchestrator knows where it is.
- Rationale: loom.md's crash-recovery discipline ("loom never depends on `claude
  --resume` for correctness") applied one level down; the round/batch is idempotent.
- Rejected: re-attaching the old orchestrator session.

### builder.yaml keys — everything needed, nothing speculative

- Decision: `builder.yaml` (configreg registration, template.yaml pattern) holds:
  the four role model-specs (`orchestrator`, `implementer`, `implementer_oversized`,
  `recovery`); `self_fix_cap` (default 2, stencil-templated into the implementer
  prompt); `poll_wait` (default ~8m); `batch_timeout_min` (default 60) for
  implementer/recovery spawns; `orchestrator_timeout_min` (default 480) for the `run`
  spawn; `batch_context_cap_tokens` (default 100000) and `batch_card_cap` (default 10)
  for validation check 5. Role strings are validated at load via `modelspec.Parse`
  (fail-loud on grammar) **and all four roles are resolved against the registry
  (`Registry.Resolve`) at `run`/`spawn-batch` entry as a pre-flight** — a well-formed
  but unknown alias (a typo'd role spec) fails before any agent spawns, never hours
  into a run when that role first spawns. Spawn-time resolution reuses the same values.
- Rationale: builder's run units are far longer than review-sized shuttle runs, so it
  sets `Spec.Timeout` explicitly and never inherits shuttle's `run_timeout_min`. Cap
  values are operational-experience tunables. Scope-drift is deliberately
  **unconfigurable**: the digest always flags it, the orchestrator always judges —
  plan-format pins that; a `judge|block` policy knob moves judgment back into a Go
  branch (rejected as YAGNI).
- Rejected: inheriting shuttle timeouts; hardcoded caps; a scope-drift policy key.

## Technical context

- **shuttle** (`internal/shuttleengine`): `NewRunner(mux, engine, layout, cfg)`;
  `Runner.Start(Spec) → *Run` (non-blocking, persists `run.json`) and `Runner.Run(Spec)
  → Result` (Start+Wait). `Spec`: `Prompt`, `OutputFiles` (the file contract — entries
  must NOT pre-exist; validate rejects), `Model`/`Effort`/`Version` (engine-validated
  pass-throughs — modelspec's `Resolved` maps straight on), `Role`/`Round`/`Parent`
  (strand display), `Display`, `Timeout` (0 → cfg default; builder always sets it),
  `KeepPane`. `Result.Outcome`: done/asking/died/timeout. Cross-process resolution via
  `FindRun` + `run.json`; `builder poll` classifies the in-flight implementer from the
  batch-report file + the run dir's `events.jsonl` (Stop-event detection for the
  `asking` branch; `run.json` carries `EventsPath`) + mux strand liveness +
  elapsed-vs-`batch_timeout_min` (loom.md crash-recovery pattern), not by holding the
  in-process `Run` handle — `spawn-batch` exits after `Start`. Whether poll parses
  events directly or reconstructs a handle via `FindRun` and reuses shuttle's
  classification is a plan-level choice.
- **modelspec** (`internal/modelspec`): `Parse(s) → Spec` (grammar only),
  `LoadRegistry(baseDir)` (absent file → built-ins; `hubgeometry.ConfigFile(baseDir,
  "models")`), `Registry.Resolve(spec) → Resolved{Engine, Model, Params}`. Documented
  consumer mapping: `spec.Model = resolved.Model; spec.Effort =
  resolved.Params["effort"]; spec.Version = resolved.Params["version"]`. Leaf — builder
  may import it freely.
- **perch as the sibling pattern** (`internal/perchengine`/`perchcli`): engine/cli
  split, weft-blindness (engine operates on caller-supplied paths; the CLI owns weft
  commits), pause flag file + clearing rules, fail-loud verdict parsing, config
  resolution profile > module.yaml > built-in. Builder mirrors the discipline but note
  the ownership difference: builderengine itself is geometry-aware (it owns
  `_lyx/plan` / `_lyx/builder` via the new hubgeometry helpers) because the paths are
  part of the pinned plan contract, and the verbs (not a long-lived Go loop owner)
  perform the weft commits.
- **stencil** (`internal/stencil`): marker-filling with fail-loud on unfilled markers.
  perchengine's `judge-*-template.md` + burlerengine's templates show the
  embed+fill+test pattern (`template_test.go` pins template properties — e.g. burler's
  `TestTemplate_StatesRoundDiscipline`).
- **hubgeometry**: `ConfigFile/ConfigDir`, `PerchRunsDir` (the helper pattern to
  mirror), `Layout` (WorktreeRoot etc.). The geometry-literal enforcement test will fail
  any `_lyx`-token construction outside hubgeometry — the new helpers must land there.
- **weftengine**: `Sync`/`Commit` for the builder-artifact commits (poll/spawn-batch/run
  boundaries).
- **output** (`internal/output`): `Ok`/`Err` envelope — every verb result and error,
  one JSON object per line. Parent group sets `RunE = clihelp.GroupRunE`.
- **git mechanics**: batch start-SHA = host `HEAD` at spawn-batch time;
  `files_changed`/drift = `git diff --name-only <start-SHA>..HEAD` (committed work only
  — implementers commit per card); `dirty` = non-empty `git status --porcelain`. Host
  git runs via `internal/gitexec`.
- **Plan fixture**: hand-written under `internal/builderengine/testdata/`, following
  plan-format.md's worked example (00-overview.md + NN-batch files + reports), including
  an oversized batch and a deferred-verify chain for validation/rollback coverage.
- **cmd/lyx pinned sets**: `drift_test.go`, `helptree_test.go`, `registration_test.go`,
  `longlist_test.go`, `sandbox_coverage_test.go` all need updating in the registration
  commit.

## Constraints

From `CONSTRAINTS.md`, all binding on this task:

- **Hub Geometry Invariant** — `_lyx/plan` / `_lyx/builder` paths only via new
  hubgeometry helpers; the token-enforcement test machine-checks this. Config via
  `ConfigFile(base, "builder")`.
- **Modelspec Leaf Invariant** — builder imports modelspec; never the reverse.
- **CLI / Cobra Invariant** — `buildercli` Command()/RunCLI seam, `Short` everywhere,
  JSON envelope for all results/errors, registration + pinned-set test updates in the
  same commit. Package naming: `buildercli`/`builderengine`; cli imports engine, engine
  never imports cobra.
- **Shuttle Provider-Seam Invariant** — builder stays provider-invariant: it hands
  modelspec-resolved Model/Effort/Version to shuttle and never references Claude
  specifics.
- **Weft Git Invariant** — builder verbs (Go) commit `_lyx` artifacts via weftengine;
  the orchestrator/implementer agents never run weft git (prompt templates must not
  instruct it — review obligation); implementers DO commit host code per card.
- **lyxtest Leaf Invariant** — tests needing config use `lyxtest.SeedConfig`.
- **Sandbox Suite Coverage** — a `**Covers:** builder` scenario (validate/status on the
  fixture) or an explicit allowlist entry; scenario chosen (see Testing).
- **Documentation Lifecycle** — module doc, overview.md table, loom.md correction,
  model-spec.md/plan-format.md rename edits, roadmap milestone flip — all in the same
  commits as the behaviour.
- **Review Round Invariant** — not directly applicable (builder has no review rounds),
  but the implementer template's commit-per-card + bounded-self-fix statements are the
  analogous template-pinning candidates (template property tests).

## Testing

- **builderengine unit tests (TDD candidates):** plan parsing + all six validation
  checks against the fixture (each check has a deliberately-broken fixture variant);
  digest distillation (report YAML → digest fields, fail-loud on unparseable report);
  drift computation (scope prefix matching, `out_of_scope` vs `drift_unreported`
  separation, `dirty` detection); chain membership + chain-start-SHA recording +
  `--restart-chain` reset logic (against a scratch git repo, the gitexec test pattern);
  state.json round-trip + resume classification (report present / strand live / dead);
  pause-flag check + clearing rules; outcome.yaml fail-loud parsing; role selection
  (oversized frontmatter → `implementer_oversized`; `--role recovery`); config
  load/validation (modelspec.Parse fail-loud on bad role strings, defaults).
- **Prompt template tests:** stencil fill with no unfilled markers; property tests
  pinning the contract statements (orchestrator: digest-fields-only, verbs named,
  never-weft-git; implementer: commit-per-card subject format, self-fix cap, report
  schema, report-as-final-action) — the burler `template_test.go` pattern.
- **buildercli tests:** verb surface through the RunCLI seam with a fake/scratch setup;
  envelope shapes; `cmd/lyx` pinned-set updates (help-tree, registration, longlist,
  drift, sandbox coverage).
- **Long-poll:** poll's wait loop against a fake clock / short intervals — report
  appearing mid-wait returns early; deadline returns `running`.
- **Shuttle interaction:** fake `MuxOps`/`Engine` (shuttleengine `fakes_test.go`
  pattern) where builderengine's spawn path needs exercising without real agents.
- **Sandbox:** one `SANDBOX-CORE-SUITE.md` scenario, `**Covers:** builder` — drives
  `validate` + `status` on the fixture plan, no real agent spawns. A real end-to-end
  orchestrator+implementer run is a manual sandbox scenario, not CI (slow,
  subscription-burning, flaky).

## Q&A log

- **Q:** Orchestrator prompt — embedded stencil template or skill? **A:** Embedded
  stencil `.md` in builderengine, `//go:embed`'d, filled via `internal/stencil`,
  co-versioned with the verb/digest parser. The prompt is half of a Go-parsed contract;
  a skill versions independently — the exact coupling that must not break. lyx has no
  skill layer yet; skills belong to the future ly plugin layer. mill-go's length is the
  machine-in-prose symptom, not an argument for skills — with the machine in Go verbs
  the prompt collapses to the judgment core.
- **Q:** Plan-validation surface? **A:** Both — `validate` verb + automatic hard gate in
  `run`/`spawn-batch`.
- **Q:** Must the orchestrator poll — can it get a notification instead? **A:** A push
  notification cannot reach a Claude session (mid-turn: only tool results arrive;
  turn-end without the outcome file = `asking` under the file contract; `Send`-nudging
  fights shuttle's no-re-wait limitation). The long-poll IS the notification: `poll
  --wait` blocks inside Go (file-watch costs no tokens) and returns the instant the
  batch terminates — ~1 tool call per batch, each a terse digest.
- **Q:** Builder run-state placement? **A:** Durable `_lyx/builder/state.json` + new
  hubgeometry helpers `PlanDir`/`BuilderDir`/`BuilderReportsDir`.
- **Q:** Who picks the model at spawn? **A:** Go auto-selects
  implementer/implementer_oversized from the batch's `oversized:` frontmatter;
  orchestrator overrides only for recovery (`--role recovery`); roles → model-specs in
  builder.yaml.
- **Q:** Digest content? **A:** Fixed terse JSON fields (batch, status, tests,
  stuck_reason, out_of_scope, drift_unreported, files_changed count, dirty); no prose,
  no file lists — "not so much that it clutters the orchestrator."
- **Q:** When do builder artifacts weft-commit? **A:** "When it makes sense" → at batch
  boundaries: poll on terminal classification, spawn-batch on start-SHA record, run
  backstop at exit.
- **Q:** Terminal holistic review inside builder run? **A:** No — that is perch's job
  (the Builder-review gate); loom.md's sentence corrected in the same commit.
- **Q:** What is a "fixer"? **A:** The model-spec for the fresh escalated recovery spawn
  after a batch reports stuck (never a `/model` switch). Renamed `recovery` — the
  operator's own confusion plus the burler-fixer collision made the case;
  model-spec.md + plan-format.md updated in the same commit.
- **Q:** builder.yaml contents? **A:** "Everything needed": four roles, self_fix_cap,
  poll_wait, batch_timeout_min (60), orchestrator_timeout_min (480),
  batch_context_cap_tokens (100000), batch_card_cap (10). Scope-drift deliberately not
  configurable.
- **Q:** Chain rollback — who runs the reset? **A:** Go, behind
  `spawn-batch --restart-chain`, from the recorded chain-start SHA; orchestrator decides
  when, never types git.
- **Q:** Pause? **A:** Mirror perch: flag file + batch-boundary check in spawn-batch +
  clear-at-entry/terminal rules; `outcome: paused` is operational, not judged.
- **Q:** Orchestrator output contract? **A:** `_lyx/builder/outcome.yaml`
  (done|stuck|paused + stuck_reason + batches_done), fail-loud parse; it is `run`'s
  shuttle OutputFile; stale file refused/archived before spawn.
- **Q:** Crash/resume? **A:** Resume-on-files with an always-fresh orchestrator (never
  `claude --resume`): report present → advance; strand live → poll; dead + no report →
  respawn.
- **Q:** Timeouts? **A:** Builder-owned keys (batch 60min, orchestrator 480min); never
  inherit shuttle's review-sized default.
- **Q:** Validation cap values? **A:** Config keys `batch_context_cap_tokens` (100000) +
  `batch_card_cap` (10) with defaults, not hardcoded.
- **Q:** Drift computation timing? **A:** Terminal classification only; `running`
  snapshots are just {batch, status, elapsed}.
- **Q:** Testing depth? **A:** Unit tests with fakes + fixture plan; sandbox scenario
  drives validate/status (**Covers:** builder); real end-to-end run is manual, not CI.
- **Q:** (review r1 gap) How does poll terminate for an idle implementer — turn ended,
  no report, pane still alive? **A:** Two extra terminal branches: Stop-event-without-
  report → `dead` with `dead_reason: asking`; elapsed > `batch_timeout_min` → `dead`
  with `dead_reason: timeout` (plus pane-gone → `dead_reason: died`). Digest gains the
  `dead_reason` field; panes/run dirs kept for diagnosis.
