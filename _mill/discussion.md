# Discussion: Scope the Shed producer-model rewrite into buildable tasks

```yaml
task: Scope the Shed producer-model rewrite into buildable tasks
slug: shed-producer-model-scoping
status: discussing
parent: main
```

## Problem

A design conversation landed a revised model for `Shed`, the shared outer phase-FSM behind `loom` and the Someday `Hardener`.
The old model — "two swappable slots (Preflight, producer) plus Finalize as literally-shared code" — is gone.
The new model: `Shed` has **no predefined slots at all**;
it is a generic engine that walks one ordered, flat list of **producers**, each atomic (one mechanical action or one LLM session), with a two-part **Input/Output** contract where each part is a *pointer* to a format-contract file, never a restated copy.
Review is never a property of the producer it reviews — it is always the next, separate producer in the list.
`Finalize` is an ordinary producer both products name in their own list, and Raddle-regeneration folds into `Finalize`'s own contract rather than being a step of its own.

**Why now:** the model is settled, but the documentation set that describes it is only *partly* converted, and the partial conversion introduced real contradictions rather than merely leaving stale text.
This task is a **scoping pass**, not the rewrite: survey the affected docs, reconcile them against the landed model, and emit the actual ordered set of buildable follow-up wiki tasks with `depends_on` wired.

**Critical finding that reframes the task.**
Two commits landed immediately before this worktree was spawned and already did much of what the task body assumes is outstanding:

- `256b8262` — rewrote `manifest/designs/shed.md` for the flat producer-model, retiring the two-slot description.
- `51eb180c` — rewrote `manifest/designs/loom.md`'s phase-machine section into a full 11-row producer table with Input/Output columns, the pointer rule, and the Raddle-into-Finalize fold.
- `manifest/roadmap.md`'s Planned `Shed` item (lines 24–33) is likewise already on the revised model.

So the remaining work is residue plus contradictions, not three full rewrites.
The user's ruling: **those three rewritten files are the newest and the decided state** — everything else reconciles *to* them, never the reverse.

## Scope

**In:**

- Produce the ordered set of follow-up wiki tasks (six, below), with bodies carrying their own full reasoning trail and `depends_on` wired per the dependency graph in [Decision: follow-up-task-set](#follow-up-task-set).
- Create those tasks in the wiki via `wiki._client.upsert_task`.
- Commit a short summary/rationale doc in this worktree recording the ordering reasoning.
- Surface — not silently resolve — the genuine open questions listed under [Surfaced open questions](#surfaced-open-questions).

**Out:**

- Writing the rewritten `shed.md` / `loom.md` / format docs themselves — that is the follow-up work this task's output *describes*.
- Deleting `internal/builderengine` / `internal/buildercli`, performing the `plan-format-v3` rename sweep, or moving the `batcher:` config key — all are follow-up tasks (A, B, F), not this task's own edits.
- `Hardener` / `Tenter`'s equivalent Raddle-fold-into-Finalize — deferred by the landed design, noted but not designed.
- Any behavioral code change in this worktree.

Note: "no code changes" constrains **this** task only.
The follow-up set it emits explicitly may — and does — include code tasks.

## Decisions

### deliverable-is-reconcile-the-residue

- Decision: treat `shed.md`, `loom.md`, and `roadmap.md` as landed and authoritative; scope follow-ups only for the residue and the contradictions they left or introduced.
- Rationale: they were rewritten immediately before this worktree spawned, on the same model this task was created to apply. Re-scoping a second full rewrite would discard the newest decided state.
- Rejected: re-scoping a full `shed.md` rewrite (treats yesterday's decided work as a draft); emitting a task list with no verdict on what already landed (leaves the next session to re-derive the same finding).

### batchifier-splits-out-of-webster

- Decision: `Batchifier` **is** a `Shed`-level producer, position 8 in `loom`'s list, between `Plan-Review` and `Webster`. Batching is no longer webster-internal execution policy.
- Rationale: `loom.md`'s rewritten producer table (row 56) is the decided state, and the flat producer list is precisely the mechanism that lets a mechanical, zero-LLM step like batching be named and resumed like any other producer.
- Rejected: dropping `Batchifier` from the list to preserve the shipped `internal/batcher` framing (would subordinate the newest decision to older doc text); surfacing it as unresolved (the user resolved it).
- **Consequence, deliberately accepted:** this contradicts shipped artifacts — `internal/batcher/doc.go`'s package comment ("100% webster's own execution-policy decision... never an LLM's decision"), `CONSTRAINTS.md`'s **Batcher Registry+Config Invariant**, and `plan-format-v3.md`'s "Batch is gone / the card is the unit" section. Follow-up task **F** cleans up all of it.

### batcher-config-key-moves-to-loom-yaml

- Decision: the `batcher:` config key moves from `webster.yaml` to `loom.yaml`.
- Rationale: `loom.yaml` is where a product declares **which producers its list contains**; which batchifier that list's `Batchifier` producer uses belongs in the same place. Matches `finalize.md:41`'s existing precedent — "per-phase profiles live in loom, not perch.yaml."
- Rejected: leaving it in `webster.yaml` (webster is now merely the *consumer* of the batches, not the decider); a standalone `batcher.yaml` (a producer definition's config does not warrant its own file).

### discussion-stays-two-files-with-current-names

- Decision: `_lyx/discussion/` stays a two-file directory — `decision-record.md` (the Plan producer's **sole** input) and `support-log.md` (read only by the Discussion-review gate). **No rename** of `decision-record.md`.
- Rationale: `discussion-format.md:15` states the filenames are "self-describing rather than terse" *on purpose*, and `decision-record.md` pairs with its sibling `support-log.md`. `decisions.md` would break that pairing and would be actively misleading: the file holds seven sections (Goal, Scope, Decisions, Constraints, Auto-mode assumptions, Open risks, Acceptance criteria), so naming the whole file after one of its own sections misrepresents it.
- Rejected: `decisions.md`; `decision.md` (singular) — both terser, both lose the sibling parallelism, and both would force a code sweep (`DiscussionDecisionRecord` in `internal/loomengine/config.go`, `discussionpath_test.go`, `discussion_test.go`, `prompttemplate.go`) for no contract gain.

### loom-table-names-real-artifacts

- Decision: `loom.md`'s producer table is scoped-edited to name the artifacts that actually exist — `_lyx/discussion/decision-record.md` (not `discussion.md`) and the `_lyx/plan/` directory (not `plan.md`) — and the two-file access boundary becomes part of the `Plan-Write` Input pointer.
- Rationale: the table currently names two artifacts that do not exist anywhere in the pinned contracts. `discussion-format.md` pins a directory of two files; `loom.md:188` itself says the Planner writes `_lyx/plan/NN-<card>.md` per card plus `00-overview.md` as the done-sentinel. A producer table whose Input/Output pointers name nonexistent files defeats the pointer rule it is meant to demonstrate.
- Rejected: deferring to the format-docs task without a verdict; surfacing as an open question (there is nothing open — the real paths are already pinned).

### pointer-rule-becomes-a-short-constraints-invariant

- Decision: add a new, **short** `CONSTRAINTS.md` invariant naming the pointer rule as a review obligation. Enforced by review, not machine-checked.
- Rationale: matches the file's existing seam-invariant precedent (Treadle Runner-Seam, Scout Engine-Seam, Shuttle Provider-Seam, Batcher Registry+Config), and the risk the rule guards against — drift via well-intentioned duplication of a format-contract's content into a producer's instruction file — is exactly the kind reviewers must be told to check for **by name**.
- Rejected: keeping it in `shed.md` only (no reviewer reads a design doc as a checklist); doing both (redundant).
- **Explicit constraint on the entry itself:** it must be short. One invariant statement plus an "Enforced by: review obligation" line, in the same shape as the existing entries. Not a treatise.

### finalize-shared-by-reference

- Decision: `Finalize` is shared **by reference** — both `loom`'s and `Hardener`'s lists name the same producer definition. Fix `shed.md:18`, which says "by *value*".
- Rationale: two of three sources already say "by reference" (`loom.md:59`, `roadmap.md:26`), and it is the phrasing that carries the actual meaning — one definition, named twice, never copied.
- Rejected: "by value" (would require changing two sources to match one); dropping the metaphor entirely.

### preflight-finalize-thin-output-is-permitted

- Decision: resolve `loom.md:75`'s open question — the Output contract **permits a pass/fail gate signal with no artifact**. Say so explicitly in `shed.md`'s producer-contract section.
- Rationale: `Preflight` and `Finalize` genuinely have no output artifact, and forcing one solely to satisfy a uniform contract would be ceremony. The resume-on-output-files rule degrades gracefully: a producer with no artifact simply re-runs on resume, which is correct for both — `Preflight` is a cheap idempotent validation, and `Finalize`'s merge is idempotent against an already-merged parent.
- Rejected: surfacing it unresolved (the user resolved it); requiring every producer to emit an artifact (`Preflight` would write a result file purely to be skippable — ceremony for a validation that costs nothing to repeat).

### discussion-review-gate-exists

- Decision: the `Discussion` side is **not** inherently asymmetric — scope a `Discussion-Review-Gate` mechanical producer, mirroring `Plan-Review-Gate`, running exactly the three checks `discussion-format.md:80–83` already specifies.
- Rationale: those three checks are already written down and already mechanical — both files exist under `_lyx/discussion/`; `decision-record.md` has all seven required sections present; the Plan-never-reads-`support-log` boundary holds (the Plan producer's declared input set never names `support-log.md`). No new design is needed, only naming them as a producer. The third check in particular is a structural guard worth failing hard on, since violating it silently destroys the access boundary the two-file split exists to create.
- Rejected: surfacing as open; confirming the asymmetry and dropping the gate (would leave three already-specified mechanical checks with no producer to run them).

### builder-is-deleted-contract-doc-is-retired-not-code

- Decision: delete `internal/builderengine` and `internal/buildercli` outright. Keep `docs/reference/builder-contract.md`, re-statused as a **retired-design reference**. Then delete `docs/reference/plan-format.md` (v2), freeing that filename.
- Rationale: `builder` is not dormant — `cmd/lyx/main.go:107` registers `buildercli.Command()`, and it appears in `cmd/lyx/helptree_test.go`'s module list and `cmd/lyx/notransients_test.go`. Parking it in-tree therefore costs real, recurring maintenance: it stays in the CLI help tree per the **CLI/Cobra Invariant**, keeps a second plan parser alive against the **Planparser Sole-Parser Invariant**, and every future refactor must carry it. The genuinely reusable asset is the *design* — the recovery ladder, chain rollback, the `mutate.lock` state-mutation lease, the three fabric-commit points, crash/resume semantics — and that already lives in `builder-contract.md`'s 247 lines, rewritable onto the flat card list later. The implementation itself is one `git show` away, permanently.
- Rejected: keeping the code frozen in-tree (pays help-tree, test, and refactor cost indefinitely for a reference binary); moving it to `sandbox/` or an `attic/` (invents an excluded-directory convention this repo does not have).
- `roadmap.md:196` already calls `builder` "superseded as an active plan-format consumer" and `:202` says it "becomes obsolete" — nobody had removed it.

### webster-sibling-section-moves-to-websterengine

- Decision: before `builder-contract.md` is retired, extract its `## Webster: the fork-based sibling` section (line 222) into `internal/websterengine`'s package documentation.
- Rationale: that package is already the authoritative home for webster's internals, and the extraction resolves the gap `loom.md:94` explicitly assigns to this task — "`builder-contract.md` documents the older Builder/plan-v2 pairing and does not yet have a Webster/plan-v3 equivalent." It also fixes `finalize.md:36`, which currently deep-links `builder-contract.md#webster-the-fork-based-sibling` for the PR-body summary artifact.
- Rejected: a new `docs/reference/webster-contract.md` (duplicates what the package doc owns); leaving live webster content inside a retired doc.

### plan-format-v3-renamed-to-plan-format-mechanically

- Decision: rename `docs/reference/plan-format-v3.md` → `docs/reference/plan-format.md`. It is the current plan format;
  the "v3" suffix goes. Sweep **all** references — docs and Go alike — in one task, performed **mechanically, by script**.
- Rationale: v3 is the only live format once `builder` is gone, and a version suffix on the sole format is exactly the kind of stale guard `discussion-format.md` already argues against (see its `no-schema-version` reference to `status-schema.md`). A half-done rename is worse than either end state: `planparser`/`websterengine` identifiers and template prose must move with the filename.
- Rejected: docs-only rename with Go identifiers deferred (leaves the codebase mid-rename); renaming the file but leaving in-text "v3" as a historical label (the suffix is what is being retired).
- **Scale:** 30 files reference `plan-format-v3`, including `internal/planparser` (7 files), `internal/websterengine` (4), `internal/webstercli` (3), `internal/loomengine` (3), `internal/batcher/doc.go`, `README.md`, `docs/overview.md`, `docs/reference/model-spec.md`, `docs/reference/builder-contract.md`, `manifest/roadmap.md`, and four `manifest/designs/*.md`. `CONSTRAINTS.md`'s Planparser Sole-Parser Invariant is also affected.
- **Execution discipline:** scripted find/replace plus a full `go test ./...`, not a hand-edit pass. Per the repo's own tooling rules the script must not use `sed`.

### follow-up-task-set

- Decision: six follow-up wiki tasks, split by file-cluster, with `depends_on` wired strictly only where a real dependency exists and left parallel otherwise.
- Rationale: a strict linear chain would serialize two clusters (D and E) that touch disjoint files and have no ordering need; fully parallel would collide on `plan-format.md`'s filename and on `finalize.md:36`'s link target.
- Rejected: one consolidated reconciliation task (too large, and mixes mechanical sweeps with genuine design edits); a single linear chain; all-parallel.

The set, with the reason each exists:

| ID | Task | Kind | `depends_on` |
|----|------|------|--------------|
| A | `builder-retire` | code | — |
| B | `plan-format-drop-v3-suffix` | code (mechanical) | A |
| C | `format-docs-name-producers` | docs | B |
| D | `raddle-finalize-fold-and-link-repair` | docs | A |
| E | `shed-model-contradiction-sweep` | docs | — |
| F | `batchifier-split-cleanup` | code + docs | B |

**A — `builder-retire`.**
Delete `internal/builderengine` and `internal/buildercli`; unregister `buildercli.Command()` from `cmd/lyx/main.go:107`; remove `builder` from `cmd/lyx/main.go:75`'s module list, `cmd/lyx/helptree_test.go` (lines 28, 106–107), and `cmd/lyx/notransients_test.go` (lines 21, 57–58).
Extract `builder-contract.md`'s `## Webster: the fork-based sibling` section into `internal/websterengine`'s package doc and re-point `finalize.md:36`.
Re-status `builder-contract.md` as a retired-design reference.
Delete `docs/reference/plan-format.md` (v2).
Update `docs/overview.md`'s module table and `manifest/roadmap.md`.
No `depends_on` — nothing blocks it.

**B — `plan-format-drop-v3-suffix`.**
Rename `plan-format-v3.md` → `plan-format.md` and sweep all 30 referencing files by script.
`depends_on: A` — the filename is not free until v2's doc is deleted.

**C — `format-docs-name-producers`.**
Rewrite `discussion-format.md` and the renamed `plan-format.md` to name their producers and contracts explicitly in producer-model terms.
Add the `Discussion-Review-Gate` producer covering `discussion-format.md:80–83`'s three mechanical checks.
Scoped-edit `loom.md`'s table rows 2–7 to name `_lyx/discussion/decision-record.md` and `_lyx/plan/`, and insert `Discussion-Review-Gate` into the list.
`depends_on: B` — it edits the renamed file.

**D — `raddle-finalize-fold-and-link-repair`.**
Fold Raddle into `finalize.md`'s own contract as a first-class part of the merge, not a Related-section mention.
Remove `raddle.md`'s superseded "reserved phase slot between Builder and Finalize" text (lines 3, 85) and close its explicitly-open question (line 54) — the fold is decided.
Fix `finalize.md:3`'s verbatim two-slot text ("not a swappable per-instance slot the way Preflight and the producer are").
Fix `finalize.md:11` and `:52`, which link `fabric.md` — **that file does not exist** in `manifest/designs/`.
Fix the dead `loom.md#the-phase-machine` anchor (renamed to `#the-phase-machine--a-flat-producer-list-no-predefined-slots`) in `raddle.md:3`, `raddle.md:54`, and `self-report.md:30`.
Fix `roadmap.md:68`'s "deferred phase slot between Builder and Finalize".
`depends_on: A` — `finalize.md:36`'s link target moves in A.

**E — `shed-model-contradiction-sweep`.**
Fix `shed.md:7` and `:19`, which say "superseding ... **below**" and "the pre-revision text **below**" — that text was deleted in `256b8262`, so both references dangle.
Fix `shed.md:18` to "by reference".
Fix `loom.md:15–17`'s naming note, which still says "`loom` = `Shed` + loom's own Preflight + the Discussion/Plan/Webster producer" — old slot framing, contradicting the table 25 lines below it, and its "This doc has not been rewritten to extract `Shed` explicitly" claim is now false.
Fix `hardener.md:17`'s "producer-slot".
Fix `docs/overview.md:272`'s stale chain "Preflight → Discussion → Plan → Builder → Raddle → Finalize".
Resolve `loom.md:75`'s thin-Output question per the decision above and record it in `shed.md`'s contract section.
Add the short `CONSTRAINTS.md` pointer-rule invariant.
Record the surfaced open questions below wherever each belongs.
No `depends_on` — touches files disjoint from D.

**F — `batchifier-split-cleanup`.**
Amend `internal/batcher/doc.go`'s package comment to state that `Batchifier` is a `Shed`-level producer whose output webster consumes, not webster's own internal policy.
Amend `CONSTRAINTS.md`'s **Batcher Registry+Config Invariant** for the same reason and for the config-key move.
Amend the renamed `plan-format.md`'s "Batch is gone / the card is the unit" section — the card stays the plan's unit (unchanged), but the "entirely internal to webster" framing goes.
Move the `batcher:` config key from `webster.yaml` to `loom.yaml`, updating `websterengine`'s config loading and `loomengine`'s config accordingly.
`depends_on: B` — it edits the renamed file.

### run-mill-plan-and-mill-go

- Decision: this task proceeds through `/mill-plan` and `/mill-go` normally rather than authoring the wiki tasks inline in the `mill-start` session.
- Rationale: the deliverable is six follow-up task bodies that will each be executed autonomously later, by sessions with zero conversation history, from the body text alone. That authoring is the bulk of the work and benefits more from a review pass than typical code does. `mill-plan` is cheap even where the plan is thin.
- Rejected: skipping both and authoring the tasks inline (no review pass over bodies that must stand alone); skipping `mill-plan` only.

## Surfaced open questions

Per the task body's item 4, these are surfaced deliberately and **not** resolved here.
They belong in task **E**'s output as named open questions, and in the relevant follow-up task bodies.

1. **`Webster` violates the producer-atomicity rule.**
   The landed model states a producer is *always* atomic — one mechanical action, or one LLM session, never an internal multi-step process of its own.
   But `loom.md:57` lists `Webster` as `black box (LLM + mechanical internally)` with "its own per-batch fork loop... opaque to `loom`'s flat list."
   That is precisely an internal multi-step process.
   Either atomicity admits a carve-out for black-box producers that own their own loop, or `Webster` must decompose into flat producers the way `Plan` did.
   This is the single largest unresolved tension in the model and it should be decided before `Shed` is built, not during.
2. **`Discussion-Write` has no Input.**
   `loom.md:50` records its Input as "— (starting point)".
   The thin-Output carve-out is now decided for `Preflight`/`Finalize`; the symmetric thin-*Input* case has not been.
   The task body itself is arguably the Input, which would make the pointer target the wiki task record rather than a format-contract file — a different kind of pointer than every other row in the table.
3. **`shed` is an overloaded name in this repo.**
   `docs/overview.md:289` and `:318` record that earlier `reed` drafts split the model and view into separate modules named `shed` and `glance`.
   The historical mentions are harmless in isolation, but "shed" now names the outer phase-FSM, and a reader hitting `overview.md:289` first will mis-resolve it.
   Worth an explicit disambiguating note rather than leaving two unrelated `shed`s in one doc set.
4. **`Hardener` / `Tenter`'s equivalent Raddle-into-Finalize fold** is deferred by the landed design (`shed.md:20`, `loom.md:67`) and stays deferred — noted here only so a future pass does not mistake the silence for an oversight.

## Technical context

- **Repo layout:** design docs in `manifest/designs/`, pinned contracts in `docs/reference/`, the roadmap in `manifest/roadmap.md`, cross-cutting invariants in `CONSTRAINTS.md`, the module table and execution stack in `docs/overview.md`. No `_codeguide/` in this repo.
- **`internal/batcher`** ships today: `batcher.go` (the `Batcher` interface), `registry.go` (name-keyed registry + `Select`), `identity.go` (the one-card-one-batch default), plus tests. Task F changes **only** its package doc and the config-key location — the interface, registry, and `Select` are untouched, because the caller that would move is `Shed`, which does not exist yet.
- **`internal/loomengine`** holds the already-built Discussion and Planner producers (`discussion-template.md`/`prompt.go`/`discussion.go`, `plan-template.md`/`plantemplate.go`/`plan.go`) plus the `DiscussionDir` / `DiscussionDecisionRecord` / `DiscussionSupportLog` path accessors in `config.go`. `Preflight` is built here too, engine-only, no cobra module.
- **`internal/planparser`** is the sole plan parser (see the Planparser Sole-Parser Invariant); it is the largest single cluster in task B's rename sweep.
- **Existing precedent for the shared-engine seam** `Shed` will need: `internal/treadleengine`'s `RoundRunner` and `internal/batcher`'s `Batcher` — both already documented as `Shed`'s reference pattern in `shed.md:36–37`.
- **Grounding for mechanical producers like `Plan-Sweep`:** `docs/benchmarks/scout-vs-grep.md` found no reliable win from giving an LLM the *option* to call `lyx scout`, and found a trust-marker gap. `manifest/designs/scout-plan-symbol-fields.md` recommends the mechanical, code-driven integration on that evidence, and `manifest/designs/review-finding-classification.md` independently reaches the same conclusion for call-site/reference completeness. Wiki task `fabric-host-to-warp-rename` already applies the pattern.
- **`review-finding-classification.md` item 5** carries a rule the follow-up tasks must honor: a "what NOT to look for" instruction must be written symmetrically into **both** the producer's own format-contract and the reviewing producer's rubric. Writing it into only one side recreates the non-convergent review loop that motivated the doc.

## Constraints

From `CONSTRAINTS.md` (authoritative; read before writing or reviewing):

- **CLI / Cobra Invariant** — module `Command()`/`RunCLI` seam, `Short` on every command, help-tree tests. Task A must remove `builder` from the help tree cleanly, not orphan it.
- **Planparser Sole-Parser Invariant** — one plan parser. Task A's deletion of `builderengine` is what finally makes this literally true; task B updates the invariant's wording for the renamed format.
- **Batcher Registry+Config Invariant** — currently states "webster's execution unit is the batchifier-derived batch" and pins the `batcher:` key to `webster.yaml`. Task F amends it.
- **Documentation Lifecycle** — governs when a design doc folds into a package doc and is deleted. Task A's retirement of `builder-contract.md` and extraction into `websterengine`'s package doc must follow it.
- **Cwd Resolution Invariant** — `internal/lyxcwd` alone resolves cwd; each module owns its own relative subpath. Relevant to task F if the config-key move touches path resolution.
- **New invariant to add (task E):** the pointer rule — an instruction file must never duplicate or paraphrase another producer's format-contract content, only point at it. Keep the entry short.

From `CLAUDE.md`:

- Docs land in the **same commit** as the change. Every task in this set is doc-touching by construction, so this is not an afterthought for any of them.
- `manifest/roadmap.md` moves only on completing or adding a planned item — tasks A and F do complete/alter planned items and may touch it; D and E touch it only for stale-text repair, which is legitimate.
- Markdown uses semantic line breaks, never fixed-column hard-wrap.
- No `sed` — task B's rename script must use another mechanism.

## Testing

This task itself produces wiki tasks and a rationale doc, so it has no test surface of its own.
The follow-up tasks do, and their bodies should carry these expectations:

- **A (`builder-retire`)** — the real test is the existing suite. `go build ./...` and `go test ./...` must pass with `builderengine`/`buildercli` gone; `cmd/lyx/helptree_test.go` is the specific guard that fails loudly if `builder` is half-removed, and `cmd/lyx/notransients_test.go` is the second. No new tests needed — this is a deletion whose correctness is exactly "nothing else referenced it."
- **B (`plan-format-drop-v3-suffix`)** — scripted sweep followed by a full `go test ./...`. The meaningful scenario is *incompleteness*: a grep for `plan-format-v3` / `plan_format_v3` / `plan-format v3` returning zero hits across the repo is the acceptance check, run as a step in the task, not an assertion in a test file. `internal/planparser`'s existing tests and `internal/webstercli/cli_test.go` cover behavior preservation.
- **C (`format-docs-name-producers`)** — docs-only, no test surface. The `Discussion-Review-Gate`'s three checks are *specified* here, not implemented; implementation lands with `Shed`.
- **D and E** — docs-only. The one mechanical check worth running: every relative markdown link and anchor introduced or touched resolves. A link-check pass over `manifest/` and `docs/` is the acceptance criterion, and it is what would have caught the dead `fabric.md` links and the dead `#the-phase-machine` anchors in the first place.
- **F (`batchifier-split-cleanup`)** — the config-key move is the only behavioral change. TDD candidate: a config-loading test asserting `batcher:` resolves from `loom.yaml` and that a `batcher:` key left in `webster.yaml` is no longer consulted. `internal/batcher`'s existing registry/`Select` tests must continue passing untouched, which is the evidence that only the caller moved.

## Q&A log

- **Q:** Given `shed.md`/`loom.md`/`roadmap.md` were already rewritten in the two commits before spawn, what is this task's actual deliverable? **A:** Reconcile the residue and emit the task list; treat the three rewritten files as landed.
- **Q:** `Batchifier` is listed as a `Shed` producer, but `internal/batcher`'s package doc and `plan-format-v3.md` both say batching is entirely webster-internal. How is that resolved? **A:** The docs rewritten yesterday are the newest and decided — we have decided to **split `Batchifier` out so it is no longer part of Webster**. Amend `batcher/doc.go`, `CONSTRAINTS.md`, and the format doc to match.
- **Q:** `loom.md`'s table says `discussion.md` and `plan.md`, but neither exists — the pinned contracts are a two-file `_lyx/discussion/` directory and a `_lyx/plan/` card directory. Fix the table? **A:** Yes, two-file format for discussion, with only `decision-record.md` read by the Plan producer.
- **Q:** Should `decision-record.md` be renamed to `decisions.md`? **A:** No — keep `decision-record.md`. It pairs with `support-log.md`, the filenames are deliberately self-describing rather than terse, and the file holds seven sections, so naming it after one of them would mislead.
- **Q:** Where does the pointer rule become a named review obligation? **A:** A new `CONSTRAINTS.md` invariant — but keep the entry **short**. One constraint, not a treatise.
- **Q:** With `Batchifier` split out, where does the `batcher:` config key live? **A:** `loom.yaml` — that is where you declare which producers a given `loom` setup contains, so which batchifier belongs there too.
- **Q:** How far does the cleanup of the shipped `internal/batcher` docs reach? **A:** One task must clean up **all** of it — package doc, `CONSTRAINTS.md` invariant, format doc, and the config-key move together.
- **Q:** `Preflight`/`Finalize` have no real Output artifact. Surface as open, or resolve? **A:** Resolve now — the Output contract permits a pass/fail signal with no artifact.
- **Q:** `Discussion` has no mechanical pre-gate the way `Plan-Review-Gate` does. **A:** Resolve now — scope a `Discussion-Review-Gate` producer running the three checks `discussion-format.md:80–83` already specifies.
- **Q:** `plan-format-v3.md` is the current format — drop the "v3"? **A:** Yes, rename it to `plan-format.md`; the existing v2 `plan-format.md` is outdated and gets deleted.
- **Q:** `builder` still ships and still parses v2, so deleting v2's doc orphans live code. Delete `builder`, or park it as a reference that could later be rewritten onto plan-format v3? **A:** Delete the code, keep `builder-contract.md` re-statused as a retired-design reference — that is where the reusable design actually lives, and git history holds the implementation.
- **Q:** Where does `builder-contract.md`'s "Webster: the fork-based sibling" section go? **A:** Extract into `internal/websterengine`'s package doc before retiring the doc.
- **Q:** Scope of the `plan-format-v3` → `plan-format` rename sweep? **A:** One task covering all 30 files, docs and code together — and it is a **mechanical** sweep, to be done with a script.
- **Q:** Do we need `mill-plan` and `mill-go` at all, given no code is written and the follow-up tasks each get their own full mill cycle? **A:** Yes, run both — the six task bodies must stand alone for autonomous sessions, so the review pass is worth it.
- **Q:** This task's body says "no code changes"; may the follow-up set include code tasks? **A:** Yes — that constrains this task, not its output.
- **Q:** Ordering of the follow-up tasks? **A:** Strict chain only where a real dependency exists, parallel otherwise.
