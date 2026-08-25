# Discussion: loom: Plan-Review producer

```yaml
task: 'loom: Plan-Review producer'
slug: loom-plan-review-producer
status: discussing
parent: main
```

## Problem

Row 9 of loom's producer list, `Plan-Review`, is still a `Stub` — `internal/loomshed`'s `stubProducer`, which unconditionally reports `Done`.
The plan artifact reaches `Batchifier` and then Webster with no LLM judgment applied to it at all; the only gate it passes is `Plan-Validate`, which runs `planparser`'s sixteen mechanical checks and has no opinion about whether the plan is a *good* plan.

Why now: the two things this row needed both landed.
`loom: Discussion-Write producer` and `loom: Discussion-Review producer` (commit `eb5f091b`) built the generic review-segment machinery — `internal/shedadapters`' `Bouncer` and `BurlerRound` producers, the `shedrecipe` registry entries for both, and the run-wide `Env.Review*` model/timeout plumbing from `loom.yaml` — and shipped the first real segment on top of it.
`loom: Plan-Write producer` (commit `6a100417`) made row 8 write a real plan.
`Plan-Review` is now the only thing standing between a real plan and Webster executing it, and every piece it needs already exists except its own rubric and its own two recipe rows.

There is a second, narrower gap: **no rubric exists for the Card format at all.**
The prior `contracts/specs/loom-plan-spec.md` rubric covered the superseded format-3 file-op fields (`**Edits:**`, `**Creates:**`, `**Deletes:**`, and the rest), which `planparser` now routes into `Card.RetiredLabels`.
The format-4 Card model (`Targets`/`Uses`/`Intent`/`ImpactSummary`, per `manifest/designs/plan-card-format.md`) has never had review criteria written for it.
This task writes them from scratch.

## Scope

**In:**

- A new stencil `contracts/stencils/loom/loom-rubric-plan-review.md`, registered in `contracts/stencils/stencils.go` (embedded var + `entries` row).
- `contracts/recipes/loom-recipe.yaml`: replace the single `Plan-Review` `Stub` row with a `Plan-Bouncer` + `Plan-Burler` pair carrying `segment: Plan-Review`; repoint `Plan-Validate`'s `on_done`.
- `internal/loomshed/loomshed.go`: drop `NamePlanReview`, add `NamePlanBouncer` and `NamePlanBurler`; fourteen rows become fifteen.
- `internal/loomshed/stub.go`: doc comment retargeted — `stubProducer` now backs one row, `Webster-Review`, not two.
- `internal/shedadapters/bouncer.go`: `BouncerConfig` gains an optional `Commit func() error`, called on `settle`'s `verdictApproved` branch before it returns `shedengine.Done`.
  Nil is the absent value and means "commit nothing", which is what keeps `Discussion-Bouncer`'s behaviour byte-identical.
- `internal/shedrecipe/entries_bouncer.go`: a new optional `commit_seam` config key, added to `configRejectUnknown`'s recognised set, resolving `plan` → `env.CommitPlan` and `discussion` → `env.CommitDiscussion`; absent leaves `BouncerConfig.Commit` nil.
- Tests: `contracts/stencils/rubric_test.go`, `contracts/stencils/registry_test.go` (whatever its on-disk-tree assertion needs), `internal/loomrecipe`'s `coverage_guard_test.go` / `recipe_test.go` / `shape_test.go` / `sequence_test.go` / `fixture_test.go`, `internal/shedadapters`' Bouncer settle tests, `internal/shedrecipe/entries_bouncer_test.go`, `internal/loomcli/wiring_test.go`.
- Docs: `manifest/designs/loom.md` (producer-table row 9, the `### Plan-Review rubric` section, and the stale text the sweep below finds), `manifest/designs/shed-recipe.md` (the `commit_seam` key), `manifest/roadmap.md` (this Planned item removed, and one new Planned item added — see the `fix-scope` decision below).

**Out:**

- `internal/burlerengine`, `internal/shedrecipe/entries_burler.go`, `internal/loomengine/review.go`, `internal/loomengine/config.go`, and `internal/loomcli/wiring.go` **production code**.
  All of it is already generic and already shipped; this task changes none of it.
  `wiring.go` in particular already fills `StencilsDir`, `RunRoot`, `Burler`, `Now`, the four `Review*` fields, **and `CommitPlan`** — the commit seam this task adds reuses that existing closure rather than adding an `Env` field.
- **Changing `Discussion-Burler`'s `fix-scope`, or adding a `commit_seam` to `Discussion-Bouncer`.**
  The shipped Discussion row's `fix-scope: source` over `_lyx/discussion/*` contradicts the Fabric Git Invariant (see the `fix-scope` decision below), but correcting it changes shipped behaviour and its tests, and belongs to its own task.
  This task adds the seam that makes the correction possible and files the roadmap item; it does not flip the Discussion row.
- `Webster-Review` (its own roadmap item), `Plan-Sweep` (deferred to Someday), and `Plan-Write`'s own stencil.
- `contracts/specs/loom-plan-spec.md`. It is the format contract and stays the authority; the rubric **points at** it and never restates it.
- Any change to `planparser`'s sixteen validation checks, or to what `Plan-Validate` does.
- `docs/overview.md` — the module table and execution stack are unchanged.
- `loom.yaml`'s `review:`/`review_timeout_min:` keys and their `Config` validation — already shipped, already run-wide.

## Decisions

### Row names and segment label

- Decision: two rows named `Plan-Bouncer` and `Plan-Burler`, both carrying `segment: Plan-Review`.
  Constants `NamePlanBouncer = "Plan-Bouncer"` and `NamePlanBurler = "Plan-Burler"` replace `NamePlanReview`.
  The segment label itself stays a recipe-literal yaml string with no Go constant, exactly as `Discussion-Review` does.
- Rationale: the roadmap item names this pair verbatim ("as a `Plan-Bouncer`/`Plan-Burler` segment"), and `internal/shedengine`'s validator rejects an `OnStuck` naming a producer in a different `Segment`, so both rows need the same non-empty label for the mutual-bounce edges to build at all.
- Rejected: keeping `Plan-Review` as the Bouncer's row name (asymmetric with the shipped Discussion pair, and makes the segment label collide with a row name); `Plan-Review-Bouncer`/`Plan-Review-Burler` (longer, and diverges from the shipped naming for no gain).

### Dropping `NamePlanReview` breaks resume for an in-flight task, and that is accepted

- Decision: `NamePlanReview` is removed rather than kept as an alias.
- Rationale: `loomshed.go`'s own header comment states the row name is the durable on-disk identity in `current_producer` and a rename breaks resume for any in-flight task.
  Commit `eb5f091b` made exactly this trade for `Discussion-Review` → `Discussion-Bouncer`/`Discussion-Burler` and accepted it; there is no in-flight task to protect and no alias mechanism in the recipe format to express one.
- Rejected: keeping `NamePlanReview` as a segment-label constant (unused by the recipe loader, and `Discussion-Review` has no such constant — it would be a lone inconsistency).

### `artifact_paths` is the plan **directory**, not an enumerated file list

- Decision: `Plan-Bouncer`'s `artifact_paths` is a single entry, `_lyx/plan`.
  `Plan-Burler`'s `profile.target.paths` is the same single entry.
- Rationale: a plan is `00-overview.md` plus a variable number of `NN-<card-slug>.md` card files, so no enumeration written into the recipe can stay correct across plans.
  Both consumers accept a directory: `shedadapters.NewBouncer` explicitly stats nothing (its own comment: "an artifact that does not exist yet is legitimate"), and only newline-joins the entries into the seed/judge prompt's `{{.artifacts}}` marker; `burlerengine`'s `requireExistingPaths` documents that "a file or a directory both satisfy the check".
  Both the Bouncer's judge and the Burler's round have tool use and can walk the directory from the Card Index.
- Rejected: `_lyx/plan/00-overview.md` alone (the cards, which are the actual subject, would only be reachable by inference); listing both the directory and the overview (redundant — the overview is inside the directory, and the two entries are newline-joined into one prompt marker).
- Consequence to state in the rubric: the plan directory can contain `archive-<stamp>/` rotation subdirectories (see `planparser.ArchiveDirName`).
  The rubric must say the subject is the current plan — `00-overview.md` and the card files the Card Index names — and that `archive-*/` subdirectories are out of scope.

### The fasit is `decision-record.md`, not the format spec

- Decision: `Plan-Burler`'s `profile.fasit` is `paths: [_lyx/discussion/decision-record.md]` plus an `instructions` string that names `contracts/specs/loom-plan-spec.md` as the format authority and states that `Plan-Validate` already owns the mechanical checks.
- Rationale: "fasit" is the answer key — what the artifact under review is measured *against*.
  For a plan that is the decision record: the plan's job is to implement the decisions and constraints the discussion settled.
  Format conformance is a different question and is already mechanically gated upstream by `Plan-Validate`, so putting the spec in the fasit's `paths` would invite the round to re-derive checks a deterministic parser already ran.
  `decision-record.md` is guaranteed present and committed by the time this segment runs: `Discussion-Validate` gates it and `Env.CommitDiscussion` commits the whole discussion directory.
- Rejected: `contracts/specs/loom-plan-spec.md` as the fasit path (duplicates `Plan-Validate`); both paths (same problem, plus it dilutes the decision-record signal); instructions-only, mirroring `Discussion-Burler` (Discussion had no upstream artifact to measure against — Plan does, and passing it up would waste the one real fasit in the whole run).

### `Plan-Burler`'s `fix-scope` is `overlay`, and the segment commits through the Bouncer

- Decision: `Plan-Burler`'s profile sets `fix-scope: overlay` and `tool-use: true`.
  Because an `overlay` round runs no git at all, `Plan-Bouncer` gains `commit_seam: plan`, and `shedadapters.Bouncer.settle` calls the injected `Commit` closure on the `verdictApproved` branch before returning `shedengine.Done`.
  The closure is `Env.CommitPlan`, which `internal/loomcli/wiring.go` already builds and which is already idempotent — `CommitAnchoredPaths` reports `committed == false` for an already-clean, already-tracked path, and the closure discards that result.
- Rationale: `internal/burlerengine/doc.go` names the `FixScopeOverlay` class as exactly "lyx system/orchestration state (plan, discussion, review artifacts), reached through the `_lyx` junction", and states that such a round's write surface is exactly `Target.Paths` plus the two output files and that it "runs NO git commands at all — the Fabric Git Invariant reserves committing that class of file to the loop owner, never an agent."
  `CONSTRAINTS.md`'s Fabric Git Invariant is unambiguous: "Agents ride the file contract: they **write** overlay files into `_lyx` via the junction … Go **reads and commits** them. An agent does commit its own code to the **warp** repo (commit-per-fix) — the weft, never."
  `_lyx/plan` is weft content reached through the junction, so `source` is doctrinally forbidden here, and is likely inert besides: the same invariant excludes the `_lyx` junction from warp git via `.git/info/exclude`, so an agent's `git add _lyx/plan/...` in the warp worktree has nothing to stage.
  The commit seam is required rather than optional because nothing else commits a review round's fixes: neither `shedadapters.Bouncer` nor `shedadapters.BurlerProducer` performs any git operation, and `Env.CommitPlan` is otherwise reached only by `Plan-Write`'s own commit decorator (`internal/shedrecipe/entries_planwrite.go`), which never re-fires after the segment — so an `overlay` round with no seam would leave every approved fix uncommitted in the weft working tree.
  `settle`'s `case verdictApproved` is the hook point: one branch, already the segment's single exit.
- Rejected: `fix-scope: source`, copying the shipped `Discussion-Burler` row (smallest diff, and it is what ships today — but it instructs an agent to git-commit weft content, which the invariant forbids in the plainest terms it uses anywhere, and carrying the bug forward doubles the cost of fixing it later).
  Rejected: `overlay` with no commit seam and the gap filed as a follow-up (leaves approved plan fixes as uncommitted weft dirt that the next `CommitAnchoredPaths` caller sweeps up under an unrelated commit message, or that a crash loses outright).
  Rejected: a new `Env` field for the commit closure (`Env.CommitPlan` already exists, is already wired, and already has the exact pathspec — `planparser.PlanDirRel()`).
  Rejected: making `commit_seam` accept an arbitrary closure name (a two-value enum matching the two closures `Env` actually carries is the whole vocabulary that exists).
- Consequence, recorded and deliberately not fixed here: **the shipped `Discussion-Burler` row's `fix-scope: source` is the same violation.**
  This task adds the `Commit` seam that makes the correction a two-line recipe change, and adds a Planned roadmap item — "loom: `Discussion-Burler`'s `fix-scope: source` violates the Fabric Git Invariant" — pointing at this decision.
  It does not flip the Discussion row, because that changes shipped behaviour and its tests.
  The two segments therefore ship divergent `fix-scope` values on purpose, and the recipe comment on `Plan-Burler` must say so, so the divergence does not read as a copy-paste slip.
- `tool-use: true` rationale: the round must walk `_lyx/plan`'s card files (the set is variable and only the Card Index names it), resolve symbol-shaped target entries against the repo, and read `decision-record.md`.
  None of that is possible without tool use.

### `support-log.md` is excluded from Plan-Review entirely

- Decision: `support-log.md` appears in neither `artifact_paths` nor `fasit.paths`, and the rubric says so explicitly, in one named item.
- Rationale: `manifest/designs/loom.md` records the `Plan-never-reads-support-log` boundary, asserted at build/test time by `TestPlanSpec_PromptNeverNamesSupportLog` in `internal/loomengine/plan_test.go` — the composed plan prompt names neither `support-log.md` nor its absolute path.
  A reviewer that *can* read the support log will raise findings grounded in content the plan writer provably never saw, and the fixer can only satisfy such a finding by inventing the missing link.
  The boundary is only meaningful if it extends to the gate that judges the writer's output.
- Rejected: including it as a secondary fasit path (breaks the boundary in the one place it matters most).

### The rubric's shape: four "Also flag" items and a "Do not flag" section

- Decision: the stencil carries two sections, mirroring `loom-rubric-discussion-review.md`.

  **Also flag** — four items:
  1. **Granularity.** One card per independently reviewable/testable unit, not one card per literal symbol (`plan-card-format.md`'s own granularity rule).
     A private supporting type or a constructor inseparable from its type belongs in the other symbol's card; an independently testable symbol gets its own card even if one card is its only consumer.
  2. **`ImpactSummary` carries a real conclusion.** A one-line blast-radius conclusion ("3 callers, all local to the billing package, no cross-module effects"), never a restatement of `Intent`.
  3. **`Custom` is a last resort.** Used only where none of `Create`/`Edit`/`Delete`/`Rename`/`Move`/`Prosa` genuinely fits, never as a shortcut around correct typing — `Custom` is exempt from `path-missing` on its own targets and from `prosa-symbol-target`, so a mistyped `Custom` card silently escapes two checks.
  4. **Fidelity to the decision record.** Every Decision and Constraint in `decision-record.md` is carried by some card, and no card introduces scope the decision record does not license.
     This item **names the path `_lyx/discussion/decision-record.md` in the rubric text itself**, and states in the same breath that the file is the measuring stick and never the subject — findings are raised against the plan, never against the decision record.

  **Do not flag** — three items:
  1. **Anything `Plan-Validate` already checks.** The sixteen `planparser` check IDs (`format-unrecognized` through `commit-subject-mismatch`) are enforced deterministically upstream; re-deriving them here is duplicated work whose only possible outcome is disagreement with the parser.
  2. **A missing `DependsOn`/`Produces` field, or an incomplete dependency list.** Dependency edges are derived, never authored — a card's `Uses` intersected against every other card's target list.
     Plan-time completeness of that intersection is explicitly not provable; the real gate is the post-merge build+test.
  3. **A `Rename`, `Move`, `Prosa`, or `Custom` card with no `ImpactSummary`.** It is required for `Edit` and `Delete` only, and `plan-card-format.md` gives the reason for `Rename` specifically: a correctly executed AST-aware rename is binary, with no graded blast radius to summarise.

- Rationale: `Discussion-Review`'s design records over-flagging as the judgment failure mode the LLM producer has and the mechanical one cannot.
  `Plan-Review` sits directly downstream of a sixteen-check mechanical validator, so its over-flagging surface is larger, not smaller — the "Do not flag" half is what keeps it from becoming a second, worse `Plan-Validate`.
  The three "Also flag" items 1–3 are the criteria `manifest/designs/loom.md`'s `### Plan-Review rubric` section already pins; item 4 is added because the fasit decision above makes the decision record the round's measuring stick, and the rubric is what tells the judge to use it.
- Rejected: the three roadmap criteria alone (no over-flagging fence, and no instruction to use the fasit); three plus fidelity without a "Do not flag" section (same problem).

### How the judge reaches the decision record

- Decision: the rubric text names `_lyx/discussion/decision-record.md` explicitly, under "Also flag" item 4.
  `artifact_paths` stays `_lyx/plan` alone.
- Rationale: one rubric stencil feeds both rows of the perch, but only the Burler's `profile.fasit` carries the decision record — `contracts/stencils/bouncer/bouncer-template-judge.md` gives the judge a closed three-item reading order (`{{.artifacts}}`, `{{.report_path}}`, `{{.previous_ledger}}`) and nothing else.
  Without this, the row that actually emits `APPROVED`/`BLOCKING` would be told to check decision-record fidelity with no path to the file.
  Naming the path in the shared rubric reaches both rows with one edit, and the judge already reads files by contract, so it can act on it.
  A `_lyx/...` literal inside a stencil is precedented — `contracts/stencils/webster/webster-body-implementer.md`, `webster-template-master.md`, and `webster-prefix-recovery.md` all name `_lyx/plan`.
  The Lyxdirs Single-Declarer Invariant binds production path-construction Go, not stencil prose.
- Rejected: adding `_lyx/discussion/decision-record.md` as a second `artifact_paths` entry.
  `BouncerConfig.ArtifactPaths`' own doc defines the field as "the subject under review — what the rubric is applied *to*", so listing the fasit there mislabels it, and invites the judge to raise findings against the decision record — the wrong artifact to be fixing at this stage, and the exact relocation-finding confusion `Discussion-Review`'s rubric already had to fence off.
- Rejected: scoping item 4 to the Burler round only.
  The Burler produces the report; the Bouncer produces the verdict.
  An item the verdict-emitting row cannot evaluate is not a gate.

### Recipe routing

- Decision:
  - `Plan-Validate`: `on_stuck: Plan-Write` (unchanged), `on_done: Plan-Bouncer`.
  - `Plan-Bouncer`: `engine: Bouncer`, `segment: Plan-Review`, `max_bounces: 5`, `on_stuck: Plan-Burler`, `on_done: Batchifier`.
  - `Plan-Burler`: `engine: BurlerRound`, `segment: Plan-Review`, `max_bounces: 5`, `on_stuck: Plan-Bouncer`, `on_done: Plan-Bouncer`.
- Rationale: structurally identical to the shipped Discussion pair.
  The Burler's `on_done` is unreachable — `BurlerProducer` never returns `Done` — but an empty `on_done` is load-bearing and ends the whole run silently, which is the worse failure; the shipped Discussion row carries the same explicit-but-unreachable edge with a comment saying so, and this one should carry the same comment.
- Rejected: keeping an `on_stuck: Plan-Write` edge on the Bouncer.
  In the perch pattern the Bouncer's `on_stuck` is the Burler; exhausting the bounce budget escalates to a human rather than re-routing to the writer.

### Stale text is found by a scan, not by a hand-written list

- Decision: the set of files this change invalidates is produced by a repo-wide scan, run as the first step of implementation, over three patterns:
  1. `Plan-Review` — every prose mention, in `.md`, `.go`, and `.yaml`.
     A mention describing it as a `Stub`, or using its `on_stuck: Plan-Write` edge as a routing example, is stale; a mention naming the *segment* is still correct.
  2. `NamePlanReview` — every Go reference, production and test.
     All of them break at compile time, so this pattern is a completeness check on the first, not an independent risk.
  3. The fourteen-row count claim — `fourteen`, plus `14` in a row-count context.
     Each hit is classified against which fourteen it counts: loom's **recipe rows** (becomes fifteen), `manifest/designs/loom.md`'s **design table** (deliberately kept at fourteen and NOT changed — the table collapses each review segment's two rows into one entry by design, see its own "The table and the shipped recipe diverge deliberately" paragraph), `internal/shedrecipe`'s **engine registry** (stays fourteen — `TestRegistry_ShipsFourteenEntries` must not change), and `landingshed.Deps`' **fourteen fields** (unrelated).
- Rationale: the first draft of this discussion hand-listed the doc set and named one stale site.
  A scan run while writing this decision found, beyond that one: `manifest/designs/loom.md:16` and its lines 49–54, `manifest/designs/shed.md:91` and `:148` (both carrying the same `Plan-Review` → `Plan-Write` routing example), `manifest/designs/review-finding-classification.md:47` ("Plan-Review's own future rubric"), `manifest/designs/shed-recipe.md:9`, `contracts/recipes/loom-recipe.yaml:2`, `internal/loomshed/loomshed.go:1`/`:5`/`:13`, `internal/loomshed/doc.go:1`, `internal/loomcli/smoke_test.go:21` ("two of its fourteen rows -- Plan-Review and Webster-Review -- with stub producers"), and comment text in `internal/loomrecipe`'s `coverage_guard_test.go:16`/`:40`, `sequence_test.go:71`, `shape_test.go:2`/`:232`, and `recipe_test.go:21`.
  A hand-written list is the wrong instrument at this count.
- Note on classification, not a mechanical rule: `manifest/designs/loom.md`'s design table is the one place where "fourteen" stays correct after this change.
  The table already collapses `Discussion-Review`'s two recipe rows into one entry and states that it is "kept at fourteen entries by design, not required to track the recipe's row count row-for-row".
  Row 9 keeps its single `Plan-Review` entry for the same reason; only its `Input`/`Output` cells and the parenthetical naming its two rows change, exactly as row 5's did.
  The "Both count fourteen, but not the same fourteen" paragraph must be updated to say the recipe now carries fifteen and to name the second collapsed pair.

### `run_subdir` and the model/effort/timeout omission

- Decision: both rows carry `run_subdir: plan`.
  Neither row carries a `model`, `effort`, `version`, or `timeout_s` key, and each gets the same explanatory comment the Discussion rows carry.
- Rationale: the shared `run_subdir` value is what makes both rows write into one joined run directory under `Env.RunRoot` (`loomengine.LoomReviewsDir`), which is what lets `shedadapters`' `roundComplete` find the Burler's report where the Bouncer looks for it.
  `plan` mirrors `discussion` and reads as the phase, not the row.
  Omitting the model keys is what makes both rows fall back to `Env.Review*`, supplied from `loom.yaml`; a recipe-literal model would be untunable without a rebuild, since the recipe is embedded in the binary.
- Rejected: `plan-review` as the subdir (the Discussion segment's dir is `discussion`, not `discussion-review`).

### `stubProducer` stays

- Decision: `internal/loomshed/stub.go` keeps `stubProducer` and `NewStub`; only the doc comments change — it now backs one row of a fifteen-row list, `Webster-Review`, not two of fourteen.
- Rationale: `Webster-Review` is still a `Stub` in the recipe and has its own roadmap item.
- Rejected: deleting it (would leave `Webster-Review` with no engine and break the coverage guard).

## Technical context

**Almost everything generic is already built.** Read commit `eb5f091b` (`loom: Discussion-Review producer`) as the template for this task — it is a near-exact structural mirror, with one addition: the `commit_seam`/`BouncerConfig.Commit` pair, which exists because `Plan-Burler` is `overlay` and an overlay round commits nothing itself (see the `fix-scope` decision).

- `internal/shedrecipe/entries_bouncer.go` — `bouncerEntry`. Recognises exactly `run_subdir`, `artifact_paths`, `rubric_stencil`, `model`, `effort`, `version`; `configRejectUnknown` rejects anything else, so a typo in the recipe fails at construction.
  Resolves `artifact_paths` against `env.WorktreeRoot` via `resolveUnderRoot`, joins `run_subdir` under `env.RunRoot` and `MkdirAll`s it, pins `ReportName` to `round-<n>-review.md` (not recipe-authorable — any other value resolves the round to 0 forever, silently).
  `NewBouncer` probes the rubric stencil eagerly, so a mistyped `rubric_stencil` fails at wire time, not at first `Call`.
- `internal/shedrecipe/entries_burler.go` — `burlerRoundEntry` and `burlerRoundProfile`.
  The profile map recognises exactly `target`, `fasit`, `rubric`, `rubric_stencil`, `fix-scope`, `tool-use`, `cluster-fan`; each `FileSet` recognises exactly `paths` and `instructions`.
  `rubric` and `rubric_stencil` are mutually exclusive and exactly one is required.
  `profile.target.paths` / `profile.fasit.paths` are the one documented exception to the relative-path-resolution rule — passed through relative and unjoined, because `burlerengine.Profile.validate` resolves them against its own told worktree root and stats each one.
  So `_lyx/plan` and `_lyx/discussion/decision-record.md` go into the recipe **relative**, exactly as the Discussion rows write them.
- `internal/loomcli/wiring.go` — needs **no change**.
  `Env.StencilsDir`, `Env.RunRoot` (`loomengine.LoomReviewsDir(location)`), `Env.Burler`, `Env.Now`, and `Env.ReviewModel`/`ReviewEffort`/`ReviewVersion`/`ReviewTimeout` are all already filled, and all are run-wide rather than per-segment.
  `Env.CommitPlan` is already filled too, which is why `commit_seam` reuses it instead of adding a field.
- `internal/shedadapters/bouncer.go` — `settle` (around line 268) is the one function this task edits.
  Its `case verdictApproved: return shedengine.Done, ptr, nil` is a single branch and the segment's only success exit, so the `Commit` call has exactly one correct home.
  `degrade` is the existing error path to route a failing `Commit` through.
  Note the surrounding contract: both `settle` returns "survive cancellation — a genuinely parsed verdict is the one exception `cancelErr` never applies to", so think about whether a commit belongs before or after that guarantee, and say so in the plan.
- `internal/loomengine/review.go` — `ResolveReview` / `ReviewSettings`, already shipped, already called from `wire`.
  No new config key, no `loom.yaml` change.

**The stencil.** `contracts/stencils/loom/loom-rubric-discussion-review.md` is the exact model to copy in form.
Two properties are load-bearing:

- It is a **marker value, never a template.** It carries no `{{.` substring anywhere — it is interpolated into `bouncer-template-seed.md`'s and `bouncer-template-judge.md`'s `{{.rubric}}` marker and into `burlerengine`'s round prompt the same way, and a marker inside it would render literally or be silently swallowed.
- It opens with an HTML comment explaining what it is and who reads it.
  `internal/stencil`'s `StripLeadingComment` removes that comment before either consumer sees it, and `burlerRoundProfile` calls it explicitly for the same reason `shedadapters`' Bouncer does — to strip `stencilstore`'s `<!-- lyx-stencil: sha256=... -->` banner, which would otherwise land mid-prompt.

Registration is one place: `contracts/stencils/stencils.go`, which is the only file where a stencil's on-disk path and its Go identifier are both named — add the `//go:embed loom/loom-rubric-plan-review.md` var (`LoomRubricPlanReview`, placed directly after `LoomRubricDiscussionReview`) and its `entries` row, `{"loom-rubric-plan-review", &LoomRubricPlanReview}`, in the same position.
`entries` order is `lyx stencil list`'s print order.

**Rubric source material.** Three docs, all already written; the rubric points at them rather than restating:

- `manifest/designs/plan-card-format.md` — the Card fields, the seven type labels, the per-type table (which types require `ImpactSummary`), the `Intent`-vs-`ImpactSummary` distinction, the granularity rule, the Delete-vs-Edit rule, the derived-dependency rule, and the post-merge correctness backstop.
- `contracts/specs/loom-plan-spec.md` — the as-built format-4 contract, and its `## Validation checks` section listing the sixteen check IDs `Plan-Validate` enforces.
  That list is what the rubric's "Do not flag" item 1 fences off.
- `manifest/designs/loom.md`'s `### Plan-Review rubric` — the three criteria this task's stencil is transcribed from.
  Per the Producer Pointer-Rule Invariant this section stays as the durable human-readable record and must be reworded from "the text the future `Bouncer` rubric must point at" to a doc *about* the now-shipped stencil, exactly the way the two `### Discussion-Review rubric` subsections were reworded in `eb5f091b`.

**Test fixtures already generalise.** `internal/loomrecipe/fixture_test.go` carries `seedBouncerStencils` (seeds `bouncer-template-seed`, `bouncer-template-judge`, and the Discussion rubric from the real embedded bytes), `fakeLoomBurler` (a `shedadapters.BurlerRunner` writing the review and fixer-report paths it is handed), and `fakeLoomShuttle` (one `Shuttle` dispatching by role, with a `runBouncerJudge` branch writing the three files the Bouncer's judge spec names).
The Plan segment reuses all three; `seedBouncerStencils` needs the new rubric added to its map, and nothing else in the fixture is Discussion-specific.
`seedPlanValidateFixture` already writes a valid plan into `_lyx/plan`, so `Plan-Bouncer`'s artifact directory exists when the sequence test runs.

**Gotcha — construction-time vs. run-time path existence.**
`NewBouncer` stats nothing, and `burlerengine.Profile.validate` (which does stat, and accepts directories) runs per round, not at construction.
So neither row fails to build in a worktree where `_lyx/plan` does not yet exist, which is the normal case at `wire` time.
Do not add a construction-time existence check.

## Constraints

From `CONSTRAINTS.md`:

- **Producer Pointer-Rule Invariant** — the rubric stencil must never duplicate or paraphrase `contracts/specs/loom-plan-spec.md` or `manifest/designs/plan-card-format.md`; it points at them.
  Enforced by review obligation only, so it is a review focus for this task, not a test.
  The invariant binds instruction files and format-contract docs, not design docs restating a rule for a human reader — which is why `designs/loom.md`'s rubric section may hold the human-readable copy.
- **Stencil Ownership Invariant** — `//go:embed` in `contracts/stencils` is a seed default only, never a live read path.
  The rubric is read at call time from the told absolute stencils directory via `stencilstore.Read`.
  Nothing in this task may read `stencils.LoomRubricPlanReview` from production code; only `cmd/lyx`'s root pre-run and `internal/stencilcli` touch the registry.
- **Shed Recipe Registry Invariant** — no new registry entry.
  `Bouncer` and `BurlerRound` are already registered, and `internal/shedrecipe/registry_test.go`'s `TestRegistry_ShipsFourteenEntries` pins the engine-registry count at fourteen and must **not** change: fifteen *rows* in loom's list, still fourteen *engines* in the registry.
  Confusing the two counts is the likeliest mistake in this task.
- **Fabric Git Invariant** — the governing constraint on this task, and the reason `Plan-Burler` is `overlay` rather than `source`.
  An LLM agent never commits weft content; Go commits it in-process through `internal/fabricengine`, at a boundary the loop owner controls.
  The `commit_seam` this task adds is exactly that boundary, and it reuses `Env.CommitPlan`, which already goes through `fabricengine.CommitAnchoredPaths` with a positive-only pathspec (`planparser.PlanDirRel()`) — never a stage-all, never `:(exclude)`.
  Enforcement for this call site is a review obligation, not a machine check.
- **Recipe-Format Sole-Parser Invariant** — the recipe change is yaml data only; no new parsing.
- **Config Strictness Invariant** — `configRejectUnknown` in both entry constructors means every key written into the two new `config:` blocks must be in the recognised set.
  This task adds exactly one key to that set, `commit_seam` on the Bouncer entry, and adding it to the constructor's recognised list is part of the change, not an afterthought — omitting it makes every recipe using the key fail to build.
  No key is added to the Burler entry.
- **Review Round Invariant** — A-before-B, all severities fixed, no self-grading, commit-per-fix.
  Already enforced inside `burlerengine`; this task adds no round mechanics and must not restate the discipline in the rubric.
- **Test Tier Purity Invariant** — the `loomrecipe` tests are tier 1 and must stay offline: no cwd resolution, no process spawn.
  The existing fakes are what keep them there.
- **Documentation Lifecycle** — `manifest/designs/loom.md` and `manifest/roadmap.md` update in the same commit.
  `manifest/roadmap.md` moves because this completes a Planned item.
- **Markdown Link Integrity** — the new stencil and the edited design doc must keep their relative links resolvable.
- Project rule from `CLAUDE.md`: semantic line breaks in every `.md` file touched — one sentence per line, plus a break at internal independent-clause boundaries.
  This binds the new stencil and every design-doc edit.

## Testing

TDD candidates are the two stencil tests and the coverage guard: each fails for a clear, specific reason before the corresponding production change lands.

**`contracts/stencils/rubric_test.go`** — extend, following the existing `TestLoomRubricDiscussionReview_*` pair as the precedent.
Add:

- `TestLoomRubricPlanReview_NamesEveryRequiredItem` — one subtest per rubric item, asserting a short distinctive substring rather than a whole paragraph, so ordinary prose edits do not break it.
  Cover all seven items decided above (four "Also flag", three "Do not flag").
  The `plan-card-format.md` granularity phrasing ("independently reviewable/testable unit") and the `Custom`-exemption phrasing are natural anchors.
- `TestLoomRubricPlanReview_CarriesNoStencilMarkers` — asserts no `{{.` substring.
  This is the one that catches the failure mode where a rubric author reaches for a template marker; it must exist for the new stencil, not only the Discussion one.
- Update the file's own header comment, which currently names only `loom-rubric-discussion-review.md`.

**`contracts/stencils/registry_test.go`** — `TestRegistry_MatchesOnDiskTree` and `TestRegistry_DefaultsAndRelPathAreConsistent` should pass automatically once the embed var and the `entries` row are both added; if either carries a hardcoded count, it needs the increment.
Check both before assuming.

**`internal/loomrecipe`** — the row-count and row-name surface:

- `coverage_guard_test.go`'s `TestCoverageGuard_EveryLoomRowHasAnEngine` — its row table keys off the `loomshed.Name*` symbols, so it needs the `NamePlanReview` row replaced by two rows mapping to `Bouncer` and `BurlerRound`.
- `recipe_test.go`'s `TestNew_ShapeMatchesRecipe` and `TestRecipe_SeedAndResumeRowNamesExist` — fifteen rows.
- `shape_test.go`'s `TestNew_ProducerTable`, `TestNew_ProducerTableOrderUnchangedByWiring`, `TestNew_PassesShedValidation`, `TestNew_RoutingGraphIsClean` — the last is the one that proves the new mutual-bounce edges and the `segment` labels are consistent, and is the most valuable assertion in this task.
- `sequence_test.go`'s `TestSequence_FullRunBlocksAtPublish` — the numbers, exactly:
  `wantSequenceOrder` is **14** entries today and becomes **16**.
  The single `{NamePlanReview, Done}` entry is replaced by the same three-entry segment shape the Discussion pair already contributes — `{NamePlanBouncer, Stuck}`, `{NamePlanBurler, Stuck}`, `{NamePlanBouncer, Done}` — so it is minus one, plus three.
  (The round-2 review stated 17; that adds the three without removing the one.)
  Two counters increment with the second segment: `loomBurler.calls` from 1 to 2, and `loomShuttle.bouncerJudgeCalls` from 1 to 2.
  Check `bouncerSeedCalls` the same way — if the fixture asserts it anywhere, it goes 1 to 2 as well.
  The run must still block on `Publish`, with `commitDiscussionCalls` and `commitPlanCalls` unchanged at 1 each **unless** the new `commit_seam` fires in this fixture, which it will: `Plan-Bouncer`'s approved settle calls `Env.CommitPlan`, so `commitPlanCalls` becomes 2.
  Assert that explicitly rather than letting it drift — it is the scenario proof that the commit seam is reached on approval.
- `fixture_test.go` — add `loom-rubric-plan-review` to `seedBouncerStencils`' map.
  If the fake shuttle's `runBouncerJudge` is keyed on anything Discussion-specific, generalise it; it should already be role-keyed.
- `resume_test.go`'s bounce-routing tests — check whether any names `Plan-Review` as a routing target.

**`internal/shedadapters`** — the new `BouncerConfig.Commit` seam.
TDD candidate, and the highest-value new test in this task:

- A `settle` reaching `verdictApproved` with a non-nil `Commit` calls it exactly once, before returning `Done`.
- A `settle` reaching `verdictBlocking` never calls it — an unapproved plan must not be committed.
- A nil `Commit` is not an error and commits nothing, which is what pins `Discussion-Bouncer`'s unchanged behaviour.
- Decide and test the failure mode: a `Commit` returning an error must not silently swallow into `Done`.
  Route it through the existing `degrade` path rather than inventing a fourth outcome.

**`internal/shedrecipe/entries_bouncer_test.go`** — `commit_seam` resolution:
`plan` resolves to `env.CommitPlan`, `discussion` to `env.CommitDiscussion`, absent leaves `Commit` nil, and an unrecognised value is a construction error naming the key.
`configRejectUnknown`'s recognised set must gain `commit_seam`, or every recipe using it fails to build — assert the key is accepted.

**`internal/loomcli/wiring_test.go`** — `TestWire_ReviewSegmentSeamsFilled` and `TestWire_ReviewTripleMatchesLoadedConfig` already assert the `Env` fields both segments share and need no change, since no `Env` field is added.
Verify rather than assume; if `wiring_test.go` names the Discussion segment specifically in a comment, widen the wording.

**`internal/loomshed/stub_test.go`** — `TestStubProducer_Call` is unaffected; only the doc comment above `stubProducer` changes.

**Key scenarios that must be covered:**

- A recipe whose two new rows carry mismatched `segment` labels fails `shedengine` validation — covered by `TestNew_PassesShedValidation` / `TestNew_RoutingGraphIsClean` continuing to pass with the correct labels.
- The rubric is a marker value, not a template — `TestLoomRubricPlanReview_CarriesNoStencilMarkers`.
- The engine registry stays at fourteen while the row list goes to fifteen — `TestRegistry_ShipsFourteenEntries` must be left untouched and must still pass.
- An approved plan is committed exactly once by the loop owner and a blocked one is not committed at all — the `shedadapters` settle tests plus the `sequence_test.go` `commitPlanCalls == 2` assertion.
- A `commit_seam`-less Bouncer behaves exactly as before — the nil-`Commit` test, which is what keeps `Discussion-Bouncer` unchanged.

Build gate: `go build ./... && go test ./...` from the worktree root.

## Q&A log

- **Q:** Row names for the two new rows? **A:** [auto-pick] `Plan-Bouncer` + `Plan-Burler` with `segment: Plan-Review`. **Why:** the roadmap item names this pair verbatim, and it mirrors the shipped Discussion perch exactly.
- **Q:** How should `artifact_paths` name a variable card set? **A:** [auto-pick] a single directory entry, `_lyx/plan`. **Why:** no enumeration in the recipe stays correct across plans; `NewBouncer` stats nothing and `burlerengine`'s `requireExistingPaths` documents that a directory satisfies it.
- **Q:** What is the Burler's `fasit`? **A:** [auto-pick] `paths: [_lyx/discussion/decision-record.md]` plus instructions naming `loom-plan-spec.md` as format authority and `Plan-Validate` as owner of the mechanical checks. **Why:** the decision record is what the plan must implement; format conformance is already mechanically gated upstream, so putting the spec in `paths` invites duplicated checking.
- **Q:** Does Plan-Review get to read `support-log.md`? **A:** [auto-pick] no — excluded from both `artifact_paths` and `fasit.paths`, and stated explicitly in the rubric. **Why:** `Plan-Write` provably never reads it (`TestPlanSpec_PromptNeverNamesSupportLog`), so a finding grounded in it is unsatisfiable except by invention.
- **Q:** Does the rubric go beyond the three criteria `designs/loom.md` pins? **A:** [auto-pick] yes — four "Also flag" (the three plus decision-record fidelity) and a three-item "Do not flag" section. **Why:** `Plan-Review` sits directly downstream of a sixteen-check mechanical validator, so its over-flagging surface is larger than `Discussion-Review`'s, not smaller.
- **Q:** Keep or delete `stubProducer`? **A:** [auto-pick] keep, retargeting its doc comment to `Webster-Review` alone. **Why:** `Webster-Review` is still a `Stub` with its own roadmap item.
- **Q:** Keep `NamePlanReview` as a segment-label constant? **A:** [auto-pick] no — drop it, add `NamePlanBouncer`/`NamePlanBurler`. **Why:** `Discussion-Review` has no such constant; the segment label is recipe-literal, and keeping one would be a lone inconsistency.
- **Q:** `run_subdir` value? **A:** [auto-pick] `plan`. **Why:** mirrors `discussion`; the value names the phase, not the row, and its sameness across the pair is what joins their run directory.
- **Q:** `max_bounces` on the two rows? **A:** [auto-pick] `5` on both. **Why:** mirrors the shipped Discussion pair; no evidence yet that Plan needs a different budget.
- **Q:** Should the Bouncer keep a `Plan-Write` stuck edge? **A:** [auto-pick] no — `on_stuck: Plan-Burler`, and budget exhaustion escalates to a human. **Why:** that is the perch pattern; `Discussion-Bouncer` routes to `Discussion-Burler`, not `Discussion-Write`. Consequence: `designs/loom.md`'s "`Plan-Review`'s stuck routes back to `Plan-Write`" example goes stale and is rewritten in the same commit.
- **Q:** How wide is the test surface? **A:** [auto-pick] mirror the Discussion-Review commit exactly — stencil rubric tests, `loomrecipe`'s five test files, and a check of `wiring_test.go`. **Why:** the change is structurally the same change, so the same tests are the ones that would catch it going wrong.
- **Q:** [round 2, BLOCKING] The judge reads only `{{.artifacts}}`, the round report, and the prior ledger — how does it reach the `decision-record.md` that rubric item 4 measures against? **A:** [auto-pick] name the path in the rubric text under item 4, stating it is the measuring stick and never the subject. **Why:** one stencil feeds both rows so one edit reaches both; the judge already reads files by contract; and `artifact_paths` keeps meaning what its own doc says it means. Listing the fasit as an artifact would invite findings raised against the decision record itself.
- **Q:** [round 2] What are `Plan-Burler`'s `fix-scope` and `tool-use`? **A:** [auto-pick] `fix-scope: overlay`, `tool-use: true`, plus a new `commit_seam: plan` on `Plan-Bouncer` and a `Commit` closure called on `settle`'s approved branch. **Why:** `burlerengine/doc.go` names plan/discussion/review artifacts as exactly the overlay class, and the Fabric Git Invariant states an agent never commits weft content. An overlay round runs no git, and nothing else in the segment commits, so the seam is required rather than optional. This expands scope into `internal/shedadapters` and `internal/shedrecipe/entries_bouncer.go`, which the first draft listed as Out.
- **Q:** [round 2] The shipped `Discussion-Burler` uses `fix-scope: source` over `_lyx/discussion/*` — is that fixed here? **A:** [auto-pick] no. Record it as the same violation, add the `Commit` seam that makes the correction a two-line recipe change, and file a Planned roadmap item. **Why:** flipping it changes shipped behaviour and its tests; the divergence ships on purpose and the recipe comment says so, so it does not read as a copy-paste slip.
- **Q:** [round 2] How is stale text enumerated? **A:** [auto-pick] state the scan method — `Plan-Review`, `NamePlanReview`, and the fourteen-row count claim — and let scope follow from it. **Why:** a hand-written list missed a dozen sites; the count claim in particular needs per-hit classification, because `designs/loom.md`'s design table and `shedrecipe`'s engine registry both stay at fourteen while the recipe goes to fifteen.
- **Q:** Which docs land in the same commit? **A:** [auto-pick] `designs/loom.md` (table row 9, the rubric section reworded as a doc *about* the shipped stencil, and every site the stale-text scan finds), `designs/shed-recipe.md` (the new `commit_seam` key), and `manifest/roadmap.md` (this Planned item removed, the `Discussion-Burler` `fix-scope` item added). **Why:** the Documentation Lifecycle requires it, and this both completes a Planned item and adds one. `docs/overview.md` is untouched — no module-table or execution-stack change.
