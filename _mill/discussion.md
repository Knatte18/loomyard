# Discussion: webster: DAG-derived card sequencing

```yaml
task: 'webster: DAG-derived card sequencing'
slug: webster-dag-card-sequencing
status: discussing
parent: main
```

## Problem

webster executes a plan's cards in the order the plan happens to declare them, and nothing checks that that order matches what the cards actually depend on.
`internal/websterengine`'s package doc has carried a sketch of the eventual fix since v0 — the "dead DAG seam" (`if card.HasSymbolFields() { … } else { declared order }`) — with the `else` arm as the only live one.
A plan whose author listed card 8 before card 5, where card 8 reads a symbol card 5 creates, runs card 8 against a symbol that does not exist yet;
the failure surfaces as an implementer fork flailing against a missing definition, not as a plan defect.

**Why now:** Wave 2 of the "loom: rewrite for the new Plan Card format" roadmap group shipped the `planparser` Card-format migration (commit `d5c2fe36`), so every card now carries a machine-readable target list (`Card.Targets`) and read list (`Card.Uses`).
The dependency graph the seam always wanted is now derivable from data already on disk — `contracts/specs/loom-plan-spec.md` already states the flat card list *is* a DAG of intent and that turning it into an execution order is the executor's job.
This task makes webster do that job.

## Scope

**In:**

- A new sequencing mechanism in `internal/websterengine` that derives a dependency graph from the plan's cards (`Targets` vs `Uses`), condenses strongly-connected components, and returns the plan's execution batches in a deterministic topological order.
- Applying that sequencing at every site that computes execution batches, so all of webster's verbs and its Master prompt agree on one order.
- Rendering Master's `{{.batch_index}}` in the sequenced execution order, and rewording `contracts/stencils/webster/webster-template-master.md` so "in order" unambiguously means "in the order listed", not "in ascending batch number".
- Surfacing detected cycles (non-trivial SCCs) so a plan defect is visible rather than silently absorbed.
- Tier1 unit tests for edge derivation, SCC condensation, ordering stability, and cross-call determinism;
  a render test proving `{{.batch_index}}` reflects the sequenced order;
  master-template property-test updates.
- Same-commit doc updates: `internal/websterengine/doc.go`, `contracts/specs/loom-plan-spec.md`, `docs/overview.md`'s webster bullet, and moving the roadmap's Wave 3 item to Done.

**Out:**

- **Any concurrency or parallelism.** Still one worktree, still one fork at a time, still strictly sequential — the ready-set/wave recomputation and worktree-per-card spawning belong to the Someday **webster: worktree-per-card parallel execution** item, which depends on this one.
- **Any change to `internal/batcher`.** Grouping stays the batchifier's decision (Batcher Registry+Config Invariant);
  this task sequences the batches a batchifier already produced. No new registry entry, no `batcher.yaml` change, no change to the default identity batcher.
- **Any change to `internal/planparser`.** No new parse rule, no new validation check, no new `Card` field, no exported shape classifier.
  The plan format does not change (`loom-plan-spec.md` explicitly pins that an execution-policy change must not require a format change).
- **Any change to batch identity, numbering, report filenames, or `state.json` keys.** A batch's number stays its first card's `Number`.
- **Reconciling `manifest/designs/webster-parallel-execution.md`.** That doc is stale, and the roadmap explicitly assigns its reconciliation to the Someday worktree-per-card item. Leave it alone.
- **Symbol resolution against ground truth.** No `go doc`, no AST, no disk stat. Matching is string equality over the already-normalized refs `planparser` hands back.
- The Someday `scout-backed plan symbol fields` item and `manifest/designs/scout-plan-symbol-fields.md`.

## Decisions

### sequencing-lives-in-websterengine

- Decision: The graph derivation and topological ordering live in `internal/websterengine`, in a new file (`sequence.go`), operating over `[]batcher.Batch` — not over raw `[]planparser.Card`.
  No new package, no new batchifier.
- Rationale: The **Batcher Registry+Config Invariant** reserves *grouping* for `internal/batcher` and states batching is "not webster's own execution-policy decision" — but sequencing the batches a batchifier produced *is* webster's execution policy, and `loom-plan-spec.md`'s "Plan vs. schedule" section says exactly that ("Whoever executes the plan … decides *how* to turn the DAG into an actual run").
  The roadmap item itself names `websterengine`'s `doc.go` seam as what this activates.
  Operating at the **batch** level rather than the card level means the mechanism composes with any future grouping batchifier without change — under today's identity batcher batch ≡ card, so the two levels coincide, but the general form is the batch-level one.
  websterengine's package doc already establishes that this module owns its mechanism end to end, with webster-local helpers (`fingerprint.go`, `pause.go`, `classify.go`) rather than imported ones — a `sequence.go` is the same shape.
- Rejected: A new leaf package `internal/carddag` — nothing outside webster consumes it, and the Someday worktree-per-card item that would be its second consumer also lives in webster.
  A `dag` batchifier registered in `internal/batcher` — it would make sequencing config-selectable and off by default, conflate ordering with grouping, and would not compose with a genuine grouping batchifier (only one `active:` batchifier exists at a time).

### always-on-no-config-key

- Decision: DAG-derived sequencing is unconditional. No config key, no flag, no opt-in.
- Rationale: The roadmap says "execute in topological order **instead of** blind declared order".
  A plan whose declared order already matches its dependencies sequences to exactly its declared order (see `stable-kahn-tiebreak` below), so for every correct plan the change is a no-op — there is nothing to opt out of except the defect detection.
  A config key would also be a second execution-policy knob alongside `batcher.yaml`'s, with no stated need.
- Rejected: A `sequencing:` key in webster's config defaulting to `dag`, and a pure opt-in flag. Both add a surface with no consumer.

### edge-derivation-rule

- Decision: For cards A and B, derive a directed edge **A → B** ("A must run before B") when `B.Uses ∩ A.Targets ≠ ∅`.
  Matching is exact string equality over the refs as `planparser` returns them (already `root:`/`//`-normalized), with **no distinction between symbol-shaped and path-shaped refs** — both participate.
  Additionally, when two cards share a `Targets` entry, derive an edge from the lower-numbered card to the higher-numbered one (declared order settles two writers of the same thing).
  A card's `Uses` never matches another card's `Uses` — a read creates no ordering against another read.
- Rationale: This is verbatim what `manifest/designs/plan-card-format.md` specifies: "Dependency edges are derived, never authored: a card's `Uses` intersected against every other card's target list in the same plan is the dependency graph."
  Treating paths and symbols alike is both simpler and more correct — a `Prosa` card targeting `docs/overview.md` and a later card that `Uses` that file have a real ordering, and `planparser`'s shape classifier is unexported anyway (Planparser Sole-Parser Invariant), so re-deriving it outside the package would be a duplicate parser.
  The shared-`Targets` rule covers the plan format's own `Create`-then-`Edit` sequencing, which `loom-plan-spec.md`'s `card-field-overlap` check explicitly calls "legitimate cross-card sequencing" and deliberately does not flag.
  Rename endpoints need no special handling: `parseTypeLabelCase` already projects both sides of every `Pairs` entry into `Targets`.
- Rejected: Symbol-shaped refs only (drops real path dependencies, and requires duplicating `classifyRef`).
  `Uses`↔`Targets` edges only, with no shared-`Targets` rule (leaves two writers of one symbol unordered, so their relative order would be an arbitrary tie-break rather than the plan's declared intent).

### producer-first-regardless-of-declared-position

- Decision: Edge direction is determined by the `Uses`/`Targets` relationship alone and is **independent of declared position**.
  If card 3 `Uses` a symbol card 8 `Targets`, card 8 runs before card 3 — the plan's declared order is overridden.
- Rationale: This is the entire point of the task: "Catches a plan whose declared card order doesn't actually match its real dependencies."
  A `Uses` edge asserts the card needs that ref in its post-plan state;
  honoring declared position instead would reduce the feature to a linter that changes nothing.
  `plan-card-format.md`'s line about "plan order settles it" describes the already-consistent case (card 5 precedes card 8), which this rule reproduces unchanged;
  it is not a rule for the inconsistent case, which that doc does not address.
  Declared order survives only as the tie-break among genuinely independent cards and as the intra-SCC order.
- Rejected: Respecting declared order and emitting only a warning — leaves the broken plan broken.

### scc-condensation-not-refusal

- Decision: Handle cycles with Tarjan strongly-connected-component condensation: topologically order the condensed DAG, and inside a non-trivial SCC keep the members' declared (card-number) order.
  Non-trivial SCCs are surfaced — not fatal.
- Rationale: `contracts/specs/loom-plan-spec.md` already names "SCC-merging" as part of the intended scheduling design, and `websterengine`'s doc.go names its absence ("no strongly-connected-component merging in v0") as a v0 gap this item closes.
  Mutual references between two cards are plausible and not always a defect (card A edits `Foo` which reads `Bar`, card B edits `Bar` which reads `Foo`);
  refusing the run would make a previously-working plan un-runnable, which is a regression, not a fix.
  Condensing keeps the run correct wherever the graph is acyclic and degrades to declared order only inside the cycle — the smallest possible blast radius.
- Rejected: Hard-refusing the run at `Run` pre-flight (the way zero-batches is refused) — too brittle for a heuristic graph whose completeness `plan-card-format.md` itself says "cannot be proven mechanically at plan time".
  Falling back to whole-plan declared order on any cycle — throws away correct ordering everywhere else in the plan because of one local cycle.

### cycle-visibility

- Decision: Non-trivial SCCs are reported, not silent: the sequencing function returns them alongside the ordered batches, `Run` logs one line per detected cycle naming its member batch numbers, and the same information is included in the run's result so `lyx webster status`-shaped output can carry it.
  Detection never changes the exit path.
- Rationale: Silently absorbing a cycle would hide a genuine plan defect the author should fix, and this repo's style is fail-loud;
  reporting-without-refusing is the compromise the `scc-condensation-not-refusal` decision needs to not be a silent behavior change.
- Rejected: Silent condensation (hides a real defect);
  writing a findings file (a whole new artifact for one diagnostic).

### stable-kahn-tiebreak

- Decision: Order the condensed DAG with Kahn's algorithm, where the ready set is a min-heap keyed on the SCC's lowest member batch number.
  The result is the topological order closest to the plan's declared order.
- Rationale: Determinism is a hard requirement, not a nicety: `internal/webstercli` recomputes `c.batcher.Batch(plan.Cards)` independently in `begin-batch`, `await-batch`, `record-batch`, and `recover-batch`, and `Run` computes it a fifth time.
  Two calls that disagreed on order would desynchronize Master's loop from the verbs.
  Keying the tie-break on declared number additionally guarantees the no-op property stated in `always-on-no-config-key`: an already-correctly-ordered plan sequences to precisely its declared order, so no existing plan's behavior changes.
- Rejected: DFS post-order (deterministic but drifts arbitrarily far from declared order for no benefit);
  unordered map iteration (non-deterministic — would break the five-call-site agreement outright).

### cards-with-no-refs-float

- Decision: A card with neither `Targets` nor `Uses` matching anything else in the plan produces no edges. It lands at its declared position via the min-heap tie-break.
  No synthetic pinning edges.
- Rationale: With the declared-number tie-break, "no edges" already means "stays where it was" relative to everything else unconstrained.
  Synthetic edges would over-constrain the graph and defeat the point for the Someday parallel-execution item that reads it.
  This also retires the `HasSymbolFields()` framing in `doc.go`'s sketch: there is no branch on whether a card has symbol fields — a field-less card is just a vertex with degree zero.
- Rejected: Pinning field-less cards to their declared neighbours with synthetic edges.

### batch-identity-unchanged

- Decision: Batch identity, numbering, report filenames, and `state.json` keys are untouched.
  `batchIdentity` still returns the first card's `Number` and `Slug`;
  `ReportFileName(number, slug)` is unchanged;
  `State.Batches` stays keyed by that number;
  the reserved `-1` integration key is unaffected.
  Only the **order of the slice** changes.
- Rationale: Renumbering to execution position would invalidate every existing on-disk report and state file, break crash-resume (which rehydrates from `state.json` plus the reports directory), and break `RenderProgress`'s `st.Batches[c.Number]` lookup — all for cosmetics.
  `doc.go` already anticipates that batch-number/card-number coincidence is a property of the identity batcher, "not this package's contract".
- Rejected: Renumbering batches 1..N by execution position.

### master-drives-the-listed-order

- Decision: `RenderBatchIndex` renders in **sequenced** order, one line per batch, each line carrying the batch number it must pass to the verbs.
  `RenderMasterPrompt` takes the sequenced `[]batcher.Batch` (in addition to, or instead of, the raw plan) so the rendered index and the verbs cannot diverge.
  `contracts/stencils/webster/webster-template-master.md` is reworded from "Drive it STRICTLY in order — batch N assumes every batch before it is already committed" to make "in order" mean **the order listed above, top to bottom** — explicitly not ascending batch number — while keeping the existing "no batch is ever skipped or reordered because it 'looks independent'" prohibition verbatim.
  `RenderProgress` likewise lists batches in sequenced order.
- Rationale: The verbs are order-insensitive (`findBatch` looks up by number), so the *only* thing that actually determines execution order today is the list Master reads and drives top-to-bottom.
  Rendering a sequenced list while leaving the template saying "strictly in order" would be actively ambiguous to the model — the template is the load-bearing half of this change, not an afterthought.
  Keeping a single list (rather than adding a second `{{.execution_order}}` marker beside `{{.batch_index}}`) avoids handing Master two orderings to reconcile, which is exactly the failure this task exists to remove.
- Rejected: A separate `{{.execution_order}}` template value alongside the existing declared-order `{{.batch_index}}` — two lists, one of which must be ignored, is a worse prompt.
  Leaving the template untouched and relying on the reordered list alone — the surviving "strictly in order" wording contradicts it.

### sequencing-applied-at-every-batch-computation-site

- Decision: Export one function from `websterengine` and call it at all five sites that compute batches: `runlevel.go`'s `Run`, and `internal/webstercli`'s `beginbatch.go`, `awaitbatch.go`, `recordbatch.go`, `recoverbatch.go`.
  The four CLI verbs are order-insensitive today, so this is defensive rather than load-bearing — but it makes "the batch list webster works with is always the sequenced one" true by construction.
- Rationale: A single sequenced list everywhere removes the possibility of a future order-sensitive verb quietly reading the declared order.
  The cost is one pure function call per verb over an already-parsed plan.
- Rejected: Applying it only in `Run` — cheaper, but leaves two different orderings live in the codebase with only a comment separating them.

### no-planparser-change

- Decision: `internal/planparser` is not modified. No new validation check for cycles, no exported ref classifier, no new `Card` field.
- Rationale: The **Planparser Sole-Parser Invariant** scopes that package to parsing the on-disk format;
  a dependency graph over parsed cards is not parsing.
  The **Gate Self-Check Parity Invariant** would additionally require a matching CLI self-check verb and parity test for any new mechanical gate added to `planparser.Validate` — real work with no stated need, and `plan-card-format.md` already argues dependency-list completeness is not mechanically provable at plan time anyway.
  Refs arrive already normalized (`normalizeCard` runs once per card at parse time), so string equality is sufficient without touching the package.
- Rejected: Adding a `card-dependency-cycle` validation check to `planparser` (drags in the parity-verb obligation, and duplicates cycle detection that must exist in webster regardless).

## Technical context

**Where the pieces are today:**

- `internal/planparser/plan.go:67` — `Card`. The two fields that matter: `Targets []string` (line 93, "the card's own flat target ref list — symbols and paths mixed, in body order"; Rename `Pairs` endpoints are already projected here) and `Uses []string` (line 104).
  Note the field is named **`Targets`**, not `Edits` — the roadmap and design doc still use the `Edits`/`Uses` framing from before the Wave 2 implementation renamed it.
- `internal/planparser/normalize.go` — `normalizeCard` runs once per card inside `ParsePlan`, so every downstream consumer sees clean, forward-slash, worktree-relative paths for path-shaped refs and verbatim text for symbol-shaped ones. String equality across two cards' ref lists is therefore well-defined without further work.
- `internal/planparser/classify.go` — `classifyRef`/`isPathRef` are **unexported** and deliberately so. Do not try to reach them.
- `internal/batcher/batcher.go` — `Batch{Cards []planparser.Card}` and the `Batcher` interface.
  `internal/batcher/identity.go` — the default: one card per batch, input order.
- `internal/websterengine/runlevel.go:648` — `batchIdentity(b) (number, slug)` returns `b.Cards[0].Number, b.Cards[0].Slug`.
- `internal/websterengine/beginbatch.go:89` — `findBatch(batches, number)` is a linear scan by identity, so it is order-insensitive.
- `internal/websterengine/runlevel.go:339` — `Run` computes `batches := deps.Batcher.Batch(plan.Cards)`, refuses a zero-length result, and holds the only in-package call to `RenderMasterPrompt` (line 483).
- `internal/websterengine/render.go:264` — `RenderBatchIndex(plan)` renders `plan.Cards` in declared order as `"%02d — %s — %s"` (number, slug, summary).
  `render.go:277` — `RenderProgress(plan, st)` walks `plan.Cards` and looks up `st.Batches[c.Number]`.
  `render.go:232` — `RenderMasterPrompt(plan, st, …)`, one caller, in-package.
- `internal/webstercli/{beginbatch,awaitbatch,recordbatch,recoverbatch}.go` — each independently re-runs `c.batcher.Batch(plan.Cards)`. This is why determinism is non-negotiable.
- `contracts/stencils/webster/webster-template-master.md` — line 34 `{{.batch_index}}`, line 37 ("navigation source, not the execution unit"), lines 38–40 ("Drive it STRICTLY in order … no batch is ever skipped or reordered because it 'looks independent'"), line 58 ("For each batch not already reported, in order").
  Lines 38–40 and 58 are the wording that must change;
  the "never skip / never reorder on your own judgment" prohibition must survive.
- `internal/websterengine/doc.go` — the section **"# Declared order now, a dead DAG seam for later"** is the doc this task rewrites. Its `HasSymbolFields()` sketch describes a method that has never existed;
  the only two references to that name in the repo are inside that doc comment. There is nothing to delete in code.

**Docs that make a now-false claim and must be corrected in the same commit** (Documentation Lifecycle):

- `internal/websterengine/doc.go` — the dead-seam section.
- `contracts/specs/loom-plan-spec.md` — the "Plan vs. schedule" section's "sequential-in-declared-order today" (line ~43) and the "Deferred / forward-compat" section's "v0 runs strictly in declared order; the eventual DAG scheduler is the roadmap's Wave 3 `webster: DAG-derived card sequencing` item" (lines ~195–197).
- `docs/overview.md` — the webster module bullet (~line 300–303), which describes plan consumption and batching.
- `manifest/roadmap.md` — move the Wave 3 `webster: DAG-derived card sequencing` item to Done, and update the Someday **webster: worktree-per-card parallel execution** item's "Depends on `webster: DAG-derived card sequencing` (Planned, above)" pointer, which will no longer be accurate.
- `contracts/specs/webster-spec.md` — line 7 says Master "forks one implementer per execution batch in-session, sequentially, until the plan is built", which stays true;
  check whether anything else there implies declared order before deciding to touch it.

**Do not touch:** `manifest/designs/webster-parallel-execution.md` and `manifest/designs/scout-plan-symbol-fields.md` — both stale, both explicitly owned by other roadmap items.

**Gotchas:**

- The five independent `Batch(plan.Cards)` recomputations are the sharpest constraint in this task. Any map iteration, any `sort.Slice` with a non-total comparator, any reliance on Go map ordering will produce a run that desynchronizes mid-plan and is very hard to diagnose.
- `RenderProgress`'s `st.Batches[c.Number]` lookup is why batch identity must not be renumbered.
- `State.Batches[-1]` is the reserved integration-escalation key (`internal/websterengine/integration.go:90`). It is not a real batch and must never enter the graph.
- `verifyEveryBatchDone` (`runlevel.go:655`) iterates batches to assert every one reached terminal-done. It is order-insensitive and needs no change, but it must still see the full batch set after sequencing — sequencing reorders, never filters.
- `beginbatch.go:178` and `recoverbatch.go:216` read `deps.State.Batches[batchNumber-1]` to fetch "the previous batch's digest" for the next fork's prompt.
  **`batchNumber-1` is arithmetic on the card number, not the execution predecessor.** Under a reordering plan this fetches the wrong digest — the card numerically before it, not the batch that actually ran before it.
  Deciding whether to correct this to a genuine execution-predecessor lookup is in scope for the plan;
  it is the one place where reordering makes an existing correct-by-coincidence lookup wrong.
- `Run` refuses a zero-batch plan (`runlevel.go:344`). Sequencing must preserve that: it is length-preserving, and the plan should assert it.

## Constraints

From `CONSTRAINTS.md`:

- **Batcher Registry+Config Invariant** — batching (grouping) is `internal/batcher`'s, selected by `batcher.yaml`'s `active:` key, "not webster's own execution-policy decision".
  This task adds no batchifier and changes no grouping;
  it sequences the batches a batchifier already returned. Reviewers should check this line specifically.
- **Planparser Sole-Parser Invariant** — no other package parses `_lyx/plan/`;
  consumers read the plan only from the `planparser.Plan` model a caller hands in, and `planparser` never resolves cwd.
  This task consumes the model only.
- **Test Tier Purity Invariant** — untagged test files must not call `gitexec.Run`/`gitexec.RunGit`, `exec.Command`/`exec.CommandContext`, `gitkit.Copy*`, or `hubforge.NewHub`, and must not `time.Sleep` for a compile-time-constant ≥ 1s.
  All of this task's new tests are pure and untagged.
  Enforced by `cmd/lyx/tierpurity_test.go`.
- **Hermetic Git Test Environment Invariant** — irrelevant here as long as no new test spawns git;
  none should.
- **Cwd Resolution Invariant** — `websterengine` is `_lyx`- and fabric-blind: every path arrives already resolved via a `Geometry` value, never a `*lyxcwd.Location`. New code takes no path at all.
- **Stencil Ownership Invariant** — check it before editing `webster-template-master.md`;
  the template is webster's own stencil, but the invariant governs who may edit what.
- **Producer Pointer-Rule Invariant** — an instruction file must never duplicate or paraphrase another producer's format-contract content, only point at it.
  The master-template reword must not restate the plan format;
  it should describe the ordering rule and nothing more.
- **Documentation Lifecycle** — see the doc list under Technical context. Docs land in the same commit.
- **Markdown Link Integrity** — the roadmap and spec edits both carry cross-doc links;
  the link-integrity test will catch a broken one.
- **Config Strictness Invariant** — `websterengine` and `batcher` are both on the degrading side. This task adds no config, so nothing moves.

From `CLAUDE.md`:

- Markdown uses semantic line breaks (one sentence per line, plus breaks at internal independent-clause boundaries) — applies to every `.md` file touched, including the roadmap and spec edits.
- A task changing observable CLI behavior or cross-cutting infrastructure updates its module doc, `docs/overview.md`, and `CONSTRAINTS.md` in the same commit.
  `manifest/roadmap.md` moves because this completes a planned item.
- No new cross-cutting invariant is expected from this task, so `CONSTRAINTS.md` likely needs no new section — but if the plan concludes "webster sequences, batcher groups" deserves pinning, it belongs in the existing Batcher Registry+Config Invariant rather than as a new one.

Discovered during discussion:

- The sequencing function must be **pure and deterministic** — same input slice, same output order, every call, in every process. This is a correctness constraint, not a style preference (five independent recomputations).
- The sequencing function must be **length-preserving**: it reorders and never drops, filters, or merges batches. `verifyEveryBatchDone` and `Run`'s zero-batch refusal both depend on the full set surviving.
- An already-correctly-ordered plan must sequence to **exactly** its declared order. This is what makes the change a no-op for every existing plan and is worth an explicit test.

## Testing

All new tests are Tier 1: pure, untagged, no spawn, no disk, no git.

**`internal/websterengine` — edge derivation (TDD candidate).**
Table-driven over hand-built `[]batcher.Batch` fixtures.
Scenarios that must be covered:

- `B.Uses` matching `A.Targets` on a symbol-shaped ref produces `A → B`.
- The same on a path-shaped ref (proves paths are not excluded).
- Two cards sharing a `Targets` entry produce a lower-number → higher-number edge.
- Two cards sharing only a `Uses` entry produce **no** edge.
- A card whose `Targets`/`Uses` match nothing produces no edges.
- A ref appearing in one card's own `Targets` and its own `Uses` (the `card-field-overlap` defect) produces no self-edge.
- A Rename card's `Pairs` endpoints, already projected into `Targets`, participate as targets.
- A multi-card batch (non-identity batchifier): card-level edges lift to batch-level, and an edge internal to one batch produces no self-loop on the condensed vertex.

**`internal/websterengine` — SCC condensation and ordering (TDD candidate).**

- An acyclic graph orders topologically.
- A plan already in dependency-correct declared order sequences to **exactly** its declared order — the no-op property.
- A plan where a consumer is declared before its producer sequences with the producer first (the headline case from the roadmap).
- A two-card cycle condenses into one SCC whose members run in declared order, with the rest of the plan still correctly ordered around it.
- A three-card cycle, and two disjoint cycles in one plan.
- A cycle is reported: the returned SCC list names the right member batch numbers, and a fully acyclic plan reports an empty list.
- Output length always equals input length (length-preserving).
- Determinism: sequencing the same input twice returns identical order;
  sequencing an input whose slice order has been shuffled returns the same order (order of the *input* slice must not affect the result beyond the declared-number tie-break).

**`internal/websterengine` — render.**

- `RenderBatchIndex` emits lines in sequenced order, and each line carries the batch number the verbs expect (not an execution position).
- `RenderProgress` lists terminal batches in sequenced order and still resolves `st.Batches` by card number.
  The existing `TestRenderProgress_ListsOnlyTerminalBatches` must keep passing.
- `RenderMasterPrompt` still fills every marker after the signature change — `TestRenderMasterPrompt_*` and `TestMasterTemplate_FillsWithAllMarkers` cover this.

**`internal/websterengine` — master template properties.**
Existing template property tests live in `internal/websterengine/template_test.go`.
`TestMasterTemplate_StatesBracketSequenceAndRecoveryLadder` and `TestTemplates_NoDroppedBatchConceptsRemain` are the two most likely to need updating for the reworded ordering instruction.
Add a property asserting the template tells Master to drive **the listed order** and still forbids skipping or self-directed reordering.

**Digest-predecessor lookup.**
If the plan corrects `beginbatch.go:178` / `recoverbatch.go:216`'s `batchNumber-1` arithmetic to a true execution-predecessor lookup, that needs its own test: a reordered plan where the execution predecessor's number is not `batchNumber-1`, asserting the correct digest is rendered into the fork prompt.
Existing `beginbatch_test.go` / `recoverbatch_test.go` coverage of the current behavior must be updated rather than left asserting the old arithmetic.

**Regression surface to keep green:** `internal/websterengine`'s full untagged suite, `internal/batcher`'s suite (should be untouched), `cmd/lyx/tierpurity_test.go`, `cmd/lyx/hermeticenv_test.go`, and the markdown link-integrity test.

## Q&A log

- **Q:** Where should the dependency graph and topological ordering live — `internal/websterengine`, a new leaf package, or a new batchifier in `internal/batcher`? **A:** [auto-pick] `internal/websterengine`, a new `sequence.go` over `[]batcher.Batch`. **Why:** the Batcher Registry+Config Invariant reserves *grouping* for `internal/batcher` while `loom-plan-spec.md` assigns *scheduling* to the executor, the roadmap names websterengine's `doc.go` seam, and batch-level sequencing composes with any future grouping batchifier.
- **Q:** Should DAG sequencing be always-on or gated behind a config key? **A:** [auto-pick] Always on, no config key. **Why:** a dependency-correct plan sequences to its own declared order, so the change is a no-op for every existing plan and there is nothing to opt out of but the defect detection.
- **Q:** What exactly derives an edge? **A:** [auto-pick] `A → B` when `B.Uses ∩ A.Targets ≠ ∅`, exact string equality over normalized refs, paths and symbols alike;
  plus a lower→higher declared-number edge when two cards share a `Targets` entry;
  never `Uses`↔`Uses`. **Why:** verbatim `plan-card-format.md`'s derived-edges rule, and `planparser`'s shape classifier is unexported so re-deriving it outside the package would be a duplicate parser.
- **Q:** When a card that `Uses` a ref is declared *before* the card that targets it, does the producer move first or does declared order win? **A:** [auto-pick] The producer always runs first;
  edge direction ignores declared position. **Why:** that mismatch is precisely what the roadmap item exists to catch — honoring declared position would make the feature inert.
- **Q:** What happens on a dependency cycle? **A:** [auto-pick] Tarjan SCC condensation — order the condensed DAG topologically, keep declared order inside an SCC, and report non-trivial SCCs without failing the run. **Why:** `loom-plan-spec.md` already names SCC-merging as the intended design, mutual references are not always a defect, and refusing would turn previously-runnable plans un-runnable.
- **Q:** Should a detected cycle be visible to the operator? **A:** [auto-pick] Yes — returned from the sequencing function, logged by `Run`, carried in the run result. Never fatal. **Why:** silent condensation would hide a real plan defect, and this is what keeps the non-refusal decision from being a silent behavior change.
- **Q:** How are independent batches tie-broken? **A:** [auto-pick] Kahn's algorithm with a min-heap on lowest member batch number. **Why:** determinism is mandatory (five independent recomputations across `Run` and four CLI verbs), and a declared-number key yields the topological order closest to the plan's own.
- **Q:** What about a card with no `Targets`/`Uses` matches at all? **A:** [auto-pick] No edges;
  it floats to its declared position via the tie-break, with no synthetic pinning. **Why:** over-constraining the graph would defeat the Someday parallel-execution item that reads it, and this retires the `HasSymbolFields()` branch framing entirely.
- **Q:** Does batch numbering / report naming / state keying change? **A:** [auto-pick] No — only the slice order changes. **Why:** renumbering would invalidate every on-disk report and state file and break crash-resume and `RenderProgress`'s `st.Batches[c.Number]` lookup, for cosmetics.
- **Q:** How does Master actually execute the new order, given the verbs are order-insensitive? **A:** [auto-pick] `RenderBatchIndex` renders the sequenced order (each line still carrying its batch number), `RenderMasterPrompt` takes the sequenced batches, and the master template is reworded so "in order" means the listed order — keeping the existing no-skip / no-self-reorder prohibition. **Why:** the rendered list is the only thing that determines execution order today;
  leaving "strictly in order" in the template beside a reordered list would be actively ambiguous.
- **Q:** One `{{.batch_index}}` list in execution order, or a second `{{.execution_order}}` marker? **A:** [auto-pick] One list. **Why:** two orderings, one of which must be ignored, is exactly the confusion this task removes.
- **Q:** Where is sequencing applied — only in `Run`, or at every batch-computation site? **A:** [auto-pick] Every site: `Run` plus the four `internal/webstercli` verbs, through one exported function. **Why:** defensive today (the verbs look up by number), but it makes "the batch list is always the sequenced one" true by construction rather than by comment.
- **Q:** Does `internal/planparser` change — a cycle-detection validation check, an exported ref classifier? **A:** [auto-pick] No change at all. **Why:** the Planparser Sole-Parser Invariant scopes it to parsing, and a new mechanical gate would drag in the Gate Self-Check Parity Invariant's verb-plus-parity-test obligation for no stated need.
- **Q:** `beginbatch.go:178` and `recoverbatch.go:216` fetch the previous digest with `State.Batches[batchNumber-1]`, which is arithmetic on the *card* number. Reordering makes that the wrong batch. Fix it here? **A:** [auto-pick] Flag it as in scope for the plan to decide and test. **Why:** it is the one existing lookup that is correct only by the batch-number/execution-order coincidence this task breaks, so it cannot be left unexamined — but whether the fix is a true execution-predecessor lookup or a documented accepted deviation is a plan-level call.
- **Q:** Which docs land in the same commit? **A:** [auto-pick] `internal/websterengine/doc.go`, `contracts/specs/loom-plan-spec.md`, `docs/overview.md`, `manifest/roadmap.md` (item to Done + the Someday item's dependency pointer);
  check `contracts/specs/webster-spec.md`;
  explicitly **not** `manifest/designs/webster-parallel-execution.md`. **Why:** the Documentation Lifecycle requires it, and the roadmap assigns that stale design doc's reconciliation to the Someday worktree-per-card item, not this one.
