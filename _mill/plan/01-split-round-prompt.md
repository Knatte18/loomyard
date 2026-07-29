# Batch: split-round-prompt

```yaml
task: 'burler: split the round prompt into an orchestrator + three instruction files'
batch: split-round-prompt
number: 1
cards: 9
verify: go test ./internal/burlerengine/... ./internal/stencil/...
depends-on: []
```

## Batch Scope

Replace burlerengine's single monolithic `review-prompt-template.md` with four embedded assets — a thin orchestrator plus three self-contained instruction files — and make the round deliver them lazily: `composePrompt` renders all four and returns the orchestrator string plus three `(path, content)` instruction pairs; `Engine.Run` writes the three instruction files to a fresh per-round dir under `.lyx` (`layout.DotLyxDir()`), bakes their absolute paths into the orchestrator, and hands only the orchestrator to the shuttle as `spec.Prompt`. All test pins relocate to their new per-asset homes, a new orchestrator guard test and `Engine.Run` materialization tests are added, and `doc.go` + `CONSTRAINTS.md` are updated in the same batch. This is a prompt-delivery refactor: round semantics (A-before-B, fix-everything, cluster-fork discipline, `Verdict`/`Findings`) are unchanged. No `shuttleengine` change, no `internal/stencil` change. The whole batch is one Go package that must compile and pass `go test` together — see the overview's "Single atomic batch" decision.

Batch-local specifics beyond `## Shared Decisions`: the exact marker→asset distribution is fixed in Card 1's Requirements and is the contract every later card reads against.

## Cards

### Card 1: Author the four instruction assets and repoint .gitattributes

- **Context:**
  - `internal/burlerengine/prompt.go`
  - `internal/stencil/stencil.go`
- **Edits:**
  - `.gitattributes`
- **Creates:**
  - `internal/burlerengine/round-orchestrator-template.md`
  - `internal/burlerengine/instruction-1-explore-template.md`
  - `internal/burlerengine/instruction-2-review-template.md`
  - `internal/burlerengine/instruction-3-fix-template.md`
- **Deletes:**
  - `internal/burlerengine/review-prompt-template.md`
- **Moves:** none
- **Requirements:** Split the monolith's sections and markers across four new template files, each carrying its own leading `<!-- ... -->` banner comment documenting its markers (stripped before parsing, exactly like the monolith's banner and builder's per-asset banners). Every required top-level `{{.X}}` marker stays at the top level of its asset — never inside an `{{if}}`/`{{range}}` (there are none; keep it that way) — so `stencil.Fill` errors by name when one is empty. Distribution (verbatim marker names):
  - **`round-orchestrator-template.md`** — the "you are a burler / two jobs A (review) then B (fix), in order, in one session" framing, retaining a one-line summary of each job (A: form your own judgment against the fasit, hunt defects, write findings to the review file; B: fix every finding you recorded, even if the verdict was APPROVED — non-blocking polish still gets fixed); the BLOCKING sequencing invariant ("Sequencing rule", job A fully written to `{{.review_path}}` on disk before you touch — edit/create/delete — a single target file, "before you touch", record findings as found never fix on sight); and the ordered list of the three instruction file paths with the "read & execute each in turn, never preview a later file's content early" rule. Markers: `{{.instruction_1_path}}`, `{{.instruction_2_path}}`, `{{.instruction_3_path}}`, `{{.review_path}}`. Do NOT copy the full fix-everything body, the review-file YAML format block, or the cluster fork-spawn prose into this file (they live in instructions 2/3) — see the overview's guard-test decision for the disjoint tokens that must stay absent here (`"not whether it gets fixed"`, `"verdict:"`, `"findings:"`, `"SINGLE message"`, `"subagent_type"`).
  - **`instruction-1-explore-template.md`** — read/understand the target against what it is judged by. `{{.pattern_directive}}` (the one OPTIONAL marker, filled via `FillOptional`; keep it at the top level BEFORE the first work heading "## What to review (the target)" so its optional-blank semantics hold), then `{{.target}}`, the fasit block plus the "a review that ignores the fasit degenerates" prose (`{{.fasit}}`), the rubric block plus the four-value severity vocabulary prose (`{{.rubric}}`), and `{{.tool_use_rules}}` (job-A evidence gathering).
  - **`instruction-2-review-template.md`** — `{{.cluster_rules}}` under a `## Cluster rules` heading; the review-file format block that writes to `{{.review_path}}` (the `---`-delimited YAML frontmatter spec: `verdict`, `findings` with per-entry `id`/`severity`/`location`/`summary`, the `origin:` key for cluster rounds); the source-grounding rule ("never fabricate file contents"); and `{{.prior_rounds}}` (clean-room: form your own findings first, read prior-round files only after your review is saved). Markers: `{{.cluster_rules}}`, `{{.review_path}}`, `{{.prior_rounds}}`.
  - **`instruction-3-fix-template.md`** — the fix-everything rule (all severities incl. LOW/NIT; "Severity affects how a finding is reported, not whether it gets fixed"); `{{.fix_scope_rules}}`; the fixer-report rule writing `{{.fixer_report_path}}` (unconditional every round, "nothing fixed" when APPROVED-and-clean, deferred-with-reason section); and the never-push / never-touch-weft rule ("never push", the `_lyx`/weft prohibition). Markers: `{{.review_path}}`, `{{.fix_scope_rules}}`, `{{.fixer_report_path}}`.
  In `.gitattributes`, replace the single line `internal/burlerengine/review-prompt-template.md text eol=lf` with four lines, one per new asset above, each `text eol=lf`.
- **Commit:** `burler: split round prompt into orchestrator + 3 instruction assets`

### Card 2: Replace the single embed with four in template.go

- **Context:**
  - `internal/burlerengine/round-orchestrator-template.md`
  - `internal/burlerengine/instruction-1-explore-template.md`
  - `internal/burlerengine/instruction-2-review-template.md`
  - `internal/burlerengine/instruction-3-fix-template.md`
- **Edits:**
  - `internal/burlerengine/template.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Delete the `//go:embed review-prompt-template.md` directive and its `reviewPromptTemplate` var. Add four `//go:embed` directives with four package-private `[]byte` vars (no exported accessors — `composePrompt` is in-package): `roundOrchestratorTemplate` ← `round-orchestrator-template.md`, `instruction1Template` ← `instruction-1-explore-template.md`, `instruction2Template` ← `instruction-2-review-template.md`, `instruction3Template` ← `instruction-3-fix-template.md`. Update the file banner comment to describe the four-asset layout (orchestrator owns ordering; the three instruction files each carry one step's rules) instead of the old single-template description.
- **Commit:** `burler: embed the four round prompt assets`

### Card 3: Reshape composePrompt to render four assets and return the split

- **Context:**
  - `internal/stencil/stencil.go`
  - `internal/burlerengine/template.go`
  - `internal/burlerengine/round-orchestrator-template.md`
  - `internal/burlerengine/instruction-1-explore-template.md`
  - `internal/burlerengine/instruction-2-review-template.md`
  - `internal/burlerengine/instruction-3-fix-template.md`
- **Edits:**
  - `internal/burlerengine/prompt.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Change `composePrompt` to signature `func composePrompt(p *Profile, patternDirective, inst1Path, inst2Path, inst3Path string) (orchestrator string, files []instructionFile, err error)`, where `instructionFile` is a new package-private struct `type instructionFile struct { Path string; Content string }`. Render the four assets separately, each with only its own markers: fill `roundOrchestratorTemplate` via `stencil.Fill` with `instruction_1_path`/`instruction_2_path`/`instruction_3_path` (= the three path params) and `review_path`; fill `instruction1Template` via `stencil.FillOptional(..., []string{"pattern_directive"})` with `pattern_directive`, `target`, `fasit`, `rubric`, `tool_use_rules`; fill `instruction2Template` via `stencil.Fill` with `cluster_rules`, `review_path`, `prior_rounds`; fill `instruction3Template` via `stencil.Fill` with `fix_scope_rules`, `review_path`, `fixer_report_path`. Return the rendered orchestrator plus `files` as exactly `[]instructionFile{{inst1Path, <instr1 render>}, {inst2Path, <instr2 render>}, {inst3Path, <instr3 render>}}` in that order. Reuse the existing block helpers unchanged — `formatFileSet`, `fixScopeRules`, `toolUseRules`, `priorRoundsBlock`, `clusterRulesBlock` — only the asset each value fills changes. Wrap any fill error as before (`fmt.Errorf("burler: compose prompt: %w", err)`). Keep `composePrompt` filesystem-free apart from the existing `formatFileSet` `os.Stat` directory check — it takes the three paths as plain string parameters and does no writes. Update the prompt.go file-level banner comment to describe the four-asset render and the new return shape, AND update `composePrompt`'s own function doc comment (the block immediately above the `func composePrompt` signature, prompt.go:23-31 today) — it currently says composePrompt fills `reviewPromptTemplate` via `stencil.FillOptional` with "the template's nine required top-level marker values"; rewrite it to describe rendering the four assets, each with its own marker subset, and returning the orchestrator string plus the three `(path, content)` instruction pairs.
- **Commit:** `burler: compose orchestrator plus three instruction files`

### Card 4: Materialize instruction files in Engine.Run

- **Context:**
  - `internal/burlerengine/prompt.go`
  - `internal/hubgeometry/hubgeometry.go`
- **Edits:**
  - `internal/burlerengine/engine.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `(*Engine).Run`, after `p.validate(...)` and computing `directive`, mint a fresh per-round dir and materialize the instruction files before building the `Spec`: `burlerDir := filepath.Join(e.layout.DotLyxDir(), "burler")`; `os.MkdirAll(burlerDir, 0o755)`; `roundDir, err := os.MkdirTemp(burlerDir, "round-")`. Compute the three absolute instruction paths as `filepath.Join(roundDir, "instruction-1-explore.md")`, `"instruction-2-review.md"`, `"instruction-3-fix.md"`. Call the new `composePrompt(&p, directive, inst1Path, inst2Path, inst3Path)` to get `orchestrator, files, err`. Write each returned `instructionFile` with `os.WriteFile(f.Path, []byte(f.Content), 0o644)`. Any `MkdirAll`/`MkdirTemp`/`WriteFile` failure is a hard error returned before `e.shuttle.Run` is ever called: `return Result{}, fmt.Errorf("burler: materialize instruction files: %w", err)`. Set `spec.Prompt = orchestrator` (the thin orchestrator, no longer the full monolith). Leave `OutputFiles`, `Model`, `Effort`, `Timeout`, `Role`, `Round`, `ForkSubagents` exactly as they are today. Add the `path/filepath` import. Update the doc comment on `Run`'s sequence to mention the per-round instruction-file materialization step (the fuller narrative lives in `doc.go`, Card 8).
- **Commit:** `burler: write per-round instruction files under .lyx`

### Card 5: Relocate template pins and add the orchestrator guard test

- **Context:**
  - `internal/burlerengine/template.go`
  - `internal/burlerengine/prompt.go`
  - `internal/burlerengine/round-orchestrator-template.md`
  - `internal/burlerengine/instruction-1-explore-template.md`
  - `internal/burlerengine/instruction-2-review-template.md`
  - `internal/burlerengine/instruction-3-fix-template.md`
- **Edits:**
  - `internal/burlerengine/template_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Re-home every assertion in `template_test.go` off the deleted `reviewPromptTemplate` var:
  - `TestTemplate_StatesRoundDiscipline`: assert the sequencing phrases ("Sequencing rule", "fully written to", "before you touch") against `roundOrchestratorTemplate` bytes; assert fix-everything/never-push phrases ("not whether it gets fixed", "never push", "nothing fixed") against `instruction3Template` bytes; assert the review-file `origin` key against `instruction2Template` bytes.
  - `TestTemplate_HasClusterRulesSection`: re-home to assert `"Cluster rules"` and the `"{{.cluster_rules}}"` marker against `instruction2Template` bytes (the marker is static template text there; the single-reviewer-vs-cluster line is dynamic and stays covered by the composePrompt-render test below).
  - `TestTemplate_StatesClusterForkDiscipline`: keep sourcing from a `composePrompt` render of a cluster profile, but scope the assertions ("SINGLE message", "subagent_type", "never pass a `name`", "READ-ONLY", "never run any git command", "never call the Agent tool", "HOLISTIC", "before job B touches anything", "origin:", "Rejected") to the returned instruction-2 file's `Content` (`files[1].Content`), adapting to `composePrompt`'s new `(orchestrator, files, err)` return.
  - `TestTemplate_FillsWithAllMarkers`: convert to per-asset fill tests — each asset renders when its own required markers are supplied and fails naming the marker when any single required one is missing. Provide a per-asset marker map (orchestrator: `instruction_1_path`, `instruction_2_path`, `instruction_3_path`, `review_path`; instruction 1: `target`, `fasit`, `rubric`, `tool_use_rules` required + `pattern_directive` optional; instruction 2: `cluster_rules`, `review_path`, `prior_rounds`; instruction 3: `fix_scope_rules`, `review_path`, `fixer_report_path`). Keep `pattern_directive` out of instruction 1's deletion sweep (it is optional).
  - `TestTemplate_PatternDirectiveOptional`: re-home to `instruction1Template` — empty `pattern_directive` renders cleanly (no leftover `{{`, no orphan `## Constraints` heading, no stray blank-line block), and a non-empty value precedes "## What to review (the target)".
  - Add `TestTemplate_OrchestratorExcludesDownstreamBodies` (the guard, new): render via `composePrompt` (a solo profile is fine) and assert the returned `orchestrator` string does NOT contain `"not whether it gets fixed"`, `"verdict:"`, `"findings:"`, `"SINGLE message"`, or `"subagent_type"` (use `requireNotContains`). Add a comment naming the retained job-B one-liner so a future reader knows why the guard uses colon-form and cluster-spawn tokens rather than the generic "fix-everything" phrase.
  - Also update `template_test.go`'s top-of-file package comment (template_test.go:1-6 today), which currently says the file pins "the embedded prompt template's" load-bearing statements and "proves the template actually fills through stencil with all nine required markers" — rewrite it to describe the four-asset split: per-asset substring pins across the orchestrator and the three instruction assets, per-asset fill tests (each asset filling its own marker subset), and the new orchestrator guard test.
- **Commit:** `burler: relocate template pins and add orchestrator guard test`

### Card 6: Update composePrompt unit tests for the split return

- **Context:**
  - `internal/burlerengine/prompt.go`
  - `internal/burlerengine/round-orchestrator-template.md`
  - `internal/burlerengine/instruction-1-explore-template.md`
  - `internal/burlerengine/instruction-2-review-template.md`
  - `internal/burlerengine/instruction-3-fix-template.md`
- **Edits:**
  - `internal/burlerengine/prompt_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Adapt every `composePrompt(&p, "")` call site to the new signature `composePrompt(&p, "", inst1, inst2, inst3)` (pass any three distinct non-empty placeholder paths). Add a package-private test helper `combinedPrompt(orchestrator string, files []instructionFile) string` that joins the orchestrator and all instruction files' `Content` with newlines. Point the presence/absence assertions in `TestComposePrompt_FillsAllMarkers`, `TestComposePrompt_FixScope`, `TestComposePrompt_ToolUse`, `TestComposePrompt_PriorRounds`, `TestComposePrompt_DirectoryAnnotation`, and `TestComposePrompt_ClusterRules` at that combined text (this preserves their present/absent semantics across the four assets). Add `TestComposePrompt_ReturnsThreeInstructionFiles`: the happy path returns a non-empty orchestrator and exactly three `instructionFile` entries whose `Path` values equal the three path params in order. Add `TestComposePrompt_BlockHelpersLandInIntendedAsset`: `fix_scope_rules` content (e.g. "Write surface") appears in the instruction-3 file's `Content` and NOT in the orchestrator; `cluster_rules` content appears in the instruction-2 file; `pattern_directive`/`target` content appears in the instruction-1 file. Keep `requireContains`/`requireNotContains`/`findLineContaining` usage intact.
- **Commit:** `burler: update composePrompt tests for the split return`

### Card 7: Add Engine.Run materialization tests and fix layout Cwd in existing tests

- **Context:**
  - `internal/burlerengine/engine.go`
  - `internal/burlerengine/prompt.go`
  - `internal/hubgeometry/hubgeometry.go`
- **Edits:**
  - `internal/burlerengine/engine_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Update `newEngineForTest` to build `New(shuttle, &hubgeometry.Layout{WorktreeRoot: root, Cwd: root}, Config{})`, and add `Cwd: root` to every other inline `&hubgeometry.Layout{WorktreeRoot: root}` literal in this file (the ForkSubagents and ClusterAuditPolicy subtests) — without it, `DotLyxDir()` resolves to `.lyx` under the test process cwd and `Run` would write `.lyx/burler/round-*` into the real package tree (see the overview's "DotLyxDir is Cwd-anchored" decision). Add `TestEngine_Run_MaterializesInstructionFiles`: with a `fakeShuttle` (done outcome, scripted `approvedReview`/`nothing fixed`) and a layout whose `Cwd` is a `t.TempDir()`, after `Run` assert (a) three files exist on disk under `filepath.Join(layout.DotLyxDir(), "burler")` (glob `round-*/instruction-*.md` — exactly three), (b) `shuttle.spec.Prompt` contains each of the three absolute instruction paths, and (c) an instruction file's content reflects a filled marker (e.g. the target/fasit/rubric text from the profile appears in the instruction-1 file). Add `TestEngine_Run_MaterializeFailure`: set the layout's `Cwd` to a path whose `.lyx` parent cannot be created — create a regular file `notdir` in the temp root and set `Cwd = filepath.Join(root, "notdir")` so `MkdirAll(<notdir>/.lyx/burler)` fails — then assert `Run` returns an error containing `"materialize instruction files"` and that `shuttle.called` is false (the shuttle must never run). Leave all other engine tests' assertions unchanged beyond the `Cwd` addition.
- **Commit:** `burler: test per-round instruction-file materialization`

### Card 8: Update the module doc for the four-asset layout

- **Context:**
  - `internal/burlerengine/engine.go`
  - `internal/burlerengine/round-orchestrator-template.md`
  - `internal/burlerengine/instruction-1-explore-template.md`
  - `internal/burlerengine/instruction-2-review-template.md`
  - `internal/burlerengine/instruction-3-fix-template.md`
- **Edits:**
  - `internal/burlerengine/doc.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In the "# The A/B round" section, replace the parenthetical reference "the embedded prompt template (review-prompt-template.md via template.go) that states this rule to the agent every round" with a description of the new layout: a thin orchestrator plus three instruction assets (`round-orchestrator-template.md` and `instruction-{1-explore,2-review,3-fix}-template.md` via `template.go`), the orchestrator being the single source of truth for ordering. Add one sentence on runtime materialization: `Engine.Run` renders the three instruction files per round and writes them to a fresh dir under `.lyx` (`layout.DotLyxDir()`, machine-local, never committed — distinct from the weft-synced `_lyx`), then hands the shuttle only the orchestrator, which names their absolute paths so the agent reads each step's rules when it reaches that step. Keep the A-before-B and fix-everything substance unchanged.
- **Commit:** `burler: document the orchestrator + instruction-file layout`

### Card 9: Update the Review Round Invariant "Enforced by" line

- **Context:**
  - `internal/burlerengine/template_test.go`
- **Edits:**
  - `CONSTRAINTS.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In the Review Round Invariant's "**Enforced by**" bullet, update the named tests to reflect the restructured pins now spread across the four assets and the new guard test: `TestTemplate_StatesRoundDiscipline` (now pinning the orchestrator's sequencing statements and instruction 3's fix-everything/never-push statements), `TestTemplate_StatesClusterForkDiscipline` (instruction 2's cluster sequencing/read-only statements via a cluster-profile `composePrompt` render), and the new `TestTemplate_OrchestratorExcludesDownstreamBodies` (guarding that the inline orchestrator does not carry the downstream instruction bodies — the lazy-read separation). Leave the invariant's substance (review fully on disk before B) unchanged; keep the "no self-grading, commit-per-fix" clause as a review obligation.
- **Commit:** `burler: update Review Round Invariant enforced-by tests`

## Batch Tests

`verify: go test ./internal/burlerengine/... ./internal/stencil/...` (native Go runner, no `PYTHONPATH=` prefix — this is a Go project, not the mill Python suite; plain-string form implies `cwd: git_root`, correct here since the hub root equals the git root — not a nested layout). Scope covers the whole burlerengine package (the only package whose source, embeds, and tests change) plus `internal/stencil` as a sanity check that the reused `Fill`/`FillOptional` marker semantics still hold against the four new assets, exactly as the discussion's Testing section specifies. The package's own test suite — `template_test.go`, `prompt_test.go`, `engine_test.go` (all edited here) plus the untouched `smoke_round_test.go`/`smoke_cluster_test.go` (opt-in real-engine, behavior-level, expected green unchanged) and `cluster_test.go`/`verdict_test.go`/etc. — is the machine half of the Review Round Invariant and the materialization contract. Repo-wide regressions outside this package are caught by the configured `done_gate` (`go test ./...`) at task completion, so the batch verify stays scoped to the two affected packages rather than running the whole tree on every implementer round.
