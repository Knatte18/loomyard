# Batch: stencil-prompt-and-docs

```yaml
task: 'loom: Plan-Write producer'
batch: 'stencil-prompt-and-docs'
number: 4
cards: 3
verify: go test ./internal/loomengine/...
depends-on: [3]
```

## Batch Scope

This batch finishes the producer's prompt half and lands the documentation the Documentation Lifecycle requires in the same task: four new sections in `contracts/stencils/loom/loom-template-plan.md`, the `internal/loomengine/plan_test.go` assertions that pin each of them plus the `Plan-never-reads-support-log` build-time assertion, and the module-doc and roadmap updates.
It is one batch because the prompt-content assertions are meaningless without the stencil text they assert on, and because the design-doc statements being corrected are statements about the stencil and the producer this batch completes.
No later batch consumes anything from this one — it is the last.
Batch-local decision, differing from nothing in `## Shared Decisions`: `internal/loomengine/plan_test.go` reads the stencil through the embed (`stencils.LoomTemplatePlan`, seeded into a temp dir by `newTestStencilsDir`), so editing `contracts/stencils/loom/loom-template-plan.md` is what the assertions see — no separate seeding or sync step is needed to make the tests exercise the new text.

## Cards

### Card 12: four new sections in the plan stencil

- **Context:**
  - `contracts/stencils/loom/loom-template-discussion.md`
  - `contracts/specs/loom-plan-spec.md`
  - `manifest/designs/plan-card-format.md`
  - `internal/loomengine/plan.go`
  - `internal/loomcli/validate.go`
- **Edits:**
  - `contracts/stencils/loom/loom-template-plan.md`
  - `plugins/scribe/skills/INDEX.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add four sections to `contracts/stencils/loom/loom-template-plan.md`, introducing no new `{{.X}}` marker anywhere — the file's four markers stay exactly `{{.decision_record_path}}`, `{{.plan_dir}}`, `{{.overview_path}}`, and the optional `{{.pattern_directive}}`, so the leading HTML comment's marker inventory stays accurate and needs no edit. Every added line follows this repo's semantic-line-break rule: one sentence per line, breaking long sentences at internal independent-clause boundaries, never hard-wrapping at a fixed column. Keep the existing step numbering intact by numbering the skill-loading section Step 0 and the self-check section Step 5.

  **First**, a `## Step 0 — Load the writing skills` section, placed after the two-line "You are the Plan producer" intro paragraph and before the `{{.pattern_directive}}` line, modelled on the identically-named section in `contracts/stencils/loom/loom-template-discussion.md`. It instructs the agent to load `scribe:prose` first and `scribe:testing` second, states that `scribe:prose` comes first because it is the always-active writing discipline every other skill's output is judged against, states that `scribe:testing` is loaded because the card-granularity rule and the bundle-your-own-test rule are testing judgments rather than prose judgments, and states that both loads are best-effort — an unresolvable skill name is not an error and the agent continues without it. Do not instruct it to load `scribe:conversation`, `scribe:code-quality`, or any `scribe:golang-*` skill.

  **Second**, a degraded-mode subsection under the existing `## Step 2 — Explore the codebase`, headed `### No quarry inventory exists — do the lookups yourself`. It must state plainly that no quarry inventory is handed to the agent, that this is the normal state and is never an error and never a reason to stop, and that the agent therefore performs the mechanical lookups itself. Name the three substitutes as bullets: `go doc <pkg> <Symbol>` for a symbol's existence and definition, a `grep -rn` scoped to the package that owns the symbol for its call sites, and a manual read of each call site's own enclosing function for blast radius. Do not add a marker for the inventory: `Plan-Sweep` is not a row in loom's producer list, so a marker could only ever render one constant string.

  **Third**, a Verify-model paragraph appended to the existing paragraph that begins "Every `Verify:`/`verify:` value", inside the `### Each NN-<card-slug>.md` section. It states the authoring rule only: a per-card `**Verify:**` is exceptional rather than routine, written only for what a package-scoped automatic test run cannot catch on its own, and the plan-level `## verify:` section is the single integration check for the whole plan. It then points at `manifest/designs/plan-card-format.md`'s Verify model section for the tier definitions themselves and says not to restate them here. Do not reproduce the three-tier taxonomy — that is format-contract content the Producer Pointer-Rule Invariant forbids duplicating into a producer's own instruction file.

  **Fourth**, a `## Step 5 — Self-check before ending your turn` section, placed after the existing `## Step 4 — Write {{.overview_path}} LAST` section and before the closing `## Never use AskUserQuestion` section, modelled on the Step 6 self-check in `contracts/stencils/loom/loom-template-discussion.md`. It instructs the agent to run `lyx loom validate-plan` in a fenced `bash` block, states the verb takes no arguments, states that it exits 0 on a clean gate and 1 otherwise with its findings under the failure envelope's `findings` key, and instructs the agent to fix whatever it reports and re-run until it exits 0 before ending its turn. Placing it after Step 4 is deliberate: the verb parses the plan through the overview, which Step 4 writes last.

  In `plugins/scribe/skills/INDEX.md`, the lyx-stencil bullet says the "Load these skills: ..." wiring now ships in `contracts/stencils/loom/loom-template-discussion.md`'s Step 0, which loads `scribe:prose` then `scribe:conversation` — extend that sentence to name `contracts/stencils/loom/loom-template-plan.md`'s Step 0 alongside it, loading `scribe:prose` then `scribe:testing`. Change nothing else in that file.
- **Commit:** `feat(loom): finish the plan stencil for the Card format`

### Card 13: pin the new stencil sections and the support-log exclusion

- **Context:**
  - `contracts/stencils/loom/loom-template-plan.md`
  - `internal/loomengine/prompt_test.go`
  - `internal/loomengine/plan.go`
  - `internal/loomengine/config.go`
- **Edits:**
  - `internal/loomengine/plan_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add five tests to `internal/loomengine/plan_test.go`, each modelled on the existing `TestPlanSpec_PromptStates…` family: a `renderedPlanPrompt(t)` call followed by a `for _, want := range []string{…}` loop over substrings, with a `t.Errorf` naming the missing substring and the contract it carries. Place them beside the existing prompt-content tests, before `renderedPlanPrompt`'s own definition.

  `TestPlanSpec_PromptStatesSkillLoads` asserts the prompt contains `scribe:prose` and `scribe:testing`, and does not contain `scribe:conversation` — the last of these being the point of the choice, since chat-reply discipline has no operator to serve in an autonomous session.

  `TestPlanSpec_PromptStatesDegradedQuarryMode` asserts the prompt states that no quarry inventory is handed to the agent, that its absence is never an error, and that the agent does the lookups itself — pin at least the `go doc` and `grep -rn` substrings plus one phrase from the never-an-error sentence, choosing whatever exact substrings card 12's text actually uses.

  `TestPlanSpec_PromptStatesSelfCheck` asserts the prompt contains `lyx loom validate-plan` and a phrase instructing a re-run until it exits 0.

  `TestPlanSpec_PromptStatesVerifyIsExceptional` asserts the prompt states that a per-card `Verify:` is exceptional rather than routine and that the plan-level `## verify:` is the single integration check for the whole plan. Do not weaken or delete the existing `TestPlanSpec_PromptStatesVerifyIsRunnable`, which pins the separate never-prose rule.

  `TestPlanSpec_PromptNeverNamesSupportLog` is not a substring-presence test but an absence test, and it builds its own layout rather than calling `renderedPlanPrompt`, because it needs the `*lyxcwd.Location` in hand to compute the path it asserts against. Build the same `&lyxcwd.Location{HubPath: …, WorktreeName: …}` shape `TestPlanSpec` uses, call `PlanSpec(layout, newTestStencilsDir(t), cfg, reg)`, and assert the returned `spec.Prompt` contains neither the literal `support-log.md` nor `DiscussionSupportLog(layout)`'s returned absolute path. Its doc comment must record what the test is for: `manifest/designs/loom.md` states that the `Plan-never-reads-support-log` boundary is asserted once, at build/test time, over `Plan-Write`'s producer definition rather than per run, and that the assertion lands with the real `Plan-Write` — this is that assertion. Note that the stencil's existing prose "never read the support log" uses a space rather than a hyphen and therefore does not collide with the `support-log.md` substring being asserted absent.
- **Commit:** `test(loomengine): pin the new plan-stencil sections and the support-log boundary`

### Card 14: record the as-built producer in the module doc and the roadmap

- **Context:**
  - `internal/loomshed/planwrite.go`
  - `internal/shedrecipe/entries_planwrite.go`
  - `internal/loomcli/wiring.go`
  - `contracts/stencils/loom/loom-template-plan.md`
  - `internal/planparser/parse.go`
  - `internal/loomengine/plan_test.go`
- **Edits:**
  - `manifest/designs/loom.md`
  - `manifest/roadmap.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Three statements in `manifest/designs/loom.md` go stale with this task, and each must be corrected in place, following this repo's semantic-line-break rule.

  In the producer table, row 7's Input cell reads "`_lyx/discussion/decision-record.md` (**never** `support-log.md`) + `Plan-Sweep`'s inventory". `Plan-Sweep` is not a row in the built list and produces no artifact, so rewrite the trailing clause to say the inventory arrives only once `Plan-Sweep` is built for real, and that its absence today is the normal state rather than an error — the degraded mode the stencil now names outright. Leave the "**never** `support-log.md`" parenthetical exactly as it stands.

  The paragraph headed "**The `Plan-never-reads-support-log` boundary is not a per-run check.**" ends with a sentence beginning "This assertion lands with the real `Plan-Write`: today `Plan-Write` is a stub declaring no input set at all". Replace that sentence with the as-built statement: the assertion has landed, as `TestPlanSpec_PromptNeverNamesSupportLog` in `internal/loomengine/plan_test.go`, which proves the composed plan prompt names neither `support-log.md` nor the support log's own absolute path. Leave the preceding two sentences — the boundary itself and the once-at-build-time framing — unchanged.

  In the module table near the end of the file, the "producers (discussion / plan)" row says `PlanSpec` is "still ✅ **built** but not yet wired into `Shed` — see `manifest/roadmap.md`'s `loom: Plan-Write producer` item". Rewrite it to say `PlanSpec` is built and wired, as the recipe's `Plan-Write` row's `PlanWrite` engine, matching how the same row already describes `DiscussionSpec`, and drop the now-satisfied roadmap pointer. Name the row rather than numbering it: the module table's neighbouring `DiscussionSpec` clause says "recipe row 3", which happens to be unambiguous only because `Discussion-Write` sits at index 3 in both this file's 14-row design table and the recipe's own 13-row list — `Plan-Write` does not, being 7 in the design table and 6 in the recipe, so a number here would be wrong under one reading whichever value it took.

  In `manifest/roadmap.md`, move the Wave 3 item **loom: Plan-Write producer** out of the Planned list and into the Done list, following the shape of the Done **loom: Discussion-Write producer** entry directly: a lead sentence naming what replaced what, then short follow-up sentences for the notable specifics, then a `See [designs/loom.md](designs/loom.md#…)` pointer line. The entry must record that the `Plan-Write` stub was replaced by a real `SingleLLMProducer` behind a new `loomshed` rotate-and-commit decorator, reached through a new `PlanWrite` registry entry over two injected `shedrecipe.Env` closures (`PlanSpec`, `CommitPlan`); that the decorator archives every top-level `.md` file of `_lyx/plan/` into an `archive-<stamp>/` subdirectory before delegating, which is what keeps a `Plan-Validate` or `Plan-Review` bounce from ping-ponging on `index-file-mismatch`; that `internal/planparser` gained one pure string helper, `ArchiveDirName`, as the sole declarer of that subdirectory's name; that the registry grew to fourteen entries; and that the stencil gained a `scribe:prose`/`scribe:testing` load step, a degraded-mode section for the absent quarry inventory, a Verify-authoring rule, and a closing `lyx loom validate-plan` self-check. Note the same already-recorded operator prerequisite the Discussion-Write entry names: `lyx stencil sync`, because an already-seeded hub keeps the old stencil text until a sync forces a refresh.

  Then reconcile the three surrounding statements the move falsifies. The Wave 3 preamble numbers its items — renumber what remains so **webster: DAG-derived card sequencing** is the sole remaining Wave 3 item, and say so, mirroring how the Wave 2 line already records its own completion. The Someday item **loom: build `Plan-Sweep` for real** says it "stays a stub past the split-out `loom: Plan-Write producer` item above" — retarget that reference to the Done entry. The Someday item **shedrecipe: capability-declaration instead of manual seam-threading** cites the Discussion-Write entry's two new `Env` fields as its concrete data point — add `PlanWrite`'s own two (`PlanSpec`, `CommitPlan`) as a second data point, since the identical three-layer threading was repeated verbatim one task later, which strengthens the case that motivates the item.
- **Commit:** `docs(loom): record the as-built Plan-Write producer`

## Batch Tests

`verify: go test ./internal/loomengine/...` covers the one Go package this batch touches.
That package is where the stencil's rendered content is asserted: `newTestStencilsDir` seeds `stencils.LoomTemplatePlan` — the embed of `contracts/stencils/loom/loom-template-plan.md` — into a temp directory, and `renderedPlanPrompt` runs the real `PlanSpec` over it, so card 12's stencil edits and card 13's assertions are verified together by this one run.
It is untagged tier 1.
Cards 12 and 14 also touch three Markdown files with no Go test of their own (`plugins/scribe/skills/INDEX.md`, `manifest/designs/loom.md`, `manifest/roadmap.md`);
their correctness is a review obligation under the Documentation Lifecycle, not a machine check, which is why the verify scope stays at the one package rather than widening pointlessly.
