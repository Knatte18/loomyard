# Discussion: loom: Discussion producer (interactive interview, auto-mode capable)

```yaml
task: 'loom: Discussion producer (interactive interview, auto-mode capable)'
slug: loom-discussion-producer
status: discussing
parent: main
```

## Problem

loom (the Go phase machine, `lyx loom run`) drives a task through Preflight →
**Discussion** → Plan → Builder → Raddle → Finalize. Preflight is built
(`internal/loomengine`); the contracts the Discussion phase produces are pinned
(`docs/reference/discussion-format.md`). The **Discussion phase is loom's one
interactive phase** — the loom analogue of millhouse's `mill-start`: an agent
interviews the operator (or self-answers) about a task and distils the outcome
into the pinned `_lyx/discussion/` artifact that the Plan producer consumes.

This task builds the **Discussion producer**: the prompt + profile + Go seam that
turns a task slug into a `shuttleengine.Spec` a caller runs to produce the two
discussion files. It does **not** build the phase machine that drives it (that is
milestone 12.5). Why now: the contract it targets (`discussion-format.md`) landed,
so per roadmap milestone 12.3 the producer is the next independently
buildable/testable piece on the path to loom.

**Design principle in force** (overview.md Principle 7): the producer is judgment,
so it is an LLM prompt — but everything deterministic around it (path geometry,
config, model resolution, the Spec, output-file contract) is Go. The producer is
**not a module** (loom.md module-decomposition): "just a prompt + profile fed to
`shuttle.Run`," with per-phase profiles living in loom → `internal/loomengine`.

## Scope

**In:**

- A new Discussion producer in `internal/loomengine` (beside `Preflight`):
  - An embedded `discussion-template.md` prompt (the interview instructions,
    distilled from `mill-start`) with `internal/stencil` markers.
  - A `stencil`-based prompt composer.
  - A `Profile`/params type and a `DiscussionSpec(...) (shuttleengine.Spec, error)`
    factory that composes the prompt, names the two output files, sets
    `Interactive = !autonomous`, `Role = "discussion"`, and resolves the model.
- A **`loom.yaml`** config file (new config module) holding the discussion role
  model-spec and a discussion timeout knob, materialized/reconciled through the
  same mechanism as `builder.yaml`:
  - `internal/loomengine/config.go` — `Config` type + `LoadConfig` (validates the
    role model-spec grammar via `modelspec.Parse` at load, like `builderengine`).
    Config fields: `Discussion string` (yaml `discussion`) and
    `DiscussionTimeoutMin int` (yaml `discussion_timeout_min`).
  - `internal/loomengine/template.yaml` — the seed
    (`discussion: opus[effort=high]`, `discussion_timeout_min: 480`).
  - A `ConfigTemplate()` accessor (embed).
  - Register `{Name: "loom", Template: loomengine.ConfigTemplate}` in
    `internal/configreg/configreg.go` `Modules()`.
- `internal/hubgeometry` accessor(s) for the `_lyx/discussion/` directory and its
  two files (Hub Geometry Invariant: hubgeometry owns all `_lyx` paths).
- Unit tests for every Go piece (prompt composition incl. auto-vs-interactive
  block, profile/config validation, Spec factory output-file + Interactive
  mapping + model resolution, hubgeometry accessor, configreg membership).
- Doc updates in the same commit (overview.md config-module list / loom.md /
  CONSTRAINTS.md if a new invariant is added; see Constraints).

**Out:**

- The phase machine that *drives* the producer — sequencing, `shuttle.Run`
  invocation, multi-turn interactive `asking`/yield handling, resume,
  crash-recovery, pause. **Milestone 12.5 (`loom-phase-machine`).**
- The Discussion-review gate (perch/burler) and the `support-log.md`
  **Review-rounds** ledger *content* — the producer only seeds an empty
  Review-rounds section; the gate populates it later.
- Any `lyx` subcommand / CLI for the producer (producers are not modules). No
  live-tmux / mux end-to-end harness in this task (would break isolation
  testing).
- The Plan producer (milestone 12.4) — the sibling, separate task.
- Fetching the task title/description in Go — the **agent** reads the board live
  via the slug; the composer takes only the slug string.
- Changing the pinned `discussion-format.md` contract.

## Decisions

### output-contract-follows-pinned-doc

- Decision: The producer targets the **pinned contract**, not the board brief's
  shorthand: a `_lyx/discussion/` **directory with two files** —
  `decision-record.md` (Plan's sole input) and `support-log.md` (review-gate
  only). The brief's `discussion.md` / `discussion-log.md` names are stale.
- Rationale: `docs/reference/discussion-format.md` is a durable, ✅-landed,
  pinned contract; a durable contract outranks a board brief. Two files (not two
  sections) is a hard filesystem boundary so Plan cannot ingest the raw log.
- Rejected: honouring the brief's names (would require repinning a landed
  contract — larger blast radius, contradicts the spec).

### producer-lives-in-loomengine-no-module

- Decision: The producer is Go files in `internal/loomengine` (embed + composer +
  profile + Spec factory), **no new module, no CLI**.
- Rationale: loom.md's module-decomposition table calls producers "not modules —
  just a prompt + profile fed to `shuttle.Run`," and says per-phase profiles
  "live in loom." Preflight already lives in `loomengine`.
- Rejected: a dedicated `internal/discussionengine` package (mirrors
  `burlerengine`) — symmetric but implies a module/CLI the design says must not
  exist; scope creep.

### producer-contract-only

- Decision: This task ships the producer *contract* — prompt template, composer,
  profile, `Spec` factory, `loom.yaml`, hubgeometry accessor, tests — and defers
  the driving loop, review gate, resume, and multi-turn `asking` handling to the
  phase machine (12.5).
- Rationale: keeps the piece independently buildable/**testable in isolation**
  (the brief's own requirement) without pulling in mux/live-tmux; matches the
  roadmap's decomposition.
- Rejected: bundling a standalone `shuttle.Run` driver for end-to-end exercise —
  overlaps 12.5, drags in live infrastructure, breaks isolation testing.

### auto-mode-is-templated-conditional-block

- Decision: One template with an `{auto_mode_rules}` (and related) marker the
  composer fills from an `autonomous bool`. Interactive → "ask the operator
  conversationally in the pane, in batches, recommend an answer." Autonomous
  (`--auto`) → "make your own best guess per question, proceed, record each pick +
  rationale in the support-log Question ledger."
- Rationale: mirrors `burler`'s conditional prompt blocks; both variants are
  unit-testable from one source of truth.
- Rejected: two separate template files (drift risk); a single unconditional
  prompt leaning on the guardrail (weakest, least explicit for the auto path).

### autonomous-maps-to-shuttle-interactive-false

- Decision: `--auto` ⇔ `Spec.Interactive = false` (autonomous); normal discussion
  ⇔ `Spec.Interactive = true`. The factory sets `Interactive = !autonomous`.
- Rationale: `shuttleengine.Spec.Interactive == false` adds
  `--dangerously-skip-permissions` **and denies the `AskUserQuestion` PreToolUse**
  — i.e. the agent may not block on a question dialog and must self-answer. This
  is exactly loom.md's `--auto` semantics. It is **not** about human presence — a
  human may still watch either way; the axis is "may the agent block to ask."
- Rejected: treating auto as "no human" (a misreading — corrected during
  discussion).

### slug-is-the-only-input-agent-reads-board

- Decision: The producer's input is the **slug** only. The composer injects the
  slug; the prompt instructs the agent to read its task live via
  `lyx board get '{"slug":"<slug>"}'` (returns `{"task": {...}}`), and to explore
  the codebase itself.
- Rationale: the board is the source of truth for title/description ("SLUG er
  nok"); keeping the composer a pure function of a slug string preserves
  isolation testing while the agent still explores live, exactly as `mill-start`
  does.
- Rejected: fetching title/description in Go and injecting them (adds a live board
  dependency to the composer, breaking pure/isolated composition).

### model-in-loom-yaml

- Decision: The discussion agent's model lives in **`loom.yaml`** as a role
  model-spec (`discussion: opus[effort=high]`), resolved via `internal/modelspec`
  and mapped to `Spec.Model/Effort/Version` exactly as `builderengine/roles.go`
  does. This task introduces `loom.yaml` as a new config module.
- Rationale: this producer *is* part of loom, so its agent-model choice belongs in
  loom's config — "hvor ellers?" The model-spec bracket notation
  (`sonnet[effort=high]`) is the landed convention (`docs/reference/model-spec.md`).
  Default `opus[effort=high]` reflects that design-interview judgment is the
  hardest reasoning in the pipeline (`builder.yaml` already uses `opus[effort=high]`
  for `recovery`).
- Rejected: a hardcoded model / profile-default constant with no config seam
  (diverges from the roles pattern; the user wants it in loom's config now, not
  deferred).

### discussion-timeout-knob-is-live

- Decision: `loom.yaml` carries `discussion_timeout_min` (Config
  `DiscussionTimeoutMin int`), default **480**, and the factory maps it to
  `Spec.Timeout = time.Duration(DiscussionTimeoutMin) * time.Minute` — the knob is
  live, not decorative. A factory test asserts the mapping.
- Rationale: an interactive design interview with a human in the loop runs long;
  a discussion-specific timeout must be able to exceed shuttle's global
  `RunTimeoutMin`. If the factory left `Spec.Timeout = 0`, the knob would be dead
  (shuttle's `Spec.validate` replaces `0` with the global `RunTimeoutMin`), so the
  mapping must be explicit and tested. Minutes-int → `time.Duration` mirrors
  `builder.yaml`'s `*_timeout_min` fields.
- Rejected: dropping the knob and relying on the global `RunTimeoutMin` (a long
  interactive interview could hit the global default and time out mid-session);
  leaving the mapping as "defaults or the knob" (ambiguous — the r1 review gap).

### mill-start-reuse-discipline-not-plumbing

- Decision: The prompt ports `mill-start`'s **interviewing discipline** and
  **principles**, and the output-writing instructions — and drops everything now
  owned by Go/perch.
- Port: relentless **batched** interview across categories (Scope, Constraints,
  Architecture, Edge cases, Security-if-relevant, Testing); recommend an answer
  per question; propose 2–3 approaches with trade-offs, leading with a
  recommendation; challenge the problem not just the solution; YAGNI;
  **design full scope (no MVP phases / "add later")**; explore the codebase
  before asking; write both contract files.
- Drop: board/wiki/status plumbing, phase transitions, git commits, the
  discussion-review loop, `mill-receiving-review`, and all skill-orchestration
  prose — those are Go's or perch's job now.
- Rationale: the interview discipline is the hard-won value; the plumbing is
  exactly what the loom inversion moves out of prompts.
- Rejected: a thin "interview and write two files" prompt (discards the
  discipline, less reliable).

### interactive-qa-is-conversational-pane-text

- Decision: In interactive mode the agent asks the operator as ordinary
  numbered-list **conversational pane text** and reads the typed reply; it does
  **not** use `AskUserQuestion`.
- Rationale: `shuttle`'s `wait.go` classifies `OutcomeAsking` at **any** turn-end
  where the output files are still unmet — so a conversational-pane question and
  an `AskUserQuestion` call **both** yield `OutcomeAsking` per turn; that per-turn
  yield is not the discriminator. The real difference is **answerability on
  resume**: the phase machine (12.5) answers a yielded turn by `shuttle`'s `Send`
  (typed text into the pane), which a human's typed pane reply can carry forward,
  but which **cannot** dismiss/answer a modal `AskUserQuestion` dialog. So
  conversational pane text is the channel the resume mechanism can actually drive.
  (Note: `AskUserQuestion` is *not* inherently banned — that ban was a
  millhouse/VS-Code leftover; we do not carry that moralizing into the prompt.)
  The multi-turn asking/resume loop itself is 12.5's job, not this task's.
- Rejected: `AskUserQuestion` for in-interview questions (a modal dialog the
  `Send`-based resume path cannot answer).

### producer-seeds-support-log-sections

- Decision: The prompt instructs the agent to write `support-log.md`'s
  **Interview**, **Rejected alternatives**, and **Question ledger** sections, and
  to seed an **empty "Review rounds"** section (header + `_No rounds yet._`
  placeholder). It writes `decision-record.md`'s seven required sections (Goal,
  Scope, Decisions, Constraints, Auto-mode assumptions, Open risks, Acceptance
  criteria) plus the optional "Notes for the plan writer."
- The prompt **must also encode `discussion-format.md`'s compaction rules**:
  `decision-record.md` Decisions carry **Decision + Rationale only** — rejected
  alternatives go to `support-log.md`'s Rejected alternatives section, **not** the
  record; must-cover test scenarios go under Acceptance criteria (no standalone
  Testing section); the record is terse structured prose with no italic
  prose-coaching. This is prompt-instruction discipline the section-completeness
  validation checklist does not enforce, so a compliant-but-wrong record could
  otherwise re-litigate rejected options in the Plan producer's sole input.
- Rationale: keeps both files contract-complete (per `discussion-format.md`
  validation checklist) without the producer owning review state; the perch gate
  appends rounds later (authorship of Review-rounds content is an explicit
  milestone-12 detail the contract leaves open).
- Rejected: omitting Review rounds entirely (a section-completeness validator
  could flag it; appending to a missing header is messier).

## Technical context

Everything below already exists in the repo unless marked **(new)**.

**Producer pattern to mirror** — every engine ships: an embedded `*-template.md`
prompt, a `template.go` (`//go:embed`) accessor, a `prompt.go` composer over
`internal/stencil`, a `Profile`/params + `validate`, and per-file unit tests.
Concrete references:
- `internal/burlerengine/prompt.go` — `composePrompt` builds a `map[string]string`
  of marker values and calls `stencil.Fill(template, values)`. Conditional prose
  blocks (`fixScopeRules`, `clusterRulesBlock`) are the model for `auto_mode_rules`.
- `internal/burlerengine/template.go` — the `//go:embed review-prompt-template.md`
  accessor pattern.
- `internal/stencil/stencil.go` — `stencil.Fill(template []byte, values map[string]string)`
  the marker engine (verify exact marker syntax there).

**Shuttle seam** (`internal/shuttleengine`), the factory's output target:
- `Spec` (spec.go): the factory sets `Prompt`, `OutputFiles` (the two discussion
  files — **both** must exist for the run to be "done"; that is the whole return
  channel), `Interactive = !autonomous`, `Role = "discussion"`, `Model/Effort/Version`
  from the resolved model-spec, `Timeout = time.Duration(cfg.DiscussionTimeoutMin) * time.Minute`
  (see the `discussion-timeout-knob-is-live` decision), and leaves `Display` at
  its default (`AnchorBelowParent`). `shuttle` **never templates prompt content**
  — the caller composes it (dumb transport). `Spec.validate` **rejects
  pre-existing OutputFiles** — a re-run/resume with stale discussion files fails
  loud there, which is correct and is the phase machine's cleanup concern, not the
  producer's.
- `Interactive` doc (spec.go lines ~64–70): `false` (autonomous) adds
  `--dangerously-skip-permissions` + the `AskUserQuestion` deny; the `Agent`-tool
  deny is on in both modes.

**Model resolution** — mirror `internal/builderengine/roles.go` + `config.go`:
`modelspec.Parse` at config load (fail-loud on a typo), and at the spawn site map
`resolved.Model → Spec.Model`, `resolved.Params["effort"] → Spec.Effort`,
`resolved.Params["version"] → Spec.Version`. `opus` is a built-in alias.

**Config wiring** — mirror `builderengine`:
- `internal/builderengine/config.go` — `Config` struct mirrors `template.yaml`
  keys; `LoadConfig` uses `configengine.Load` with `ConfigTemplate()` and validates
  each role model-spec via `modelspec.Parse`.
- `internal/builderengine/template.yaml` — the seed shape.
- `internal/configreg/configreg.go` — `Modules()` returns `[]Module{{Name, Template}}`;
  add the `loom` entry. Note `configreg` already imports `builderengine`; add a
  `loomengine` import (no cycle — `loomengine` imports `configengine`/`modelspec`,
  not `configreg`).

**Hub geometry** (`internal/hubgeometry/hubgeometry.go`) **(new accessor)**:
- Existing: `LyxDir()` (`Cwd/_lyx`), `LoomStatusFile()` (`WorktreeRoot/_lyx/status.json`),
  and private dir helpers for `perch`/`plan`/`builder` (`filepath.Join(baseDir, "_lyx", "<name>")`).
- Add a `DiscussionDir()` (and/or `DiscussionDecisionRecord()` / `DiscussionSupportLog()`)
  anchored on `WorktreeRoot` like `LoomStatusFile` (the discussion artifact is the
  true per-worktree-root one, not a per-subdir copy). `_lyx/discussion/` is durable
  weft-synced state. Filenames: `decision-record.md`, `support-log.md`.

**Board read** (agent-side, in the prompt, not Go): `lyx board get '{"slug":"<slug>"}'`
→ `{"task": {...}}`; `{"task": null}` if not found — instruct the agent to stop and
report if its task is missing.

**Suggested factory signature** (plan may refine):
`func DiscussionSpec(layout *hubgeometry.Layout, cfg Config, reg modelspec.Registry, slug string, autonomous bool) (shuttleengine.Spec, error)`
— resolve the model-spec, compose the prompt (slug + auto block + the two absolute
output paths + board-read + write-both-files instructions), set the fields above.

## Constraints

From `CONSTRAINTS.md` (read this session) and the design docs:

- **Hub Geometry Invariant** — all `_lyx`/geometry paths resolve through
  `internal/hubgeometry`. The `_lyx/discussion/` paths **must** be added there;
  do not construct them ad hoc in `loomengine`. (No raw `os.Getwd` /
  `git rev-parse` outside `hubgeometry`/`main.go`.)
- **CLI / Cobra Invariant** — this task adds **no** cobra command, so the
  `Short`-on-every-command / help-tree tests are not triggered. Adding a config
  *module* to `configreg` is config wiring, not a CLI command; confirm no CLI
  seam is implied.
- **Config reconcile** — `loom.yaml` joins the reconciled set; follow the
  `builder.yaml` template/reconcile shape exactly. `models.yaml` is seed-only;
  `loom.yaml` is normal reconciled config (not seed-only) unless the plan finds a
  reason otherwise.
- **Package naming** (`<module>cli`/`<module>engine`) — loom's engine is
  `internal/loomengine` (already exists); the producer lives here, no new package.
- **Documentation Lifecycle** — update `docs/overview.md` (config-module list /
  the loom module entry) and `docs/modules/loom.md` (the Discussion producer is
  now built as prompt+profile in `loomengine`; `loom.yaml` exists) in the **same
  commit**. If a genuinely new cross-cutting invariant emerges (none expected),
  record it in `CONSTRAINTS.md`. `docs/roadmap.md`: mark milestone 12.3
  (Discussion producer) progress only if it is completed by this task.
- **Model-spec** — role strings use the pinned bracket notation
  (`docs/reference/model-spec.md`); validate at config load, fail loud on a typo.

## Testing

Per-file unit tests next to the source (repo convention: `store.go` ↔
`store_test.go`). No LLM-in-the-loop test — the producer is exercised as pure Go
over fixtures.

- **Prompt composition** (`discussion` producer): all markers filled (no leftover
  marker tokens); the **auto** block and the **interactive** block each render
  correctly for `autonomous = true/false` and are mutually exclusive; the slug and
  both absolute output paths appear; the board-read and write-both-files
  instructions are present. **TDD candidate.**
- **Spec factory:** `OutputFiles` equals exactly the two `_lyx/discussion/` paths
  (absolute, via `hubgeometry`); `Interactive == !autonomous`; `Role == "discussion"`;
  resolved model-spec maps to `Model/Effort/Version`;
  `Spec.Timeout == time.Duration(cfg.DiscussionTimeoutMin) * time.Minute` (assert
  the knob is live — a non-default `DiscussionTimeoutMin` produces a matching
  non-zero `Timeout`, not `0`); the composed prompt is non-empty. A pre-existing output file makes a subsequent `shuttle` run fail — but
  that is `shuttle`'s tested behaviour, not re-tested here; the factory itself does
  not stat the files.
- **Config load** (`loom.yaml`): valid `discussion` role parses; a **typo'd
  model-spec fails loud at load** (mirror `builderengine` config tests); the
  timeout knob parses; unknown keys rejected (strict load). **TDD candidate.**
- **hubgeometry accessor:** `DiscussionDir()` / file accessors return
  `WorktreeRoot/_lyx/discussion/{decision-record.md, support-log.md}`; anchored on
  `WorktreeRoot` (not `Cwd`), matching `LoomStatusFile`'s test.
- **configreg membership:** `Modules()` includes a `loom` entry with a non-nil
  `Template` (extend the existing `configreg_test.go`).
- Full suite green via the Go build/test skill (`golang-build` / `golang-testing`)
  incl. the `hubgeometry` enforcement test and any config-reconcile test that
  enumerates modules.

## Q&A log

- **Q:** Brief says `discussion.md`/`discussion-log.md`; contract says
  `_lyx/discussion/{decision-record.md, support-log.md}`. Which governs? **A:**
  The pinned contract governs; the brief's names are stale.
- **Q:** Where does the code live? **A:** `internal/loomengine`, no new
  module/CLI.
- **Q:** Scope vs the phase machine? **A:** Producer contract only; defer the
  driving loop / review gate / resume to milestone 12.5.
- **Q:** How does the prompt express auto vs interactive? **A:** One template, a
  templated conditional block filled from an `autonomous` flag.
- **Q:** Does `Interactive=false` mean no human present? **A:** No — it means the
  agent may not block on `AskUserQuestion` and must self-answer (plus
  `--dangerously-skip-permissions`). A human may still watch.
- **Q:** How does the task intent reach the producer? **A:** Slug only; the agent
  reads the board live via `lyx board get`. The composer takes just the slug.
- **Q:** Where does the discussion agent's model come from? **A:** From
  `loom.yaml` (this is part of loom), as a role model-spec `discussion:
  opus[effort=high]`, resolved via `modelspec` — "hvor ellers?"
- **Q:** How much of `mill-start` to port? **A:** The interviewing discipline and
  principles + output-writing; drop all board/status/git/review plumbing.
- **Q:** How does the agent ask the operator? **A:** Conversational pane text
  (numbered lists), never `AskUserQuestion` (which shuttle treats as a yield).
- **Q:** Which `support-log.md` sections does the producer write? **A:** Interview,
  Rejected alternatives, Question ledger; seed an empty Review-rounds section for
  the gate.
- **Q:** (r1 review gap) Does the `loom.yaml` discussion timeout knob actually map
  to `Spec.Timeout`? **A:** Yes — make it live: `discussion_timeout_min`
  (`DiscussionTimeoutMin int`, default 480) → `Spec.Timeout =
  time.Duration(mins) * time.Minute`, with a factory test. Not dropped (a long
  interactive interview could hit shuttle's global `RunTimeoutMin` and time out).
```
