# Batch: docs-task-bodies

```yaml
task: "Scope the Shed producer-model rewrite into buildable tasks"
batch: "docs-task-bodies"
number: 2
cards: 3
verify: null
depends-on: []
```

## Batch Scope

This batch authors the three docs-only follow-up task bodies — C (`format-docs-name-producers`), D (`raddle-finalize-fold-and-link-repair`), and E (`shed-model-contradiction-sweep`) — as staged files under `_mill/followup/`.
It is one batch because the three share a single ownership map that has to be held whole: C and E both edit loom.md in chain order, D is the one task deliberately kept parallel, and E is the terminal task that carries every surfaced open question.
Splitting them would mean re-deriving that map twice.
The external interface batch 3 consumes is the staged-file format fixed in `## Shared Decisions`.
No batch-local decisions differ from the overview.

## Cards

### Card 4: Body for task C — format-docs-name-producers

- **Context:**
  - `_mill/discussion.md`
  - `_mill/plan/00-overview.md`
- **Edits:** none
- **Creates:**
  - `_mill/followup/C-format-docs-name-producers.md`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Write the staged file for follow-up task C.
  The header carries `slug: format-docs-name-producers`, a double-quoted `title` of "format docs: name their producers and contracts in producer-model terms, add Discussion-Review-Gate", `depends_on: ["plan-format-drop-v3-suffix"]`, and a one-paragraph `brief` naming the two format docs, the new gate producer, and the scoped loom.md table edit.
  `## Why` explains that discussion-format.md and the renamed plan-format.md are the two pinned contracts the flat producer model points at, and that both still describe themselves in pre-producer terms — so the pointer rule they are meant to anchor has nothing coherent to point at.
  Include the Decision `loom-table-names-real-artifacts` from `_mill/discussion.md`: loom.md's producer table currently names two artifacts that exist nowhere in the pinned contracts, and a producer table whose Input/Output pointers name nonexistent files defeats the pointer rule it is meant to demonstrate.
  `## What needs to happen` is a numbered list covering, at minimum, these six items, each transcribed from the **C — `format-docs-name-producers`** subsection of `### follow-up-task-set`:
  rewrite discussion-format.md and the renamed plan-format.md to name their producers and contracts explicitly in producer-model terms;
  add the Discussion-Review-Gate producer covering checks 1–2 of discussion-format.md:80–82;
  scoped-edit loom.md's table rows 2–7 to name the real artifacts and insert Discussion-Review-Gate into the list;
  fix discussion-format.md:1's own title, which still reads "the `discussion.md` ↔ Plan contract" — the same nonexistent artifact the loom table named;
  restate discussion-format.md:14 in producer-model terms, since it currently grounds the two-file split in Builder's "distilled digest, never raw prose" rule and cites builder-contract.md's digest contract, i.e. a live contract justified by a retired design doc — the rule itself is sound and stays, only its attribution is rewritten;
  and rewrite plan-format.md:5's "Coexistence, not replacement" section, which asserts the format does not retire v2 and is false once A deletes v2, so the renamed file would otherwise carry the claim forward about itself.
  The Discussion-Review-Gate item needs its own subsection carrying the whole Decision `discussion-review-gate-exists`: the gate runs checks 1–2 — both files exist under `_lyx/discussion/`, and decision-record.md has all seven required sections present (Goal, Scope, Decisions, Constraints, Auto-mode assumptions, Open risks, Acceptance criteria) — because both are per-run, artifact-observable properties of exactly the kind Plan-Review-Gate already hard-fails on.
  State explicitly, and as its own paragraph, that check 3 at discussion-format.md:83 is **not** a gate check: the claim that the Plan producer's declared input set never names support-log.md is a property of the producer *definition*, not of any run's artifacts, so there is nothing per-run for a gate to evaluate.
  It becomes a build-time test assertion over the producer definition instead — a static property caught once and forever by a compile- or test-time guard rather than re-evaluated on every run — and the body must say this in so many words, so nobody re-files it later as a missing gate check.
  Record the Decision `discussion-stays-two-files-with-current-names` as a settled non-change: `_lyx/discussion/` stays a two-file directory, decision-record.md is the Plan producer's sole input, support-log.md is read only by the Discussion-review gate, and decision-record.md is **not** renamed.
  Carry its reasoning — discussion-format.md:16 states the filenames are self-describing on purpose, decision-record.md pairs with support-log.md, and the file holds seven sections so naming it after one of them would mislead — and its rejected alternatives (`decisions.md`, `decision.md`), including that either would force a code sweep across `DiscussionDecisionRecord` in internal/loomengine/config.go, discussionpath_test.go, discussion_test.go, and prompttemplate.go for no contract gain.
  Note that `_mill/discussion.md` cites this sentence as discussion-format.md:15;
  the self-describing-filenames sentence is at :16, and :15 is the preceding sentence about the filesystem boundary.
  Use :16 in the body — do not propagate the off-by-one into the wiki page.
  Add the rule from the Technical context section that C must honour when writing the gate's rubric: per review-finding-classification.md item 5, a "what NOT to look for" instruction is written symmetrically into **both** the producer's own format-contract and the reviewing producer's rubric, because writing it into only one side recreates the non-convergent review loop that doc exists to prevent.
  `## Scope` states the loom.md boundary precisely: C owns the producer table's rows 2–7 only — the artifact-name fixes and the Discussion-Review-Gate insertion — and E owns everything else in the file and runs after both C and F.
  `## Sequencing` records `depends_on: plan-format-drop-v3-suffix`, because C edits the renamed file, and records that E depends on C so E writes loom.md's finished state rather than guessing at it.
  `## Acceptance` transcribes C's bullet from `_mill/discussion.md`'s `## Testing` section: docs-only, no test surface of its own;
  the Discussion-Review-Gate's checks are specified here, not implemented, and implementation lands with Shed;
  check 3's build-time assertion is likewise specified rather than written, since the producer definition it would assert over does not exist yet.
  Add the link-and-anchor check the discussion sets for the docs tasks: every relative markdown link and anchor introduced or touched must resolve.
- **Commit:** `scoping: stage follow-up task body C (format-docs-name-producers)`

### Card 5: Body for task D — raddle-finalize-fold-and-link-repair

- **Context:**
  - `_mill/discussion.md`
  - `_mill/plan/00-overview.md`
- **Edits:** none
- **Creates:**
  - `_mill/followup/D-raddle-finalize-fold-and-link-repair.md`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Write the staged file for follow-up task D.
  The header carries `slug: raddle-finalize-fold-and-link-repair`, a double-quoted `title` of "finalize: fold Raddle into its own contract and repair the dead links in raddle.md, finalize.md and self-report.md", `depends_on: ["builder-retire"]`, and a one-paragraph `brief` naming the fold, the dead-link repairs, and the fact that D is the one follow-up that stays parallel.
  `## Why` states that the landed model folds Raddle-regeneration into Finalize's own contract rather than keeping it as a step of its own, so finalize.md and raddle.md still describe a machine that no longer exists — and that the same files carry three separate dead references, which is what makes this a repair task rather than a prose pass.
  `## What needs to happen` is a numbered list transcribing the **D — `raddle-finalize-fold-and-link-repair`** subsection of `### follow-up-task-set`:
  fold Raddle into finalize.md's own contract as a first-class part of the merge, not a Related-section mention;
  remove raddle.md's superseded "reserved phase slot between Builder and Finalize" text at lines 3 and 85, and close its explicitly-open question at line 54, since the fold is decided;
  fix finalize.md:3's verbatim two-slot text ("not a swappable per-instance slot the way Preflight and the producer are");
  fix finalize.md:11 and :52, which link fabric.md — a file that does not exist in manifest/designs/;
  fix the dead loom.md#the-phase-machine anchor, renamed to #the-phase-machine--a-flat-producer-list-no-predefined-slots, in raddle.md:3, raddle.md:54, and self-report.md:30;
  and fix finalize.md:26, which cites a "Weft Git Invariant" in CONSTRAINTS.md that does not exist — the real entry is the Fabric Git Invariant (warp + weft) at CONSTRAINTS.md:173.
  State as its own paragraph that D re-reads finalize.md end to end rather than working the fixed line list above, because the line numbers are a starting inventory and not a bound.
  Name the known additional residue the discussion already found: :45–46 still calls Finalize "Shed's literally-shared code ... both share this exact code", which is the retired shared-code framing;
  :48 asserts "Shed hasn't been extracted from it yet (see that doc's own naming note)", which is false once E fixes loom.md:15–17;
  and :9 references Builder's escalation behavior, which A retires.
  Include the Decision `finalize-shared-by-reference` as the framing the fold is written against: Finalize is shared **by reference** — both loom's and Hardener's lists name the same producer definition, one definition named twice and never copied.
  Note that shed.md:18's "by value" wording is E's to fix, not D's, so the two tasks do not both edit shed.md.
  `## Scope` states what D deliberately does not touch: manifest/roadmap.md.
  Carry the reason — roadmap.md:68's "deferred phase slot between Builder and Finalize" is real residue, but roadmap.md is edited by A and E too, so scoping it to D would recreate exactly the shared-file collision that forced E to be serialized;
  it moves to E, roadmap.md's last owner.
  State that D owns finalize.md, raddle.md, and self-report.md, and that no other task in the set touches any of them, which is what makes D genuinely parallel rather than parallel-by-assertion.
  Also record the deferral from the discussion's `## Surfaced open questions` item 4, so a future reader does not mistake the silence for an oversight: Hardener and Tenter's equivalent Raddle-into-Finalize fold is deferred by the landed design at shed.md:20 and loom.md:67, and stays deferred.
  `## Sequencing` records `depends_on: builder-retire`, because finalize.md:36 and :50's link targets move in A, and states that D branches off A in parallel with the B → {C, F} → E chain.
  `## Acceptance` transcribes the D and E bullet from `_mill/discussion.md`'s `## Testing` section: docs-only, and the one mechanical check worth running is that every relative markdown link and anchor introduced or touched resolves — a link-check pass over manifest/ and docs/ is the acceptance criterion, and it is exactly what would have caught the dead fabric.md links, the dead phase-machine anchors, and the non-existent Weft Git Invariant citation before they shipped.
- **Commit:** `scoping: stage follow-up task body D (raddle-finalize-fold-and-link-repair)`

### Card 6: Body for task E — shed-model-contradiction-sweep

- **Context:**
  - `_mill/discussion.md`
  - `_mill/plan/00-overview.md`
- **Edits:** none
- **Creates:**
  - `_mill/followup/E-shed-model-contradiction-sweep.md`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Write the staged file for follow-up task E.
  The header carries `slug: shed-model-contradiction-sweep`, a double-quoted `title` of "shed: sweep the remaining producer-model contradictions and add the pointer-rule invariant", `depends_on: ["format-docs-name-producers", "batcher-standalone-split"]`, and a one-paragraph `brief` naming the contradiction sweep, E's role as loom.md's and roadmap.md's final owner, and the new CONSTRAINTS.md invariant.
  `## Why` states that shed.md, loom.md, and roadmap.md are the landed, authoritative statement of the model — transcribe the Decision `deliverable-is-reconcile-the-residue`, including that they were rewritten immediately before this scoping worktree spawned, on the same model, and that everything else reconciles *to* them and never the reverse.
  E exists because the partial conversion left contradictions inside those same three files, plus a set of claims that only become false once A through F have landed, so E runs last and writes the finished state.
  `## What needs to happen` is a numbered list transcribing the **E — `shed-model-contradiction-sweep`** subsection of `### follow-up-task-set`.
  Group it into five parts.
  Part one, shed.md's own contradictions: :7 and :19 say "superseding ... **below**" and "the pre-revision text **below**", but that text was deleted in commit 256b8262 so both references dangle;
  :18 says Finalize is shared "by value" and becomes "by reference" per the Decision `finalize-shared-by-reference`, whose rationale — two of three sources already say by reference, and it is the phrasing that carries the actual meaning — the body should carry;
  :13 enumerates loom's producer list verbatim and :41 lists the mechanical Go-function producers, and both must gain Discussion-Review-Gate once C inserts it into loom.md's table, or the two docs silently disagree about what loom's list contains.
  Part two, the stale "this task is still pending" claims that this scoping task itself falsifies: shed.md:63's claim that wiki task `shed-producer-model-scoping` is the dedicated pass that reconciles any remaining detail mismatch between the two docs, loom.md:76's version of the same claim about the producer table, and loom.md:91–94's version of it.
  Part three, loom.md's remaining residue, which E owns as the file's final owner: :15–17's naming note, which still says "loom = Shed + loom's own Preflight + the Discussion/Plan/Webster producer" — old slot framing contradicting the table 25 lines below it — and whose "This doc has not been rewritten to extract Shed explicitly" claim is now false;
  :29, which links the v2 plan-format.md that A deletes and frames v3 as "the target format is changing", and which B's mechanical sweep deliberately leaves self-contradicting for E to repair;
  :91–94, the naming note calling internal/builderengine and internal/buildercli "a real, separate, already-shipped sibling implementer loop", plus its builder-contract.md link;
  :187, the module-decomposition row repeating the same already-shipped-sibling claim and builder-contract.md link;
  and :56, row 8's Batchifier entry, rewritten to match whatever F landed.
  Part four, the other files: hardener.md:17's "producer-slot";
  docs/overview.md:272's stale chain "Preflight → Discussion → Plan → Builder → Raddle → Finalize";
  and manifest/roadmap.md, where E is the last owner and therefore carries :68's "deferred phase slot between Builder and Finalize" (moved off D) and retires :31's "**A dedicated scoping task should run first** ... this item is not yet broken down into buildable units", which is stale the moment this scoping task lands — E is the right place to declare the breakdown done and name the six tasks.
  Part five, the two additions: resolve loom.md:75's thin-Output question per the Decision `preflight-finalize-thin-output-is-permitted` and record the resolution in shed.md's producer-contract section — the Output contract permits a pass/fail gate signal with no artifact, because Preflight and Finalize genuinely have no output artifact and the resume-on-output-files rule degrades gracefully (a producer with no artifact simply re-runs on resume, which is correct for both);
  and add the new short CONSTRAINTS.md invariant naming the pointer rule as a review obligation.
  The invariant needs its own paragraph carrying the Decision `pointer-rule-becomes-a-short-constraints-invariant` in full: an instruction file must never duplicate or paraphrase another producer's format-contract content, only point at it;
  it matches the file's existing seam-invariant precedent (Treadle Runner-Seam, Scout Engine-Seam, Shuttle Provider-Seam, Batcher Registry+Config);
  it is enforced by review, not machine-checked;
  and it must be **short** — one invariant statement plus an "Enforced by: review obligation" line, in the same shape as the existing entries, not a treatise.
  State as its own paragraph that E re-reads shed.md and loom.md end to end rather than working the line lists above, exactly as D does for finalize.md, because shed.md:63 sits inside a whole "Why this doc doesn't rewrite loom.md's full detail" section whose premise changes once C and E have run.
  Add a `## Open questions` section carrying the three questions the discussion assigns to E, each with its owner note intact.
  Question 1, Webster violates the producer-atomicity rule: the landed model states a producer is always atomic — one mechanical action or one LLM session, never an internal multi-step process of its own — but loom.md:57 lists Webster as a black box with its own per-batch fork loop, opaque to loom's flat list, which is precisely an internal multi-step process;
  either atomicity admits a carve-out for black-box producers that own their own loop, or Webster decomposes into flat producers the way Plan did.
  Record it as the single largest unresolved tension in the model, to be decided before Shed is built rather than during, and state E's specific obligation: it is recorded as a **named precondition on manifest/roadmap.md's Planned Shed item**, not merely as prose in a design doc, because recording it without gating it is how it gets skipped.
  Question 2, Discussion-Write has no Input: loom.md:50 records its Input as "— (starting point)";
  the thin-Output carve-out is now decided for Preflight and Finalize but the symmetric thin-*Input* case has not been, and the task body itself is arguably the Input, which would make the pointer target the wiki task record rather than a format-contract file — a different kind of pointer than every other row in the table.
  State that E records it in shed.md's producer-contract section immediately beside the thin-Output carve-out it mirrors, and that it does **not** get a roadmap gate the way question 1 does, because it is a contract-wording decision rather than a precondition that could invalidate Shed's design.
  Question 3, `shed` is an overloaded name in this repo: docs/overview.md:289 and :318 record that earlier reed drafts split the model and view into separate modules named `shed` and `glance`, and a reader hitting :289 first will mis-resolve it now that "shed" names the outer phase-FSM — so an explicit disambiguating note is worth more than leaving two unrelated meanings in one doc set.
  E owns it as docs/overview.md's last owner in the chain.
  Add the deferred-phase-enum record as its own item, per the Decision `phase-enum-realignment-is-deferred-to-the-shed-build`: internal/loomengine/coherence.go:14–22's `validPhases` map and docs/reference/status-schema.md's matching phase enum are deliberately left alone by tasks A through F, and realigning them lands with the Shed build task, because the flat producer list replaces the phase enum rather than editing it and rewriting it now would invent an interim phase set that Shed would immediately discard.
  State that E records this deferral explicitly alongside its roadmap edits so a later reader finds a decision rather than an oversight.
  `## Scope` states E's three ownership positions — loom.md's final owner after B and C, roadmap.md's last owner after A, and docs/overview.md's last owner after A and B — and states that E writes the finished state rather than guessing, which is the whole reason it is serialized last.
  `## Sequencing` records `depends_on: format-docs-name-producers, batcher-standalone-split`, with both reasons: E must see C's finished table, and it cannot write loom.md row 8 before F has decided what row 8 says.
  `## Acceptance` uses the same docs-only criterion as D — every relative markdown link and anchor introduced or touched resolves, via a link-check pass over manifest/ and docs/.
- **Commit:** `scoping: stage follow-up task body E (shed-model-contradiction-sweep)`

## Batch Tests

`verify: null`.
Three markdown files, no runnable surface — the same situation as batch 1.
The correctness question is whether each body carries its decisions completely and states its ownership boundaries against the sibling tasks, which is a review judgement rather than an assertion.
Batch 3's parse of the fenced yaml headers is the only mechanical check these files face, and it runs there.
