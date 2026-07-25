# Discussion: loom: Planner producer

```yaml
task: 'loom: Planner producer'
slug: loom-planner
status: discussing
parent: main
```

## Problem

loom (the phased orchestrator, see `manifest/designs/loom.md`) drives a task through
Discussion → Plan → Builder → Raddle → Finalize, each producing phase a **prompt + profile
fed to `shuttle.Run`** rather than a Go module. The **Discussion producer is already built**
(`internal/loomengine`: `discussion-template.md`, `prompt.go`, `discussion.go`,
`DiscussionSpec(...)`); it writes `_lyx/discussion/decision-record.md` (the Plan producer's
sole input) and `_lyx/discussion/support-log.md` (read only by the Discussion-review gate).

This task builds the **next producer in that chain: the Planner producer.** Its whole job is
"read the decision record, write a plan-format-v3 flat-card-list plan into `_lyx/plan/`."
Nothing else — no human interaction, no review logic of its own (review is `perch`/`burler`'s
separate job). It is the direct sibling of the Discussion producer and is built the same way:
a template + embed + a `Spec` factory in `internal/loomengine`, plus `loom.yaml` knobs.

**Why now:** the Discussion producer and `plan-format-v3` (the flat-card schema, see
`docs/reference/plan-format-v3.md`) are both done. The Planner producer is the piece that
turns a discussion into a runnable plan, and it is a self-contained, testable unit that can
land well before the loom phase-machine, `perch`, or `webster` exist.

## Scope

**In:**

- New `internal/loomengine` files mirroring the Discussion producer:
  - `plan-template.md` — the embedded prompt (carries the **compact** plan-format-v3 spec inline).
  - `plantemplate.go` — `//go:embed plan-template.md` accessor.
  - `plan.go` — `PlanSpec(layout, cfg, reg) (shuttleengine.Spec, error)` factory + its
    prompt composer (`composePlanPrompt`).
  - Test files mirroring `discussion_test.go` / `prompt_test.go`.
- `loom.yaml` config: add `plan` (model-spec) and `plan_timeout_min` keys to `template.yaml`,
  the `Config` struct, and `LoadConfig`'s grammar validation. **Same edit must refresh the
  prose**: `config.go`'s `Config` type godoc and `LoadConfig`'s grammar-validation comment
  currently name only the "discussion role model-spec"; add the `plan` key to both (same-commit
  docs rule) so the doc doesn't lie about which keys the config carries.
- `internal/hubgeometry`: add `Layout.PlanDir()` (WorktreeRoot-anchored) and
  `Layout.PlanOverview()` helpers so the `00-overview.md` filename stays owned by hubgeometry
  (mirroring `DiscussionDir()` / `DiscussionDecisionRecord()`); fix the stale "plan-format v1"
  wording in the existing free `PlanDir(baseDir)` godoc.
- Doc-lifecycle work (per `loom-planner.md`'s own stated lifecycle and CLAUDE.md's
  same-commit docs rule): fold the durable design into `loomengine` godoc, flip
  `loom.md`'s module-table Planner row to built, move the roadmap item Planned→Done, and
  **delete `manifest/designs/loom-planner.md`**.

**Out:**

- The **loom phase machine** — spawning the Planner Spec through `shuttle.Run`, reacting to
  its outcome, sequencing phases. `PlanSpec` is a pure composer that returns a `Spec`; nothing
  in this task runs it. (Same boundary as `DiscussionSpec`.)
- **`perch`/`burler`** (the review gate) and any **Plan-review** logic. The Planner has no
  review logic of its own.
- The **`approved: false` → `true` flip**. The producer writes `approved: false`; flipping it
  is the future loom orchestrator's job (see Decision `approved-flag-flip`). Not built here.
- **`webster`** (the plan *consumer*) and the plan **validator** (the 14 `plan-format-v3`
  checks land with webster, not here).
- A separate compact **reference doc** for the plan format — the compact spec lives inline in
  `plan-template.md` only (see Decision `compact-format-inline`).
- Any **interactive** Planner mode — the Planner is autonomous-only.

## Decisions

### done-sentinel-overview-last

- Decision: `PlanSpec.OutputFiles = []string{ layout.PlanOverview() }` — i.e. the single
  file `_lyx/plan/00-overview.md`. The prompt **mandates writing `00-overview.md` LAST**,
  after every `NN-<card-slug>.md` card file.
- Rationale: `shuttle` marks a run "done" by bare existence of every named `OutputFiles`
  entry (see `internal/shuttleengine/spec.go`). The plan is a directory of N card files whose
  names the planner invents, so shuttle cannot know them up front. `00-overview.md` is the one
  deterministic, always-present file; writing it last makes its existence a reliable
  "plan complete" signal — the same "write the output file(s) at the very end" discipline the
  Discussion producer uses.
- Rejected: a separate `.done` marker file (adds a non-schema file v3 doesn't define);
  naming the card files in `OutputFiles` (impossible — names unknown before the run).

### input-decision-record-only

- Decision: the Planner's **sole input is `_lyx/discussion/decision-record.md`** (via
  `layout.DiscussionDecisionRecord()`). It never reads `support-log.md` and never reads the
  board. `PlanSpec` therefore takes **no `slug`** and does no board read.
- Rationale: the built Discussion producer pins `decision-record.md` as "the Plan producer's
  **sole** input … it never reads anything else out of `_lyx/discussion/`" and guarantees it is
  self-contained. A second input channel is exactly what `loom.md` warns destroys phase
  independence. The support log is the Discussion-review gate's input, not the Planner's.
- Rejected: passing a `slug` and reading the board for extra context (contradicts the
  self-contained decision-record contract, reintroduces a second input).

### approved-false-draft

- Decision: the Planner writes `approved: false` in `00-overview.md`'s frontmatter.
- Rationale: the plan has not been through the (separate, not-yet-built) Plan-review gate when
  the producer finishes. "The Planner has no review logic of its own" — it must never
  self-approve. `plan-format-v3`'s `plan-unapproved` check (a future webster/validator gate)
  refuses to run an unapproved plan, which is the intended safety: an unreviewed draft cannot
  be executed.
- Rejected: `approved: true` (lets the producer self-certify a plan nothing reviewed).

### approved-flag-flip

- Decision: the `approved: false` → `true` flip is done by the **loom orchestrator (Go)**,
  after `perch` returns `APPROVED` — **not** by `perch` and **not** by the producer. `perch`
  stays a generic gate whose only return value is `APPROVED | stuck`; it knows nothing about
  plan-format frontmatter. This task does **not** implement the flip (loom/perch are unbuilt);
  it only records the contract so the future tasks wire it correctly and so `approved: false`
  is justified.
- Rationale: flipping durable artifact state at a phase boundary is a deterministic Go
  orchestration act — the loom thesis (Go owns the machine and state mutation; agents ride the
  file contract; the gate engine stays profile-generic). Putting the flip in `perch` would leak
  plan-format knowledge into a generic gate; putting it in the agent would let a non-deterministic
  LLM mutate approval state.
- Rejected: `perch` flips it (couples the generic gate to plan-format); the producer flips it
  (agent self-approval — the thing `approved: false` exists to prevent).

### compact-format-inline

- Decision: the plan-format-v3 spec the agent writes against lives **inline in
  `plan-template.md`** as a **compact** description — a literal fill-in skeleton for
  `00-overview.md` plus one `NN-card.md`, with terse per-field rules — in the style of mill's
  `plan-overview.md` / `plan-batch.md` templates. **No** separate reference doc is created.
  `docs/reference/plan-format-v3.md` stays a **development-only reference** and is NOT what the
  producer consumes.
- Rationale: `plan-format-v3.md` is ~440 lines (full grammar, 14 validation checks, rationale
  prose, a long worked example) — far too heavy for a prompt and it would drift if inlined
  verbatim or if the agent were told to read it at runtime. mill proves the compact
  fill-in-skeleton approach works: concrete skeletons + terse field rules are what an autonomous
  writer needs. The producer must still emit **every required v3 field**; "compact" trims the
  *description* (drop the validator-check catalogue and rationale, keep the grammar + a minimal
  example), never the schema.
- Rejected: point the agent at the 440-line doc (drift + prompt bloat; explicitly rejected by
  the operator); extract a new shared compact `docs/reference/plan-format.md` for producer +
  future webster (YAGNI — webster isn't built; second file to keep in sync).

### autonomous-only

- Decision: the Planner is **autonomous-only**. `PlanSpec(layout, cfg, reg)` has no
  `autonomous` param; the returned `Spec` always has `Interactive: false` (so shuttle adds
  `--dangerously-skip-permissions` and the `AskUserQuestion` PreToolUse deny), `Role: "plan"`.
  The prompt carries a "no operator present; never call `AskUserQuestion`; make best-judgment
  calls" guard, but — unlike the Discussion producer — there is **no interview** and no
  interactive/autonomous branch (`modeRules` has no analogue here).
- Rationale: the design (`loom-planner.md`) is explicit: "No human interaction: autonomous."
  The Planner reads, explores, and writes; it never asks. Carrying an unused interactive branch
  would be dead complexity.
- Rejected: keep an `autonomous bool` param for symmetry with `DiscussionSpec` (it would always
  be `true` — dead parameter).

### files-mirror-discussion-producer

- Decision: file/naming layout mirrors the Discussion producer exactly: `plan.go` (factory +
  `composePlanPrompt`), `plan-template.md` (embedded prompt), `plantemplate.go`
  (`//go:embed` accessor, `planTemplate` var). Config keys `plan` / `plan_timeout_min`.
  `Layout.PlanOverview()` mirrors `Layout.DiscussionDecisionRecord()`.
- Rationale: the Discussion producer is the explicit model; matching its shape keeps the two
  producers parallel and reviewable, and reuses the established `stencil.Fill` + `modelspec`
  + `shuttleengine.Spec` seams.
- Rejected: none of substance — this is the established pattern.

## Technical context

**The Discussion producer is the blueprint — read it first:**

- `internal/loomengine/discussion.go` — `DiscussionSpec(layout, cfg, reg, slug, autonomous)`:
  resolves the role model-spec via `modelspec.Parse` + `reg.Resolve`, names output paths from
  `layout`, composes the prompt, returns a `shuttleengine.Spec`. The Resolved→Spec mapping is
  `Spec.Model = resolved.Model`, `Spec.Effort = resolved.Params["effort"]`,
  `Spec.Version = resolved.Params["version"]`. **`PlanSpec` copies this exactly**, minus the
  `slug`/`autonomous` params and with `Interactive: false` hardcoded, `Role: "plan"`, timeout
  from `cfg.PlanTimeoutMin`.
- `internal/loomengine/prompt.go` — `composePrompt` builds a `map[string]string` of markers and
  calls `stencil.Fill(discussionTemplate, values)`. `composePlanPrompt` does the same with the
  plan template's markers (see below). Note `modeRules` is discussion-only — **do not** port it.
- `internal/loomengine/discussion-template.md` — shows the template style: static prose around
  top-level `{{.X}}` markers, **no `{{if}}`/`{{range}}`** (a required marker inside a conditional
  renders silently blank — see `internal/stencil/stencil.go`). `stencil.Fill` requires every
  marker non-empty.
- `internal/loomengine/prompttemplate.go` — the one-line `//go:embed` accessor pattern to copy
  into `plantemplate.go`.
- `internal/loomengine/config.go` + `template.yaml` + `configtemplate.go` — how a role model-spec
  + timeout knob are declared, embedded, loaded, and grammar-validated (`modelspec.Parse` at load
  time so a typo fails loud). Add `plan` / `plan_timeout_min` alongside `discussion` /
  `discussion_timeout_min`.

**Markers the plan template needs** (all filled by `composePlanPrompt`, all non-empty):

- `{{.decision_record_path}}` — absolute path to the input (`layout.DiscussionDecisionRecord()`).
- `{{.plan_dir}}` — absolute `_lyx/plan/` dir the agent creates and writes into
  (`layout.PlanDir()`).
- `{{.overview_path}}` — absolute `_lyx/plan/00-overview.md` (`layout.PlanOverview()`), named so
  the prompt's "write this file LAST" instruction points at a concrete path.

  (Exact marker set is the plan writer's call; these three cover the contract. Keep them all
  top-level and non-empty per the stencil rule.)

**hubgeometry (see `internal/hubgeometry/hubgeometry.go`):**

- Existing free function `PlanDir(baseDir string)` returns `filepath.Join(baseDir, LyxDirName,
  "plan")` = `_lyx/plan`. Its godoc says "plan-format **v1**" — stale; the directory is
  format-agnostic (builder v2 and loom v3 both use it). Fix the wording (don't pin a format
  version).
- Add `Layout.PlanDir()` returning `PlanDir(l.WorktreeRoot)` (delegate to the free function — one
  definition) and `Layout.PlanOverview()` returning `filepath.Join(l.PlanDir(), "00-overview.md")`,
  mirroring `DiscussionDir()`/`DiscussionDecisionRecord()`. WorktreeRoot-anchored, so a caller in
  a subdirectory still resolves the single plan dir at the worktree root. The `00-overview.md`
  literal stays owned by hubgeometry (do not construct it in `loomengine`).
- **Hub Geometry Invariant** (`CONSTRAINTS.md`): geometry tokens (`_lyx`, `plan` under it) and
  cwd/geometry resolution live only in `internal/hubgeometry`. Enforced by
  `internal/hubgeometry/enforcement_test.go`. This is why the overview path is a Layout method,
  not a `filepath.Join` in `loomengine`.

**shuttle contract (see `internal/shuttleengine/spec.go`):**

- "Done" = bare existence of every `OutputFiles` entry. Entries must NOT already exist at
  validate time (a pre-existing file classifies the run done on turn 1) — so `00-overview.md`
  must not pre-exist, and the prompt must tell the agent to **create `_lyx/plan/` first if
  absent** (the Discussion producer's Step 5 does the analogous thing for `_lyx/discussion/`).
- `Spec.Effort`/`Version` are provider vocabulary — `validate` does not inspect them; the engine
  validates. Field mapping comes straight from the resolved model-spec.
- **Pre-flight / cleanup is NOT this task's job — do not add it to `PlanSpec`.** `PlanSpec` is a
  pure composer: like `DiscussionSpec` it does **not** stat its input (`decision-record.md`) and
  does **not** stat/clean its output dir. Two sequencing concerns are therefore the future loom
  **phase machine**'s responsibility, out of scope here — but named so the plan writer doesn't
  bolt them onto the producer:
  - **Missing/empty input:** a missing or empty `decision-record.md` would surface only as an
    agent-runtime failure, not a loud pre-flight error. Verifying the input exists before
    spawning the Planner is the phase machine's job (`DiscussionSpec` leaves the analogous board
    read to runtime the same way).
  - **Re-run collision on `_lyx/plan/`:** `_lyx/plan/` is the same directory builder's plan
    artifacts use, and shuttle `validate` rejects a **pre-existing** `00-overview.md`. A re-run
    or a leftover file makes the run fail at `validate`. Cleaning/rotating a stale plan dir is
    the phase machine's job (again mirroring the Discussion producer, which leaves
    `_lyx/discussion/` freshness to its driver); `PlanSpec` neither stats nor removes anything.

**plan-format-v3 essentials the compact prompt must convey** (distilled from
`docs/reference/plan-format-v3.md` — the dev reference; do NOT reproduce it whole):

- On-disk: `_lyx/plan/00-overview.md` + one `NN-<card-slug>.md` per card (`NN` zero-padded,
  equals the card's flat heading number `N`, running 1..M with no gaps).
- `00-overview.md`: scalar-only frontmatter `format: 3`, `approved: false`, optional
  `root:`; body = a short task-framing paragraph, an ordered **Card Index** whose entries read
  `N — <card-slug> — <one-line intent>`, and optional plan-level sections `## Shared Decisions`,
  `## Rename mechanic` (**required** iff any card has a non-empty `Moves:`), `## verify:`.
- Each `NN-<card-slug>.md` card, in order: `# Card N — <name>`; `**What:**`; the **five typed
  file-op fields, all required, in this order** — `**Context:**`, `**Edits:**`, `**Creates:**`,
  `**Deletes:**`, `**Moves:**` (empty → literal `none` on the label line; non-empty → indented
  backtick-wrapped path sub-bullets, no commentary, no line ranges; `Moves:` sub-bullets are
  `` `old` -> `new` `` pairs); then `**Depends-on:**` (card numbers or `none`, references only
  earlier cards in the same plan); optional `**Commit:**` (must start `N: `) and `**verify:**`.
- A card = the smallest change that compiles on its own, is independently committable, and
  bundles its own test when it adds behavior. `Context:` is advisory (read-not-change); fields
  are mutually exclusive within one card. `## Rename mechanic` text is canonical — reproduce it
  verbatim (the 4-step `git mv`-first block) when any `Moves:` exists.

**Doc-lifecycle targets:**

- `manifest/designs/loom.md` — the module-decomposition table row for "producers (discussion /
  plan)" currently says the Planner producer is "not built" with its own doc; flip it to built
  and drop the pointer to the deleted doc.
- `manifest/roadmap.md` — move the loom-planner item Planned→Done (link the loomengine package
  doc / `plan.go` if useful).
- `manifest/designs/loom-planner.md` — **delete** after folding its durable content into
  `loomengine` godoc (`plan.go`'s package/factory comment), per the file's own header note and
  the documentation-lifecycle rule.

## Constraints

From `CONSTRAINTS.md` (repo root) and CLAUDE.md, the ones this task touches:

- **Hub Geometry Invariant** — all `_lyx`/plan-path construction stays in `internal/hubgeometry`;
  `loomengine` gets the overview path via a `Layout` method, never a raw `filepath.Join` with a
  geometry token. Enforced by `internal/hubgeometry/enforcement_test.go`.
- **Modelspec Leaf Invariant** — `modelspec` stays a leaf; `loomengine` importing `modelspec` is
  the allowed direction (already how `discussion.go` works). No new edges.
- **Test Tier Purity Invariant** — the new unit tests must be untagged Tier-1: pure Go over an
  in-memory `Config` + a `t.TempDir()` `modelspec` registry, **no** git spawn / `exec.Command` /
  `lyxtest.Copy*` (mirror `discussion_test.go` / `config_test.go` exactly). Enforced by
  `cmd/lyx/tierpurity_test.go`.
- **Documentation Lifecycle / task-completion docs** (CLAUDE.md) — the loom.md table, roadmap,
  and `loom-planner.md` deletion happen in the **same commit** as the code; a commit that ships
  behaviour without the doc update is incomplete.
- **`stencil` no-conditional rule** — `plan-template.md` uses only top-level `{{.X}}` markers,
  no `{{if}}`/`{{range}}`; every marker `composePlanPrompt` supplies must be non-empty.
- **Not a module** — nothing here registers a cobra command; there is no CLI surface, so the
  CLI/Cobra and Sandbox-Coverage invariants do not apply (the Planner is a prompt+profile fed to
  `shuttle.Run`, exactly like the Discussion producer).

## Testing

Mirror the Discussion producer's untagged Tier-1 tests (`discussion_test.go`, `prompt_test.go`,
`config_test.go`). TDD candidates:

- **`PlanSpec` field mapping** (like `TestDiscussionSpec`): against a hand-built
  `hubgeometry.Layout{WorktreeRoot: ...}`, an in-memory `Config{Plan: "opus[effort=high]",
  PlanTimeoutMin: 120}`, and `modelspec.LoadRegistry(t.TempDir())`, assert:
  - `OutputFiles == []string{ <WorktreeRoot>/_lyx/plan/00-overview.md }` (exactly one entry).
  - `Interactive == false` (always — no interactive case to table-drive).
  - `Role == "plan"`, `Model != ""`, `Effort == "high"`, `Timeout == 120*time.Minute`,
    `Prompt != ""`.
- **`composePlanPrompt` / template asset** (like `prompt_test.go`): the rendered prompt is
  non-empty, contains the resolved `decision_record_path` / `plan_dir` / `overview_path`, and has
  **no leftover `{{`** (proves every marker was filled). A missing/empty marker must error.
- **Config** (extend `config_test.go`): the seeded `template.yaml` round-trips `plan` /
  `plan_timeout_min`; a malformed `plan` model-spec fails loud at `LoadConfig` naming the `plan`
  key (mirror `TestLoadConfig_MalformedDiscussionSpec`).
- **hubgeometry**: `Layout.PlanOverview()` returns `<WorktreeRoot>/_lyx/plan/00-overview.md`
  and `Layout.PlanDir()` matches the free `PlanDir(WorktreeRoot)` — a small table test alongside
  the existing hubgeometry Layout tests.

No integration test is needed (no spawn, no live hub) — the producer is a pure composer, exactly
like `DiscussionSpec`.

## Q&A log

- **Q:** How does shuttle detect the Planner is done when card filenames are unknown up front?
  **A:** `OutputFiles = [_lyx/plan/00-overview.md]`; the prompt mandates writing that file LAST.
- **Q:** Does `PlanSpec` take a slug / read the board? **A:** No — sole input is the
  self-contained `decision-record.md`; no slug, no board read.
- **Q:** What `approved:` value does the producer write, and who flips it? **A:** Producer writes
  `approved: false`; the **loom Go orchestrator** flips it to `true` after `perch` returns
  `APPROVED` — never `perch`, never the producer. The flip is future loom work, out of scope here.
- **Q:** Where does the plan-format spec the agent follows live? **A:** Compact, inline in
  `plan-template.md` (mill-template style). `plan-format-v3.md` is a dev-only reference — not
  what the producer consumes, and NOT to be inlined at 440 lines.
- **Q:** Model tier and timeout? **A:** `plan: opus[effort=high]`, `plan_timeout_min: 120`
  (high-effort Opus like discussion, but a much shorter timeout since it's autonomous, not an
  8-hour interview).
- **Q:** Interactive or autonomous? **A:** Autonomous-only — `Interactive: false` always, no
  `autonomous` param, no interview, never `AskUserQuestion`.
- **Q:** Doc-lifecycle on completion? **A:** Fold durable design into `loomengine` godoc, flip
  loom.md's table to built, move the roadmap item Planned→Done, and delete `loom-planner.md` —
  all in this task.
</content>
</invoke>
