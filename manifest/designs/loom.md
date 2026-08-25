# Loom: the phased orchestrator

> **Status: Design — not built.** This is a plan draft. Per the [documentation lifecycle](../../docs/overview.md#documentation-lifecycle), when the modules land the durable parts of this doc fold into `overview.md` and the package headers, and this file is deleted. Until then it is the single design reference for the loom orchestration model.

## What it is

Loom is the orchestrator that takes a task from intent to a merged change through a fixed sequence of **phases**, each guarded by a uniform **review gate**.
The control flow — phase order, the review round-loop, gate decisions, resume — lives in Go (`lyx loom run`).
The judgment — discussing, planning, building, reviewing, fixing — lives in agents spawned one-shot per step.
Go owns the machine;
the LLM owns the thinking.

The orchestrator is the **`loom`** module (`lyx loom run`); the gate is a **review segment** in loom's own producer list — a generic `Bouncer` review-gate producer paired with a `Burler`-round producer, both in `internal/shedadapters` — the iterative review loop, hand-wired once per phase. The `Burler`-round producer composes `burler` (see the `internal/burlerengine` package documentation), the review+fix round worker. The `/ly-*` skill layer shrinks to thin human-facing wrappers over these. The everyday call has a convenience alias: **`lyx run` → `lyx loom run`**. (Naming: `lyx` is the binary, `loom`/`burler` are modules, `ly-*` are the skills — see [overview.md](../../docs/overview.md).)

`loom` = `Shed` (see [shed.md](shed.md), the generic outer phase-FSM: sequencing, resume, crash recovery, pause, the status-file contract) + `loom`'s own ordered producer list, given in full in [the producer table below](#the-phase-machine--a-flat-producer-list-no-predefined-slots).
That list is recipe-backed: `contracts/recipes/loom-recipe.yaml` names the recipe's seventeen rows and their routing, and `internal/loomrecipe` assembles it into the `[]shedengine.ProducerDef` `Shed` consumes — see `manifest/designs/shed-recipe.md`.
The recipe carries seventeen rows against the table below's fifteen entries — see the note beneath the table for why the two counts differ and by how much.

## The phase machine — a flat producer list, no predefined slots

`Shed` (see [shed.md](shed.md)) has no predefined slots — no Preflight-slot, no Producer-slot, no shared Finalize.
It is a generic engine that walks one ordered, flat list of **producers**, honoring resume/crash-recovery/pause uniformly across the whole list;
atomicity — one mechanical action or LLM session — binds **simple** producers only, per the carve-out in [`shed.md`'s producer contract vs. producer definition](shed.md#producer-contract-vs-producer-definition).
`loom`'s own identity is entirely this list, nothing else — what makes `loom` "loom" (versus, say, `Hardener`) is purely which producers are in the list.
The table below is the durable record of that list's rows and routing;
the shipped list itself lives at `contracts/recipes/loom-recipe.yaml`, built into `[]shedengine.ProducerDef` by `internal/loomrecipe`.
The list's order is display and enumeration order only, never routing — see [`shed.md`'s bounce-budget and routing sections](shed.md#the-shed-loop--exact-mechanics) for the full argument; this doc does not restate it.
The table's `Kind` column records each producer's simple/bespoke typology;
see [`shed.md`'s producer contract vs. producer definition](shed.md#producer-contract-vs-producer-definition) for the carve-out that defines it.
Every row whose `Type` is `LLM` and `Kind` is `simple` is a `SingleLLMProducer` instance — see [`shed.md`'s engine-adapters section](shed.md#engine-adapters--a-thin-shared-seam-not-one-per-producer) for that generic type — configured here by its own Input pointer, Output pointer, and instruction file, nothing more:

| # | Producer | Kind | Type | Input | Output |
|---|---|---|---|---|---|
| 1 | `Preflight` | simple | mechanical | git/filesystem state (no format-contract file) | pass/fail — no artifact, a gate signal only |
| 2 | `Loom-Preflight` | simple | mechanical | loom's own status file → `loom-status-spec.md`'s check-4 validation checklist | pass/fail — no artifact, a gate signal only |
| 3 | `Discussion-Write` | simple | LLM | — (starting point) | `_lyx/discussion/` (`decision-record.md` + `support-log.md`), shape pinned in the producer's own stencil (`contracts/stencils/loom/loom-template-discussion.md`) |
| 4 | `Discussion-Validate` | simple | mechanical | `_lyx/discussion/` → [validation checks](#discussion-producer-detail--validation-checks-and-review-rubric) below | pass/fail, also callable standalone as `lyx loom validate-discussion` |
| 5 | `Discussion-Review` (`Discussion-Bouncer` + `Discussion-Burler`) | bespoke | LLM/review segment | `_lyx/discussion/` (both files) → [review rubric](#discussion-producer-detail--validation-checks-and-review-rubric) below | verdict (APPROVED/stuck) + review file |
| 6 | `Plan-Sweep` | simple | mechanical | `_lyx/discussion/decision-record.md` (approved) | quarry inventory (internal artifact, not gated) |
| 7 | `Plan-Write` | simple | LLM | `_lyx/discussion/decision-record.md` (**never** `support-log.md`) + `Plan-Sweep`'s inventory, once `Plan-Sweep` is built for real — its absence today is the normal degraded state the stencil now names outright, not an error | `_lyx/plan/`, shape pinned in `contracts/stencils/loom/loom-template-plan.md` |
| 8 | `Plan-Validate` | simple | mechanical | `_lyx/plan/` → `loom-plan-spec.md`'s existing hard-fail checks (e.g. `depends-on-order`) | pass/fail, also callable standalone as `lyx loom validate-plan` |
| 9 | `Plan-Review` (`Plan-Bouncer` + `Plan-Burler`) | bespoke | LLM/review segment | `_lyx/plan/` (current plan directory) → `_lyx/discussion/decision-record.md` (answer key) | verdict (APPROVED/stuck) + review file |
| 10 | `Plan-Revalidate` | simple | mechanical | `_lyx/plan/` → `loom-plan-spec.md`'s existing hard-fail checks, re-run because the segment's fixer rounds rewrite the plan after `Plan-Validate` already ran and no row between the segment and `Webster` parses the plan otherwise | pass/fail — no artifact, a gate signal only |
| 11 | `Batchifier` | simple | mechanical | `_lyx/plan/` (approved) + `batcher.yaml`'s `active:` key | pass/fail — a fail-fast gate confirming the active batchifier resolves cleanly before `Webster` spawns any LLM session, no artifact — already shipped as `internal/batcher`, "never an LLM's decision" per its own package doc |
| 12 | `Webster` | bespoke | black box (LLM + mechanical internally) | `_lyx/plan/` (approved); resolves the active batchifier itself, lazily, on every call — never a value handed across from `Batchifier`, since that row writes no artifact | committed diff — `internal/websterengine`'s own per-batch loop is a bespoke, multi-spawn producer, exempt from `Shed`'s atomicity rule by design, and stays opaque to `loom`'s flat list, same "black box loom drives, exactly like a review segment" framing as [below](#webster--a-black-box-loom-drives-the-sibling-of-the-review-segment) |
| 13 | `Webster-Review` (`Webster-Bouncer` + `Webster-Burler`) | bespoke | LLM/review segment | full diff → plan's card contract | verdict + review file — the full converge-loop gate over the whole diff |
| 14 | `Publish` | simple | mechanical | approved diff | PR opened, or no-op; not `loom`'s own — a generic `Shed` producer, shared by reference with `Hardener`'s producer list, see [internal/landingshed](../../internal/landingshed/doc.go) |
| 15 | `Finalize` | bespoke | mechanical | approved diff (+ open PR, if any) | merge-back; not `loom`'s own — a generic `Shed` producer, shared by reference with `Hardener`'s producer list, see [internal/landingshed](../../internal/landingshed/doc.go) |

**The table and the shipped recipe diverge deliberately.**
The recipe carries seventeen rows against the table's fifteen entries, and the difference has two separate causes, not one: the table carries `Plan-Sweep` (row 6) as its own row, which `contracts/recipes/loom-recipe.yaml` does not,
while the recipe carries three collapsed segment pairs the table shows as single entries — `Discussion-Bouncer`/`Discussion-Burler` collapsed into row 5's `Discussion-Review`, `Plan-Bouncer`/`Plan-Burler` collapsed into row 9's `Plan-Review`, and `Webster-Bouncer`/`Webster-Burler` collapsed into row 13's `Webster-Review` — the `Kind` column's own black-box framing, not an oversight;
see [the gate](#the-gate) below for why each pair stays collapsed to one here.
`contracts/recipes/loom-recipe.yaml` is the shipped list, authoritative for row names and routing.
This table is the human-readable design record, not required to track the recipe's row count row-for-row: its own count moves whenever a genuine new row lands, like row 10's `Plan-Revalidate` above, and only a collapsed segment pair leaves it unchanged.

`Preflight` and `Loom-Preflight` are **built**, together giving `loom` the two-row shape.
Row 1, `Preflight`, is the generic, product-agnostic gate: built as `internal/preflightshed`'s general producer over `internal/preflight.Check`, it validates worktree geometry and at-root (cwd resolution via `internal/lyxcwd`, sibling/Prime lookup via `internal/fabricengine`), the worktree pair's cleanliness, and fabric readiness and sync — warp branch == weft branch, via `warp`'s drift detection.
Row 1 is reusable verbatim by a second product's producer list, and that reuse is what makes `manifest/designs/hardener.md`'s "its own Preflight" possible.
Row 2, `Loom-Preflight`, is loom's own: built as `internal/loomengine.CheckSeed` over told paths, it validates that `_lyx/loom/status.json` exists and is a coherent fresh seed (no half-finished prior run).
On `stuck`, `Shed` bounces to the producer's own explicit `OnStuck` target (e.g. `Plan-Validate`'s stuck route bounces back to `Plan-Write`), which may sit anywhere in the list, or escalates to a human when none is set — never "keep fixing symptoms."
The same explicitness now governs the done direction too: `Done` routes via this producer's own `OnDone`, not by list position — see [`shed.md`'s routing and bounce-budget sections](shed.md#the-shed-loop--exact-mechanics) for the full design, not restated here.

**Raddle folds into `Finalize`'s own contract** — not a separate producer, and not a separate step after Webster the way earlier drafts of this doc had it.
`Finalize` is not `loom`'s own (see rows 13–14 above and [internal/landingshed](../../internal/landingshed/doc.go)), so this fold is a fact about `Finalize` itself, inherited by every `Shed` list that names it — not something `loom` defines.
Raddle-regeneration (git-diff-targeted docs over `git diff <start-SHA>..HEAD`, building heavily on millhouse's `codeguide-update`, committed into the weft via `lyx fabric sync`) is scoped to run as part of the `Finalize` merge, not before it — updating Raddle before the merge is impractical given merge-conflict risk, so it happens as part of the merge itself.
`Hardener`'s `Tenter` will need the equivalent fold eventually — not designed here.

Each row's Input and Output, in the normal case, are *pointers* into a format-contract file defining the consumed/produced artifact's shape, never a restated copy of its content.
This is a producer-*authoring* convention, not a `Shed`-level mechanism — `Shed` itself never reads Input or Output through a pointer at all (see [`shed.md`'s producer contract vs. producer definition](shed.md#producer-contract-vs-producer-definition)); it is [CONSTRAINTS.md](../../CONSTRAINTS.md)'s Producer Pointer-Rule Invariant that enforces it, by review, over instruction files and format-contract docs.
The thin-Input carve-out (a chain-head producer, human intent instead of an artifact) and thin-Output carve-out (a gate producer's pass/fail signal, or a terminal producer's no-downstream-consumer) apply to specific rows of the table above — see `shed.md`'s own section for both, stated once rather than restated here.
Review is never a property attached to the producer it reviews — it stays a separate producer, reached by explicit routing rather than by position in the list, consistent with the review gate being one generic mechanism reused for every phase (see [the gate](#the-gate) below).

**The phase-machine skeleton is testable against fake phases before real producers are wired in**, the same fake-tested approach the round loop used against a fake `burler`.
Build order follows from this as a deliberate operator decision, not just a testing technique: every `mechanical` row `loom` itself owns (plus `Webster`, already shipped) is built for real first, every `LLM`/review-segment row stays a stub until then.
`Publish` and `Finalize` (rows 13–14) sit outside this ordering entirely — they are not `loom`'s to build; `loom: phase-machine scaffolding` stubbed both, then swapped in the real, shared-by-reference producers once `landing: Publish + Finalize producers` landed, on its own schedule (see [internal/landingshed](../../internal/landingshed/doc.go)).
The shipped `landing: parent-fabric resolution chain` item completed their construction chain by filling `Env.Landing`, so `Publish`/`Finalize` are now genuinely constructible in a real `lyx loom drive` run, not merely implemented.
The concrete breakdown of `loom`'s own rows — which land in `loom: phase-machine scaffolding` vs. `loom: session bootstrap` vs. the deliberately-last per-producer prompt/rubric tasks (`loom: Discussion-Write producer`, `loom: Discussion-Review producer`, `loom: Plan-Write producer`, `loom: Plan-Review producer` (shipped), `loom: Webster-Review producer` (shipped)), and exactly which rubrics are missing — lives in `manifest/roadmap.md` and the tasks' own wiki briefs, not restated here.

`Discussion`'s mechanical pre-gate and `Preflight`/`Finalize`'s thin-Output shape are both resolved by `Discussion-Validate` (row 4) and `shed.md`'s producer-contract section respectively — see [`shed.md`'s producer contract vs. producer definition](shed.md#producer-contract-vs-producer-definition).

## Discussion producer detail — validation checks and review rubric

`_lyx/discussion/` is produced by `Discussion-Write` (stencil: `contracts/stencils/loom/loom-template-discussion.md`, which pins `decision-record.md`'s and `support-log.md`'s section shape as the agent's own instructions).
This section carries the detail that belongs to `Discussion-Validate` and `Discussion-Review` instead, rather than to the Discussion-Write stencil itself: a mechanical validator's checklist and a review rubric are not part of what the *writing* agent needs to read.

### Validation checks (spec for `Discussion-Validate`)

Per-run checks:

- Both files exist under `_lyx/discussion/` (`decision-record.md` and `support-log.md`).
- `decision-record.md` has all seven required sections present (Goal, Scope, Decisions, Constraints, Auto-mode assumptions, Open risks, Acceptance criteria);
  "Notes for the plan writer" is optional and its absence is not a violation.

This mechanical producer is **exhaustively defined by the checks listed above** — it has no judgment, and nothing beyond these two checks is "its" to look for.

The same two checks are callable standalone as `lyx loom validate-discussion`.
The verb and this row call the identical package function, `discussionparser.Validate`, so they can never disagree — see the [Gate Self-Check Parity Invariant](../../CONSTRAINTS.md#gate-self-check-parity-invariant) for the rule itself.
The verb reports *which* file or heading failed, while the row's `Stuck` deliberately carries an empty pointer.

**The `Plan-never-reads-support-log` boundary is not a per-run check.**
The boundary itself: `Plan-Write`'s declared input set never names `support-log.md`.
It is asserted once, at build/test time, over `Plan-Write`'s producer *definition* — never re-evaluated per run — because it is a property of the definition itself, and there is nothing per-run for a mechanical producer to evaluate about it.
This assertion has landed, as `TestPlanSpec_PromptNeverNamesSupportLog` in `internal/loomengine/plan_test.go`, which proves the composed plan prompt names neither `support-log.md` nor the support log's own absolute path.

### Discussion-Review rubric — what not to flag

The shipped stencil `contracts/stencils/loom/loom-rubric-discussion-review.md` is `Discussion-Review`'s own rubric — read by both `Discussion-Bouncer` and `Discussion-Burler`, the row's two-producer perch.
This subsection is a doc *about* that stencil, per the Producer Pointer-Rule Invariant, not a second copy it must point at — it is the durable human-readable record the stencil was transcribed from, kept in step with the stencil rather than restated inside it.

`Discussion-Review` is the LLM producer, not the mechanical one — over-flagging is a judgment failure mode a mechanical producer (which has only checks, never judgment) cannot exhibit.
Do not flag any of the following as a finding:

- **A missing "Notes for the plan writer" subsection.**
  It is optional by contract; its absence is never a deficiency.
- **Missing rejected alternatives in `decision-record.md`.**
  Rejected alternatives belong in `support-log.md`'s Rejected alternatives section, not in `decision-record.md`;
  their absence from `decision-record.md` is by design, not an omission.
- **Incomplete call-site or cross-reference enumeration.**
  That enumeration belongs to the compiler and to `Plan-Sweep`'s mechanical inventory, not to `Discussion-Review`.

### Discussion-Review rubric — what to also flag (relocation and exclusion)

This subsection is likewise a doc *about* the shipped stencil `contracts/stencils/loom/loom-rubric-discussion-review.md`, per the Producer Pointer-Rule Invariant — the durable human-readable record, not a second copy the stencil must point at.
The writer-side half of this same principle now lives in `contracts/stencils/loom/loom-template-discussion.md` itself;
this subsection remains the durable copy.

- **Relocation and exclusion findings are legitimate, on equal footing with gap-filling findings.**
  `Discussion-Review` must accept "this belongs in `support-log.md`, not `decision-record.md`" and "this doesn't belong in Discussion at all" as legitimate findings.
  A review loop that can only resolve a finding by adding content is the concrete mechanism behind discussion files that only grow across rounds.
- **The completeness-before-leanness test.**
  Before any relocation finding, check whether the content carries a requirement or constraint Planner needs.
  Extract that into `decision-record.md`'s own Decisions or Constraints first, and move only the surrounding deliberation narrative — because `Plan-Write` never reads `support-log.md`, making a careless move a silent loss rather than a relocation.
- **The writer/reviewer symmetry note.**
  Whatever `Discussion-Write`'s stencil says not to gather, this rubric must say not to flag as missing, or the additive bias reappears even with the writer-side fix in place.

### Plan-Validate detail

`lyx loom validate-plan` makes the same three `planparser` calls this row's `ShedProducer` makes, in the same order: `planparser.PlanDir`, `planparser.ParsePlan`, then `planparser.Validate`.
The verb and the row call the identical `planparser` functions, so they can never disagree — see the [Gate Self-Check Parity Invariant](../../CONSTRAINTS.md#gate-self-check-parity-invariant) for the rule itself.

### Plan-Review rubric

The shipped stencil `contracts/stencils/loom/loom-rubric-plan-review.md` is `Plan-Review`'s own rubric — read by both `Plan-Bouncer` and `Plan-Burler`, the row's two-producer perch.
This subsection is a doc *about* that stencil, per the Producer Pointer-Rule Invariant, not a second copy it must point at — it is the durable human-readable record the stencil was transcribed from, kept in step with the stencil rather than restated inside it.

The subject under review is the current plan (`_lyx/plan/00-overview.md` and the card files its Card Index names) against `_lyx/discussion/decision-record.md` as the answer key, per the Card model in [plan-card-format.md](plan-card-format.md).
`support-log.md` is excluded from this review entirely — it appears in neither the artifact list nor the answer key, and `Plan-Write` provably never reads it, so a finding grounded in its content cannot be satisfied except by inventing the missing link.

`Plan-Review` is the LLM producer, not the mechanical one — over-flagging is a judgment failure mode a mechanical producer (which has only checks, never judgment) cannot exhibit.

Do not flag any of the following as a finding:

- **Anything `Plan-Validate` already checks.**
  The sixteen check IDs `contracts/specs/loom-plan-spec.md`'s own validation-checks section lists, `format-unrecognized` through `commit-subject-mismatch`, are enforced deterministically upstream;
  re-deriving them here is duplicated work whose only possible outcome is disagreement with the parser.
- **A missing `DependsOn`/`Produces` field, or an incomplete dependency list.**
  Dependency edges are derived, never authored — a card's `Uses` intersected against every other card's target list.
  Plan-time completeness of that intersection is explicitly not provable;
  the real gate is the post-merge build and test.
- **A `Rename`, `Move`, `Prosa`, or `Custom` card carrying no `ImpactSummary`.**
  It is required for `Edit` and `Delete` only, per the per-type table in [plan-card-format.md](plan-card-format.md).
  For `Rename` the reason is specific: a correctly executed AST-aware rename is binary, with no graded blast radius to summarise.

Also flag:

- **Granularity.**
  One card per independently reviewable/testable unit, not one card per literal symbol.
  A private supporting type, or a constructor inseparable from its type, belongs in the other symbol's card;
  an independently testable symbol gets its own card even when one card is its only consumer.
- **`ImpactSummary` carries a real conclusion.**
  A one-line blast-radius conclusion, never a restatement of `Intent`.
- **`Custom` is a last resort.**
  Used only where none of `Create`, `Edit`, `Delete`, `Rename`, `Move`, or `Prosa` genuinely fits, never as a shortcut around correct typing.
  A `Custom` card is exempt from `path-missing` on its own targets and from `prosa-symbol-target`, so a mistyped one silently escapes two checks the rest of the plan is held to.
- **Fidelity to the decision record.**
  Every Decision and every Constraint in `_lyx/discussion/decision-record.md` is carried by some card, and no card introduces scope that file does not license.
  The decision record is the measuring stick and never the subject — every finding is raised against the plan, never against the decision record.

## Plan-Sweep detail — the quarry-inventory spec

**Build order note:** `Plan-Sweep` is not built in `loom: phase-machine scaffolding` — it stays a stub there, alongside `Plan-Write`, its only consumer.
Building a real `Plan-Sweep` before `Plan-Write` is real would have nothing to feed.
Unlike `Plan-Write` (its own split-out `loom: Plan-Write producer` roadmap item), `Plan-Sweep` stays a stub past that point too — deferred to its own Someday roadmap item, since quarry-backed work is low-priority project-wide right now and this is the only row in the initiative that touches quarry.
`Discussion-Validate` and `Plan-Validate`, which do land in scaffolding, carry no such dependency.

`Plan-Sweep` (row 6) is `simple`/`mechanical` like `Discussion-Validate` — no judgment, exhaustively defined by the checks below, not a smaller version of what `Plan-Write` (the LLM) does.
Its job is grounding, not selection: hand `Plan-Write` real `quarry` lookups for whatever the decision record already named, so the writing agent starts from resolved definitions/references instead of re-grepping blind.

**Deterministic extraction.**
The repo's own doc convention is the extraction rule: every code identifier, file path, and symbol name in `decision-record.md`'s prose is backtick-quoted, the same convention this doc and every other `manifest/designs/*.md` file already follows.
`Plan-Sweep` reads `decision-record.md`'s Scope section (the same section-parsing `Discussion-Validate` already does to check presence, now living in `internal/discussionparser`) and collects every backtick-quoted span inside it — nothing outside Scope, and no judgment about which spans "matter."

**Resolution, not selection.**
Each collected span is classified mechanically, by shape, not meaning: a span containing `/` or a `.go`/`.md`-style extension is treated as a path and checked for existence on disk;
anything else is treated as a symbol name and looked up through `quarry`'s existing symbol lookup, then enumerated via its `References` API.
A span that resolves to nothing — a prose word that happened to be backtick-quoted, a symbol `quarry` can't find — is silently dropped, never a failure;
`Plan-Sweep` has no pass/fail outcome of its own (the table's Output column already marks it "not gated").

**No persisted artifact.**
Unlike `Discussion-Write`'s output, the inventory is never written to `_lyx/plan/` or anywhere else — it costs nothing to recompute (a handful of `quarry` lookups, not an LLM call), so `Shed`'s resume-on-output-files model doesn't apply to it: on resume, `Plan-Sweep` just reruns before `Plan-Write` starts, exactly like the first pass.
This also means it needs no format-contract doc under `contracts/`; the shape below is `Plan-Write`'s own prompt-assembly concern, not a pinned cross-producer contract.

**Shape handed to `Plan-Write`.**
A flat list, one entry per resolved span: the original span text, its kind (`path` or `symbol`), and — for a symbol — its definition site(s) plus reference sites from `quarry`'s `References` API.
Deduplicated and sorted;
order carries no meaning `Plan-Write` should read into it.

## The gate

Each producing phase is guarded by a **review gate**, and from loom's view that gate is a **black box with two exits — `APPROVED` or `stuck`.** loom calls it, advances on `APPROVED`, and on `stuck` routes to the same stuck handler described above. loom does not see the rounds, the handler/fixer, the cluster reviewers,
or the progress-judge inside.

That black box is a **review segment** — a `Bouncer` review-gate producer paired with a `Burler`-round producer — hand-wired once per phase (discussion / plan / webster) out of the same two generic adapters.
The whole point of the black-box boundary is that loom drives all phases **identically** because the verdict contract is invariant;
only the review *profile* (rubric + fasit), the fixer round's *fix-scope*, and the segment's *commit seam* differ per phase.
See the `internal/shedadapters` package documentation for the two producers and their round-artifact contract,
and the `internal/burlerengine` package documentation for the combined handler/fixer round and the profile schema.

**Which segment fixes what, and commits how.**
The Discussion and Plan segments fix overlay content and commit it through the loop owner's commit seam, while the Webster segment fixes warp source and commits each fix itself.
The Fabric Git Invariant is the reason the split exists: it reserves every weft commit to the loop owner in Go,
and it names the agent's own commit-per-fix to the warp repo as the one exception.

**The review model's home is `loom.yaml`, not the recipe.**
`loom.yaml`'s `review:` and `review_timeout_min:` keys are the review segments' model and timeout, validated at load time exactly like the existing `discussion:` and `plan:` keys.
They thread through as run-wide `shedrecipe.Env.Review*` values, which a segment's two rows fall back to whenever their own recipe `config:` block omits a per-row model/effort/version/timeout key — the recipe is embedded in the binary, so a recipe-literal model would be untunable without a rebuild.

## Webster — a black box loom drives, the sibling of the review segment

From loom's view, **Webster is a black box loom calls, exactly like a review segment**: `loom` runs `webster` and, once it returns `done`, drives the terminal **Webster-Review gate** — a full converge-loop over the whole diff. loom does not see Webster's per-batch fork loop, its verbs,
or its escalation mechanics, the same way it doesn't see the review segment's rounds.
Webster's own internal design lives in the `internal/websterengine` package documentation, not here.

**Naming note.** `webster` (`internal/websterengine`/`internal/webstercli`, `lyx webster`) is the stack's implementer module; its cross-module contract is [webster-spec.md](../../contracts/specs/webster-spec.md).
This doc's producer list above targets `internal/websterengine` (plan-format, in-session forks) as `loom`'s own Webster producer.

Pause stays uniform across loom/review-segment/Webster (see [pause](#graceful-pause)) because every loop checks the same `pause_requested` flag at its own step boundary, regardless of which module holds the loop.

## Webster-Review rubric

The shipped stencil `contracts/stencils/loom/loom-rubric-webster-review.md` is `Webster-Review`'s own rubric — read by both `Webster-Bouncer` and `Webster-Burler`, the row's two-producer perch.
This section is a doc *about* that stencil, per the Producer Pointer-Rule Invariant, not a second copy it must point at — it is the durable human-readable record the stencil was transcribed from, kept in step with the stencil rather than restated inside it.

Two dimensions on top of ordinary diff review:

- **Comment-convention compliance.** Any new/changed doc comment follows [code-comment-conventions.md](code-comment-conventions.md) — no unnecessary symbol cross-references.
- **Per-card mechanical check.** Confirms the card's Type-specific mechanical check actually ran and passed (e.g. the AST-script-plus-grep for a Rename card, `assert-no-callers` for a Delete card), not only that the diff compiles and tests pass.

The stencil adds two things beyond those two bullets, so this durable record is not thinner than the shipped file: it derives its own review range from `product.parent` in loom's status file rather than guessing, and it blocks with a BLOCKING finding when that value cannot be read; and it carries a do-not-flag list keeping the three upstream gates' subjects — the plan's format, the plan itself, and the overlay artifacts — out of this gate's findings.

## `loom` — the autonomous driver

`lyx loom run` (alias `lyx run`) is the phase machine,
and it is essentially autonomous.
It reads loom's **status file** in `_lyx/`, sees which phase (and review sub-state) the task is on, and continues from there.
It is idempotent and re-entrant: **stop anywhere — Ctrl-C, crash, close the laptop — and the next `lyx run` continues where it left off.**

This is the lyx model applied to orchestration: one-shot, daemonless, file-coordinated, resume-from-disk. `lyx run` is a pure function of {status file + artifact files} with no hidden process state.
Because the status lives in the weft repo (git-synced), resume works across machines too.
It is per-task and cwd-authoritative ([Principle 4](../../docs/overview.md#principles)).

**Human boundaries.** `lyx run` drives every phase it *can* drive **unattended** — the agents are interactive tmux sessions,
but no human sits in them ([Agent execution](#agent-execution)).
When it reaches an inherently interactive boundary — Discussion input, or a `stuck` escalation — it stops cleanly, writes the next action to the status file, and exits.
The human does the interactive part (which advances the status),
and the next `lyx run` resumes unattended.
So `lyx run` is autonomous for everything it can advance and yields only at the human gates.

**Auto mode.**
A run can be told to *never* yield — `lyx run --auto`.
The phase machine is unchanged;
the only difference is that at a would-be human gate the agent is instructed to **make its own best guess and proceed** instead of asking (and the `AskUserQuestion` guardrail — see the `internal/shuttleengine/claudeengine` package documentation — already forbids it from blocking on a dialog).
Auto mode does **not** turn off the view: reed still shows every strand (incl. the `lyx loom status` line), because you still want to watch.
The difference is in loom's *yielding*, not in whether anyone is looking.

### State & contracts

- **The status file (`_lyx/loom/status.json`, JSON via `internal/state` — see [loom-status-spec.md](../../contracts/specs/loom-status-spec.md)) is the single source of truth** for orchestration state: `current_producer` names which producer this run is at, and a **per-producer-call outcome** trail (`history`) records every call, including stuck-handler bounce-backs — per-round verdicts live in the review segment's own round artifacts, not here.
  Nothing orchestration-relevant lives anywhere else.
  The pause flag (`pause_requested`) is also kept **in-status** (see [Graceful pause](#graceful-pause)).
  Product-scoped under `loom/`, not bare `_lyx/status.json`, because `Shed` (see [shed.md](shed.md)) is instantiated by more than one product — the Someday `Hardener` will need its own status file too, and a bare `_lyx/status.json` could not serve both without colliding.
  `Shed` itself has no opinion on this path at all: it is told its status-file path, never derives it (see `shed.md`'s own producer-contract section) — this scoping is entirely `loom`'s own choice as the caller.
- **It also carries a human-readable *current-activity* `activity`, mechanically composed by `Shed` itself** — not just the machine enum, but "*now:* spawned plan-handler round 2, waiting on Stop hook / *last:* round 1 BLOCKING, 3 findings / *wait:* —".
  This is what the `lyx loom status --watch` strand prints (a 1-line pane at the top, per the `internal/reedengine` package documentation on the strand contract) so the operator sees what the Go driver is *doing*, not only what the agents are saying.
  The driver writes the file;
  the status strand reads and prints it — reed never parses it, it just hosts the pane.
- **Round-level resume.**
  Handler/fixer artifacts are already on disk, so resuming inside a review block continues at the current round rather than restarting the phase.
- **Separation of state.** The review segment owns its round state in its own run-directory files; `lyx run`'s status only needs phase + the segment's outcome. When the segment's `Bouncer` returns `Done`, `lyx run` advances.

### Crash recovery — resume on output files, not live processes

After a crash, a restarted `lyx run` cold-starts from the `_lyx/` status file and must reconcile its logical state with whatever agents may or may not still be alive.
The discipline that makes this tractable: **loom resumes on output files AND on live-agent evidence — file existence is never used on its own to skip a step.**
The file contract still decouples "was the work done" from "is the process alive," but a check that reads output-file existence alone cannot tell an interrupted step from a re-entered one whose files were left behind by something other than a finished agent — see "The crash-versus-bounce question," below.
For the step it was on:

1. **Is there a complete output file?** → *inside* an attached or started run's own wait loop — where this check lives — the step finished;
   read it and advance.
   (The agent's process may be long dead — its result survived.
   This is the common case.)
   This is never a producer-level shortcut answering "do files exist" on its own: inside a run there is an agent to attribute the completion to, while at the producer level a `Discussion-Validate` bounce makes the files-exist question true without any agent having run — the trap the next section resolves.
2. **Else, is the agent's session still alive?** `shuttleengine.Runner.Attach`, probed by `shedadapters.SingleLLMProducer.Call` before it archives anything, scans the run-dir root for a `run.json` whose `OutputFiles` match the spec's and whose persisted `Outcome` is still `"running"`, then asks `reed` (see [overview.md#modules](../../docs/overview.md#modules)) whether that record's `StrandGUID` is still tracked with a live pane.
   A match: re-attach, just wait on its `Stop` hook (do **not** respawn — that would duplicate).
3. **Else (dead, no output):** `SingleLLMProducer.Call`'s unchanged archive-then-spawn fallback respawns a **fresh** agent for the step, hydrated from the prior round's on-disk artifacts.
   The round is idempotent, so a fresh handler is deterministic.

loom therefore **never depends on `claude --resume` for correctness** — an unfinished step is respawned, not resumed (reed's `--resume` is finicky for programmatically-driven sessions,
and a never-conversed session has nothing to resume). reed's pane-`--resume` is a *separate, non-critical* layer that restores the **visible** sessions for the operator (see the `internal/reedengine` package documentation on resume);
loom's correctness rests on files and live-agent evidence together.
A dead claude with a finished output file is, to loom, a **done step** — not a problem.

**The crash-versus-bounce question, resolved.** A `Discussion-Validate` bounce re-enters `Discussion-Write` with both discussion files already present on disk, byte-identical to the state a crash mid-interview leaves.
The two are told apart purely by whether an agent for this producer is still alive: a surviving `run.json` matching the spec's output files, whose persisted `Outcome` is still `"running"` and whose `StrandGUID` reed still tracks with a live pane, means attach;
anything else — including both files present but nothing alive — means respawn.
File existence alone never answers this question, which is exactly what made a naive producer-level file-existence check ping-pong the two cases until the bounce budget was exhausted.

**Accepted residual.** A crash in the window between a run reaching `done` and Shed persisting that outcome re-runs the completed step from scratch: `Attach` finds nothing live, because `finalize` already ran its cleanup, so the producer archives a finished interview and re-interviews from the top — in interactive mode, the operator answers the whole interview twice.
This trade is the right direction.
The only thing that would rescue that window is a producer-level file-existence check, and that check is the trap itself: on a `Discussion-Validate` bounce the files are also present and complete-looking, and treating that as `Done` there ping-pongs the run until its bounce budget blocks it — a frequent hard failure is worse than a narrow one that costs only rework.
The deliberate non-fix, should this window ever bite in practice: a marker distinguishing "these files were produced by a run that reached `done`" from "these files are merely present" — a new durable-state contract, not designed here.

**This section's own heading is pinned.**
`manifest/roadmap.md` and this file's own [Graceful pause](#graceful-pause) section both link `#crash-recovery--resume-on-output-files-not-live-processes`, so the heading text above stays exactly as written even though the rule beneath it has changed — `CONSTRAINTS.md`'s Markdown Link Integrity invariant binds.
If a future editor judges the heading actively misleading, renaming it is a separate change that must update both inbound links in the same commit.

## Graceful pause

`lyx loom pause` requests a pause;
the running orchestration honours it at the next **step boundary**, never mid-operation — `mill-pause`'s natural-stopping-point property, made systematic.

- **A property of the loop pattern, not loom alone.**
  Every loop — loom (phases), the review segment (rounds), [Webster](#webster--a-black-box-loom-drives-the-sibling-of-the-review-segment) (batches;
  its loop is LLM-held,
  but the batch-spawn verb checks the flag in Go before spawning) — checks a `pause_requested` flag in the [status file](#state--contracts) at its step boundary and stops before spawning the next unit.
  The **innermost active loop** honours it first, so pause lands at the finest active boundary (next batch / round / phase).
  The Go code is almost always *between* steps (it spawns and waits), so catching it there is trivial.
- **The leaf agent finishes its unit;
  nothing is killed.**
  Boundary pause lets the in-flight worker complete its small unit (one batch / round — its output file written), then the driver stops.
  Resume (`lyx loom run`) spawns the next step from the status file — the same resume-on-files-and-live-agent-evidence discipline as [crash recovery](#crash-recovery--resume-on-output-files-not-live-processes), minus the crash.
- **In-agent interrupt is optional.**
  To pause *faster* than the current unit finishes, `shuttle` (see the `internal/shuttleengine` package documentation) can ESC-and-hold the live agent (session kept warm in the reed server — see [overview.md#modules](../../docs/overview.md#modules), not killed;
  resume continues it in place).
  With Webster decomposed into batches/cards the boundary wait is short, so this is a latency nicety, not a correctness requirement.
- **Distinct from crash recovery.**
  Crash (involuntary death) respawns a fresh agent from the on-disk output files (loom never relies on `claude --resume` for correctness — see above).
  Pause deliberately stops at a boundary, so there is nothing to respawn — the cheaper path.
  Both rest on the file contract;
  pause just avoids the death.

## Module decomposition

| Piece | Form | Notes |
|-------|------|-------|
| `loom` (`lyx loom run`) | new Go module | the phase machine / autonomous driver |
| the review segment (`Bouncer` + `Burler`-round producer) | new Shed adapters | the gate loop: run `burler` rounds → `APPROVED`/`stuck` + progress-judge + cap |
| `burler` | new Go module | one review+fix round: A-review (+ optional cluster) → B-fix; composed by the segment's `Burler`-round producer |
| webster | LLM orchestrator (Master session, in-session forks) + Go verbs (`internal/websterengine`/`internal/webstercli`) | a black box from loom's view — see `internal/websterengine`'s package documentation and [webster-spec.md](../../contracts/specs/webster-spec.md), webster's own cross-module contract |
| producers (discussion / plan) | prompt/profile files | **not** modules — a prompt + `shuttleengine.Spec` factory in `internal/loomengine` each (`DiscussionSpec`, `PlanSpec`); `DiscussionSpec` is ✅ **built and wired**, as recipe row 3's `DiscussionWrite` engine, and `PlanSpec` is likewise ✅ **built and wired**, as the recipe's `Plan-Write` row's `PlanWrite` engine. |
| `lyx loom status` | a loom subcommand | the 1-line status view; runs as a strand (see `internal/reedengine`; `below-parent` + `ShrinkWhenWaitingOnChild`), not a separate module |
| execution stack | existing/new infra | `proc` → reed → shuttle — see [overview.md#execution-stack](../../docs/overview.md#execution-stack-orchestration-layers) — built once, used by both modules above |
| Preflight (row 1, generic) | new Go package (`internal/preflightshed`) | ✅ **Done**, engine-only (no cobra module yet) — validates the tier-1/tier-2 preconditions (geometry + at-worktree-root, warp worktree clean, weft paired & in sync) over git/filesystem state, over `internal/preflight.Check`; reusable verbatim by a second product's producer list |
| Loom-Preflight (row 2, loom's own) | new Go package (`internal/loomengine`) | ✅ **Done**, engine-only (no cobra module yet) — validates that loom's own status file exists and is a coherent fresh seed (no half-finished prior run), over told paths; builds on `internal/state` |
| `/ly-*` skills | thin wrappers | over `lyx loom run` |

The new Go specific to loom is the **two modules** (`loom`, `burler`) plus the **webster module** (`internal/websterengine`/`internal/webstercli` — the fat verbs + distillation the Master orchestrator drives) and the `lyx loom status` subcommand;
beneath them is the shared [execution stack](../../docs/overview.md#execution-stack-orchestration-layers) (`proc`, `reed`, `shuttle`);
and everything else is prompt files, profiles,
and the existing lyx modules.
The display is **not** a module — it is `lyx loom status` running in a strand that `reed` (see [overview.md#modules](../../docs/overview.md#modules)) hosts and arranges.

## Entry point — the session bootstrap

Today: launch `claude` in a terminal, then `/mill-start` — an interactive LLM session drives everything.
Loom inverts this: `lyx loom run` (alias `lyx run`) is the **session bootstrap** — more than the driver alone.
Run in a worktree's pane, it:

```
lyx loom run:
  0a. resolve the recorded parent branch                  (fabricengine.ReadOrigin, plus --parent for a
                                                           legacy worktree created before the record existed;
                                                           refused when --parent disagrees with a recorded value)
  0b. seed the status file when it is absent               (loomshed.Seed; a re-run's already-seeded case is
                                                           tolerated via its own sentinel, never re-seeded)
  0c. commit that seed weft-side, before anything below    (fabricengine.CommitWeftPaths; this must land
                                                           before the driver spawns, or the phase machine's
                                                           own first precondition row sees an uncommitted
                                                           status file and fails immediately)
  1. ensure the worktree's tmux session is up           (reed)
  2. add the status strand                                (reed.AddStrand "lyx loom status --watch",
                                                           display: below-parent, shrinkWhenWaitingOnChild:true —
                                                           full height while it has no live child, collapsing to
                                                           collapsed_strip_rows once a forked child exists. A
                                                           childless status strand rendering full-height is
                                                           intended, not a bug to re-file (discussion Decision
                                                           childless-full-height-is-acceptable).)
  3. spawn the loom driver DETACHED, unless one is         (internal/proc — it needs no TTY;
     already alive, then wait for its handshake            it reads/writes files, drives strands via reed;
                                                           the handshake polls for the driver taking the run
                                                           lock, so the spawner never returns before a driver
                                                           is actually running)
  4. attach the current terminal to the tmux session     (reed takes the foreground)
```

So **loom goes to the background and the tmux session takes the window.** loom needs no terminal — it coordinates through files and drives strands via reed — so the screen is free for the reed view (the status line on top, agents below as they spawn). loom and the view are independent: loom writes the `_lyx/` status file;
the status strand reads and prints it;
neither blocks the other.

**The run-launcher.**
A double-click shortcut makes this one click: `lyx fabric add` drops a third script, `run<ext>`, into the pair's existing per-slug hub launcher directory, beside the `ide` and `fabric-checkout` scripts already written there.
It is written by the same builder and torn down by the same pair as those two, cross-platform by the same GOOS-selected extension (`.cmd` on Windows, `.sh` elsewhere).
It invokes the explicit two-word verb, `lyx loom run`, rather than the root alias, so it keeps working regardless of what happens to the alias.
Because everything is [cwd-authoritative](../../docs/overview.md#principles), the launcher needs no arguments — geometry resolves from cwd, so you cannot run it from the wrong place.
It embeds no absolute path: it climbs relatively to the worktree subpath, so nothing is machine-bound.
It reuses the [launcher geometry](../../docs/overview.md#hub-geometry-invariants) already in `internal/fabricengine`.

**One terminal per worktree.**
Scope for now is exactly that — each worktree its own terminal / tmux session.
The cross-worktree multi-column view (all worktrees in one window) is a deferred reed feature (see the `internal/reedengine` package documentation) — cheap when it comes (a `worktree` strand field + a grouping rule), but not now.

## Agent execution

Every agent loom spawns — producers, the review handler, cluster reviewers, the progress-judge — runs through the `internal/shuttleengine` layer as an **interactive tmux session, never headless `claude -p`** (an economic constraint;
see the `internal/shuttleengine` package documentation).
**I/O still rides the file contract** — the agent writes its output files and Go reads them — so the file-contract design above is unchanged;
only the *spawn + completion-detection* mechanism differs from a headless model.

The consequence for loom: it sits on top of the [`proc → reed → shuttle`](../../docs/overview.md#execution-stack-orchestration-layers) stack, so that stack is on loom's critical path. loom (via its review segments — see the `internal/shedadapters` package documentation — → `burler`, see the `internal/burlerengine` package documentation) calls `shuttle.Run` per spawn and stays ignorant of strands, layout, and engines — those belong to `reed` (see [overview.md#modules](../../docs/overview.md#modules);
the strand bookkeeping + render: which pane is which, layout, focus, the cluster window where N reviewers go) and `shuttle` (see the `internal/shuttleengine` package documentation;
the swappable provider engine).
What loom owns is everything in this document: the phase machine, the gate wiring,
and the status contract.
