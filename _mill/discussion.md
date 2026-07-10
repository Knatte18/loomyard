# Discussion: Pin the plan format (Builder input contract)

```yaml
task: Pin the plan format (Builder input contract)
slug: plan-format
status: discussing
parent: main
```

## Problem

The `builder` module is the next spine milestone, but it cannot be built until the
plan-artifact format it consumes is pinned. The future Planner phase produces plans; builder
executes them; neither exists yet, so the contract between them must be settled first — then
builder can be built and tested against a hand-written plan fixture before any Planner exists.

The proposal carried an agreed starting design (anchored on mill's plan format, DAG stripped,
batch-sizing as principle #0) plus a list of open decisions. This discussion closed all of
them, and additionally pinned a **model-spec notation** (provider/model/params) that the
config side needs because the plan itself is model-agnostic. It also corrected a load-bearing
misreading: the Builder orchestrator is a **long-lived LLM session**, not a Go loop (see
Decisions).

## Scope

**In:**

- A new contract doc `docs/modules/plan-format.md` pinning plan-format **v1**: plan directory
  layout, `00-overview.md`, batch files, cards, `verify:` placement, scope semantics,
  oversized-batch exceptions, the batch-report schema, and on-disk locations.
- One complete worked example inside the doc (overview + one batch file + one batch-report)
  that doubles as the template for builder's future hand-written fixture.
- A new contract doc `docs/reference/model-spec.md` pinning the model-spec notation:
  `alias[key=value,...]` grammar, the `models.yaml` registry, precedence rules, fail-loud
  handling.
- Surgical same-commit fixes to existing docs that contradict the pinned contract:
  `docs/modules/loom.md` (Builder described as a Go loop; "dependency order"; "Haiku by
  default") and a `docs/roadmap.md` pointer/milestone update per the roadmap rules in
  CLAUDE.md.

**Out:**

- **No code.** No `internal/builderengine`, no verbs, no plan validator, no registry loader,
  no hubgeometry path helpers. Those land with the builder task (and friends).
- The hand-written plan **fixture** (testdata) — builder's task writes it against this
  contract, next to builder's tests.
- The Planner phase, prompt templates as shipped artifacts, and any perch/burler changes.
- Non-Claude providers (Gemini, Ollama). The model-spec grammar is provider-agnostic by
  design, but only the Claude engine exists; Ollama-style `num_ctx` tuning is explicitly out
  of scope.
- `pipeline`/loom behaviour changes. Config keys are *documented* as contract (builder.yaml
  roles, loom overlay), not implemented.

## Decisions

### Consumer model — LLM orchestrator drives Go verbs (not a Go loop)

- Decision: The Builder orchestrator is a **long-lived LLM session** (model chosen via
  config; builder.yaml default, e.g. Sonnet) that holds the batch loop and drives fat
  `lyx builder` verbs (`spawn-batch`, `poll`, `status`). Go (`internal/builderengine`)
  provides ONLY those verbs plus the distillation behind them — it does not hold the loop,
  does not iterate batches, does not make orchestration decisions. The batch-report is read
  by Go inside the `poll` verb, which returns a **distilled digest** to the orchestrator; the
  orchestrator never ingests raw session prose (the mill-go bloat lesson: bloat came from the
  LLM orchestrator swallowing verbose sub-agent output, not from the loop being LLM-held).
  Recovery (scoped fixer on a stuck batch) is spawned by the orchestrator's judgment, not a
  Go branch.
- Rationale: Decided in the formats-pass; a pure Go-driven loop was explicitly rejected.
  Digest-only consumption keeps a persistent LLM lean.
- Rejected: Go-held batch loop (`internal/builderengine` as the driver) — the proposal's
  wording suggested this, and parts of `docs/modules/loom.md` still describe it; the
  plan-format doc must not be designed around a Go loop, and loom.md gets fixed in the same
  commit.

### Structure — ordered batch list, no DAG

- Decision: A plan is an ordered sequence of batches executed strictly in order; batch N may
  assume 1..N−1 are committed. Layout: `00-overview.md` + `NN-<batch-slug>.md`; the `NN`
  prefix *is* the order — no separate ordering metadata, no `depends_on`, no topological
  sort.
- Rationale: mill's DAG existed only for intra-plan parallelism that was never used.
  Parallelism lives one level up: separate tasks, each its own worktree + `lyx run`.
- Rejected: DAG/depends_on (mill's model).

### On-disk locations

- Decision: The plan lives at `_lyx/plan/` (overlay in the weft repo, like the status file);
  batch-reports at `_lyx/builder/reports/NN-<batch-slug>.yaml`. When builder is implemented,
  these paths resolve through `internal/hubgeometry` helpers like every other `_lyx` path
  (Hub Geometry Invariant); the doc states the paths in those terms.
- Rationale: Task-state artifacts are weft overlay per the Weft Git Invariant — agents write
  them via the junction, Go reads and commits them. A hand-written fixture (the task's
  stated purpose) needs the locations pinned now.
- Rejected: host-repo plan à la mill's `_mill/plan/` (collides with the weft-overlay model);
  deferring path decisions to the builder task.

### `00-overview.md` — frontmatter + index + framing, nothing else

- Decision: Frontmatter carries exactly `format: 1` and `approved: true|false`. Body carries
  the **Batch Index** (ordered list: `NN — <slug> — <one-line intent>`) and a short
  task-framing paragraph. Builder refuses to run a plan that is unapproved **or** whose
  `format` it does not recognize — fail loud, never misread.
- Rationale: `format: 1` is the same fail-loud discipline as the burler verdict-parse and
  the psmux capability-probe; one field now means plans are self-identifying the day v2
  arrives. Everything else the driver needs is per-batch or config; batch count is derivable
  from the index.
- Rejected: extra driver metadata in the overview (totals, duration estimates, model hints);
  no version field ("version lives only in the doc" — fragile the day the format changes).

### Batch file contents

- Decision: `NN-<batch-slug>.md` = title + **intent** (what this batch delivers as a
  stand-alone unit), **Scope** (see scope decision), **Cards** (ordered), per-batch
  **`verify:`** (mandatory; may be `deferred` — see oversized/exceptions decision), and
  optional frontmatter `oversized: true`. An oversized batch MUST justify the flag in its
  intent section.
- Rationale: One batch = one implementer session = one stand-alone unit: read the batch +
  relevant code + implement + verify + commit, within budget.
- Rejected: nothing beyond the proposal here; fields were confirmed as proposed.

### Principle #0 and its two exception mechanisms

- Decision: Batches stay small — a batch must fit comfortably inside a standard
  implementer's context window; prefer many small batches over few large ones. Two explicit
  exception mechanisms for work that resists decomposition (e.g. a large atomic refactor):
  1. **`oversized: true`** on the batch → the orchestrator spawns the
     `implementer_oversized` role's model-spec (a model/variant that *has* a large window —
     for Claude today the 1M-Sonnet variant, realized however claudeengine realizes it,
     e.g. `--model sonnet[1m]`). Context size is NOT a generic tunable param.
  2. **Split with deferred verify**: the Planner splits the refactor into consecutive small
     batches where intermediates declare `verify: deferred` (their batch-reports say
     `tests: skipped`); the final batch of the chain runs the full `verify:`. The green
     invariant holds at chain level: the last batch must be green.
- Rationale: mill's 200k-overflow pain made sizing principle #0; but real refactors
  sometimes cannot pass through compiling intermediate states, so the format needs a
  declared escape rather than ad-hoc violation. Both mechanisms are cheap to express;
  Planner picks per case.
- Rejected: no exceptions (unrealistic); oversized-marker only (a thrashing 1M session is
  expensive and indivisible); split-only (some operations are genuinely indivisible per
  session).

### `oversized` governance — author decides, machine enforces

- Decision: The flag is set at plan-authoring time (Planner, or the human hand-writing a
  plan) only when a batch demonstrably cannot be decomposed, with justification in the
  intent; plan review (perch) carries an explicit rubric point challenging unjustified
  flags. Complementing that, plan **validation** (Go, when built) mechanically estimates
  each batch's context — sum of file sizes in bytes over the batch's referenced files
  (Scope list + card Where files), divided by 4 — plus a card-count cap; a batch over cap
  **without** `oversized: true` fails validation loudly, forcing the Planner to split or
  flag. The flag is therefore never a silent default. (Precedent: millhouse
  `_plan_validate.py` `batch-oversized` checks — bytes//4 estimate and max-cards cap.)
- Format implication: a batch's read surface must be mechanically derivable — the Scope
  path list and each card's Where files serve as the estimator's input.
- Rejected: author-only (loses the proven cheap mechanical catch); mechanics-only (the
  estimate cannot judge "splittable vs genuinely indivisible" — that is the author's call);
  runtime detection (that is escalation, which already exists via stuck-recovery; mixing it
  in makes the plan less declarative).

### Card — planning unit ≈ one commit; commit message names the card

- Decision: A card is the smallest implementable unit: **What** (concrete change, detailed
  enough for a cheap model), **Where** (files touched), optional per-card `verify:`. "One
  coherent commit" is the **planning rule** for card sizing, not a runtime invariant —
  fix-commits after a red verify are legitimate, and commit *count* is never used as a
  check. Commit-per-card with the commit subject referencing batch+card (pinned convention,
  e.g. subject prefix `02.3: <what>` for batch 02 card 3).
- Rationale: Commit-per-card is the **resume mechanism**: a fresh session sees from git log
  exactly which card the previous session reached; a half-done card is resumed by
  discarding uncommitted changes and restarting that card. (Host-repo commits by the agent
  itself — Weft Git Invariant asymmetry.)
- Rejected: hard "exactly one commit" both ways (collides with self-fix commits; false
  stucks); no commit linkage (loses backtracking granularity and resumability).

### `verify:` placement — per-batch mandatory, per-card optional

- Decision: The batch's `verify:` is the gate (value may be `deferred` per the exceptions
  decision). A card MAY additionally carry a cheap check (e.g. `go build ./...`) where the
  Planner sees value. `verify:` output must be filtered to pass/fail + failures — never raw
  build/test noise (the dotnet-warning lesson; language plugins own the filtering).
- Rejected: per-batch only (a card-1 break surfaces after card 5); per-card only (expensive
  and redundant when batches are small).

### Red tests — bounded self-fix, then stuck; review runs on green code only

- Decision: The implementer gets a small bounded number of in-session fix attempts (e.g. 2)
  after a red `verify:`; still red → batch-report `status: stuck`, `tests: red`. The
  orchestrator then spawns a **fresh** escalated fixer that reads the durable reports —
  never a `/model` switch inside the polluted session. The holistic perch/burler review
  runs only after the final batch, so it sees green-by-construction code: review is a
  quality/design gate, never a test-fixing mechanism. Deferred-verify chains keep the
  invariant at chain level.
- Rationale: Unbounded self-fix is the Haiku/Go thrashing mode observed in practice;
  zero self-fix wastes the one-line-fix cases.
- Rejected: self-fix until context exhaustion; immediate stuck on first red.

### Batch-report — terse YAML, decision fields only

- Decision: Written by the implementer as its final action, via the file contract, to
  `_lyx/builder/reports/NN-<batch-slug>.yaml`:

  ```yaml
  batch: 02-<batch-slug>
  status: done | stuck
  tests: green | red | skipped   # skipped = deferred-verify intermediate
  stuck_reason: null | "<short>"
  out_of_scope:                  # optional; present only when needed
    - path: <path>
      why: "<one line>"
  ```

  Principle: the report carries only decision fields plus what Go cannot cheaply compute
  itself. `commits` is dropped (git log with card-referencing messages is the authoritative
  source; count is never a check). `files_changed` is dropped (the `poll` verb computes it
  from `git diff` against the start-SHA — authoritative, not agent-claimed). `duration` is
  dropped (Go owns spawn/exit times).
- Rejected: keeping `commits` as informational (unused noise); `files_changed`/`duration`.

### Scope — declared ownership, orchestrator-judged drift (no auto-revert)

- Decision: Scope is a plain **path list with prefix semantics** (files and/or directories;
  a directory covers everything under it). No blind auto-revert of out-of-scope changes:
  the implementer — especially when self-fixing against an incomplete plan — may
  legitimately need to touch unlisted files, and MUST justify each such change in the
  batch-report's `out_of_scope` field. The `poll` verb (Go) computes changed files from git,
  compares against declared scope, and flags drift in the digest; the **orchestrator
  judges** — accept as legitimate fix or demand revert. Unreported drift (changes outside
  scope with no `out_of_scope` entry) is the rot signal.
- Rejected: glob lists (must pin a glob dialect; Planner must write globs flawlessly —
  YAGNI until a real need); prose scope (not mechanically checkable); hard gate with
  `scope: open` escape (the Planner cannot foresee exactly what an incomplete plan missed);
  blind auto-revert (reverts legitimate fixes).

### Model-spec notation (`docs/reference/model-spec.md`)

- Decision: The plan is **model-agnostic** — it carries no model fields (only the
  `oversized` flag, which selects a *role*, not a model). All roles (orchestrator,
  implementer, implementer_oversized, fixer/escalation, evaluator) are configured as
  **model-specs**:
  - **Grammar:** `<alias>[key=value,key=value,...]` — e.g. `implementer: sonnet[effort=high]`.
    Bracket part optional. Escape form `provider:model-id[...]`
    (e.g. `claude:claude-sonnet-4-5[effort=high]`) for models not (yet) in the registry.
  - **Registry:** a dedicated config file `models.yaml` (resolved via
    `hubgeometry.ConfigFile`, like other module config), readable/editable separately,
    mapping alias → engine (which shuttle provider), model string, and **default values**
    for optional params (effort, …). Go ships a small built-in fallback set
    (sonnet/opus/haiku → claude engine) so everything works with no file present; the file
    overrides/extends when it exists.
  - **Newest by default:** the registry passes the provider-side alias through (Claude CLI
    resolves `--model sonnet` to the newest Sonnet) — zero version maintenance. Pinning is
    always deliberate: set an explicit model id in the registry (e.g. steer away from a
    fresh release back to `claude-sonnet-5`), or per-spec via `version=4.5`, which the
    provider engine translates to its id scheme (claudeengine: `sonnet` + `4.5` →
    `claude-sonnet-4-5` — provider naming lives in claudeengine per the Shuttle
    Provider-Seam Invariant).
  - **Precedence — whole-spec replacement:** the most specific config layer that sets a
    role wins **as a unit**: loom's config section (when loom drives builder) > builder.yaml
    default > nothing. No cross-layer param merging — a losing spec contributes nothing.
    Within the winning spec: bracket param > registry default.
  - **Fail loud:** unknown alias, unknown param key, unrecognized provider → loud rejection,
    never silent ignoring (claudeengine already hard-errors on invalid `--effort`).
  - **`context` is not a generic param:** Claude context windows are not tunable; the
    oversized role simply points at a spec whose model/variant has a large window.
    Ollama-style `num_ctx` is out of scope.
- Rejected: per-language default map (per-repo config suffices); model hints in the plan
  (Planner lacks the runtime knowledge; plan stays portable); registry storing explicit
  full ids per release (the maintenance treadmill the alias-passthrough avoids); per-param
  merge across layers (reading three files to know one param's value; stale brackets leak
  invisibly); registry-table lookup for `version=` (reintroduces version tables).

### Default implementer model — Sonnet, per-repo config

- Decision: Sonnet is the default implementer (builder.yaml `implementer: sonnet`); Haiku
  becomes an opt-in per-repo optimization where it is proven (it coped with C# but failed
  at Go self-fixing — red-test diagnosis needs more capability, independent of plan
  detail). No hardcoded models anywhere; builder.yaml holds defaults for every role and
  loom's config section overrides per role.
- Rejected: Haiku default + escalation (first attempt burns a batch on Go-class work);
  per-language model map.

### Where the docs land

- Decision: Standalone `docs/modules/plan-format.md` (the contract has two consumers —
  builder now, Planner later — so it does not live inside a builder module doc) and
  standalone `docs/reference/model-spec.md` (model-specs apply to every role in the stack:
  perch, burler, loom — not just builder). Same commit: surgical fixes to
  `docs/modules/loom.md` (LLM-orchestrator correction, no-DAG wording, Sonnet-default) and
  the roadmap pointer, making the new docs the single source of truth for the format.
- Rejected: a section of a (new) builder module doc (mixes contract with module-internal
  design; the Planner side would have to read builder's doc); model-spec inside
  plan-format.md (wrong home — applies stack-wide).

## Technical context

- **Roadmap position:** `docs/roadmap.md` — builder branches off `shuttle` (does not need
  `perch`); loom needs both. "Plan format + builder" is the named immediate front. This
  task completes the "pinned plan format" precondition; the roadmap update marks that.
- **Stale doc passages to fix (same commit):** `docs/modules/loom.md:97-118` ("Builder is a
  Go loop", "Go drives the plan's batches in dependency order", "a cheaper model by
  default — e.g. Haiku") and the module table line ("builder | Go loop (like perch)").
  General caution from the operator: existing docs are idea collections, not always
  mutually consistent — the pinned contract docs supersede them where they conflict.
- **Existing capabilities to reference (not modify):** `internal/shuttleengine` /
  `claudeengine` already support per-spawn `model` + `effort`
  (`claudeengine/command.go: buildLaunchCmd`, `validateEffort` — hard-errors on invalid
  effort; precedent for fail-loud). Escalation-by-fresh-spawn rationale lives in
  shuttleengine docs.
- **Millhouse precedent for mechanical sizing:** `_plan_validate.py`
  (`plugins/mill/scripts/`, millhouse repo) — `batch-oversized` checks: token estimate =
  sum of referenced-file byte sizes // 4, plus a max-cards cap, run at plan validation.
  The plan-format doc cites this as the intended enforcement shape (implementation is the
  builder/validator task's job).
- **Docs conventions:** `docs/modules/` holds module docs with a Status banner (see
  `loom.md`'s "Status: Design — not built"); the new plan-format doc should carry the same
  kind of banner (contract pinned, consumer not built). `docs/reference/` exists.
- **Terminology note:** "batch-report digest" (what `poll` returns to the orchestrator) is
  builder-design territory; the plan-format doc pins only the on-disk batch-report schema
  and states that consumption happens via Go distillation.

## Constraints

From `CONSTRAINTS.md` (read in full this session):

- **Hub Geometry Invariant** — `_lyx` paths (`_lyx/plan/`, `_lyx/builder/reports/`) are
  stated in the doc as resolving through `internal/hubgeometry` helpers when implemented;
  the doc must not imply raw path construction by other packages.
- **Weft Git Invariant** — the plan and batch-reports are weft overlay: agents write them
  via the junction, Go reads/commits them. The agent commits its own **code** to the host
  repo (commit-per-card) — never the weft. The doc's card/commit sections must preserve
  this asymmetry.
- **Shuttle Provider-Seam Invariant** — model-spec registry data is provider-invariant;
  provider naming schemes (`version=` translation, 1M-variant realization) live in
  `claudeengine` only. `docs/reference/model-spec.md` must state this split.
- **Documentation Lifecycle / CLAUDE.md task-completion rules** — docs updated in the same
  commit as the change; roadmap updated only as milestone completion (this task is a named
  planned milestone: the plan-format precondition for builder).
- **CLI / Cobra Invariant** — not triggered (no code/commands in this task), but the doc's
  `lyx builder` verb references should match the `<module>cli`/`<module>engine` naming
  convention for the future module.

## Testing

Doc-only task — no code, no `go test` surface. Quality gates:

- **Plan review of this task's plan** (mill pipeline) and **review of the docs themselves**
  against this discussion: every Decision above must appear in the rendered docs; the
  worked example in `plan-format.md` must be internally consistent (index ↔ files ↔
  batch-report fields ↔ schema).
- The worked example doubles as the template for builder's future fixture — it must be
  complete enough that copying it into `testdata/` yields a runnable v1 plan.
- Future machine enforcement (explicitly not this task): the plan validator
  (bytes//4 estimate, card cap, `format`/`approved` checks) lands with builder; the doc
  describes those checks so the validator has a spec to implement against.

## Q&A log

- **Q:** Haiku as default implementer? **A:** No — Sonnet default via config; Haiku failed
  at Go self-fixing (OK for C#); per-repo opt-in, no per-language map. Builder-config holds
  defaults; loom's config section overrides.
- **Q:** Exceptions to principle #0 (large refactors)? **A:** Both mechanisms:
  `oversized: true` (→ `implementer_oversized` role) and split-with-`verify: deferred`
  (chain verified by its final batch).
- **Q:** Who fixes red tests? **A:** Bounded in-session self-fix (~2 attempts), then
  `stuck` → orchestrator spawns fresh escalated fixer. Perch/burler review runs only after
  the last batch, on green code.
- **Q:** Is builder a Go loop? **A:** No — corrected: long-lived LLM orchestrator holds the
  loop, drives fat `lyx builder` verbs; Go = verbs + distillation only; orchestrator reads
  digests, never raw prose; recovery is orchestrator judgment.
- **Q:** `verify:` placement? **A:** Per-batch mandatory (may be `deferred`) + per-card
  optional cheap checks.
- **Q:** Card = exactly one commit? **A:** Planning rule only; commit count never a check;
  commit subjects reference batch+card (e.g. `02.3: <what>`) — that is the resume
  mechanism (discard uncommitted, restart card).
- **Q:** Batch-report fields? **A:** `batch, status, tests, stuck_reason` + optional
  `out_of_scope`; drop `commits`/`files_changed`/`duration` (git/Go authoritative).
- **Q:** Scope expression? **A:** Path list, prefix semantics. No auto-revert:
  out-of-scope edits are sometimes legitimate (incomplete plan, self-fix) — must be
  justified in `out_of_scope`; orchestrator judges; unreported drift is the rot signal.
- **Q:** Overview extras? **A:** `format: 1` only ("versioned doc" in the brief meant
  git-committed; the format stamp is an added fail-loud feature, included in v1).
- **Q:** Model-spec grammar? **A:** `alias[key=value,...]` + `provider:model-id[...]`
  escape. Alias resolves to **newest** (provider-side alias passthrough — no version
  treadmill); pin via registry explicit id or `version=X.Y` (engine-translated).
- **Q:** Registry home? **A:** Dedicated `models.yaml` config file + built-in Go fallback
  so it works with no file present.
- **Q:** Param precedence? **A:** Whole-spec replacement: loom > builder.yaml; no
  cross-layer param merge; within winning spec, bracket > registry default.
- **Q:** Can `context` be a bracket param? **A:** No — not tunable for Claude; oversized
  role points at a large-window model/variant; Ollama `num_ctx` out of scope.
- **Q:** How is `oversized` decided? **A:** Author-decided with justification, challenged
  in plan review, **and** mechanically forced: bytes//4 context estimate + card cap at
  validation (millhouse `_plan_validate.py` precedent) — over cap without the flag fails
  loud.
- **Q:** Fixture in this task? **A:** No — builder task writes it; the doc's worked example
  is the template.
- **Q:** Where do docs land? **A:** `docs/modules/plan-format.md` +
  `docs/reference/model-spec.md`; surgical loom.md/roadmap fixes in the same commit.
