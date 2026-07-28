# Batch: prompt-wiring

```yaml
task: 'PATTERN wiring: conditional constraint-injection into every agent'
batch: prompt-wiring
number: 7
cards: 6
verify: go test -tags integration ./...
depends-on: [6]
```

## Batch Scope

This batch spends everything the previous six built: it adds a `{{.pattern_directive}}` marker to exactly five existing templates and plumbs `pattern.Directive` into each of their five prompt-assembly sites. No template is created — all five already exist. It is one batch because there is **no shared prompt-assembly layer**: each engine builds its own `map[string]string` inline, so the five sites are five near-identical edits that share the same reasoning and the same review criteria, and splitting them would make a reviewer re-derive that reasoning per batch.

Three rules apply to all five cards and are not restated per card. The marker sits **immediately before the template's first `##` heading** — after the `#` title and any leading orientation blockquote, one mechanical rule with no per-template exceptions. That formulation is forced rather than aesthetic: the directive block carries its own `##` heading, so dropping it inside an existing `##` section would silently terminate that section and orphan its remaining prose under the constraints heading. The marker is filled via `stencil.FillOptional` with `"pattern_directive"` in the optional list, never `stencil.Fill`. And each template's leading banner comment must be updated — the marker count it states and its claim about conditionals — because those banners are what the next reader trusts.

The inclusion criterion, for the record and for the reviewer: a template gets the directive if its agent (a) writes code, (b) reviews code, (c) is a context-inheritance root whose in-session forks do, **or** (d) authors the typed file-op instructions a later code-writing agent executes near-verbatim.

Clause (c) is what admits webster Master, whose prompt says outright that it never edits code. Clause (d) is what admits `loomengine/plan-template.md`, and it has to be stated rather than assumed, because the Plan producer is **structurally identical to the Discussion producer** — `internal/loomengine/plan.go`'s own doc comment draws that parallel in as many words, "Like the Discussion producer (discussion.go), the Plan producer is not a Go module — it is a prompt/profile fed to shuttle.Run" — and this batch excludes Discussion. Without clause (d) the plan would treat two structurally identical producers oppositely with no stated reason. What separates them is what their output *is*, not what they are: the Plan producer emits plan-format-v3 cards whose `Edits:`/`Creates:`/`Moves:`/`Requirements:` fields a builder implementer or webster fork then executes near-verbatim, so an invariant violated at planning time is not reconsidered downstream — it is carried into every card that inherits it and then implemented. The Discussion producer emits a decision record that the Plan producer itself re-derives from against the codebase, so a constraint miss there still has a gate after it: the plan prompt, which now carries the directive. Plan is the last authoring point before code; Discussion is not.

Clause (d) is also why `plan-template.md` gets `RoleImplementer` rather than a variant of its own. The block's wording addresses the agent that writes code, and that is exactly the audience the plan's `Requirements:` prose is written *for* — the directive travels through the plan to the agent that executes it, which is the same reasoning clause (d) rests on.

`builderengine/orchestrator-template.md` fails all four — its implementers are separate spawned sessions that each receive `RoleImplementer` directly, so there is no inherited context to seed, and it authors no file-op instructions of its own — and stays out, as do `websterengine/integration-template.md`, `loomengine/discussion-template.md`, the four `treadleengine` templates and `reedengine/header-template.md`. The three existing hardcoded `CONSTRAINTS.md` mentions in `loomengine/discussion-template.md`, `loomengine/plan-template.md` and `websterengine/master-template.md` stay exactly as they are: the directive is added alongside them, never in place of them, since replacing them is migration work and out of scope.

## Cards

### Card 24: wire the directive into builder's implementer prompt

- **Context:**
  - `internal/pattern/pattern.go`
  - `internal/stencil/stencil.go`
  - `internal/hubgeometry/hubgeometry.go`
- **Edits:**
  - `internal/builderengine/implementer-template.md`
  - `internal/builderengine/spawn.go`
  - `internal/builderengine/template_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add `{{.pattern_directive}}` to `internal/builderengine/implementer-template.md` immediately before its first `##` heading. In `internal/builderengine/spawn.go`'s `SpawnBatch` fill site, add `"pattern_directive": pattern.Directive(deps.Layout, pattern.RoleImplementer)` to the values map and switch the call from `stencil.Fill` to `stencil.FillOptional(ImplementerTemplate(), values, []string{"pattern_directive"})`. `SpawnDeps` already carries a `Layout *hubgeometry.Layout` field, and `internal/buildercli/spawnbatch.go` already assigns it, so no plumbing is needed here beyond the call itself. Update the template's leading banner comment: the marker count it states must go from five to six, and the sentence claiming there are no `{{if}}`/`{{range}}` conditionals anywhere in the file stays true and stays — the whole reason this task extended `stencil` rather than reusing the `{{if}}` idiom is to keep that claim uniform across these files. Add a sentence to the banner naming `pattern_directive` as the one optional marker, filled via `FillOptional`, which renders as nothing when PATTERN is inactive. In `internal/builderengine/template_test.go`, add `pattern_directive` to `implementerTemplateMarkerValues()` with a placeholder string, and **exclude it from the per-marker deletion sweep** in `TestImplementerTemplate_FillsWithAllMarkers` — deleting an optional marker must not error. Follow the precedent `internal/websterengine/template_test.go` already sets for `rename_mechanic`, including carrying an explanatory comment. Add two positive cases: filling with an **empty** `pattern_directive` succeeds and the rendered output contains no leftover `{{`, no orphan `## Constraints` heading and no stray blank-line block where the directive would have been; and filling with a non-empty directive places it in the output ahead of the first work instruction.
- **Commit:** `builder: inject the PATTERN directive into the implementer prompt`

### Card 25: wire the directive into burler's round prompt

- **Context:**
  - `internal/pattern/pattern.go`
  - `internal/stencil/stencil.go`
  - `internal/hubgeometry/hubgeometry.go`
- **Edits:**
  - `internal/burlerengine/review-prompt-template.md`
  - `internal/burlerengine/prompt.go`
  - `internal/burlerengine/engine.go`
  - `internal/burlerengine/template_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add `{{.pattern_directive}}` to `internal/burlerengine/review-prompt-template.md` immediately before its first `##` heading. `composePrompt(p *Profile)` in `internal/burlerengine/prompt.go` is a pure string function with no filesystem access, and it must stay that way: give it an additional `patternDirective string` parameter rather than a `*hubgeometry.Layout` or a path, put that value into the values map under `pattern_directive`, and switch its call to `stencil.FillOptional(reviewPromptTemplate, values, []string{"pattern_directive"})`. Compute the directive **one level up**, at `Run` in `internal/burlerengine/engine.go`, which already holds `e.layout` and already uses it: `pattern.Directive(e.layout, pattern.RoleReviewFix)`. The role is `RoleReviewFix`, not a reviewer role, because this template's own text says the agent has two jobs in order in one session and part B is fixing what part A found. The directive is **not** gated on whether the round's target is code or prose: loomyard has no target-type classification, a file-extension heuristic would be new fragile logic whose misclassification silently removes the constraints, a prose target carrying a pointer to the invariants is mild noise rather than harm, and invariants can govern prose anyway. Update the template's leading banner comment from nine markers to ten, naming `pattern_directive` as the optional one, and keep its no-conditionals claim. In `internal/burlerengine/template_test.go`, add `pattern_directive` to `allMarkerValues()`, exclude it from the deletion sweep in `TestTemplate_FillsWithAllMarkers` with an explanatory comment, and add the same two positive cases card 24 specifies.
- **Commit:** `burler: inject the PATTERN directive into the round prompt`

### Card 26: wire the directive into webster's fork prompt and swap its anchor to a Layout

- **Context:**
  - `internal/pattern/pattern.go`
  - `internal/stencil/stencil.go`
  - `internal/hubgeometry/hubgeometry.go`
- **Edits:**
  - `internal/websterengine/fork-template.md`
  - `internal/websterengine/render.go`
  - `internal/websterengine/beginbatch.go`
  - `internal/websterengine/recoverbatch.go`
  - `internal/webstercli/beginbatch.go`
  - `internal/websterengine/template_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add `{{.pattern_directive}}` to `internal/websterengine/fork-template.md` immediately before its first `##` heading. Change `RenderForkPrompt`'s signature so its `worktreeRoot string` parameter is **replaced** by an `l *hubgeometry.Layout` — one source of the fact rather than two that can disagree — and fill `"pattern_directive"` from `pattern.Directive(l, pattern.RoleImplementer)`, switching the call to `stencil.FillOptional`. **Critical, and the easiest thing to get wrong here: `{{.worktree_root}}` must keep rendering `l.Cwd`, not `l.WorktreeRoot`.** The `Deps.WorktreeRoot` field is misleadingly named — every caller assigns `c.layout.Cwd` to it (`internal/webstercli/beginbatch.go`, `internal/buildercli/run.go`, `internal/buildercli/spawnbatch.go`) and `RenderForkPrompt` feeds that straight into the marker — so at `RelPath != "."`, the exact geometry this plumbing exists for, `Cwd != WorktreeRoot` and filling the marker from `Layout.WorktreeRoot` after the swap would silently change what the fork prompt calls its worktree root. Fill it from `l.Cwd` and the rendered value is byte-identical to today's. The two anchors then agree by construction — `PatternFileHere()` resolves `WorktreeRoot`+`RelPath`, which equals `Cwd` for any `Resolve`-built Layout — but that holds because both are Cwd-equivalent, **not** because the two fields are interchangeable. Do **not** rename `Deps.WorktreeRoot`; it is out of scope. Plumb the Layout to both call sites: `RecoverDeps` already has a `Layout *hubgeometry.Layout` field, but **`BeginDeps` does not** — add one, and assign `Layout: c.layout` in `internal/webstercli/beginbatch.go` where the `BeginDeps` literal is built. Update the template's leading banner comment, which today says six markers with `rename_mechanic` as the one branch-internal exception: it becomes seven, and the banner must now distinguish two different mechanisms rather than conflating them — `rename_mechanic` is branch-internal, sitting inside this file's own `{{if .rename_mechanic}}` block, while `pattern_directive` is top-level and optional via `FillOptional`. `rename_mechanic` is deliberately left exactly as it is; `FillOptional` does not retroactively change it. In `internal/websterengine/template_test.go` — an **external `websterengine_test` package**, so the signature change breaks every call site rather than being a package-internal edit — update all ten `RenderForkPrompt` call sites to pass a Layout, updating rather than deleting them. Add `pattern_directive` to `forkTemplateMarkerValues()`, exclude it from the deletion sweep in `TestForkTemplate_FillsWithAllMarkers` alongside the existing `rename_mechanic` exclusion, and add the same two positive cases card 24 specifies. Add one case specific to this file: an empty `pattern_directive` and an empty `rename_mechanic` together render with neither an orphan `## Constraints` heading nor an orphan `## Rename mechanic` heading.
- **Commit:** `webster: inject the PATTERN directive into the fork prompt`

### Card 27: wire the orchestrator directive into webster's Master prompt

- **Context:**
  - `internal/pattern/pattern.go`
  - `internal/stencil/stencil.go`
  - `internal/hubgeometry/hubgeometry.go`
- **Edits:**
  - `internal/websterengine/master-template.md`
  - `internal/websterengine/render.go`
  - `internal/websterengine/runlevel.go`
  - `internal/websterengine/template_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add `{{.pattern_directive}}` to `internal/websterengine/master-template.md` immediately before its first `##` heading — that is, after the `#` title and after both leading orientation blockquotes, and **not** beside the existing `CONSTRAINTS.md` sentence, which sits inside the orientation section and would be split from the `_lyx` rule that follows it. The consequence is accepted and is the right order anyway: the PATTERN block sits somewhat ahead of the existing `CONSTRAINTS.md` sentence, and PATTERN is the repo-owned invariants doc while `CONSTRAINTS.md` is mill's. Give `RenderMasterPrompt` an `l *hubgeometry.Layout` parameter — it has neither a root nor a Layout today — fill `"pattern_directive"` from `pattern.Directive(l, pattern.RoleOrchestrator)`, and switch to `stencil.FillOptional`. `RoleOrchestrator`, not `RoleImplementer`: this template states in as many words that Master never edits code, which is the verbatim reason builder's orchestrator is excluded from this whole task, so an implementer-worded instruction would be one Master cannot carry out. Master qualifies on the context-inheritance clause instead — its forks are in-session and thin precisely because they inherit everything Master has read, so reading PATTERN once here is what puts the constraints in front of all of them. Update the sole caller, `internal/websterengine/runlevel.go`, whose `RunDeps` already carries a `Layout *hubgeometry.Layout` field, so it passes `deps.Layout`. Update the template's leading banner comment from seven markers to eight, naming `pattern_directive` as the optional one and keeping the no-conditionals claim, which stays true for this file. In `internal/websterengine/template_test.go`, add `pattern_directive` to `masterTemplateMarkerValues()`, exclude it from the deletion sweep in `TestMasterTemplate_FillsWithAllMarkers`, update any `RenderMasterPrompt` call site for the new parameter, and add the same two positive cases card 24 specifies.
- **Commit:** `webster: inject the PATTERN orchestrator directive into the Master prompt`

### Card 28: wire the directive into loom's plan prompt

- **Context:**
  - `internal/pattern/pattern.go`
  - `internal/stencil/stencil.go`
  - `internal/hubgeometry/hubgeometry.go`
- **Edits:**
  - `internal/loomengine/plan-template.md`
  - `internal/loomengine/plan.go`
  - `internal/loomengine/plan_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add `{{.pattern_directive}}` to `internal/loomengine/plan-template.md` immediately before its first `##` heading — before `## Step 1`, **not** beside the existing `CONSTRAINTS.md` sentence inside `## Step 2`, for the same section-splitting reason card 27 gives. `composePlanPrompt(decisionRecordPath, planDir, overviewPath string)` in `internal/loomengine/plan.go` is a pure string function and stays one: give it an additional `patternDirective string` parameter rather than a Layout or a path, add it to the values map under `pattern_directive`, and switch to `stencil.FillOptional(planTemplate, values, []string{"pattern_directive"})`. Compute the directive one level up in `PlanSpec`, which already takes `layout *hubgeometry.Layout` directly: `pattern.Directive(layout, pattern.RoleImplementer)`. Update the template's leading banner comment from three markers to four, naming `pattern_directive` as the optional one and keeping the no-conditionals claim. `internal/loomengine` has no `template_test.go`; the plan template's marker coverage lives in `internal/loomengine/plan_test.go`, whose `TestPlanSpec_PromptFilled` asserts the rendered prompt carries no leftover `{{`. Extend that file with the two positive cases card 24 specifies — an empty directive renders with no leftover `{{`, no orphan `## Constraints` heading and no stray blank block, and a non-empty directive appears ahead of `## Step 1` — driving them through `PlanSpec`. **The non-empty case needs a different fixture from every other test in this file, and that difference is the trap.** The existing tests build a Layout from a path that never exists on disk (`&hubgeometry.Layout{WorktreeRoot: filepath.Join("home", "user", "repo")}`), which is fine for their pure string-shape assertions. `pattern.Directive` performs a real `os.Stat` on `_pattern/PATTERN.md`, so reusing that fake Layout would always render the directive empty and the placement assertion would pass vacuously while proving nothing. Build the non-empty case's Layout on a `t.TempDir()` with a real `_pattern/PATTERN.md` seeded on disk. This is also the one card in this batch whose test touches the filesystem at all — cards 24 through 27 inject `pattern_directive` directly as a stencil value and never stat anything, because their templates are exercised through `stencil.FillOptional` rather than through a Layout-taking entry point. `t.TempDir()` is not a banned token under the Test Tier Purity Invariant, so `internal/loomengine/plan_test.go` stays untagged.
- **Commit:** `loom: inject the PATTERN directive into the plan prompt`

### Card 29: correct the PATTERN design doc and move the roadmap item

- **Context:**
  - `internal/hubgeometry/hubgeometry.go`
  - `internal/fabricengine/junction.go`
  - `internal/fabricengine/add.go`
  - `internal/fabricengine/status.go`
  - `internal/initengine/init.go`
  - `internal/fabricengine/template.yaml`
  - `internal/websterengine/fork-template.md`
  - `internal/pattern/pattern.go`
- **Edits:**
  - `manifest/designs/pattern.md`
  - `manifest/roadmap.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Apply six corrections to `manifest/designs/pattern.md`, each of which the implementation disproves. (1) The claim that PATTERN is "the first genuinely-conditional token in the system" is **wrong**: `websterengine`'s `rename_mechanic` predates it, sitting inside a `{{if .rename_mechanic}}` block in `fork-template.md` in production today. Restate the rationale accordingly — the optional-marker extension is a deliberate design choice, not a forced one, chosen because it puts optionality in Go where it is testable per call site and keeps the "no conditionals in templates" banner rule uniform, not because `{{if}}` could not have worked. (2) "The `_pattern` junction is fabric's responsibility" is imprecise: `fabricengine` owns the primitive, `initengine.Init` is the caller, and `fabric add` explicitly does not wire junctions — its own code says the junction is wired by `lyx init` via `WireJunctions`. (3) "sibling of `_lyx` and `_raddle`" overstates `_raddle`, which has no junction at all today, so `_pattern` is the first *second* junction rather than a third peer. (4) The doc says fabric "always creates the `_pattern/` directory in weft … and the junction"; neither half was true of fabric as the code stood — `fabric add` wires nothing, and no `WireJunctions` caller except `Init` materialised a weft directory. State the corrected split: `hubgeometry` declares the junction record and owns every `_pattern` path literal, `seedLyxJunction` materialises the weft target, and `initengine.Init` is the caller. (5) The doc is silent on weft persistence; note that activation also requires `_pattern` in fabric's weft pathspec, or content never leaves the machine, and that already-initialised worktrees keep their narrower pathspec and must be widened by hand. (6) Rewrite the `## Open questions` section. **Four of its five entries are settled by this task and one is not — do not collapse the section as if all five were.** Settled, and replaced by the decision: *Exact template set* (the five named in this batch, under the four-clause criterion above); *Home of the active-check helper* (`internal/pattern`, a new leaf, not `stencil` and not `fabric`); *Directive wording* (three role variants as literal constants, imperative checklists under their own `##` heading); and *Optional-marker surface in `stencil`* (an explicit allow-list passed as `FillOptional`'s third parameter, not a naming convention). Still open and carried forward as such: **Detail-submap layout** — whether `_pattern/<topic>/` has a fixed structure or is free-form per invariant — which is a question about PATTERN's own *content*, belongs to the content migration this task explicitly defers to loomyard-init-via-lyx, and is settled by nothing in these seven batches. Keep it in the section, marked as deferred to the content migration rather than silently dropped. Also correct the marker spelling: the doc's `{{.PatternDirective}}` matches no existing marker in the repo, and the implemented name is the snake_case `{{.pattern_directive}}`, consistent with `rename_mechanic`, `shared_decisions`, `worktree_root` and every other marker. Then update `manifest/roadmap.md`'s PATTERN entry to record that the **wiring** has landed while the **content migration out of `CONSTRAINTS.md` remains outstanding** and still happens only at loomyard-init-via-lyx — this is a roadmap-relevant event under the repo's rule that the roadmap moves on completing or adding a planned item. Do not remove the item; it is not finished. Write every paragraph and list item as one unwrapped line.
- **Commit:** `docs: correct the PATTERN design doc and record the landed wiring`

## Batch Tests

`verify: go test -tags integration ./...` is the one unbounded, repo-wide command in this plan, and the scope is deliberate rather than lazy. Three reasons make a narrower scope wrong here. First, this batch changes the signature of two **exported** functions, `RenderForkPrompt` and `RenderMasterPrompt`, whose callers include an external `websterengine_test` package with ten call sites — a compile break can land in any package that renders a webster prompt, and only a whole-tree build finds it. Second, four of the five cards edit an embedded template asset whose banner and marker count several packages' tests assert, and the fifth adds a struct field (`BeginDeps.Layout`) that `internal/webstercli` must fill. Third, this is the last batch, so it is the natural place for the repo-wide gate the earlier per-package scopes deliberately skipped: `go test -tags integration ./...` runs both tiers everywhere and exercises `cmd/lyx`'s cross-cutting guards — `TestEnforcement_GeometryLiterals` for a stray `_pattern` literal, `tierpurity_test.go` and `hermeticenv_test.go` for test-file hygiene, and `drift_test.go`/`helptree_test.go`/`longlist_test.go` for the help-text edits batches 3 through 5 made. The plan's own repo-wide expectation is that `go test ./...` passes plain **and** with `-tags integration`, and that `go vet ./...` is clean; this command covers the tagged run, which strictly includes the untagged tier, and `go vet` is left to the implementer's normal discipline rather than folded into a single `verify:` string. Tests updated rather than deleted in this batch: `internal/builderengine/template_test.go`, `internal/burlerengine/template_test.go`, `internal/websterengine/template_test.go` (ten `RenderForkPrompt` call sites plus the `RenderMasterPrompt` sites), and `internal/loomengine/plan_test.go`. No new test file is created, so no build-tag decision arises.
