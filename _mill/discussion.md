# Discussion: loom: Webster-Review producer

```yaml
task: 'loom: Webster-Review producer'
slug: loom-webster-review-producer
status: discussing
parent: main
```

## Problem

`loom`'s producer list has one row left stubbed: `Webster-Review`, row 13 of `contracts/recipes/loom-recipe.yaml`, still carries `engine: Stub`.
Every other row in the list is real.
`Stub` unconditionally reports `Done`, so today a `lyx loom run` walks straight from `Webster`'s committed diff to `Publish` with no gate at all over what Webster actually built —
the terminal quality gate `manifest/designs/loom.md` describes as "the full converge-loop gate over the whole diff" does not exist.

Why now: the two sibling gates already shipped.
`Discussion-Review` landed as a `Bouncer`+`Burler` perch in `eb5f091b`, `Plan-Review` in `6f66fff1`, and both proved the wiring pattern, the rubric-stencil seam, and the `Env.Review*` fallback.
`Webster-Review` is the same gap in the same shape, and it is the last one.
Its rubric criteria are already recorded in `manifest/designs/loom.md`'s "Webster-Review rubric" section, waiting to be transcribed into a stencil.

The one thing that makes this row genuinely different from its two siblings: its subject is a **diff**, not a file or a directory.
Every shipped mechanism — `BouncerConfig.ArtifactPaths`, `burlerengine.Profile.Target.Paths` — is built around file paths that exist on disk.
Deciding how a diff is expressed through those two seams is the substance of this task.

## Scope

**In:**

- A new rubric stencil `contracts/stencils/loom/loom-rubric-webster-review.md`, registered in `contracts/stencils/stencils.go` (embedded var + `entries` row), transcribed from `manifest/designs/loom.md`'s "Webster-Review rubric" section.
- Replacing the single `Webster-Review` recipe row (`engine: Stub`) with a two-row `Webster-Bouncer` + `Webster-Burler` perch in `contracts/recipes/loom-recipe.yaml`, carrying `segment: Webster-Review`.
- Two new durable row-name constants in `internal/loomshed/loomshed.go` (`NameWebsterBouncer`, `NameWebsterBurler`), replacing `NameWebsterReview`.
- Every "sixteen"/16-row count in production source and comments moved to seventeen: `internal/loomshed/doc.go`, `internal/loomshed/loomshed.go`, `contracts/recipes/loom-recipe.yaml`'s header, `manifest/designs/shed-recipe.md`, `manifest/designs/loom.md`.
- Test updates across `contracts/stencils/rubric_test.go`, `internal/loomrecipe/{coverage_guard,shape,sequence,fixture,recipe}_test.go`, and `internal/loomcli/smoke_test.go`.
- One new `internal/shedrecipe/entries_burler_test.go` case covering the `cluster-fan` profile key, which no shipped recipe row exercises today.
- Doc updates in the same commit per CLAUDE.md's task-completion rule: `manifest/designs/loom.md` (producer table row 13, the table/recipe divergence note, the "Webster-Review rubric" section gaining a "doc *about* the shipped stencil" framing paragraph matching the two sibling sections) and `manifest/roadmap.md` (Planned item removed).

**Out:**

- **Writing `product.start_sha`.**
  `contracts/specs/loom-status-spec.md` declares it as the Raddle diff base stamped when Webster begins, but nothing writes it — `internal/loomshed/seed.go` seeds it `nil` and it is never assigned again.
  This task works around its absence (see the diff-derivation decision below) and does not fix it.
  Recorded as an open risk, and it wants its own roadmap item.
- **A `Webster-Revalidate` row.**
  `Plan-Review` needed one because a mechanical validator (`planparser`) sits over its artifact and the fixer rounds rewrite that artifact after the validator already ran.
  There is no mechanical validator over a diff, so there is nothing to re-run.
- **Deleting `Stub`.**
  `loomshed.NewStub`, `shedrecipe`'s `"Stub"` registry entry, and `internal/shedrecipe/registry_test.go`'s `TestRegistry_ShipsFourteenEntries` all stay untouched — see the Stub decision.
- **Any change to `internal/websterengine`.**
  Webster stays a black box; this task only gates its output.
- **Any change to `internal/shedadapters`'s `Bouncer` or to `internal/burlerengine`.**
  The perch is built entirely out of existing, shipped config keys.
- **`Plan-Sweep`**, the `Discussion-Burler` `fix-scope: source` Fabric-Git-Invariant defect, and the `Env.WorktreeRoot`-vs-`AnchorPath()` divergence — all three are their own roadmap items and are not touched here, even though this task edits neighbouring lines in the same recipe file.

## Decisions

### diff-derivation-lives-in-the-rubric-not-in-go

- Decision: the round derives the diff range itself, at review time, from artifacts already on disk.
  `profile.target` carries **`instructions` only, no `paths`**, and `tool-use: true` lets the round run read-only git.
  The instructions name the derivation in order: read `_lyx/webster/state.json`, take the **lowest-numbered** entry in its `batches` map, use that entry's `startSha` as the base, and diff `<base>..HEAD`.
  When `state.json` is unreadable or carries no batches, fall back to the merge-base against the parent branch named in `_lyx/loom/status.json`'s `product.parent`.
- Rationale: `burlerengine.Profile.validate` resolves and stats every `Target.Paths` entry, so a path entry must exist on disk — a diff has no such file.
  `validate` accepts a `FileSet` with `Instructions` and no `Paths`, which is exactly the escape this needs.
  `_lyx/webster/state.json` is a durable, fabric-committed artifact (`internal/websterengine/state.go`), and `BatchState.StartSHA` is documented as "the durable base-commit record a resume or an operator diagnosis reads" — it is already the right source, it just was never wired to this consumer.
  Both the primary and the fallback are file reads plus read-only git, which the Fabric Git Invariant explicitly exempts ("read-only verbs … are exempt — only *mutating* warp git must dispatch through fabric").
- Rejected: (a) a new mechanical `Webster-Diff` row materializing the diff to a file for `target.paths` to point at — adds a production row and an artifact-lifecycle question to a task whose scope is a rubric and a perch;
  (b) stamping `product.start_sha` for real and threading it through `shedrecipe.Env` — the right long-term fix, but it is a `Webster`-row change, a spec change, and a coherence-check change, i.e. its own task.

### bouncer-artifact-paths-names-the-plan

- Decision: `Webster-Bouncer`'s `artifact_paths` is the single entry `_lyx/plan`.
  The rubric states outright that the subject under review is the diff, and carries the same derivation recipe the burler round's `target.instructions` carries.
- Rationale: `artifact_paths` is required and non-empty (`shedadapters.NewBouncer` errors on an empty list), every entry must be an absolute path under `Env.WorktreeRoot`, and the generic `bouncer-template-judge.md` renders them as "absolute paths to the artifacts under review.
  Read each one."
  A diff cannot be named there at all, so whichever value is chosen is a workaround;
  the question is only which one gives the judge the most useful reading.
  `_lyx/plan` is the card contract the diff is measured against — the judge needs it to evaluate the round's report at all, and both `Plan-Bouncer` and `Plan-Burler` already prove a bare directory entry works there (`NewBouncer` stats nothing).
- Rejected: (a) adding `_lyx/webster/state.json` as a second entry — it is not under review, and the rubric already tells the judge where the base SHA lives;
  (b) `.`, the worktree root — technically legal, but "read each one" over a whole repo is meaningless instruction.
- Recorded as an open risk: `BouncerConfig` has no way to express a non-file subject.

### fix-scope-is-source-and-there-is-no-commit-seam

- Decision: `Webster-Burler` runs `fix-scope: source`.
  `Webster-Bouncer` sets **no** `commit_seam` key.
- Rationale: `FixScopeSource` is defined as "the target is the repo's own files.
  B's write surface is the working tree;
  it commits each fix individually once green … and never pushes."
  That is precisely this round's job, and the Fabric Git Invariant names it as the one permitted agent commit: "An agent does commit its own code to the **warp** repo (commit-per-fix) — the weft, never."
  This is deliberately *not* the same call as `Plan-Burler`'s `overlay`: the plan is weft content reached through the `_lyx` junction, whereas a code diff is warp content.
  With the fixer committing its own work there is no artifact left for the loop owner to commit, and `Commit` stays nil — the shipped no-seam configuration, which `bouncerEntry` documents as "a legitimate configuration and never an error".
  The segment's own round artifacts under `_lyx/loom/reviews/webster/` are uncommitted, exactly as the two shipped segments' round artifacts are.
- Rejected: `fix-scope: overlay` — it would forbid the round from running git at all and restrict its write surface to `Target.Paths`, which this profile deliberately leaves empty;
  a fixer that cannot write source cannot fix a diff.

### the-round-runs-a-cluster-fan

- Decision: `Webster-Burler`'s profile sets `cluster-fan: standard`.
- Rationale: a whole-diff code review over everything Webster built is the archetypal multi-lens case, and it is the one gate in the list with no mechanical validator ahead of it narrowing what is left to judge.
  `standard` is the shipped fan (`generic`, `generic`, `correctness`, `error-handling`, `test-gaps` — `internal/burlerengine/template.yaml`), i.e. five forks per round, and `internal/burlerengine`'s own doc records that clustering is never on unless a profile names a fan.
  This is the first recipe row in the repo to name one.
- Rejected: no fan (single reviewer), matching the two shipped segments — cheaper, but a single generic reviewer is a weak terminal gate over an entire task's output;
  `full` (eight lenses) — more coverage than a converge loop needs per round, and the loop already re-reviews across rounds.
- Recorded as an open risk: `burler.yaml` is operator-owned and seed-only, so an operator who deletes the `standard` fan makes `loomrecipe.New` fail.
  The failure is loud and named, at construction on the `drive` path only (not in `wire()`, so `status`/`pause` stay reachable) — the same failure class as a mistyped `rubric_stencil`, which `NewBouncer` already probes eagerly for.

### perch-row-names-and-routing

- Decision: the row pair is `Webster-Bouncer` and `Webster-Burler`, both carrying `segment: Webster-Review` and `max_bounces: 5`, mirroring both shipped perches exactly.
  Routing: `Webster-Bouncer` → `on_stuck: Webster-Burler`, `on_done: Publish`.
  `Webster-Burler` → `on_stuck: Webster-Bouncer`, `on_done: Webster-Bouncer`.
  The current `Webster-Review` row's `on_stuck: Webster` edge disappears.
  Neither row sets `model`, `effort`, `version`, or `timeout_s`, so both fall back to the run-wide `Env.Review*` values `loom.yaml` supplies.
- Rationale: `internal/shedengine`'s validator rejects an `OnStuck` naming a producer in a different `Segment`, so the pair needs its own shared, non-empty segment label for the two mutual edges to build at all.
  `Webster-Burler`'s `on_done` is unreachable (`BurlerProducer` never returns `Done`) but is set explicitly because an empty `on_done` is load-bearing and ends the whole run silently — the same reasoning already written into both shipped `*-Burler` rows.
  Dropping the bounce-to-`Webster` edge is the right call: the segment resolves findings by fixing them, exactly like its two siblings, and re-running Webster over an already-built diff has no defined meaning.
- Rejected: `Webster-Review-Bouncer`/`Webster-Review-Burler` — longer, and inconsistent with the two shipped pairs.

### the-rubric-carries-both-design-named-dimensions-plus-a-do-not-flag-list

- Decision: the rubric opens by stating that **ordinary diff review is the base** — the round reviews the committed diff as code, and the cluster fan's lenses supply the emphasis vocabulary — then adds the two dimensions `manifest/designs/loom.md` names on top of it:
  1. **Comment-convention compliance** — any new or changed doc comment follows `manifest/designs/code-comment-conventions.md`, pointed at and never restated.
  2. **Per-card mechanical check** — confirms the card's Type-specific mechanical check actually ran and passed (the AST-script-plus-grep for a `Rename` card, `assert-no-callers` for a `Delete` card, per the per-type table in `manifest/designs/plan-card-format.md`), not merely that the diff compiles and tests pass.
  It then carries a **Do not flag** list, matching both shipped rubrics' shape:
  - Anything `Plan-Validate`/`Plan-Revalidate` already check — the plan's *format* is not this gate's subject.
  - Findings raised against the plan itself.
    The plan is the measuring stick and never the subject, exactly as the decision record is for `Plan-Review`;
    a plan-authoring finding cannot be satisfied by changing the diff.
  - Findings about `_lyx` overlay artifacts — the discussion pair, the plan directory, and this segment's own round artifacts are not the diff.
  - A missing `ImpactSummary` on any card, and incomplete `DependsOn`/`Produces` — both belong to `Plan-Review`, already passed.
- Rationale: the design section names only the two "also flag" dimensions, but a rubric with no do-not-flag list contradicts the framing both shipped rubrics carry ("over-flagging is a judgment failure mode a mechanical producer … cannot exhibit"), and this gate sits downstream of three separate upstream gates whose findings it would otherwise re-derive.
  Enumerating a full code-review checklist instead was rejected: `burler.yaml`'s lens library already carries that vocabulary, and duplicating it in the rubric would drift from it.
- Rejected: two "also flag" items and nothing else.

### stub-stays

- Decision: `internal/loomshed/stub.go`, `stubEntry` in `internal/shedrecipe/entries_simple.go`, the `"Stub"` registry entry, and `TestRegistry_ShipsFourteenEntries` are all left untouched.
  `"Stub"` moves into `internal/loomrecipe/coverage_guard_test.go`'s `coverageGuardAllowedUnreachableEngines` alongside `"SingleLLM"`.
  `stubProducer`'s doc comment is reworded — it no longer backs any loom row.
- Rationale: `shedrecipe`'s registry is generic `Shed` machinery shared by reference with `Hardener`'s future producer list, not loom's private property, and `Hardener` is a committed Someday item that will stub rows exactly the way loom did.
  Deleting the engine is a decision about `shedrecipe`'s registry surface, not a consequence of loom finishing its own list.
- Rejected: deleting `stub.go` and its registry entry — would also force a `TestRegistry_ShipsFourteenEntries` rename and rewrite for no present benefit.

## Technical context

**The perch pattern, as shipped twice.**
Read `contracts/recipes/loom-recipe.yaml`'s `Plan-Bouncer`/`Plan-Burler` pair as the template — it is the more recent and more complete of the two.
The invariant shape: both rows carry the same `segment:` label, the same `run_subdir` value (which is what makes them share one run directory), `max_bounces: 5`, and mutual `on_stuck` edges.
`git show 6f66fff1` is the full, self-contained precedent for everything this task does.

**Where each piece is built.**

- `internal/shedrecipe/entries_bouncer.go` — `bouncerEntry` is the `"Bouncer"` constructor.
  Recognised config keys, exactly: `run_subdir`, `artifact_paths`, `rubric_stencil`, `model`, `effort`, `version`, `commit_seam`.
  Anything else is rejected by `configRejectUnknown`.
  `artifact_paths` entries resolve against `env.WorktreeRoot`.
- `internal/shedrecipe/entries_burler.go` — `burlerRoundEntry` is the `"BurlerRound"` constructor.
  Row-level keys: `run_subdir`, `profile`, `model`, `effort`, `timeout_s`.
  `profile` keys, exactly seven: `target`, `fasit`, `rubric`, `rubric_stencil`, `fix-scope`, `tool-use`, `cluster-fan`.
  Exactly one of `rubric`/`rubric_stencil` must be set.
  `target`/`fasit` sub-maps recognise exactly `paths` and `instructions`.
  `profile.target.paths` and `profile.fasit.paths` are the documented exception to the relative-path-resolution rule — passed through unjoined, because `burlerengine.Profile.validate` resolves and stats them itself.
- `internal/shedadapters/bouncer.go` — `Bouncer.Call`'s four modes (seed, re-bounce, judge, replay) and its `NewBouncer` validation.
  `ArtifactPaths` must be non-empty and every entry absolute;
  nothing is statted.
  The rubric is read via `stencilstore.Read` and passed through `stencil.StripLeadingComment` before interpolation — that strip is load-bearing.
- `contracts/stencils/stencils.go` — the single place a stencil's on-disk path and its Go identifier are both named: add a `//go:embed`'d `LoomRubricWebsterReview` var and an `entries` row `{"loom-rubric-webster-review", &LoomRubricWebsterReview}`.
  `contracts/stencils/registry_test.go`'s `TestRegistry_MatchesOnDiskTree` covers both directions automatically once both halves land.

**The rubric stencil's own shape.**
Copy `contracts/stencils/loom/loom-rubric-plan-review.md`'s structure verbatim: a leading HTML comment explaining that the file is a marker *value* and not a template, an H1, framing prose, `## Do not flag`, `## Also flag`.
The leading comment is stripped by `internal/stencil`'s `StripLeadingComment` before either consumer sees it.
The file must contain no `{{.` substring anywhere.

**Row-count knock-on.**
`git grep -n sixteen` finds every site.
The ones this task owns: `internal/loomshed/doc.go:1`, `internal/loomshed/loomshed.go:1,5,13`, `contracts/recipes/loom-recipe.yaml:2`, `manifest/designs/shed-recipe.md:9,88`, `manifest/designs/loom.md:16,17,51`.
The ones it must **not** touch: `internal/planparser/validate.go`, `contracts/specs/loom-plan-spec.md`, `contracts/stencils/loom/loom-rubric-plan-review.md`, and `internal/fabricengine/doc.go` — those "sixteen"s count plan-validation checks and fabric destruction kinds, not producer rows.

**`manifest/designs/loom.md`'s table-vs-recipe divergence note** (the paragraph beginning "**The table and the shipped recipe diverge deliberately.**") is arithmetic that changes with this task: the recipe goes to seventeen rows against the table's fifteen entries, and the collapsed-pair count goes from two to three.

**Diff-base source, verbatim from the code.**
`internal/websterengine/state.go`: `State.Batches map[int]*BatchState`, and `BatchState.StartSHA` is "the repo HEAD immediately before this batch's implementer first forked (or, for a recovery batch, first spawned)".
The file lives at `<websterDir>/state.json`, i.e. `_lyx/webster/state.json`.
`internal/loomengine/status.go`: `Status.StartSha *string` with json tag `start_sha`, and `Status.Parent` carries the parent branch.

## Constraints

From `CONSTRAINTS.md`, the ones this task actually engages:

- **Producer Pointer-Rule Invariant.**
  The rubric stencil is an instruction file: it must *point at* `manifest/designs/code-comment-conventions.md`, `manifest/designs/plan-card-format.md`, and `contracts/specs/loom-plan-spec.md`, never duplicate or paraphrase their content.
  Symmetrically, `manifest/designs/loom.md`'s "Webster-Review rubric" section becomes a doc *about* the shipped stencil — the durable human-readable record it was transcribed from — matching the framing paragraph both sibling sections already carry.
- **Stencil Ownership Invariant.**
  The rubric is read at call time from the told, absolute stencils directory via `internal/stencilstore`;
  `//go:embed` in `contracts/stencils` is a seed default only and never a live read path.
  No engine imports `contracts/stencils`.
- **Fabric Git Invariant.**
  `fix-scope: source` commits warp code from the agent, which is the one explicitly permitted agent commit.
  Read-only git verbs are exempt.
  The rubric and the profile instructions must never mention the two-repo structure at all (`templates-describe-one-repo`) — no `weft`, no `warp`, no sibling-worktree path.
- **Fabric Vocabulary Invariant.**
  Machine-checked over `contracts/stencils/**/*.md`, so the new rubric is inside the enforcement walk: no `host`-sense phrases, no bare `weft`/`warp` outside the owner set.
- **Review Round Invariant.**
  A-before-B, every recorded finding fixed in B at all severities, no self-grading, commit-per-fix on warp source, never push.
  In a cluster round, fork reports plus the handler's holistic review plus the consolidation are all part of A, and fork reviewers are read-only — mechanically enforced by the fork audit.
  This binds the `cluster-fan` decision directly.
- **Shed Recipe Registry Invariant.**
  Registry coverage is enforced from two homes: `internal/loomrecipe/coverage_guard_test.go` and `internal/shedrecipe/registry_test.go`.
  This task touches the first and deliberately leaves the second alone.
- **Markdown Link Integrity.**
  Every relative link added to `manifest/designs/loom.md` and to the rubric must resolve.
- **CLAUDE.md's markdown rule.**
  Semantic line breaks, one sentence per line, in every `.md` file touched — including the new stencil.

## Testing

**`contracts/stencils/rubric_test.go`** — the primary TDD candidate, and the one to write first.
Add two tests mirroring the shipped pair exactly:

- `TestLoomRubricWebsterReview_NamesEveryRequiredItem` — a short, distinctive substring per required item, following the precedent that assertions are phrases and not paragraphs so ordinary prose edits do not break the test.
  Cover both design-named "also flag" dimensions and every "do not flag" item settled above.
- `TestLoomRubricWebsterReview_CarriesNoStencilMarkers` — asserts no `{{.` substring.

Update the file's own header comment, which currently enumerates the two rubrics and their item counts.

**`internal/loomrecipe`** — the assembled-graph tests, per `shed-recipe.md`'s test-ownership rule ("the coverage guard driving loom's real row list against the registry, the sequencing/cancellation/resume tests that build the real … list — live in `internal/loomrecipe`").

- `coverage_guard_test.go`: `loomRowEngines` loses the `NameWebsterReview → "Stub"` entry and gains `NameWebsterBouncer → "Bouncer"` and `NameWebsterBurler → "BurlerRound"`;
  `"Stub"` joins `coverageGuardAllowedUnreachableEngines`, and that map's comment is rewritten to say why.
  Both existing directions of the guard then cover the change for free.
- `shape_test.go`: `wantProducerTable` loses its `NewStub` row and gains two — `{NameWebsterBouncer, NameWebsterBurler, NamePublish, "Webster-Review", 5, reflect.TypeOf(&shedadapters.Bouncer{})}` and `{NameWebsterBurler, NameWebsterBouncer, NameWebsterBouncer, "Webster-Review", 5, reflect.TypeOf(&shedadapters.BurlerProducer{})}`.
  The routing-graph guard in the same file then proves the new edges resolve.
- `sequence_test.go`, `fixture_test.go`, `recipe_test.go`: row-count and walk-order updates.
  `testEnv` already fills `StencilsDir`, `RunRoot`, `Burler`, and `Now`, so the new perch needs no new `Env` seam — but the fixture must seed `loom-rubric-webster-review` into its stencils dir, or `NewBouncer`'s eager rubric probe fails construction.
  That seeding is the single most likely source of a first-run failure;
  check how `6f66fff1` did it for `loom-rubric-plan-review` and follow it.

**`internal/shedrecipe/entries_burler_test.go`** — one new case for `cluster-fan`, which no shipped recipe row exercises: assert the key maps onto `burlerengine.Profile.ClusterFan`, and assert an unknown fan name fails construction with a named error.
This is the only genuinely new coverage in `shedrecipe`;
every other key the perch uses is already covered by the two shipped segments' tests.

**`internal/loomcli/smoke_test.go`** — row-count assertion update only.

**Not tested mechanically, recorded as review obligations:** the rubric's fidelity to `manifest/designs/loom.md`'s section beyond the phrase pins, and the diff-derivation instructions' correctness against a real `state.json` (no fixture in this repo produces one, and building one would be a `websterengine` integration test, out of scope).

**Verify command:** `go build ./... && go test ./...`.
`internal/lyxcwd/enforcement_test.go`'s `TestEnforcement_FabricVocabulary` walks `contracts/stencils/**/*.md`, so the new rubric is covered by the existing suite without a new test.

## Open risks

- **`product.start_sha` is dead.**
  `contracts/specs/loom-status-spec.md` documents it as the diff base;
  nothing writes it.
  This task routes around it via `_lyx/webster/state.json`.
  If `start_sha` is later filled for real, the rubric's derivation instructions become the second, redundant source and should collapse onto it.
  Wants its own roadmap item.
- **`BouncerConfig` cannot express a non-file subject.**
  `artifact_paths` is required, absolute, path-shaped, and rendered into the generic judge prompt as "read each one".
  `Webster-Review` is the first row whose subject is not a file, and it works around this by naming the plan.
  If a second such gate appears (`Tenter` is the likely candidate), a generic `subject_instructions` key on `Bouncer` becomes the right fix.
- **Naming `cluster-fan: standard` couples recipe construction to an operator-owned config file.**
  `burler.yaml` is seed-only and never re-seeded;
  deleting the `standard` fan is a supported operator choice that would make `loomrecipe.New` fail on the `drive` path.
  Loud and named, not silent, but new — no recipe row depended on `burler.yaml` content before this one.
- **Five forks per round is a real cost step.**
  This is the first clustered round in loom, over the largest artifact in the list, at the end of the run when the most work has accumulated.
  If it proves too expensive in practice, the cheap dial is `cluster-fan` — dropping the key reverts to a single reviewer with no other change.
- **The `Webster` row loses its only inbound stuck edge.**
  Nothing bounces back to `Webster` after this change, and `Webster` itself carries no `on_stuck`.
  A diff the segment cannot fix within its bounce budget escalates to a human, which is the intended behaviour, but it means a genuinely mis-built batch has no automatic re-run path.

## Q&A log

- **Q:** How does the round obtain the diff under review, given `Target.Paths` entries must exist on disk? **A:** [auto-pick] Instructions-only `profile.target` plus `tool-use: true`, deriving `<lowest batch startSha>..HEAD` from `_lyx/webster/state.json`, falling back to the merge-base against `product.parent`. **Why:** `Profile.validate` accepts an instructions-only `FileSet`, `BatchState.StartSHA` is already documented as the durable base-commit record, and both alternatives (a `Webster-Diff` row, or stamping `start_sha` for real) add production plumbing to a task scoped as a rubric plus a perch.
- **Q:** What does `Webster-Bouncer`'s required `artifact_paths` name, when the subject is a diff? **A:** [auto-pick] The single entry `_lyx/plan`. **Why:** the key cannot express a diff at all, so the choice is only which value gives the judge the most useful reading — the card contract the diff is measured against — and `Plan-Bouncer` already proves a bare directory entry works there.
- **Q:** `fix-scope: source` or `overlay`, and does the Bouncer need a `commit_seam`? **A:** [auto-pick] `source`, no `commit_seam`. **Why:** the target is warp code, which the Fabric Git Invariant names as the one permitted agent commit;
  `overlay` would forbid git entirely and restrict writes to an empty `Target.Paths`, and with the fixer committing its own work there is no artifact left for a loop-owner seam to commit.
- **Q:** Does the round run a cluster fan? **A:** [auto-pick] Yes, `cluster-fan: standard`. **Why:** a whole-diff review is the archetypal multi-lens case and this is the only gate with no mechanical validator ahead of it;
  the burler.yaml-deletability hazard is recorded as an open risk and the failure is loud at construction.
- **Q:** Row names, routing, and what happens to the `on_stuck: Webster` edge? **A:** [auto-pick] `Webster-Bouncer`/`Webster-Burler` under `segment: Webster-Review`, mutual `on_stuck`, `on_done: Publish` from the Bouncer, and the bounce-to-`Webster` edge is dropped. **Why:** `shedengine`'s validator requires a shared segment label for the mutual edges to build, and the segment resolves findings by fixing them rather than by re-running Webster, exactly like both shipped perches.
- **Q:** Does the rubric carry only the two dimensions `loom.md` names, or also a do-not-flag list? **A:** [auto-pick] Both dimensions plus a do-not-flag list, over an explicit "ordinary diff review is the base" framing. **Why:** this gate sits downstream of three upstream gates whose findings it would otherwise re-derive, and both shipped rubrics carry the same over-flagging framing;
  a full code-review checklist was rejected because `burler.yaml`'s lens library already carries that vocabulary.
- **Q:** Is `Stub` deleted now that no loom row uses it? **A:** [auto-pick] No — kept, and moved into `coverageGuardAllowedUnreachableEngines`. **Why:** `shedrecipe`'s registry is generic machinery shared by reference with `Hardener`'s future list, so removing an engine is a decision about that registry's surface rather than a consequence of loom finishing its own list.
- **Q:** Does this task need a `Webster-Revalidate` row, mirroring `Plan-Revalidate`? **A:** [auto-pick] No. **Why:** `Plan-Revalidate` exists because a mechanical validator sits over the plan and the fixer rounds rewrite it after that validator already ran;
  there is no mechanical validator over a diff.
