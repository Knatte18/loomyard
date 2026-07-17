# Batch: discussion-producer

```yaml
task: 'loom: Discussion producer (interactive interview, auto-mode capable)'
batch: discussion-producer
number: 3
cards: 6
verify: go test ./internal/loomengine/
depends-on: [1, 2]
```

## Batch Scope

Build the discussion producer proper: an embedded interview prompt
(`discussion-template.md`), a `stencil` composer (`prompt.go`), and the
`DiscussionSpec(...) (shuttleengine.Spec, error)` factory (`discussion.go`) that
composes the prompt, names the two `_lyx/discussion/` output files, sets
`Interactive = !autonomous`, `Role = "discussion"`, maps the resolved
`loom.yaml` model-spec to `Spec.Model/Effort/Version`, and maps
`DiscussionTimeoutMin` to `Spec.Timeout`. Depends on batch 1 (the
`Layout.DiscussionDecisionRecord()` / `DiscussionSupportLog()` accessors) and
batch 2 (`loomengine.Config` for the model + timeout). No cobra command is added
(producers are not modules). Also folds in the loom-module doc updates.

## Cards

### Card 8: Interview prompt template + embed accessor

- **Context:**
  - `docs/reference/discussion-format.md`
  - `internal/burlerengine/template.go`
  - `internal/burlerengine/review-prompt-template.md`
- **Edits:** none
- **Creates:**
  - `internal/loomengine/discussion-template.md`
  - `internal/loomengine/prompttemplate.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  - `prompttemplate.go`: `package loomengine`, a short file-level doc comment
    (mirroring `burlerengine/template.go`), `import _ "embed"`,
    `//go:embed discussion-template.md` bound to `var discussionTemplate []byte`,
    and an unexported-or-exported accessor returning it (mirror
    `burlerengine`'s `reviewPromptTemplate` var usage — a package var read
    directly by `composePrompt` is sufficient; no exported accessor is required).
  - `discussion-template.md`: a leading `<!-- ... -->` banner comment (stripped by
    `stencil.Fill`) listing the four required top-level markers: `{{.slug}}`,
    `{{.decision_record_path}}`, `{{.support_log_path}}`, `{{.mode_rules}}`. The
    body is the interview prompt for the discussion agent, distilled from
    `mill-start`'s discipline, and MUST instruct the agent to:
    1. Read its task from the board first via
       `lyx board get '{"slug":"{{.slug}}"}'` (JSON output `{"task": {...}}`); if
       `task` is null, stop and report that the slug has no board task rather than
       inventing scope.
    2. Explore the relevant codebase before asking anything (read files; do not
       ask what can be discovered).
    3. Conduct a relentless, **batched** design interview covering: Scope
       (in/out), Constraints, Architecture, Edge cases, Security (only if
       relevant), Testing. For each question recommend an answer; where there are
       distinct options, propose 2–3 approaches with explicit trade-offs leading
       with the recommendation. Challenge the problem itself, not just the
       solution. Apply YAGNI. **Design the full scope — never propose MVP phases
       or "add later."**
    4. Follow `{{.mode_rules}}` for how to obtain answers (interactive vs
       autonomous — this marker is filled by the composer).
    5. Write BOTH output files, creating the `_lyx/discussion/` directory if
       absent:
       - `{{.decision_record_path}}` — the distilled decision record with these
         seven required H2 sections in order: `## Goal`, `## Scope`,
         `## Decisions`, `## Constraints`, `## Auto-mode assumptions`,
         `## Open risks`, `## Acceptance criteria`; plus an optional
         `## Notes for the plan writer`. No frontmatter (no `format:`, no
         `approved:`).
       - `{{.support_log_path}}` — the raw support log with sections in order:
         `## Interview` (turn-by-turn, distilled, not verbatim),
         `## Rejected alternatives`, `## Review rounds` (seed it with the header
         plus a `_No rounds yet._` placeholder — the Discussion-review gate fills
         it later), and `## Question ledger` (open + resolved questions, including
         any `--auto` self-picks).
    6. Encode `discussion-format.md`'s compaction rules explicitly: in the
       decision record, `## Decisions` carry **Decision + Rationale only** —
       rejected alternatives go to the support log's `## Rejected alternatives`,
       NOT the record; must-cover test scenarios go under `## Acceptance criteria`
       (no standalone Testing section); the record is terse structured prose with
       no meta prose-coaching.
    7. Never use the `AskUserQuestion` tool (see `{{.mode_rules}}` for the correct
       channel).
    Keep the four markers at the template top level (not nested inside any
    template conditional), per `stencil`'s top-level-marker fill guarantee.
    Wherever the JSON board-read example appears, the literal `{` / `}` around
    `{{.slug}}` are fine — only `{{` begins a template action.
- **Commit:** `feat(loom): add discussion interview prompt template`

### Card 9: Prompt composer

- **Context:**
  - `internal/loomengine/prompttemplate.go`
  - `internal/loomengine/discussion-template.md`
  - `internal/burlerengine/prompt.go`
  - `internal/stencil/stencil.go`
- **Edits:** none
- **Creates:**
  - `internal/loomengine/prompt.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** `package loomengine`, file-level doc comment mirroring
  `burlerengine/prompt.go`. Define
  `func composePrompt(slug, decisionRecordPath, supportLogPath string, autonomous bool) ([]byte, error)`
  that builds a `map[string]string` with keys `slug`, `decision_record_path`,
  `support_log_path`, and `mode_rules`, then calls
  `stencil.Fill(discussionTemplate, values)` and wraps any error as
  `loom: compose discussion prompt: %w`. Define a helper
  `func modeRules(autonomous bool) string` returning the block filled into
  `{{.mode_rules}}`:
  - autonomous `true`: prose stating the agent is running in autonomous
    (`--auto`) mode — no operator will answer; for every decision point it must
    make its own best-judgment choice and proceed, never blocking on input and
    never calling `AskUserQuestion`; and it must record each self-made pick and
    rationale in the support log's `## Question ledger`, marked as an auto-pick.
  - autonomous `false` (interactive): prose stating an operator is at the pane;
    the agent asks questions as plain numbered-list text **in the pane** and waits
    for the operator's typed reply before proceeding, batching related questions
    (≤5 per batch) and recommending an answer as option 1; and it must never call
    `AskUserQuestion` (a modal dialog the resume mechanism cannot drive) — ask
    conversationally in the pane.
  Both branches must return a non-empty string so the `mode_rules` top-level
  marker is never left blank.
- **Commit:** `feat(loom): add discussion prompt composer`

### Card 10: DiscussionSpec factory

- **Context:**
  - `internal/loomengine/prompt.go`
  - `internal/loomengine/config.go`
  - `internal/shuttleengine/spec.go`
  - `internal/builderengine/roles.go`
  - `internal/modelspec/registry.go`
  - `internal/modelspec/parse.go`
  - `internal/hubgeometry/hubgeometry.go`
- **Edits:** none
- **Creates:**
  - `internal/loomengine/discussion.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** `package loomengine`, file-level doc comment explaining the
  producer contract (a pure Spec composer; the phase machine drives it later).
  Define
  `func DiscussionSpec(layout *hubgeometry.Layout, cfg Config, reg modelspec.Registry, slug string, autonomous bool) (shuttleengine.Spec, error)`:
  - Reject an empty `slug` with a clear error (`loom: DiscussionSpec: slug must not be empty`).
  - Resolve the model: `spec, err := modelspec.Parse(cfg.Discussion)` (wrap error
    naming the discussion role); `resolved, err := reg.Resolve(spec)` (wrap).
  - Compute the two absolute output paths via `layout.DiscussionDecisionRecord()`
    and `layout.DiscussionSupportLog()`.
  - Compose the prompt via
    `composePrompt(slug, decisionRecordPath, supportLogPath, autonomous)` (wrap).
  - Return a `shuttleengine.Spec` with: `Prompt` = the composed string;
    `OutputFiles` = `[]string{decisionRecordPath, supportLogPath}` (both files —
    the run is "done" only when both exist); `Model = resolved.Model`,
    `Effort = resolved.Params["effort"]`, `Version = resolved.Params["version"]`
    (the exact mapping documented verbatim in `builderengine/roles.go`'s godoc);
    `Interactive = !autonomous`; `Role = "discussion"`; and
    `Timeout = time.Duration(cfg.DiscussionTimeoutMin) * time.Minute`. Leave
    `Round`, `Parent`, `Display`, and `KeepPane` at their zero values (the caller
    / phase machine sets placement later; `Spec.validate` defaults
    `Display.Anchor`). Do NOT stat or create the output files here (shuttle's
    `Spec.validate` rejects pre-existing outputs; directory creation is the
    agent's write concern). Imports: `fmt`, `time`, `shuttleengine`, `modelspec`,
    `hubgeometry`.
- **Commit:** `feat(loom): add DiscussionSpec producer factory`

### Card 11: Prompt composer tests

- **Context:**
  - `internal/loomengine/prompt.go`
  - `internal/loomengine/discussion-template.md`
  - `internal/loomengine/testmain_test.go`
- **Edits:** none
- **Creates:**
  - `internal/loomengine/prompt_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** `package loomengine` test file. Cover `composePrompt`:
  - For `autonomous=false` and `autonomous=true`, the rendered output contains
    no unrendered `{{` marker tokens, contains the given slug, contains both
    given paths (decision-record and support-log), and contains the board-read
    command substring `lyx board get`.
  - The `autonomous=true` output contains autonomous-mode language (e.g. matches
    on a stable phrase such as "auto" / "best-judgment" the template uses) and the
    `autonomous=false` output contains interactive language (e.g. "operator" /
    "pane"), and the two rendered strings differ.
  - `modeRules(true)` and `modeRules(false)` both return non-empty, distinct
    strings.
  Use stable substring assertions, not full golden-file equality, so prose
  wording can evolve without breaking the test (assert on the load-bearing tokens
  only).
- **Commit:** `test(loom): cover discussion prompt composer`

### Card 12: DiscussionSpec factory tests

- **Context:**
  - `internal/loomengine/discussion.go`
  - `internal/loomengine/config.go`
  - `internal/shuttleengine/spec.go`
  - `internal/modelspec/load.go`
  - `internal/hubgeometry/hubgeometry.go`
  - `internal/loomengine/testmain_test.go`
- **Edits:** none
- **Creates:**
  - `internal/loomengine/discussion_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** `package loomengine` test file covering `DiscussionSpec`.
  Construct a `*hubgeometry.Layout` with a known `WorktreeRoot` (mirror how
  `hubgeometry` tests build a Layout literal, or via the existing loomengine test
  fixtures), a `Config{Discussion: "opus[effort=high]", DiscussionTimeoutMin: 480}`,
  and a registry via `modelspec.LoadRegistry(t.TempDir())` (no `models.yaml` → the
  built-in registry, which includes the `opus` alias). Assert, for
  `autonomous=false`:
  - `OutputFiles` equals exactly `[]string{<WorktreeRoot>/_lyx/discussion/decision-record.md, <WorktreeRoot>/_lyx/discussion/support-log.md}` (build expected with `filepath.Join`).
  - `Interactive == true` (and in a second case with `autonomous=true`, `Interactive == false`).
  - `Role == "discussion"`.
  - `Model != ""` and `Effort == "high"` (resolved from `opus[effort=high]`).
  - `Timeout == 480 * time.Minute` (assert the knob is live — a non-zero Timeout that matches the config, NOT `0`).
  - `Prompt != ""`.
  - An empty `slug` argument returns a non-nil error.
  Do not require a live hub, mux, or network — this is pure Go over an in-memory
  Config + a temp-dir registry.
- **Commit:** `test(loom): cover DiscussionSpec factory`

### Card 13: Document the discussion producer

- **Context:**
  - `internal/loomengine/discussion.go`
  - `docs/reference/discussion-format.md`
- **Edits:**
  - `docs/modules/loom.md`
  - `docs/roadmap.md`
  - `docs/overview.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  - `docs/modules/loom.md`: in the module-decomposition section (the "producers
    (discussion / plan)" row / the phase-machine discussion of producers), note
    that the **Discussion producer** is now built as a prompt + composer + factory
    in `internal/loomengine` (`discussion-template.md`, `prompt.go`,
    `discussion.go`), fed to `shuttle.Run` by the future phase machine; and that
    `loom.yaml` now supplies its `discussion` model-spec and `discussion_timeout_min`.
    Keep it a surgical addition; do not rewrite the design doc.
  - `docs/roadmap.md`: mark milestone 12 sub-item 3 ("Discussion producer") as
    done/landed with a one-line note (the interview prompt + `DiscussionSpec`
    factory in `internal/loomengine`, auto-mode capable). Do not touch other
    milestones.
  - `docs/overview.md`: in the loom module bullet / the execution-stack notes,
    note the discussion producer exists as loom's prompt/profile-over-`shuttle`
    producer (distinct from the still-unbuilt `lyx loom run` phase machine). Keep
    it consistent with the config-module note added in batch 2 (do not contradict
    or duplicate it).
- **Commit:** `docs(loom): document the discussion producer`

## Batch Tests

`verify: go test ./internal/loomengine/` runs the full `loomengine` package test
binary — the new `prompt_test.go` and `discussion_test.go` plus the existing
Preflight/coherence tests (regression guard that the added files do not break the
package). Scoped to the single package this batch's code lands in; the doc-only
card 13 has no runnable surface and is covered by the same package build. No
LLM-in-the-loop test — the producer is verified as pure Go (prompt composition +
Spec construction) over fixtures.
