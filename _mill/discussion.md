# Discussion: Master Builder: new, parallel fork-based implementation module

```yaml
task: 'Master Builder: new, parallel fork-based implementation module'
slug: master-builder
status: discussing
parent: main
```

## Problem

The existing `builder` module drives a plan batch-by-batch, spawning each batch's
implementer as its own separate mux/tmux strand — a fresh `shuttle.Run` process per
batch. Every implementer starts cold: it re-reads the codebase orientation from
scratch, and cross-batch context is lost between processes. `burler`'s cluster review
validated a cheaper mechanism: **in-session Agent-tool forks**, where one long-lived
session forks sub-agents that inherit its full context — no new process, no new mux
strand.

This task builds **webster** (archaic for *weaver* — the fork mechanism literally
spins a web out of the master session): a new module, not a revision of `builder`,
that reads the codebase and the whole plan once in one long-lived Master session,
then forks one implementer per batch (sequential, same order as today). It is kept
contract-compatible with `builder` (same plan input, compatible `outcome.yaml`, same
batch-report shape) so both can be A/B tested on the same plan, and so a future
`loom`'s Builder phase won't care which implementation runs underneath. The working
name "Master Builder" in the wiki/roadmap was a POC name only; the module name is
`webster`.

Webster also adds a **prose summary artifact** alongside `outcome.yaml` — the future
`loom-finalize` PR-text source, since a long-lived Master session is the only party
with full oversight of what actually shipped (often diverging from the original task
description).

## Scope

**In:**

- New module `webster`: `internal/websterengine` + `internal/webstercli`, registered
  as `lyx webster` per the CLI/Cobra Invariant.
- Verb surface: `validate`, `run [--fresh]`, `status`, `pause`, `begin-batch`,
  `record-batch`, `recover-batch` (see Decisions).
- New `hubgeometry` helpers for `_lyx/webster/` (state, reports dir, outcome,
  summary) — same commit as first use, per the Hub Geometry Invariant.
- Webster's own fork-audit policy (writes allowed, nested `Agent` banned) plus the
  small `claudeengine.AuditForks` extension it needs (parent write-call facts with
  paths; incremental per-batch attribution).
- Master prompt template + Go-rendered thin fork-implementer template, with
  co-versioned property tests.
- `_lyx/webster/summary.md` prose summary artifact, validated at `outcome: done`.
- Oversized-batch model escalation: `begin-batch` injects `/model` into Master's
  pane via mux for `oversized: true` batches; `record-batch` de-escalates. Includes
  a sandbox validation scenario and a documented fallback.
- `webster.yaml` config (roles: `master`, `master_oversized`, `recovery`; knobs).
- Docs: `builder-contract.md` contract deltas, `docs/overview.md` module table row,
  sandbox suite scenario (`**Covers:** webster`), roadmap milestone 26 → Done at
  landing.
- Rename sweep: every existing doc mention of "Master Builder" (roadmap.md ×4,
  long-term-ideas.md ×2) is updated to `webster`.

**Out:**

- Parallel-batches-via-DAG — stays speculative in `docs/long-term-ideas.md` (its
  section heading is retitled to webster in the rename sweep, content unchanged).
- Any change to the existing `builder` module's behaviour (only additive: exporting
  or lightly refactoring a helper if reuse-by-import strictly needs it).
- An A/B comparison harness — this task delivers the contract compatibility that
  *enables* A/B; the comparison itself is manual operator runs / later loom config.
- Non-Claude engines (per repo policy, not a current priority).
- `loom-finalize` integration (it will consume `summary.md` later; only the artifact
  ships now).
- Wiki task slug / branch / worktree rename — `master-builder` stays; it is torn
  down at merge.

## Decisions

### module-name-webster

- Decision: The module is `webster` — `internal/websterengine`,
  `internal/webstercli`, `lyx webster`, `_lyx/webster/`.
- Rationale: "masterbuilder" was a POC label and reads like something that governs
  the whole repo. `webster` (archaic: weaver) fits the loom/weft/burler naming
  theme, and the fork fan-out literally forms a web out of the Master session.
- Rejected: `masterbuilder`, `mbuilder` (both kept the POC framing).

### own-state-dir

- Decision: Webster owns `_lyx/webster/` (its `state.json`, reports dir,
  `outcome.yaml`, `summary.md`, locks, pause flag), resolved via new `hubgeometry`
  helpers anchored at `layout.Cwd` exactly like builder's.
- Rationale: builder's and webster's `state.json` schemas differ, and A/B runs on
  the same plan must not clobber each other. `loom` picks the implementation (and
  thus the dir) via config later.
- Rejected: sharing `_lyx/builder/` — zero loom path changes but guaranteed state
  collisions under A/B.

### bracket-verbs-not-spawn-poll

- Decision: `spawn-batch` and `poll` do not exist in webster. The Master session
  forks each implementer itself (Agent tool, `subagent_type: "fork"`); the fork
  returns synchronously inside Master's own turn, so there is nothing for Go to
  spawn or poll in the normal path. Go instead provides two thin **bracket verbs**
  around each fork:
  - `begin-batch <NN> [--restart-chain]` — pause-flag check, plan-fingerprint
    check, optional chain rollback, records the batch's start-SHA in `state.json`,
    **idempotently asserts Master's model for this batch** (see
    `oversized-model-escalation`), renders the fork prompt, weft-commits state.
    Called by Master immediately before forking.
  - `record-batch <NN>` — runs the incremental fork audit first (transcript-count
    precedence — see `fork-audit-policy`), parses the batch report the fork wrote,
    distills it into the pinned digest (same `Digest` contract as builder, returned
    as the JSON envelope Master reads), updates `state.json`, weft-commits report +
    state. Called by Master immediately after the fork returns. It never touches
    the model — model assertion is `begin-batch`'s job alone.
- Rationale: preserves builder's three weft-commit points, the digest discipline
  (Master never reads raw reports — the mill-go-bloat lesson), and gives per-batch
  audit attribution plus early violation detection, none of which survive a
  "Go only acts at run exit" design.
- Rejected: keeping `spawn-batch`/`poll` names with fork semantics (the names would
  lie — nothing is spawned or polled by Go); no per-batch verbs at all (loses
  per-batch weft durability, digest distillation, and early audit).

### bracket-is-discipline-not-gate

- Decision: Go's gates only run when Master calls the bracket verbs; the fork
  itself (batch start) is Master's own un-gateable act. Enforcement is two-layer:
  the master template pins the begin → fork → record sequence (property-tested),
  and fail-loud detection catches violations after the fact — `record-batch`
  hard-errors when the batch has no `begin-batch` record in state, and the run-exit
  audit cross-checks fork-transcript count against begun-batch count.
- Rationale: this is the structural difference from builder (where batch start WAS
  the Go call and could refuse). Same class as burler's "steering guard, not a
  security boundary".
- Rejected: pretending Go can prevent an undisciplined fork (it cannot).

### reuse-by-import

- Decision: `websterengine` imports `builderengine`'s mechanism-agnostic pieces
  directly: `ParsePlan`, `Validate`, `Fingerprint`, `Distill`, `Classify` (for
  `recover-batch`), `ParseReport`, `ParseOutcome`, `ArchiveStaleOutcome`, chain
  rollback (`ChainMembers`/`ChainEndFor`/`RestartChain`), the pause helpers, the
  gitquery helpers, and the state/lease patterns. Webster defines only its own
  `State` struct, run logic, audit policy, and templates. No copied code.
- Rationale: one parser per shared contract (plan format, batch-report schema,
  outcome schema) makes drift between the two builders impossible. Import direction
  is clean: `websterengine → builderengine`, never the reverse, no cycle. If
  webster later proves superior and builder is retired, extraction can happen then.
- Rejected: copying (two parsers to co-version); extracting a shared leaf package
  now (refactor churn in builder this task doesn't need — YAGNI until a third
  consumer or real import pain).

### single-model-forks-and-cold-recovery

- Decision: In-session forks always inherit Master's current model — per-fork model
  selection is mechanically impossible. Therefore webster has no `implementer` /
  `implementer_oversized` fork roles. Model escalation happens two ways only:
  1. **Oversized batches** — via the `/model` pane-injection mechanism (see
     `oversized-model-escalation` below).
  2. **Recovery** — a genuinely cold, fresh implementer spawned as its own
     mux/tmux strand at the `recovery` model spec, exactly like today's builder
     recovery. Hopefully rare.
- Rationale: honest about the fork mechanism; recovery's whole point is escaping a
  polluted context AND escalating capability, which requires a cold strand.
- Rejected: Master switching its own model via `/model` by its own judgment
  (builder discipline bans agent-driven model switching in a polluted session —
  note the escalation decision below is Go-driven, not Master's judgment);
  single-model-only with prompt-escalated re-forks as the sole recovery (user
  explicitly wants real model escalation via cold start).

### recover-batch-reentrant-verb

- Decision: `recover-batch <NN>` is one **re-entrant, bounded long-poll** Go
  verb. The first call archives the stale stuck report (archive-never-refuse,
  builder's rename discipline), spawns the recovery implementer as a shuttle
  strand (reusing `builderengine.SpawnBatch` machinery via import, with
  webster's dirs and the `recovery` role), and records the strand in
  `state.json` (weft-committed immediately, so a crash mid-recovery leaves a
  reclaimable record). Every call — including the first — then blocks for at
  most `poll_wait_s` (builder's ~8-minute default) and returns either the
  terminal digest (weft-committing report + state) or a `running` snapshot
  (touching nothing, like builder's poll). A re-entrant call finds the recorded
  live recovery strand in state and skips straight to the bounded wait — Master
  simply re-calls `recover-batch <NN>` until terminal.
- Rationale: a single unbounded block would hold one Bash tool call open for up
  to `recovery_timeout_min` (60+ minutes) — exceeding the harness's tool-call
  ceiling and recreating exactly the shape builder's poll loop was built to
  avoid; bounded ~8-minute windows are builder's proven long-poll discipline
  (the long-poll IS the notification), with no fragile coupling between the
  tool-timeout and `recovery_timeout_min` knobs. Still one verb from Master's
  side: the template's exception path learns only "re-call until terminal",
  never a separate spawn/poll pair. The four-branch dead classification
  (`asking`/`timeout`/`died`) still applies inside the verb — its result
  surfaces in the returned digest.
- Rejected: mirroring builder's two-verb `spawn-batch --role recovery` + `poll`
  shape (teaches the template a spawn/poll split used only on the exception
  path); one unbounded blocking call (external review r2's finding — tool-call
  ceiling risk, dead-tool-result-while-Go-lives ambiguity, hour-long open call).

### fork-failure-ladder

- Decision: the Master's recovery ladder, pinned in the master template:
  - Fork returns, report present, `status: done` → `record-batch`, next batch.
  - Fork returns, report present, `status: stuck` → `recover-batch <NN>`.
  - Fork returns but wrote **no report** (file-contract violation; `record-batch`
    returns a distinct `no_report` classification) → re-fork the same batch once
    (fresh fork; prior partial work survives as host card commits, and
    `record-batch`'s multi-transcript attribution treats the retry as a warning,
    never a hard error) → if still no report, `recover-batch`.
  - Stuck deferred-verify chain → `begin-batch <lowest-or-any-member>
    --restart-chain` (Go re-points to the chain's lowest member, reusing builder's
    `RestartChain` semantics).
- Rationale: mirrors builder's dead→respawn-once, stuck→recovery ladder, adapted to
  synchronous fork completion.
- Rejected: treating a report-less fork as immediately recovery-worthy (wastes the
  cheap in-session retry).

### crash-resume-re-drive-first-unreported

- Decision: builder's semantics carried over. Resume is just re-running
  `lyx webster run`: entry-time reclaim stops Master's recorded strand if the mux
  still reports it live (plus any recorded recovery strand), then a **fresh**
  Master is spawned (never `claude --resume`), hydrated from the on-disk register —
  the reports dir + `state.json` rendered into `{{.progress}}` — and re-drives the
  **first unreported batch** from scratch. No progress lost: every card is a host
  commit; reports + state are weft-committed per batch.
- Rationale: the user framed it exactly as mill-go's model — Master reads a
  register of what is done and continues. Forks die *with* Master (same process),
  so unlike builder there is never an orphaned in-flight implementer for normal
  batches — only Master's own strand and a possible recovery strand need reclaim,
  making this strictly simpler than builder's version. The fork audit for the dead
  session is lost (audit only runs on `OutcomeDone`), but the per-batch incremental
  audit in `record-batch` has already covered every recorded batch — the gap is
  only the unfinished batch, which is re-driven anyway.
- Rejected: `claude --resume` of the Master session (polluted/untrusted context;
  provider-specific resume path — same reasons builder rejected it).

### summary-artifact

- Decision: `_lyx/webster/summary.md`, written by Master as its final action
  alongside `outcome.yaml`. Shape: first line `# <title>`, rest free-form narrative
  of what was actually built, including deviations from the original task. Go
  validates only presence + non-empty + title line, and only when
  `outcome: done` (fail loud, never guessed; optional on `stuck`/`paused`). Stale
  copies are archived with the same archive-never-refuse rename discipline as
  `outcome.yaml`. Both `outcome.yaml` and `summary.md` are shuttle `OutputFiles`
  entries of the Master spawn.
- Rationale: markdown because the consumer is PR text (`loom-finalize`); separate
  file keeps `outcome.yaml` machine-contract-compatible with builder.
- Rejected: a `summary:` field inside `outcome.yaml` (mixes machine contract and
  prose; breaks outcome-schema compatibility with builder).

### weft-ownership

- Decision: agents (Master and forks) never touch weft — no raw git, no
  `lyx weft`. All weft operations are `weftengine.Commit` + `Push` **in-process Go
  library calls** inside webstercli's verbs, at four deterministic points:
  1. `begin-batch` — state.json (start-SHA + batch entry durable before the fork).
  2. `record-batch` — batch report + state.json (the main per-batch sync).
  3. `recover-batch` — twice: on the spawning first call (state with
     recovery-strand record) and on the call that reaches terminal
     classification (report + state), mirroring builder's spawn-batch/poll
     points; intermediate `running`-snapshot calls touch neither git nor weft.
  4. `run` — exit backstop regardless of outcome (covers `outcome.yaml`,
     `summary.md`, stragglers), skipped only on `ErrRunBusy`.
  Pathspec always excludes `*.lock` and the pause flag, as builder's does.
- Rationale: the Weft Git Invariant. Master doesn't know weft exists — it calls the
  bracket verbs for orchestration reasons (gates, digest) and the weft sync is a
  Go-implemented side effect at the right boundary. This is the shipped buildercli
  pattern, not a new design.
- Rejected: Master calling `lyx weft sync` (agent-driven weft git — banned); a
  separate parallel Go orchestrator module for housekeeping (the blocking `run`
  process already IS the long-lived Go process; nothing else needed).

### fork-audit-policy

- Decision: webster's own audit policy over the same `ForkAudit`/`ForkReport`
  fact shapes (burler's `auditClusterRound` stays untouched; its
  `mutatingGitPattern` is unexported, so webster defines its own weft-scoped
  pattern):
  - **Forks:** nested `Agent` calls = hard error; named spawns = hard error;
    Write/Edit **allowed**; host-repo git **allowed** (per-card commits are
    required by the implementer contract); any Bash command referencing the weft
    worktree path or invoking `lyx weft` / `lyx warp` = hard error.
  - **Master (parent transcript):** same weft ban; Write/Edit allowed **only** to
    Master's two contract files, `_lyx/webster/outcome.yaml` and
    `_lyx/webster/summary.md` (a blanket write ban would break the outcome
    contract); any other parent write — including into webster's reports dir — =
    hard error (catches a "lazy" Master implementing batches itself or
    hand-writing a batch report — same silent-quality-degradation class as named
    forks).
  - **Attribution, with pinned check order:** `record-batch` checks the
    fork-transcript count **before** report presence. (1) Zero fork transcripts
    new since the previous batch boundary = hard error — the batch was never
    forked, **regardless of whether a report file exists** (a report with no fork
    behind it means Master wrote it itself; transcript-count-first is what makes
    that unfakeable). **Flush-timing caveat, explicit:** this moves transcript
    reading from run-finalize (today's only audit point) to the instant the
    Agent tool call returns, which assumes Claude Code has flushed
    `subagents/<id>.jsonl` by then. The zero-transcript hard error therefore
    fires only after a **bounded settle-retry** (re-scan for a few seconds)
    — builder's "first failed parse is inconclusive" discipline applied to
    transcript presence: never a guessed error, at worst one settle-window
    later. The flush timing itself is asserted in the sandbox validation
    scenario (transcript present at tool-return time) alongside the `/model`
    checks. (2) One-or-more new transcripts + report present → normal
    parse/distill; more than one transcript = warning only (legitimate retry
    after a reported `no_report`), never hard. (3) One-or-more new transcripts +
    **no** report → the `no_report` classification (the fork ran but violated the
    file contract) → Master's ladder re-forks once. An Agent tool call that
    errors before the fork ever runs surfaces synchronously as Master's own tool
    error — Master re-forks directly and never calls `record-batch` on a
    transcript-less batch.
  - **Timing:** incremental per-batch audit at `record-batch` (early detection +
    attribution), plus the standard whole-session audit at run finalize
    (`OutcomeDone` only) as the backstop cross-check.
- Rationale: fork-count and read-only rules are burler policy, not mechanism;
  shuttleengine's fact/policy split was designed for exactly this second consumer.
- Rejected: sharing burler's audit path (hard-bans writes — the opposite
  requirement); skipping the parent-write rule (Master-codes-itself goes
  undetected); hard-erroring multi-fork batches (blocks the legitimate retry).

### audit-forks-extension

- Decision: `claudeengine.AuditForks` (and the provider-invariant
  `shuttleengine.ForkAudit` shapes) grow the minimum webster needs: parent-session
  Write/Edit facts **with file paths** (today only parent spawn counts are
  collected), and an incremental entry point usable mid-run — webster records
  Master's session ID (captured at spawn into `state.json`) plus the set of
  already-attributed fork transcript filenames, so `record-batch` can parse only
  what is new. All transcript parsing stays inside `claudeengine` per the Shuttle
  Provider-Seam Invariant; websterengine consumes provider-invariant fact structs
  only.
- Rationale: the audit facts live in Claude Code's transcript files; the seam
  invariant forbids webster reading them directly.
- Rejected: post-hoc-only audit (no per-batch attribution, violations surface
  hours late, and a died Master yields no audit at all).

### fork-prompt-go-rendered

- Decision: `begin-batch` renders the fork-implementer prompt from an embedded
  webster template (adapted from builder's `implementer-template.md`) and
  **writes it to a prompt file** under `_lyx/webster/` (e.g.
  `prompts/NN-<slug>.md` — a re-renderable runtime artifact, excluded from the
  weft-commit pathspec like the locks), returning the path in the envelope.
  Master's Agent fork call is then exactly "Read this file and follow it
  exactly: <path>" per its template — the mill-brief delivery pattern; no
  paraphrase surface at all, and the prompt text never sits in Master's context.
  Cross-batch digest context is Go-rendered into the prompt by `begin-batch`
  itself, **unconditionally from batch 2 onward**: the immediately preceding
  batch's digest — read from its persisted `BatchState.Digest` (see
  `state-schema`), never re-derived — is included as a fixed template section (a
  Go-decided constant — the plan format is a flat ordered list with no DAG, so
  there is no dependency edge to consult and no selectivity to infer; see
  `docs/long-term-ideas.md`'s no-DAG decision).
  The fork prompt is thin —
  batch file path, report path, `self_fix_cap`, per-card host-commit discipline,
  and the fresh-read rule ("re-read the files your batch touches; inherited file
  content may be stale") — because the fork inherits Master's full context and
  needs no cold orientation.
- Rationale: the batch-report schema the fork writes is Go-parsed (`ParseReport`);
  rendering the prompt in Go keeps both halves of that contract co-versioned in
  one commit, per builder's templates↔parsers rule. Master paraphrasing the report
  schema would be a silent-breakage vector.
- Rejected: Master composing fork prompts freehand from rules in its template;
  Master selectively prefixing prior-batch context by its own judgment (no
  mechanical dependency signal exists in the no-DAG plan format — external
  review r2's finding); returning the prompt inline in the envelope (duplicates
  the text into Master's context and reopens a paraphrase surface);
  `begin-batch` re-parsing and re-`Distill`ing the preceding report instead of
  reading the persisted digest (re-derives `files_changed`/`drift`/`dirty`
  against a HEAD that has since moved — the reconstructed digest could differ
  from what Master actually saw).

### oversized-model-escalation

- Decision: model escalation for `oversized: true` batches is **Go-driven pane
  injection**, and the injection is **idempotent per batch**: every `begin-batch`
  synchronously injects `/model <target>` into Master's tmux pane via mux
  **before returning its envelope**, where `<target>` is `master_oversized` when
  the batch declares `oversized: true` and `master` otherwise — it asserts the
  correct model for *this* batch rather than assuming the previous batch's state.
  No race: by the time Master reads the envelope and forks, the model is
  switched, and the fork inherits it. This is the **only** injection point:
  `record-batch` never touches the model, no failure-path de-escalation exists to
  forget (a `stuck`/`no_report` batch that skips `record-batch` is self-healed by
  the next batch's `begin-batch` assertion), and no run-exit reset is needed (the
  Master session terminates at run end; no model state survives it). Go always
  makes this call — never Master's judgment. Mechanical caveat, pinned as an
  explicit early validation scenario in the sandbox suite: `/model` is a local
  CLI command and *should* apply to subsequent API calls within Master's single
  long agentic turn. The load-bearing uncertainty the scenario must exercise is
  the exact production timing: the keys are injected **while a bracket-verb Bash
  subprocess is executing in Master's pane** — the scenario must inject during a
  running foreground tool subprocess and confirm the keys reach Claude's TUI
  input (which keeps reading during tool execution) and switch the model for
  subsequent calls in the same turn, not merely assert mid-turn effect in a quiet
  pane. If real-world validation shows it does not hold, the feature degrades to
  the documented fallback: `oversized:` is accepted for plan compatibility but
  has no spawn effect in webster.
- Rationale: forks inherit the session's current model, so switching Master's
  model at the batch boundary is the only way to restore `oversized:` semantics
  in-session; the user expects real use for this. The Go-side `run` process /
  bracket verbs are the legitimate place (deterministic, Go-decided), which is
  what distinguishes this from the banned agent-driven `/model` switching.
- Rejected: an operator-only `escalate` verb (operator rarely watching); building
  a general supervisor/watchdog in `run` (YAGNI until webster has run in anger);
  `oversized:` → cold strand with `implementer_oversized` (reintroduces the
  per-batch process mechanism this module exists to remove); escalate-in-
  `begin-batch` / de-escalate-in-`record-batch` pairing (leaks the escalated
  model onto every later batch whenever a failure path skips `record-batch` —
  review r1's gap).

### config-webster-yaml

- Decision: `webster.yaml` with roles `master` (default `sonnet` — parity with
  builder's implementer default, so A/B compares mechanism, not model),
  `master_oversized` (default `opus`), `recovery` (default `opus[effort=high]`,
  as builder). Knobs: `self_fix_cap`, `master_timeout_min` (the Master spawn's
  shuttle timeout — the **whole-run** analog of builder's
  `orchestrator_timeout_min`, default 480, deliberately generous because it spans
  every batch of the sequential plan; it is NOT a per-batch timeout, and no
  per-batch watchdog exists in-session since fork completion is synchronous
  within Master's turn), `recovery_timeout_min` (the per-batch
  `batch_timeout_min` analog, applying only to the cold recovery strand —
  elapsed-since-spawn, evaluated across re-entrant `recover-batch` calls),
  `poll_wait_s` (the bounded window a single `recover-batch` call blocks for,
  builder's ~480s default), and the validate caps (`batch_context_cap_tokens`,
  `batch_card_cap`) mirroring builder's. Role grammar checked at `LoadConfig` via `modelspec.Parse`;
  all roles resolved via `ResolveRoles`-style pre-flight at `run` entry.
- Rationale: matches builder's config discipline; sonnet default keeps the A/B
  honest.
- Rejected: `master` defaulting to opus (model-confounds the A/B, costlier).

### run-verb-shape

- Decision: `lyx webster run [--fresh]` mirrors builder's `run`: exclusive
  `run.lock` for its whole duration (`ErrRunBusy` fail-fast, loser commits
  nothing), automatic validation gate, entry-time strand reclaim, plan-fingerprint
  check against `state.json` (`--fresh` archives state + reports dir and re-inits;
  mismatch without `--fresh` refuses loud), pause-flag clear once committed to
  spawning, stale outcome/summary archiving, then spawns Master via shuttle with
  `ForkSubagents: true` and blocks until terminal. `mutate.lock` guards every
  read-modify-write of `state.json` across all verbs, as builder's does.
  Master-spawn non-done outcomes (`asking`/`died`/`timeout`) map to distinct
  errors carrying session ID + kept run dir, exactly like builder's orchestrator
  errors. Requires a live mux session (`lyx mux up` first).
- Rationale: all of this is mechanism-agnostic run-level discipline that builder
  already proved; webster changes only what happens *between* the boundaries.
- Rejected: nothing — carried over deliberately.

### state-schema

- Decision: webster's own `State` in `_lyx/webster/state.json`: `RunGUID`,
  `PlanFingerprint`, `CurrentBatch`, `MasterStrand`, `MasterSessionID` (for
  transcript-audit resolution), `AttributedForkTranscripts` (per-batch map of
  transcript filenames already audited), `Batches map[int]*BatchState`,
  `ChainStartSHAs`. `BatchState`: `Slug`, `StartSHA`, `Kind` (`fork` |
  `recovery`), `SpawnedAt`, `Terminal`, `Status`, `Digest` (the distilled digest
  persisted by `record-batch` at terminal classification — the persistence home
  that carries batch N's digest forward to `begin-batch(N+1)`'s fork-prompt
  rendering and to the crash-resume `{{.progress}}` rendering; builder never
  persisted `Digest`, webster must), and — recovery only —
  `StrandGUID`/`ShuttleRunDir`/`EventsPath`. Fork batches carry no strand fields
  (there is no strand).
- Rationale: builder's three strand fields are exactly the mechanism-specific part
  of its schema; webster replaces them with fork-attribution fields and keeps the
  generic rest structurally identical.
- Rejected: reusing builder's `State` struct with nil-ed strand fields (schema
  would lie about the mechanism; the two modules' state files are independent).

### docs-and-rename

- Decision: per the Documentation Lifecycle, **no** `docs/modules/webster.md` —
  webster's design lives in its package doc comments (`websterengine/doc.go`,
  `webstercli`). The cross-module **contract deltas** go into
  `docs/modules/builder-contract.md` (a contract doc, explicitly exempt from
  deletion, like plan-format.md): webster consumes the same plan/batch-report/
  outcome contracts; `summary.md` is webster-only; the digest contract is shared.
  `docs/overview.md` module table gets a webster row. Roadmap milestone 26 is
  marked ✅ Done at landing. The rename sweep (Master Builder → webster:
  roadmap.md ×4, long-term-ideas.md ×2 including the DAG section's title) lands
  with this task.
- Rationale: module docs rot and are deleted at landing (repo rule the user
  reaffirmed); contract docs are the durable cross-module reference.
- Rejected: a persistent webster module doc; folding webster's design wholesale
  into builder-contract.md (contract file would become a hybrid design doc).

## Technical context

**Fork mechanism (validated by burler, reusable as-is):**

- `shuttleengine.Spec.ForkSubagents` authorizes forks; `claudeengine` realizes it
  via the `CLAUDE_CODE_FORK_SUBAGENT=1` env wrap
  (`internal/shuttleengine/claudeengine/command.go:111,156-177`) and a conditional
  PreToolUse hook that allows only `"subagent_type":"fork"` Agent calls
  (`settings.go:116-161`) — explicitly a steering guard, not a security boundary.
- Fork facts come from **post-hoc transcript parsing**: `claudeengine.AuditForks`
  (`audit.go:36-73`) reads `~/.claude/projects/<encoded-cwd>/<sessionID>.jsonl`
  (parent facts) and `<sessionID>/subagents/*.jsonl` (one `ForkReport` per fork:
  `AgentCalls`, `WriteCalls`, `BashCommands` verbatim, `ToolCalls`,
  `ReportReturned`). Workdir for transcript resolution must be `layout.Cwd`, never
  `WorktreeRoot` (`wait.go:311-319`).
- The audit currently runs **only on `OutcomeDone`** (`wait.go:329-335`) and
  carries **no per-batch identity** — the two structural gaps webster's
  incremental `record-batch` audit closes.
- `shuttleengine.ForkAudit` is deliberately policy-free
  (`forkaudit.go:5-6,34,50-53`): facts from the engine, interpretation by the
  caller. burler's policy is `auditClusterRound`
  (`internal/burlerengine/cluster.go:67`) — package-local, single consumer today,
  its `mutatingGitPattern` unexported.
- Nothing in shuttleengine/muxengine breaks when a fork writes files or commits to
  the host repo. Forks run inside the same claude process/pane — no new strand.
- burler's fork instruction lives in prompt prose (`burlerengine/prompt.go:139-169`
  `clusterRulesBlock`): fork spawning is an in-prompt instruction, fork reports
  return as Agent tool results inside the handler's turn.

**builderengine reuse map (import targets):**

- Mechanism-agnostic and cleanly exported: `ParsePlan` (`plan.go:250` — the only
  plan parser in the repo), `Validate` (`validate.go`), `Fingerprint`
  (`fingerprint.go:29`), `Distill` (`digest.go:81`), `Classify`/`PollUntilTerminal`
  (`poll.go:90,202` — recover-batch's terminal wait), `ParseReport`
  (`report.go:74`), `ParseOutcome`/`ArchiveStaleOutcome` (`outcome.go:54,97`),
  chain rollback (`chain.go`), pause helpers (`pause.go`), gitquery helpers
  (`gitquery.go`), `state`/`lock` patterns (`state.go:43` `AcquireStateMutation`).
- Mechanism-bound (NOT imported wholesale; recover-batch reuses the `SpawnBatch`
  machinery selectively): `spawn.go` (`Starter` seam at `spawn.go:70`,
  `shuttleengine.FindRun` coupling at `spawn.go:498`), `runlevel.go`
  (`OrchestratorStarter`/`OrchestratorHandle` at `runlevel.go:117-133`, `--fresh`
  archiving helpers at `runlevel.go:206-255` which ARE reusable).
- builderengine is weft-blind and `_lyx`-blind: it takes resolved dir strings;
  all `_lyx` path construction lives in `hubgeometry` (`PlanDir`/`BuilderDir`/
  `BuilderReportsDir`, `hubgeometry.go:229-253`). Webster needs sibling helpers
  (`WebsterDir`, `WebsterReportsDir`, …) — Hub Geometry Invariant, same commit.
- buildercli's wiring pattern to mirror: `PersistentPreRunE` resolution seam
  (`cli.go:134-228`; tests inject fakes by populating `builderCLI` fields
  directly), weft commits only in the cli layer (`weft.go:48` `weftCommit`;
  pathspec excludes `*.lock` and the pause flag), `clihelp.Execute` seam, JSON
  envelope everywhere.
- Builder's templates: `orchestrator-template.md` (152 lines; pins verb loop,
  digest-fields-only reading, recovery ladder, "never touch weft/git/`_lyx`
  [except own output files]/`/model`") and `implementer-template.md` (115 lines;
  per-card host commits, report schema, self-fix cap) — webster's master template
  and thin fork template adapt these; property tests
  (`builderengine/template_test.go` style) pin the contract statements.

**Gotchas discovered during exploration:**

- Forks always inherit the parent session's model and effort — no per-fork
  override exists. This is what forces the escalation design.
- `/model` injected into a busy pane is handled by the Claude Code CLI locally;
  whether it takes effect on subsequent API calls mid-turn is plausible but
  unvalidated — hence the pinned sandbox validation scenario + fallback.
- A fork that dies/errs surfaces as the Agent tool call's error inside Master's
  turn (synchronous) — there is no `died`/`timeout` classification for forks; the
  four-branch classification survives only inside `recover-batch`.
- Weft commits by module verbs are the established pattern (buildercli's three
  points) — the Weft Git Invariant bans *agents* driving weft git, not Go verbs
  the agent happens to invoke.
- The master template must pin: Master never Writes/Edits any file other than
  `_lyx/webster/outcome.yaml` and `_lyx/webster/summary.md`, never runs git at
  all, never calls `/model` itself, never forks without `begin-batch`, forwards
  the Go-rendered fork prompt verbatim, reads only digest fields.

## Constraints

From `CONSTRAINTS.md`, directly applicable:

- **Hub Geometry Invariant** — new `_lyx/webster` path helpers live in
  `internal/hubgeometry` only, added in the same commit as first use; the
  enforcement test will catch violations.
- **CLI / Cobra Invariant** — `websterengine`/`webstercli` split, `Command()` +
  `RunCLI` seam, `Short` on every command, registration in `cmd/lyx/main.go`
  `newRoot()` + root `Long` list, pinned sets in `drift_test.go`/
  `helptree_test.go`/`registration_test.go`/`longlist_test.go` updated in the same
  commit. Errors ride the `internal/output` JSON envelope.
- **Weft Git Invariant** — see `weft-ownership` decision; agents never drive weft
  git; prompt templates must never instruct a weft git op (review obligation).
- **Shuttle Provider-Seam Invariant** — the AuditForks extension (parent writes,
  incremental parsing) stays inside `claudeengine`; `shuttleengine` carries only
  provider-invariant fact shapes; `websterengine` never reads transcripts
  directly.
- **Sandbox Suite Coverage** — a webster scenario tagged `**Covers:** webster`
  (this is also where the `/model`-injection validation scenario lives), or an
  explicit allowlist entry; scenario chosen (see Testing).
- **Test Tier Purity Invariant** — untagged webster tests spawn nothing;
  git/fixture tests are integration-tagged.
- **Hermetic Git Test Environment Invariant** — every git-spawning webster test
  package gets a `TestMain` with `lyxtest.HermeticGitEnv()`.
- **Review Round Invariant** — not directly applicable (webster is not a review
  loop), but the fork-audit fail-loud posture follows its discipline.
- **Co-versioning rule** (builder-contract.md) — webster's templates and the Go
  parsers/renderers of their contracts move in the same commit; property tests pin
  the statements.
- **Documentation Lifecycle** — no persistent module doc; package docs + contract
  deltas in builder-contract.md.

## Testing

- **TDD candidates:** the webster audit-policy function (pure: `ForkAudit` facts
  in, hard-error/warning split out — table-driven over all violation classes:
  nested Agent, named spawn, parent write outside the two contract files,
  weft-referencing Bash, zero/multi transcript attribution and its
  transcript-count-before-report-presence check order); the `summary.md` presence/title
  validation; the state schema round-trip; the fork-prompt rendering (marker
  presence, report-path/batch-file substitution); the `no_report` classification
  in `record-batch`.
- **Template property tests** (builderengine `template_test.go` style): master
  template states the bracket sequence (begin → fork → record), digest-fields-only
  reading, the fork-failure ladder, verbatim fork-prompt forwarding, the
  no-writes-outside-`_lyx/webster/` and no-`/model` rules, final action =
  outcome + summary; fork template states per-card host commits, the fresh-read
  rule, the report schema, self-fix cap.
- **Engine unit tests with fakes:** bracket verbs against a fake audit source and
  fake mux (escalation injection recorded, not executed); `recover-batch` against
  builder's existing fake-starter pattern; run-level gates (fingerprint mismatch,
  `--fresh` archiving, `ErrRunBusy`, pause) largely exercised through the imported
  builderengine functions' own tests plus webster-level wiring tests.
- **claudeengine tests** for the AuditForks extension: parent write extraction
  with paths, incremental parsing given already-seen transcript names — fixture
  transcripts, no live sessions.
- **CLI tests** through the `RunCLI` seam with injected fakes (buildercli's
  pattern), including weft-commit pathspec exclusion of locks/pause flag.
- **Integration-tagged:** state/weft round-trips with hermetic git.
- **Sandbox suite:** one webster scenario (`**Covers:** webster`) driving a tiny
  real plan end-to-end, plus the `/model` injection validation scenario whose
  verdict decides escalation-vs-fallback — it must inject **while a foreground
  Bash tool subprocess is executing in the pane** (the production `begin-batch`
  timing) and assert two distinct properties: (a) the keys reach the TUI input
  and the model switches for subsequent calls in the same turn (a miss here is
  the benign failure mode → degrade to the documented fallback), and (b) the
  injected keystrokes do **not** leak into the running subprocess's own
  stdin/output or otherwise corrupt that tool call's result (corruption is the
  dangerous failure mode that unconditionally triggers the fallback). Not merely
  mid-turn effect in a quiet pane. The same scenario also asserts the
  transcript-flush timing the incremental audit depends on: `subagents/<id>.jsonl`
  is present on disk when the Agent fork call returns. Pinned as an early
  milestone in the plan so the fallback decision lands before the escalation
  code hardens.

## Q&A log

- **Q:** Module name? **A:** `webster` — "masterbuilder" was a POC label the user
  dislikes ("sounds like it governs the whole repo"); webster (archaic: weaver)
  fits the weaving theme and the fork mechanism "literally becomes a web".
- **Q:** Own state dir or share `_lyx/builder/`? **A:** Own dir, named
  `_lyx/webster/`.
- **Q:** Keep spawn-batch/poll? **A:** No Go-based poll needed — the fork setup
  handles completion synchronously; bracket verbs `begin-batch`/`record-batch`
  instead.
- **Q:** Reuse builder code by import or copy? **A:** Import, no duplicate code;
  if webster proves superior, extraction/refactor can happen then.
- **Q:** Per-fork model selection is impossible — how to escalate? **A:** Real
  escalation must spawn a cold, new implementer strand as today ("la oss håpe
  dette ikke trengs så ofte"); later extended with Go-driven `/model` injection
  for oversized batches.
- **Q:** Crash/resume when Master dies? **A:** Master must figure it out at
  startup from a register of what is implemented (the reports + state), like
  today's mill-go — i.e. fresh Master, re-drive first unreported batch.
- **Q:** Summary artifact shape? **A:** `summary.md` markdown, `# title` +
  narrative, validated minimally at `outcome: done`.
- **Q:** Who performs weft changes — Master? **A:** Initially assumed Master
  ("webster orch") would run `lyx weft sync`; corrected against the Weft Git
  Invariant: Go performs all weft git inside the bracket/run verbs
  (in-process `weftengine` calls) — agents never; user confirmed after the
  clarification, requiring only that weft sync happens *during* the run (it does:
  four commit points).
- **Q:** `oversized:` handling? **A:** No effect in webster unless model
  escalation is needed; escalation to a dedicated fixer is a cold start — then
  extended: include Go-driven `/model` pane injection so oversized batches get a
  stronger model in-session, with validation + documented fallback.
- **Q:** Parent-write audit rule? **A:** Accepted with the `_lyx/webster/`
  carve-out (Master writes its own outcome/summary contract files).
- **Q:** Module docs? **A:** User corrected the recommendation: docs under
  `docs/modules/` are deleted when the module lands (Documentation Lifecycle);
  contract deltas go in builder-contract.md instead.
- **Q:** Go supervisor / model escalation idea — defer to long-term-ideas?
  **A:** No — "jeg tror vi får bruk for slik modell-eskalering, så ta det med";
  included as the oversized-driven escalation.
- **Q:** A/B comparison harness? **A:** Out of scope.
- **Q:** Rename sweep? **A:** All existing docs that don't use the new name must
  be updated to `webster` as part of this task.
- **Q:** Difference from builder worth pinning? **A:** User: webster starts a
  batch by forking; builder started it via a Go call — captured as the
  bracket-is-discipline-not-gate decision (Go gates can only refuse when called;
  enforcement is template discipline + fail-loud audit).
- **Q:** (review r1 gap) Model de-escalation leaks when a failure path skips
  `record-batch` — who de-escalates? **A:** Nobody: `begin-batch` idempotently
  asserts the correct model for each batch (escalate or de-escalate as needed),
  making it the single injection point; `record-batch` never touches the model
  and no run-exit reset is needed.
- **Q:** (external review r2) `recover-batch` as one unbounded blocking call
  exceeds the tool-call ceiling and recreates the shape builder's poll avoided —
  bound it or raise the ceiling? **A:** User delegated; chose the re-entrant
  bounded long-poll shape (`poll_wait_s` windows, Master re-calls until
  terminal) over raising Master's Bash ceiling.
- **Q:** (external review r2) When does the fork prompt carry prior-batch digest
  context, given the no-DAG plan format has no dependency edges? **A:**
  Unconditionally from batch 2 onward, Go-rendered by `begin-batch` (the
  preceding batch's digest as a fixed template section) — never Master's
  selective judgment.
- **Q:** (review r2 gap) Where does batch N's digest live so `begin-batch(N+1)`
  can render it? **A:** Persisted in `BatchState.Digest` by `record-batch`;
  never re-`Distill`ed from the report (re-derivation against a moved HEAD could
  differ from what Master saw). [User standing directive: apply reviewer-round
  recommendations without per-gap prompting.]
- **Q:** (review r2 gap) The zero-transcript hard error assumes the fork
  transcript is flushed when the Agent call returns — de-risk how? **A:**
  Bounded settle-retry before the hard error (first-miss-is-inconclusive
  discipline) plus an explicit flush-timing assertion in the sandbox validation
  scenario.
