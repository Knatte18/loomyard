# Batch: planner-producer

```yaml
task: 'loom: Planner producer'
batch: 'planner-producer'
number: 2
cards: 4
verify: go test ./internal/loomengine/...
depends-on: [1]
```

## Batch Scope

Builds the Planner producer itself in `internal/loomengine` as the structural sibling of the
already-built Discussion producer, plus the doc-lifecycle work. Four cards: (2) add the `plan`
model-spec and `plan_timeout_min` knobs to `loom.yaml`'s config; (3) create the embedded
`plan-template.md` prompt (carrying a compact plan-format-v3 spec) and its `//go:embed`
accessor; (4) create the `PlanSpec` factory + `composePlanPrompt` composer + tests, folding the
durable `loom-planner.md` design into `plan.go`'s header godoc; (5) the manifest/docs updates
and the `loom-planner.md` deletion. This is one batch because all four cards share the same
`internal/loomengine` context (the Discussion producer files) and a single Sonnet session can
hold them together. It `depends-on: [1]` because card 4's `PlanSpec` calls
`layout.PlanOverview()` / `layout.PlanDir()`, created in batch 1. Cards implement in order:
2 → 3 → 4 → 5, so `plan.go` (card 4) compiles only after the `Config.Plan` field (card 2), the
`planTemplate` embed var (card 3), and batch 1's accessors all exist. All decisions inherit from
`## Shared Decisions`; no batch-local decisions.

## Cards

### Card 2: add plan / plan_timeout_min config knobs

- **Context:**
  - `internal/loomengine/configtemplate.go`
- **Edits:**
  - `internal/loomengine/template.yaml`
  - `internal/loomengine/config.go`
  - `internal/loomengine/config_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add the Planner producer's two config knobs alongside the existing
  discussion knobs, mirroring them exactly. (1) In `internal/loomengine/template.yaml`, add two
  lines after the existing `discussion` / `discussion_timeout_min` lines:
  `plan: opus[effort=high]` (with a trailing comment mirroring the discussion line, e.g.
  `# model-spec for the plan-phase producer agent (see docs/reference/model-spec.md)`) and
  `plan_timeout_min: 120` (comment e.g.
  `# minutes the plan agent's shuttle run is allowed to run (autonomous, shorter than the interview)`).
  (2) In `internal/loomengine/config.go`, add two fields to the `Config` struct:
  `Plan string \`yaml:"plan"\`` and `PlanTimeoutMin int \`yaml:"plan_timeout_min"\``, each with a
  godoc comment mirroring the `Discussion` / `DiscussionTimeoutMin` field comments (the plan
  model-spec for the plan-phase producer agent; the minutes the plan agent's shuttle run is
  allowed). Refresh the `Config` type's own doc comment and `LoadConfig`'s doc comment, which
  currently name only "the discussion role model-spec" — they must also name the `plan` key so
  the prose does not lie about which keys the config carries (same-commit docs rule). In
  `LoadConfig`, after the existing `modelspec.Parse(cfg.Discussion)` grammar-validation block,
  add an identical block validating `cfg.Plan` and wrapping the error naming the `"plan"` key:
  `if _, err := modelspec.Parse(cfg.Plan); err != nil { return Config{}, fmt.Errorf("loom config key %q: %w", "plan", err) }`.
  (3) In `internal/loomengine/config_test.go`, extend `TestLoadConfig_WellFormed` to also assert
  `cfg.Plan == "opus[effort=high]"` and `cfg.PlanTimeoutMin == 120`, and add
  `TestLoadConfig_MalformedPlanSpec` mirroring `TestLoadConfig_MalformedDiscussionSpec` — seed a
  `loom.yaml` with a well-formed `discussion` line but a malformed `plan` line (e.g.
  `plan: "opus[effort"`), assert `LoadConfig` returns a non-nil error whose message contains
  `"plan"`. Follow the repo's godoc conventions (`golang-comments` skill).
- **Commit:** `feat(loomengine): add plan/plan_timeout_min config knobs`

### Card 3: plan-template.md prompt + go:embed accessor

- **Context:**
  - `internal/loomengine/discussion-template.md`
  - `internal/loomengine/prompttemplate.go`
  - `_mill/discussion.md`
  - `docs/reference/plan-format-v3.md`
- **Edits:** none
- **Creates:**
  - `internal/loomengine/plan-template.md`
  - `internal/loomengine/plantemplate.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create the Planner producer's prompt template and its embed accessor,
  mirroring `discussion-template.md` / `prompttemplate.go`. (1)
  `internal/loomengine/plan-template.md` is the agent's entire instruction set, filled by
  `composePlanPrompt` (card 4) via `internal/stencil`. It MUST use only top-level `{{.X}}`
  markers and contain NO `{{if}}` / `{{range}}` conditionals (a required marker inside a
  conditional renders silently blank — see the leading comment in `discussion-template.md` and
  `internal/stencil/stencil.go`). Use exactly these three markers, each referenced at least once
  and all non-empty at fill time: `{{.decision_record_path}}` (the sole input),
  `{{.plan_dir}}` (the output directory the agent creates and writes into), and
  `{{.overview_path}}` (the `00-overview.md` file, named so the "write this LAST" instruction
  points at a concrete path). Structure the prose as: a title + role statement ("You are the
  Plan producer: a single autonomous agent that reads the decision record and writes a
  plan-format-v3 flat-card plan; you never interview, never ask, and have no review logic of
  your own"); **Step 1 — read the decision record** at `{{.decision_record_path}}` as the SOLE
  input (state it never reads the support log or the board; if the file is missing/empty, STOP
  and report rather than inventing scope); **Step 2 — explore the codebase** before planning
  (read `CONSTRAINTS.md` if present, follow existing patterns); **Step 3 — write the plan** into
  `{{.plan_dir}}`, creating the directory first if absent, following the COMPACT plan-format-v3
  spec inline (see below); **Step 4 — write `{{.overview_path}}` LAST**, only after every
  `NN-<card-slug>.md` card file is written, because its existence is the producer's done-signal;
  and a final "Never use `AskUserQuestion`" guard (this session is autonomous, no operator is
  present, make best-judgment calls and never block on a dialog). The **compact plan-format-v3
  spec** distilled inline must be modeled on `discussion-template.md`'s terseness (roughly
  60–120 lines total for the whole template, NOT a reproduction of the ~440-line
  `docs/reference/plan-format-v3.md`, which is a development-only reference the producer must not
  point the reader at). Distill exactly the "plan-format-v3 essentials" list under
  `## Technical context` in `_mill/discussion.md`, covering: the on-disk layout
  (`00-overview.md` + one `NN-<card-slug>.md` per card, `NN` zero-padded = the card's flat
  heading `N`, running 1..M no gaps); `00-overview.md`'s scalar-only frontmatter
  (`format: 3`, `approved: false` — the producer writes `false` and never self-approves — and
  optional `root:`), its task-framing paragraph, its ordered Card Index
  (`N — <card-slug> — <one-line intent>`), and the optional plan-level sections
  `## Shared Decisions`, `## Rename mechanic` (required iff any card has a non-empty `Moves:`,
  and its canonical `git mv`-first text), `## verify:`; and each card file's ordered content —
  `# Card N — <name>`, `**What:**`, the five REQUIRED typed file-op fields in order
  (`**Context:**`, `**Edits:**`, `**Creates:**`, `**Deletes:**`, `**Moves:**`, empty → literal
  `none` on the label line, non-empty → indented backtick-wrapped path sub-bullets with no
  commentary/line-ranges, `Moves:` sub-bullets as `` `old` -> `new` `` pairs), then
  `**Depends-on:**` (card numbers or `none`, referencing only earlier cards in the same plan),
  and optional `**Commit:**` (must start `N: `) / `**verify:**`. Include a minimal literal
  skeleton (a filled `00-overview.md` and one `NN-card.md`) as a concrete example, kept short.
  Reproduce the canonical `## Rename mechanic` 4-step block verbatim. Lead the file with an
  HTML comment (like `discussion-template.md`'s) stating it is filled by `composePlanPrompt` via
  stencil, that all three markers are required non-empty, and that there are no conditionals.
  (2) `internal/loomengine/plantemplate.go` mirrors `prompttemplate.go` exactly: a doc comment,
  `package loomengine`, `import _ "embed"`, `//go:embed plan-template.md`, and
  `var planTemplate []byte`.
- **Commit:** `feat(loomengine): add plan producer prompt template + embed`

### Card 4: PlanSpec factory + composePlanPrompt + tests

- **Context:**
  - `internal/loomengine/discussion.go`
  - `internal/loomengine/prompt.go`
  - `internal/loomengine/discussion_test.go`
  - `internal/hubgeometry/hubgeometry.go`
  - `internal/shuttleengine/spec.go`
  - `manifest/designs/loom-planner.md`
- **Edits:** none
- **Creates:**
  - `internal/loomengine/plan.go`
  - `internal/loomengine/plan_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create `internal/loomengine/plan.go` as the Planner producer's Spec factory,
  mirroring `discussion.go` / `prompt.go` but autonomous-only and slug-less. (1) Add
  `func composePlanPrompt(decisionRecordPath, planDir, overviewPath string) ([]byte, error)`
  mirroring `composePrompt` in `prompt.go`: build a `map[string]string` with keys
  `"decision_record_path"`, `"plan_dir"`, `"overview_path"` and call
  `stencil.Fill(planTemplate, values)`, wrapping any error `"loom: compose plan prompt: %w"`.
  Do NOT port `modeRules` — the Planner has no interactive/autonomous branch. (2) Add
  `func PlanSpec(layout *hubgeometry.Layout, cfg Config, reg modelspec.Registry) (shuttleengine.Spec, error)`
  mirroring `DiscussionSpec` with these differences: NO `slug` param and NO `autonomous` param;
  resolve the model-spec via `modelspec.Parse(cfg.Plan)` then `reg.Resolve(spec)` (wrap errors
  `"loom: PlanSpec: plan role model-spec: %w"`, resolving before composing/naming paths so a bad
  alias fails loud first); set `decisionRecordPath := layout.DiscussionDecisionRecord()` (the
  sole input), `planDir := layout.PlanDir()`, `overviewPath := layout.PlanOverview()`; call
  `composePlanPrompt(decisionRecordPath, planDir, overviewPath)`; and return a
  `shuttleengine.Spec{ Prompt: string(prompt), OutputFiles: []string{overviewPath}, Model: resolved.Model, Effort: resolved.Params["effort"], Version: resolved.Params["version"], Interactive: false, Role: "plan", Timeout: time.Duration(cfg.PlanTimeoutMin) * time.Minute }`.
  `OutputFiles` is exactly the single overview path (the done-sentinel per Decision
  `done-sentinel-overview-last`); `Interactive` is always `false`. `PlanSpec` must NOT stat or
  create any file (Decision `pure-composer-no-preflight`). (3) `plan.go`'s leading package/doc
  comment folds the durable design from `manifest/designs/loom-planner.md` (what the producer is:
  a prompt/profile fed to `shuttle.Run`, not a module; its sole input `decision-record.md`; its
  output the plan-format-v3 card list with `00-overview.md` written last as the done-sentinel;
  review is perch/burler's separate job; the `approved: false` → `true` flip is the loom
  orchestrator's, not the producer's) — this is the fold-into-package-doc step; card 5 deletes
  the now-redundant `loom-planner.md`. (4) Create `internal/loomengine/plan_test.go` as an
  untagged Tier-1 test mirroring `discussion_test.go`: build a `hubgeometry.Layout{WorktreeRoot:
  filepath.Join("home","user","repo")}`, `Config{Plan: "opus[effort=high]", PlanTimeoutMin:
  120}`, and `modelspec.LoadRegistry(t.TempDir())`. `TestPlanSpec` asserts `OutputFiles` equals
  `[]string{ filepath.Join(worktreeRoot, "_lyx", "plan", "00-overview.md") }` (exactly one
  entry), `Interactive == false`, `Role == "plan"`, `Model != ""`, `Effort == "high"`,
  `Timeout == 120*time.Minute`, and `Prompt != ""`. Add `TestPlanSpec_PromptFilled` asserting
  the rendered `Prompt` contains the resolved `decisionRecordPath`, `planDir`, and
  `overviewPath` and contains no leftover `"{{"` (proving every marker was filled). Add
  `TestPlanSpec_MalformedModelSpec` asserting a `Config{Plan: "opus[effort"}` yields a non-nil
  error. Pure Go, no git spawn / `exec.Command` / fixture copy (Test Tier Purity Invariant);
  loomengine already has a `TestMain` so none is added. Follow `golang-comments` / `golang-testing`.
- **Commit:** `feat(loomengine): add PlanSpec factory + composePlanPrompt`

### Card 5: doc-lifecycle — flip loom.md/roadmap, drop loom-planner.md links, delete it

- **Context:**
  - `_mill/discussion.md`
- **Edits:**
  - `manifest/designs/loom.md`
  - `manifest/roadmap.md`
  - `manifest/designs/webster-rewrite.md`
  - `docs/overview.md`
- **Creates:** none
- **Deletes:**
  - `manifest/designs/loom-planner.md`
- **Moves:** none
- **Requirements:** Complete the documentation lifecycle for the now-built Planner producer;
  every inbound link to `loom-planner.md` must be dropped in the same commit that deletes it, or
  the doc tree ends with a dangling link. (1) `manifest/designs/loom.md`, the
  `producers (discussion / plan)` module-table row (around line 234): replace the trailing
  sentence "The Planner producer (not built) has its own doc: [loom-planner.md](loom-planner.md)."
  with a "built" description mirroring the Discussion producer's clause in the same cell — the
  Planner producer is ✅ built: a `plan-template.md` prompt (carrying a compact plan-format-v3
  spec) + `stencil` composer + `PlanSpec(...) (shuttleengine.Spec, error)` factory in
  `internal/loomengine` (`plan-template.md`, `plantemplate.go`, `plan.go`); `loom.yaml` supplies
  its `plan` model-spec and `plan_timeout_min` knobs; it reads `decision-record.md` and writes
  `_lyx/plan/00-overview.md` (last, as the done-sentinel) plus one `NN-<card>.md` per card with
  `approved: false`. Do NOT link `loom-planner.md`. (2) `manifest/roadmap.md`: remove the
  `**loom: Planner producer**` entry from the `## Planned` section (around lines 40–42,
  including its `[designs/loom-planner.md]` link) and add a corresponding entry to the `## Done`
  section in the same numbered-`1.` style as the other Done entries, describing the shipped
  producer and pointing at `internal/loomengine` (NOT `loom-planner.md`) — e.g. "**loom: Planner
  producer** — reads the discussion decision-record and writes a plan-format-v3 flat-card plan;
  a prompt/profile fed to `shuttle.Run` (not a module), the `PlanSpec(...)` factory +
  `plan-template.md` in `internal/loomengine`. No review logic of its own." (3)
  `manifest/designs/webster-rewrite.md`, the "planner instruction" bullet (around line 247):
  replace `Own doc: [loom-planner.md](loom-planner.md).` with a pointer that survives the
  deletion — e.g. "Built as the `PlanSpec` producer in `internal/loomengine`; see
  [loom.md](loom.md)." (keep the rest of the bullet). (4) `docs/overview.md`, the `loom` bullet
  (around lines 319–325): update "loom's config module (`loom.yaml`, holding the `discussion`
  role model-spec and `discussion_timeout_min`)" to also name the `plan` model-spec and
  `plan_timeout_min`; and extend "The Discussion producer itself is ✅ **built**…" to state that
  the Planner producer is now built too (a prompt/profile fed to `shuttle.Run` —
  `internal/loomengine`'s `plan-template.md` + `plantemplate.go` + `plan.go`), both still
  distinct from the unbuilt `lyx loom run` phase machine. (5) Delete
  `manifest/designs/loom-planner.md` (its durable design was folded into `plan.go`'s header
  godoc in card 4). Read it before deleting to confirm nothing durable is lost that card 4 did
  not capture. Verify no remaining references to `loom-planner.md` exist anywhere under the repo
  after the edits (a `grep -rn "loom-planner\.md"` should return nothing outside `_mill/`).
  Follow the `markdown` skill for formatting.
- **Commit:** `docs(loom): mark Planner producer built, delete loom-planner.md`

## Batch Tests

`go test ./internal/loomengine/...` compiles the package and runs the new/extended tests:
`config_test.go`'s `plan`-key round-trip and malformed-spec cases (card 2), and `plan_test.go`'s
`TestPlanSpec` / `TestPlanSpec_PromptFilled` / `TestPlanSpec_MalformedModelSpec` (card 4),
alongside the existing discussion-producer tests. It also compiles `plan.go` against the
`planTemplate` embed (card 3) and the batch-1 `Layout.PlanOverview()` / `PlanDir()` accessors,
so a broken embed or signature surfaces here. Card 5 is docs-only with no runnable surface; its
correctness is a review obligation (dangling-link check via the `grep` in its Requirements). The
overview's module-wide `verify: go test ./cmd/lyx/...` runs at the batch boundary to confirm the
new untagged test files do not trip the Tier-Purity / Hermetic-Git / geometry-literal enforcement
guards in `cmd/lyx`.
