# Discussion: format docs: name their producers and contracts in producer-model terms, add Discussion-Review-Gate

```yaml
task: 'format docs: name their producers and contracts in producer-model terms, add Discussion-Review-Gate'
slug: format-docs-name-producers
status: discussing
parent: main
```

## Problem

lyx's orchestrator design moved to a **flat producer model**: `Shed` walks one ordered list of producers, and a producer's contract is exactly two parts — **Input** (a *pointer* to the format-contract file defining the consumed artifact's shape) and **Output** (same pointer discipline).
`manifest/designs/loom.md`'s producer table is that list for `loom`, and every Input/Output cell in it is supposed to point into one of exactly two pinned contract files: `docs/reference/discussion-format.md` and `docs/reference/plan-format.md`.

Three things are broken today.
First, both contract files still describe themselves in pre-producer terms ("the Discussion phase", "the Plan producer" used loosely), so the pointer rule they are meant to anchor has nothing coherent to point at.
Second, `loom.md`'s table names two artifacts that exist nowhere — `discussion.md` and `plan.md`.
The real artifacts are `_lyx/discussion/decision-record.md` and the `_lyx/plan/` directory.
A producer table whose pointers name nonexistent files defeats the very rule it exists to demonstrate.
Third, the Discussion side has no mechanical pre-check the way the Plan side has one, which `loom.md:75` records as an unresolved open question.

**Why now:** this is task C of the six-task `Shed` producer-model breakdown (see `manifest/roadmap.md:47`).
Its dependency `plan-format-drop-v3-suffix` (task B) has landed — commit `80238b3f` renamed `plan-format-v3.md` to `plan-format.md` and swept every reference — so the renamed file is on disk and ready to edit.
Task E (`shed-model-contradiction-sweep`) is the final owner of `loom.md` and `shed.md` and depends on this task, so E writes those files' finished state rather than guessing at it.

The full task specification is `manifest/designs/shed-followups.md` section **`## C — format-docs-name-producers`** (lines 263–345).
That section is authoritative for everything this discussion does not override, and it carries rationale this file summarises rather than restates.

## Scope

**In:**

- Rewrite `docs/reference/discussion-format.md` in producer-model terms:
  - fix the title (`:1`), which still names the nonexistent `discussion.md`;
  - add a `## Producer and contract` section naming producer / consumers / Input / Output;
  - re-ground `:14`'s two-file-split justification, which currently cites the deleted `builder-contract.md`;
  - rewrite `:12` to name the correct reading producer;
  - rewrite the `## Validation checklist` section into the `Discussion-Validate` producer's check list (checks 1–2), with check 3 demoted to a build-time-assertion note;
  - add the writer-side half and the reviewer-side half of the symmetric "what NOT to look for" rule.
- Rewrite `docs/reference/plan-format.md` in producer-model terms: status blockquote + a new `## Producer and contract` section. The body schema is already correct and is not rewritten.
- Add the new mechanical producer to `manifest/designs/loom.md`'s producer table, and scoped-edit the table's rows so their Input/Output cells name the artifacts that exist.
- Rename the landed producer `Plan-Review-Gate` → `Plan-Validate` across all 8 occurrences (8 lines, 4 markdown files), and name the new Discussion-side producer `Discussion-Validate` to match.
- Add `Discussion-Validate` to `shed.md:13`'s producer enumeration and `shed.md:41`'s mechanical-Go-function-producer list, per `shed-followups.md:423–424`.
- Sweep the superseded name `Discussion-Review-Gate` out of `manifest/roadmap.md:47`, and record an override note in `shed-followups.md`'s section C — see Decision `discussion-review-gate-name-disposition`.
- Repair `loom.md:75`'s open-question sentence, which this task's own edit falsifies.

**Out:**

- **Implementation.** Nothing in this task writes Go, a perch profile, a rubric file, or a test. The `Discussion-Validate` checks are *specified*, not implemented — implementation lands with `Shed`. Check 3's build-time assertion is likewise specified, not written, because the producer definition it would assert over does not exist yet.
- **`loom.md` outside the producer table and `:75`.** Task E owns the rest of the file — including `:15–17`'s naming note, `:29`, `:56`, and `:78`'s "The gate" section. See Decision `gate-terminology-collision-hands-to-E`.
- **`shed.md` and `roadmap.md` beyond the rename sweep and the `shed.md:13,41` insertion.** No surrounding prose is rewritten. Task E owns `shed.md`'s producer-contract section, `shed.md`'s Part-one/Part-two contradictions (`shed-followups.md:419–432`), and `roadmap.md`'s remaining obligations.
- **`loom.md:56`'s `Batchifier` Input cell**, which still reads `plan.md`. It is old row 8 — outside the spec's "rows 2–7 only" grant — and `shed-followups.md:447` already assigns `:56` to task E, to be rewritten against whatever task F lands. This task renumbers its number cell and touches nothing else. See Decision `batchifier-row-hands-to-E`.
- **`CONSTRAINTS.md`.** Task E adds the pointer-rule invariant there. This task adds nothing to it.
- **`review-finding-classification.md`'s finding-class vocabulary** (`design`/`scope`/`decision`/`consistency`). That doc is a DRAFT proposal with its own roadmap item (`roadmap.md:163`); only its **item 5** (the symmetric rule) is in scope here, because task C's own body invokes it explicitly.
- **Renaming `decision-record.md`.** Explicitly rejected — see Decision `decision-record-keeps-its-name`.
- **Who writes `support-log.md`'s Review-rounds ledger.** `discussion-format.md:71–74` leaves this unpinned as a later milestone-12 detail. This task restates those paragraphs in producer-model vocabulary but does not resolve the question.
- **Go code.** Zero Go files change. The word "gate" in `internal/treadleengine`, `internal/fabricengine`, and `internal/lyxcwd` means the treadle gate-command or a cwd guard — unrelated to the producer name, and untouched.

## Decisions

### gate-checks-live-in-the-contract-file-under-a-neutral-heading

- **Decision:** `discussion-format.md`'s existing `## Validation checklist` section is rewritten in place to hold the mechanical producer's checks, keeping a **neutral heading** — `## Validation checks (spec for the future validator)`. The producer *name* `Discussion-Validate` is pinned **only** in `loom.md`'s producer-table row, which points into that section. The producer name does not appear as a heading in the contract file.
- **Rationale:** this is exactly the shape that already landed on the Plan side. `plan-format.md:187` is titled `## Validation checks (spec for the future validator)` — a neutral heading — and the producer name appears only in `loom.md:54`'s table row, whose Input cell points at "`plan-format.md`'s existing hard-fail checks". The division of labour is: the contract file describes the artifact's checkable properties; the producer table names who runs them. Keeping it symmetric also means the contract file survives a later rename or split of the producer without an edit.
- **Rejected:** a `## Discussion-Review-Gate` (or `## Discussion-Validate`) heading — more discoverable, but it puts a producer name inside a contract file that the producer table is supposed to own, breaking the symmetry `plan-format.md` already established. Also rejected: a standalone `docs/reference/discussion-review-gate.md` — a third pinned contract file for two checks, when the checks are already properties of an artifact whose contract file exists.

### rename-gate-producers-to-validate

- **Decision:** rename the landed producer `Plan-Review-Gate` → **`Plan-Validate`**, and name the new Discussion-side producer **`Discussion-Validate`** rather than `Discussion-Review-Gate`.
- **Rationale:** `loom.md` overloads the word "gate" across two incompatible senses. Sense A (`loom.md:78`, "The gate" section, and `perch`'s own package documentation): perch *is* the review gate — the black box with two exits, `APPROVED` or `stuck`. Sense B (the producer table): `-Gate` is a cheap deterministic **pre-check that runs before** the LLM reviewer, so a plan whose card 5 depends on card 9 is rejected in milliseconds at zero token cost instead of burning an LLM review round. Adding a *second* `-Gate` producer to a doc that already conflates the two senses compounds the confusion rather than paying it down. `-Validate` is verb-shaped, consistent with the list's other producers (`Plan-Sweep`, `Plan-Write`), and points straight at what defines it — each contract file's `## Validation checks` section. It frees "gate" to mean perch and only perch.
- **Rejected:** `Plan-Precheck`/`Discussion-Precheck` — names the producer by its position in the list rather than by what it does. `Plan-Check`/`Discussion-Check` — "check" collides with the individual check names (`depends-on-order` is *a* check). Keeping `-Gate` and instead renaming perch's "review gate" language — one rename instead of two, but "review gate" is load-bearing in `perch`'s package docs and `loom.md:78–83`, a far wider blast radius almost entirely inside task E's territory.
- **Blast radius (verified):** `Plan-Review-Gate` occurs **8 times, on 8 lines, in 4 markdown files, and zero Go files** — `manifest/designs/loom.md:54,75`; `manifest/designs/shed.md:13,41`; `manifest/roadmap.md:45,46`; `manifest/designs/shed-followups.md:304,306`. One occurrence per line. The authoritative criterion is the zero-hit grep in Testing below, not the count.

### rename-sweep-crosses-task-ownership-boundaries

- **Decision:** the rename sweep touches `shed.md` and `roadmap.md` even though task E is their owner, and touches `shed-followups.md` even though it is the spec file. Only the literal token is replaced; no surrounding prose is rewritten.
- **Rationale:** this is the precedent task A already set and recorded in `shed-followups.md:449–453` — a bare-word or package-name sweep does not respect chain-order assignment, and leaving the old name in place through E's turn would leave the repo describing two different producers by two different names for the same thing. Task A's own override note states the reasoning verbatim for `builderengine`/`buildercli`.
- **Rejected:** renaming only `loom.md`'s table row and leaving `shed.md:13`'s producer-list enumeration saying `Plan-Review-Gate` — that is precisely the self-contradicting interim state task B created at `loom.md:29` and that this task's spec then had to schedule a repair for.
- **Note on `roadmap.md`:** `CLAUDE.md` restricts roadmap *movement* to completing or adding a planned item. A token-level rename sweep is not a roadmap move — no item changes state, no item is added or removed. Task E's remaining roadmap obligations (`:68`'s deferred-slot line) are untouched.

### discussion-review-gate-name-disposition

- **Decision:** the superseded name `Discussion-Review-Gate` gets **two different dispositions**, by file kind:
  - **`manifest/roadmap.md:47`** — swept to `Discussion-Validate`. The roadmap describes the live, forward-looking plan and names this task's deliverable; leaving it naming a producer that never lands under that name is the same defect the `Plan-Review-Gate` sweep exists to fix.
  - **`manifest/designs/shed-followups.md`** (9 occurrences: `:265,281,283,301,304,325,329,342,424`) — the **original wording stays at all nine**, including `:304`. Note that `:304` carries *both* tokens in one sentence ("scope a `Discussion-Review-Gate` mechanical producer, mirroring `Plan-Review-Gate`"): the `Plan-Review-Gate` half is swept to `Plan-Validate` with the rest of the rename, while the `Discussion-Review-Gate` half stays. `:306` carries only `Plan-Review-Gate` and is a plain rename site. Instead, add one `**Override recorded 2026-08-11 (task C, as landed).**` note under section C's `#### The Discussion-Review-Gate producer` subsection, stating that the producer landed as `Discussion-Validate` and that `Plan-Review-Gate` was renamed to `Plan-Validate` at the same time.
- **Rationale:** `shed-followups.md` is a task-body archive, not live documentation — it records what was *specified*, and rewriting its nine occurrences would falsify that record. The file has an established convention for exactly this situation: `:289`, `:296`, `:441`, `:449`, `:462`, `:470` are all `**Override recorded ...**` / `**Instruction repaired ...**` notes left in place by earlier tasks whose landed shape diverged from the spec. This task follows that convention rather than inventing a new one. `roadmap.md` is not an archive, so it gets the sweep.
- **Rejected:** sweeping `shed-followups.md` wholesale — destroys the audit trail of what was originally specified, and the spec's own reasoning (`:301–312`) reads incoherently once the name it argues for is silently replaced. Also rejected: leaving `roadmap.md:47` for task E — E's roadmap obligations are enumerated at `shed-followups.md:459–478` and do not include this token, so it would simply be dropped.
- **Not touched:** the wiki task title, `_mill/status.md`, and this discussion file's own title all carry the original name. They are task-identity records, not documentation; renaming them mid-task would break the slug↔title correspondence `mill-spawn` established.

### batchifier-row-hands-to-E

- **Decision:** `loom.md:56`'s `Batchifier` Input cell keeps its stale `plan.md` reference through this task. Only its row number changes. Record the hand-off explicitly in the commit message and here.
- **Rationale:** it is old row 8, outside the spec's "rows 2–7 only" grant (`shed-followups.md:329`), and `shed-followups.md:447` already assigns `:56` to task E to be rewritten against whatever task F lands — so E must touch the cell regardless, and a drive-by artifact-name fix here would be overwritten.
- **Rejected:** fixing it anyway "since we are already in the table". The override precedent that justifies crossing ownership for the rename sweep does not reach here: a token sweep must cross boundaries because grep does not respect chain order, whereas this is a single cell nobody's zero-hit criterion forces this task to touch.
- **Consequence for Testing:** the "no surviving `plan.md`/`discussion.md`" criterion is scoped to the rows this task owns (new rows 2–8), not the whole table. See Testing item 3.

### loom-table-row-insertion-and-renumbering

- **Decision:** insert `Discussion-Validate` as the new **row 3**, immediately after `Discussion-Write` and immediately before `Discussion-Review` — mirroring `Plan-Write` → `Plan-Validate` → `Plan-Review`. The whole table renumbers: old rows 3–11 become 4–12. Rows 9–12 (`Batchifier`, `Webster`, `Webster-Review`, `Finalize`) get their number cell renumbered and **nothing else** — no cell content is touched, so task E still writes their finished state.
- **Rationale:** the list is ordered and the order is its semantics; a pre-check appended after the producers it guards would be meaningless. Renumbering is the mechanical consequence of an ordered list, not a scope expansion.
- **Rejected:** appending as a last row to avoid renumbering — breaks the execution-order semantics of the list.
- **Standing note from the operator:** `loom.md` is *just a list of producers* and that list is **not** pinned. It does not need to be nailed down now — later tasks (E especially) may reorder or revise it. So this task should make the table internally consistent and stop; it should not treat the list as frozen, nor spend effort defending the exact membership.

### loom-table-cells-name-real-artifacts

- **Decision:** the rewritten cells for the Discussion and Plan rows read:

  | # | Producer | Type | Input | Output |
  |---|---|---|---|---|
  | 2 | `Discussion-Write` | LLM | — (starting point) | `_lyx/discussion/` (`decision-record.md` + `support-log.md`), shape: `discussion-format.md` |
  | 3 | `Discussion-Validate` | mechanical | `_lyx/discussion/` → `discussion-format.md`'s validation checks | pass/fail |
  | 4 | `Discussion-Review` | LLM/`perch` | `_lyx/discussion/` (both files) → `discussion-format.md` | verdict (APPROVED/stuck) + review file |
  | 5 | `Plan-Sweep` | mechanical | `_lyx/discussion/decision-record.md` (approved) | scout inventory (internal artifact, not gated) |
  | 6 | `Plan-Write` | LLM | `_lyx/discussion/decision-record.md` (**never** `support-log.md`) + `Plan-Sweep`'s inventory | `_lyx/plan/`, shape: `plan-format.md` |
  | 7 | `Plan-Validate` | mechanical | `_lyx/plan/` → `plan-format.md`'s existing hard-fail checks (e.g. `depends-on-order`) | pass/fail |
  | 8 | `Plan-Review` | LLM/`perch` | `_lyx/plan/` → `plan-format.md` | verdict + review file |

  The exact prose of each cell is the implementer's to word; the *content* above is pinned.
- **Rationale:** the real paths are already pinned elsewhere and were not left for this task to re-derive — `discussion-format.md:7–12` pins the two-file directory, and `loom.md:186`'s module-decomposition row already states the Planner writes `_lyx/plan/NN-<card>.md` per card plus `00-overview.md` as the done-sentinel. The two-file access boundary becomes part of `Plan-Write`'s Input pointer, per the spec at `shed-followups.md:274`. (`shed-followups.md:276` cites this as `loom.md:188`; verified against the file today, it is `:186` — `:188` is the execution-stack row. Do not propagate the spec's off-by-two.)
- **Rejected:** keeping `discussion.md`/`plan.md` as shorthand — the artifacts do not exist under those names, and a pointer to a nonexistent file is the exact failure this task exists to fix.
- **Scope note:** the spec assigns this task "`loom.md`'s producer table rows 2–7 only". Under the old numbering that is `Discussion-Write` through `Plan-Review` — the same seven producers listed above, which become rows 2–8 after the insertion. The insertion does not widen ownership; it shifts the numbers.

### repair-loom-75-open-question

- **Decision:** repair `loom.md:75`'s first open question. It currently reads that `Discussion` has no mechanical pre-gate "the way `Plan-Review-Gate` mirrors `plan-format.md`'s `depends-on-order` check — asymmetric, possibly by nature... but worth deciding rather than assuming". This task's own insertion decides it, so the clause is rewritten to record the resolution: the asymmetry was not by nature, `Discussion-Validate` closes it, and the checks it runs are `discussion-format.md`'s validation checks. The **second** open question on that line — `Preflight`/`Finalize`'s thin Output — is left untouched; `shed-followups.md:482` assigns it to task E.
- **Rationale:** `:75` is falsified by this task's own edit. Task A's override precedent (`shed-followups.md:449–453`) covers exactly this: a consequence of your own edit is yours to repair, regardless of which task's ownership list the line sits on. The alternative was already tried — task B deliberately left `loom.md:29` self-contradicting for a later task, and the spec had to schedule a repair, which task B then did anyway (`shed-followups.md:441–443`).
- **Rejected:** leaving `:75` for task E — ships a doc whose table contradicts its own prose 20 lines below it, for the duration of at least one more task.
- **Hand-off:** record in the commit message and in this file that `:75`'s first clause was repaired by task C, so E does not go looking for it. Record also that this task **widens** the second clause's subject: it currently enumerates `Preflight`/`Finalize` as the producers with an unusually thin Output (pass/fail, no artifact), and `Plan-Validate` plus the newly-inserted `Discussion-Validate` now have the identical property. This task does not rewrite that clause — the thin-Output question is `shed-followups.md:482`'s, assigned to E — but the implementer appends a short note to the hand-off record so E resolves the question over four producers, not two.

### gate-terminology-collision-hands-to-E

- **Decision:** the `-Validate` rename removes the collision from the *producer table*, but `loom.md:78`'s `## The gate` section and the surrounding prose (`:78–83`) still use "gate" in sense A for perch. This task does **not** rewrite that section. It records the observation here and in the commit message as an explicit hand-off to task E.
- **Rationale:** `:78–83` is task E's territory, and unlike `:75` it was already ambiguous before this task ran — this task's edit does not falsify it, so the override precedent does not reach it. With the producer table no longer saying `-Gate`, the remaining sense-A usage is at least internally consistent.
- **Rejected:** repairing `:78` here too — widens scope into a section E must re-read end to end anyway (`shed-followups.md:486` requires E to re-read `loom.md` end to end).

### producer-model-rewrite-shape

- **Decision:** in **both** contract files: rewrite the `> **Status: Contract — pinned.**` blockquote, add a new short `## Producer and contract` section at the top of the body naming producer / consumers / Input / Output in the vocabulary of `shed.md`'s producer-contract section (`shed.md:22–29`), **and sweep every generic phase/producer phrase in the body to the named producer** (see the enumerated sites below). What is **not** rewritten is the *schema* — the field grammar, the section lists, the compaction rules, the worked examples. Those are correct and stay.
- **The generic-phrase sweep, enumerated** (verified against the live files today, so the implementer need not re-derive):
  - `discussion-format.md` — "the Plan producer" at `:3, :7, :10, :14, :15, :31, :54, :83` → **`Plan-Write`**; "the Discussion producer" at `:47, :71` → **`Discussion-Write`**; "the Discussion phase" at `:3` → **`Discussion-Write`**; "the **Discussion-review gate**" at `:12` and "the perch discussion-review gate" at `:72` → **`Discussion-Review`** (per Decision `discussion-format-12-names-the-llm-reviewer`).
  - `plan-format.md` — "the planner" at `:24` → **`Plan-Write`**.
  - `:10`'s "The Plan producer's **sole** input" and `:83`'s "the Plan producer's declared input set" are the two load-bearing boundary statements; renaming them to `Plan-Write` is what makes the boundary a statement about a *named producer's* declared Input, which is the whole point of the producer model.
- **Rationale:** this is what makes each doc self-describing as one side of a producer contract, which is exactly what `loom.md`'s Input/Output cells point at. Leaving the bodies saying "the Plan producer" while the producer table says `Plan-Write` is the same defect as the table pointing at `plan.md` — a name that resolves to nothing in the model. The sweep is 13 sites of pure substitution, not a rewrite.
- **Rejected:** blockquote plus new section only, leaving the generic phrases — too thin to satisfy "name their producers and contracts explicitly", and it leaves the doc using two vocabularies for one thing. A full body rewrite of both files — the schemas are correct and heavily cross-referenced; churn for no contract gain, and it would collide with `plan-format.md`'s worked example being byte-consistent by design.
- **`:71–74` is part of this sweep, not an exception.** Those paragraphs ask who *writes* the Review-rounds ledger — "the Discussion producer itself, or the perch discussion-review gate". The question stays open (see Scope "Out"); only its two subjects are renamed, to `Discussion-Write` and `Discussion-Review`. Renaming the alternatives does not decide between them.
- **Content for `discussion-format.md`'s new section:** produced by `Discussion-Write`; validated by `Discussion-Validate` (checks below); reviewed by `Discussion-Review`; `decision-record.md` consumed by `Plan-Sweep` and `Plan-Write`; `support-log.md` consumed by `Discussion-Review` only.
- **Content for `plan-format.md`'s new section:** produced by `Plan-Write`; validated by `Plan-Validate` (the validation checks below); reviewed by `Plan-Review`; consumed by `Batchifier` and `Webster` (via `internal/planparser`, webster's sole parser).

### reground-discussion-format-14

- **Decision:** `discussion-format.md:14`'s "a distilled digest, never raw prose" rule **stays**, and this task **adds** an attribution it currently lacks: `internal/websterengine`'s package documentation (`doc.go:155`, which states the distilled-`Digest`-persisted-at-terminal contract directly; see also `recordbatch.go`'s `RecordResult.Digest` handling) **and** `docs/overview.md:60`'s architecture-level "Go-distilled digests, never raw prose" principle. Restate the sentence in producer-model terms while doing so.
- **This is an addition, not a repair — verify before assuming otherwise.** `shed-followups.md:286` describes `:14` as grounding the rule in "Builder's 'distilled digest, never raw prose' rule (see `builder-contract.md`'s digest contract)", and `:289–292` frames the work as *re-grounding a citation*. Both premises are stale. Task A's commit `0149776a` already rewrote the line, stripping the dead `builder-contract.md` citation outright rather than redirecting it. The line today reads, in full: "Two files, not two sections of one file, on purpose: a distilled digest, never raw prose, is what the Plan producer should ever see." — no citation to anything. So there is no dangling pointer to fix; the sentence stands uncited and correct, and this task gives it a live source.
- **Rationale:** the rule is load-bearing for the two-file split and is the reason `Plan-Write`'s Input pointer names `decision-record.md` alone. An uncited load-bearing rule invites a later task to "clean up" a claim it cannot trace. Both named sources are verified live.
- **Rejected:** citing only one of the two — `docs/overview.md:60` is the architectural statement and `websterengine`'s `doc.go` is the implementing contract; citing both is what the spec suggests and costs one clause. Also rejected: leaving it uncited, since the spec explicitly asks for the grounding even though its description of the starting state is out of date.

### discussion-format-12-names-the-llm-reviewer

- **Decision:** `discussion-format.md:12` currently reads that `support-log.md` is "Read by the **Discussion-review gate**, **never** by the Plan producer." Rewrite it to name **`Discussion-Review`** (the LLM/`perch` producer) as the reader, and name `Discussion-Validate` separately as only existence-checking the file. The load-bearing half of the boundary — `Plan-Write` never reads it — is preserved verbatim in force.
- **Rationale:** the phrase was written when there was one reviewing thing loosely called "the gate". After this task there are two producers on the Discussion side, and the one that actually opens and reasons over `support-log.md` — for the anti-circling Review-rounds ledger at `:67–69` — is the LLM one. `Discussion-Validate` only checks that the path exists. Naming the mechanical producer as the reader would misattribute the anti-circling mechanism to something with no judgment.
- **Rejected:** naming both jointly — technically true of `Discussion-Validate`, but blurs "reasons over the contents" with "stats the path". Leaving it generic ("the Discussion-review side") — this task exists to replace generic phase words with producer names.

### check-3-becomes-a-build-time-assertion

- **Decision:** the third bullet of the current `## Validation checklist` — "the Plan producer's declared input set never names `support-log.md`" — is **removed from the per-run check list** and recorded instead as a short note inside the same section, stating (a) the boundary, (b) that it is asserted once at build/test time over the `Plan-Write` producer *definition* rather than re-evaluated per run, and (c) that it lands with `Shed`. Write it in so many words, so nobody re-files it later as a missing gate check.
- **Rationale:** it is a property of the producer *definition*, not of any run's artifacts — there is nothing per-run for a mechanical producer to evaluate. `shed-followups.md:308–312` states this explicitly and asks for it to be written down explicitly, precisely to prevent re-filing.
- **Rejected:** recording it in `shed.md`'s producer-definition section (task E's file) or as a `CONSTRAINTS.md` invariant (E already adds one there; a second from this task risks a collision in the same commit range).

### symmetric-what-not-to-look-for

- **Decision:** three "what NOT to look for" instructions, each written into **both** sides:

  1. **"Notes for the plan writer" absent is never a finding** — it is optional by contract (`:82`, `:55–56`).
  2. **Rejected alternatives absent from `decision-record.md` is never a finding** — they belong in `support-log.md`'s Rejected alternatives section (`:50`, `:63`).
  3. **Incomplete call-site / cross-reference enumeration is never a finding at this stage** — that belongs to the compiler and to `Plan-Sweep`'s mechanical inventory, not to discussion review.

  Writer side: stated in `discussion-format.md`'s own body, from the writer's perspective ("do not enumerate X here, it belongs to <stage>").
  Reviewer side: stated as a short `## Discussion-Review rubric — what not to flag` section in the same file, explicitly marked as the text the future `perch` profile must carry, from the reviewer's perspective ("do not flag missing X here, it belongs to <stage>").
- **Rationale:** `review-finding-classification.md:52–56` (item 5) requires the instruction be written symmetrically into both the producer's own format-contract and the reviewing producer's rubric — writing it into only one side recreates the non-convergent review loop the doc exists to prevent. Item 5 also names the Go-repo case explicitly: "complete call-site enumeration belongs to the compiler / a mechanical sweep, not this stage, on both sides."
- **Rejected — a fourth candidate: "section *ordering* deviations are never a finding."** Excluded deliberately. `discussion-format.md:33` pins the seven sections **in this order** on the writer's side, so a reviewer-side "never flag ordering" rule would *contradict* the writer-side rule rather than mirror it. Symmetry means both sides state the same boundary; here both sides state that order is specified.

### rubric-side-lives-in-the-contract-file-for-now

- **Decision:** the reviewer-side half above goes into `discussion-format.md` as a marked section, not into a perch profile file. The future profile **points at** it rather than copying it, per the pointer rule.
- **Rationale:** the perch profile for `Discussion-Review` does not exist yet, and this task's Acceptance explicitly defers implementation to `Shed`. `discussion-format.md` is the only live file that can hold the text today, and keeping both halves of the symmetric rule one scroll apart is the cheapest guarantee they never drift.
- **Rejected:** writing the profile file now — implementation, out of scope. Putting it in `loom.md`'s `Discussion-Review` row — task E's territory, and a table cell is the wrong size for it.

### the-mechanical-producer-is-exhaustively-defined-by-its-checks

- **Decision:** state in `discussion-format.md`'s validation-checks section that the mechanical producer is exhaustively defined by the checks listed there — it has no judgment and nothing else is "its" to look for. The symmetric "what not to flag" instruction is therefore aimed at `Discussion-Review`, the LLM producer, where the over-flagging failure mode actually lives.
- **Rationale:** the spec's wording says "the `Discussion-Review-Gate`'s rubric", but a mechanical producer has checks, not a rubric, and cannot over-flag. Putting the instruction where the failure mode is — and explicitly closing the "the mechanical producer might grow judgment later" door — honours item 5's intent rather than its letter.
- **Rejected:** writing it into the mechanical producer's section only (literal reading, wrong target). Writing it into both (duplicated text the pointer rule dislikes).

### decision-record-keeps-its-name

- **Decision:** `_lyx/discussion/` stays a two-file directory with its current filenames. `decision-record.md` is **not** renamed.
- **Rationale:** pre-decided by the spec (`shed-followups.md:314–320`), recorded here so a plan writer does not reopen it. `discussion-format.md:16` states the filenames are self-describing on purpose; `decision-record.md` pairs with `support-log.md`; and the file holds seven sections, so naming it after one of its own sections would mislead.
- **Rejected:** `decisions.md` and `decision.md` — both terser, both lose the sibling parallelism, and both would force a code sweep across `DiscussionDecisionRecord` in `internal/loomengine/config.go`, `discussionpath_test.go`, `discussion_test.go`, and `prompttemplate.go`, for no contract gain.
- **Line-reference note:** the sourcing discussion cites the self-describing-filenames sentence as `discussion-format.md:15`; it is actually at `:16` (`:15` is the preceding sentence about the filesystem boundary). Use `:16`; do not propagate the off-by-one.

## Technical context

**The four files this task edits:**

| File | Lines today | What changes |
|---|---|---|
| `docs/reference/discussion-format.md` | 161 | title `:1`, blockquote `:3`, `:12`, `:14`, generic-phrase sweep at `:3,7,10,15,31,47,54,71,72,83`, new `## Producer and contract`, `## Validation checklist` rewritten, new `## Discussion-Review rubric — what not to flag` |
| `docs/reference/plan-format.md` | 337 | blockquote `:3`, generic-phrase sweep at `:24`, new `## Producer and contract`; `Plan-Review-Gate` does not occur in this file |
| `manifest/designs/loom.md` | 252 | producer table `:47–60` (row insertion + renumber + cells), `:54`, `:75` |
| `manifest/designs/shed.md` | 71 | `:13`, `:41` — rename token, **plus** inserting `Discussion-Validate` into both lists per `shed-followups.md:423–424` |
| `manifest/roadmap.md` | — | `:45`, `:46` — rename token; `:47` — `Discussion-Review-Gate` → `Discussion-Validate` |
| `manifest/designs/shed-followups.md` | 632 | `:304`, `:306` — rename token; plus one new `**Override recorded**` note under section C |

**Key source facts already verified during exploration** (a plan writer need not re-derive them):

- Task B has landed: `docs/reference/plan-format.md` exists at that path (no `-v3` suffix); commit `80238b3f`.
- `plan-format.md:5`'s "Coexistence, not replacement" section is **already gone** — tasks A and B removed it. `shed-followups.md:296–299` records this override. This task's remaining `plan-format.md` work is the producer-model rewrite only.
- `discussion-format.md:30`'s standalone-invocation justification already reads "`lyx webster run`", not "`lyx builder run`" — task A swept it. No repair needed.
- `loom.md:91–92` (the naming note) and `:185` (the module-decomposition webster row) already describe `internal/websterengine`/`internal/webstercli` and link `webster-contract.md` — task A rewrote them ahead of task E (`shed-followups.md:449–453`). Do not re-file. (`shed-followups.md:445–446` cites these as `:91–94` and `:187`; verified against the file today, they are `:91–92` and `:185`.)
- `Plan-Review-Gate` occurs in **zero** Go files. The `gate` identifiers in `internal/treadleengine` (`gate_lingering_test.go`, `engine.go`, `run.go`) are the treadle gate-*command*, an unrelated concept.
- `docs/overview.md:93` lists `discussion-format.md` and `plan-format.md` among the durable kept reference docs. Neither the module table nor the execution stack changes, so `docs/overview.md` needs no edit under `CLAUDE.md`'s task-completion rule.
- `docs/overview.md:60` carries the "Go-distilled digests, never raw prose" principle used for the `:14` re-grounding.

**Producer-model vocabulary to use** (from `shed.md:22–29`, the authoritative source):

- A producer's **contract** is exactly two parts: **Input** (a pointer to the format-contract file defining the consumed artifact's shape, never a restated copy) and **Output** (same pointer discipline).
- **The pointer rule:** an instruction file must never duplicate or paraphrase another producer's format-contract content, only point at it — so editing that one format file alone changes what both its producer and its consumers do.
- **Review is never a property attached to the producer it reviews** — it is always the next, separate producer in the list.
- A producer's **definition** (engine + config) is internal to `Shed` and invisible to the contract. Every `*-Review` producer is `engine: perch`, differing only by rubric/fasit config.

**Anchors and links.** `shed.md:3`, `shed.md:11`, and `shed.md:61` all link `loom.md#the-phase-machine--a-flat-producer-list-no-predefined-slots`. That heading text is not changed by this task, so the anchor survives — but the implementer must confirm it, since every relative link and anchor introduced or touched must resolve (Acceptance below).

## Constraints

From `CONSTRAINTS.md` and `CLAUDE.md`:

- **Documentation Lifecycle** (`CONSTRAINTS.md:368`, pointing at `docs/overview.md#documentation-lifecycle`). `docs/reference/*.md` are durable contract docs — kept, never deleted on landing. `manifest/designs/*.md` are design docs that fold into `overview.md` and package headers when the modules land. Both edited files keep their existing lifecycle status blockquote *form*; only its content is rewritten.
- **Docs land in the same commit** (`CLAUDE.md`). This task *is* the docs change; there is no code half. `docs/overview.md` needs no edit (module table and execution stack unchanged). `manifest/roadmap.md` is edited only by the token rename, which is not a roadmap move.
- **Markdown: semantic line breaks, no fixed-column hard-wrap** (`CLAUDE.md`, and the `mill:markdown` skill). One sentence per line; break inside a long sentence at an internal independent-clause boundary. Plain newlines, never trailing double-spaces or backslashes. **Applies to every `.md` file in this repo, not only newly-written prose** — so any line the implementer touches must come out conforming. Table cells and blockquotes stay on one line.
- **Markdown Link Integrity** (`CONSTRAINTS.md:229–250`). Every inline `[text](target)` link in a `.md` file under `manifest/` or `docs/` must resolve — both the file part and, for a `.md` target carrying one, its `#anchor`. **All four edited doc files are inside that scan scope**, and this task adds new `## Producer and contract` headings, so any link that later points at them must match the generated anchor. Enforced by `internal/lyxcwd/docslink_test.go`'s `TestEnforcement_MarkdownLinks` — this is a machine check, not a manual pass, which is what Testing item 4 runs. Note the invariant's stated coverage gaps: it does not scan `.md` files outside `manifest/`/`docs/` as sources, and never fetches external URLs. Do not add an allowlist entry to work around a break this task creates; fix the link.
- **Worktree isolation** (`CLAUDE.md`). All work stays in `/home/knatte/Code/loomyard/wts/format-docs-name-producers`. No pushes to `main` from this worktree.
- **No new cross-cutting invariant is introduced by this task**, so `CONSTRAINTS.md` is not edited. The pointer-rule invariant is task E's.

## Testing

**Docs-only. This task has no test surface of its own** — `shed-followups.md:340` states this outright, and nothing here compiles.

What must be verified instead, mechanically, before the task is called done:

1. **Zero surviving occurrences of `Plan-Review-Gate`** across the whole repo excluding `.git/` and `_mill/`: `grep -rn "Plan-Review-Gate" --include=*.md --include=*.go --include=*.yaml . | grep -v '^./_mill/'` must return nothing. `_mill/` is excluded because this discussion file and the review artifacts quote both tokens as subject matter; they are task-state records, not documentation. This is the rename's acceptance gate, mirroring task A's package-name zero-hit criterion.
2. **`Discussion-Review-Gate` survives only where Decision `discussion-review-gate-name-disposition` says it should.** The same grep, for that token, must return hits in `manifest/designs/shed-followups.md` **only** — zero in `manifest/roadmap.md`, `manifest/designs/loom.md`, `manifest/designs/shed.md`, and both `docs/reference/` files. Additionally `shed-followups.md`'s section C must contain the new `**Override recorded 2026-08-11 (task C, as landed).**` note.
3. **No surviving `discussion.md` or `plan.md` artifact references in the producer-table rows this task owns** — new rows 2–8 (`Discussion-Write` through `Plan-Review`). Scoped to those rows, not the whole table: row 9's `Batchifier` Input cell keeps its stale `plan.md` by Decision `batchifier-row-hands-to-E`, and the strings appear legitimately elsewhere in the file (e.g. `loom.md:27`, `:39`, which are task E's).
4. **Every relative markdown link and anchor introduced or touched resolves** — run `go test ./internal/lyxcwd/ -run TestEnforcement_MarkdownLinks`. Explicitly required by `shed-followups.md:345`, and machine-enforced by the Markdown Link Integrity invariant. Includes the inbound anchor `loom.md#the-phase-machine--a-flat-producer-list-no-predefined-slots` referenced from `shed.md:3,11,61`, and any new intra-file anchor the new `## Producer and contract` sections create.
5. **`go build ./...` and `go test ./...` still pass** — a no-op assertion here, but cheap, and it proves no Go file was touched by an over-broad sweep.
6. **The generic-phrase sweep is complete.** `grep -n "the Plan producer\|the Discussion producer\|the Discussion phase\|discussion-review gate\|the planner" docs/reference/discussion-format.md docs/reference/plan-format.md` must return nothing. Each site is replaced by the named producer per Decision `producer-model-rewrite-shape`'s enumeration, not deleted.
7. **The symmetric rule is symmetric.** For each of the three "what NOT to look for" items, both halves exist in `discussion-format.md`: the writer-side statement in the body and the reviewer-side statement in the rubric section. A missing half is the exact failure `review-finding-classification.md:53` describes.

**Execution discipline for the rename**, borrowed from task B (`shed-followups.md:201–211`): do the token replacement with a scripted, reviewable pass rather than by hand across four files, then read every hit site to confirm the surrounding sentence still reads correctly — a rename that lands inside prose describing the *old* concept is a silent regression the zero-hit grep will not catch.

## Q&A log

- **Q:** Where does the mechanical producer's spec + rubric live — rewrite `discussion-format.md`'s existing checklist section, a new standalone reference doc, or `loom.md`? **A:** Rewrite `discussion-format.md`'s section.
- **Q:** (Operator correction to the stated rationale.) Is the precedent "`plan-format.md` hosts a `## Plan-Review-Gate` section"? **A:** No — `plan-format.md:187` is `## Validation checks (spec for the future validator)`, a **neutral** heading; the producer name appears only in `loom.md:54`'s table row, pointing into that generic section. What landed is: the contract file holds the checks under a neutral heading, and the producer name is pinned in the producer table, not as a heading in the contract file.
- **Q:** Given that correction, does `discussion-format.md`'s section get a neutral heading or the producer name as its heading? **A:** Neutral heading; producer name pinned only in `loom.md`'s row.
- **Q:** Insert the new row and renumber, and repair `loom.md:75`'s now-false clause — or leave `:75` to task E? **A:** Insert, renumber, repair `:75`. Plus: `loom.md` is just a list of producers and that list is **not** pinned — it does not need to be nailed down now.
- **Q:** Which "what NOT to look for" items? **A:** Delegated to the assistant. Picked: optional "Notes for the plan writer", rejected-alternatives placement, and call-site enumeration. Deliberately excluded section-ordering, because `discussion-format.md:33` pins section order on the writer's side, so a reviewer-side "never flag ordering" rule would contradict rather than mirror it.
- **Q:** Does the symmetric rule bind the mechanical producer or the LLM `Discussion-Review` producer? **A:** The LLM producer — that is where the over-flagging failure mode lives. Also state that the mechanical producer is exhaustively defined by its checks, so nothing else is its to look for.
- **Q:** Shape of the producer-model rewrite — blockquote only, blockquote + a new `## Producer and contract` section, or a full body rewrite? **A:** Blockquote + new section.
- **Q:** Where does the reviewer-side half of the symmetric rule get written, given the perch profile does not exist yet? **A:** A marked section in `discussion-format.md`, which the future profile points at rather than copies.
- **Q:** Where is check 3's build-time assertion recorded? **A:** A note inside the same validation-checks section of `discussion-format.md`.
- **Q:** (Operator, on reading the draft.) What are the *two* things — was the gate not just the perch reviewers? **A:** Two separate producers: the mechanical pre-check (existence + seven sections, pass/fail, no judgment) and the LLM/`perch` review loop (verdict + review file). `loom.md` overloads "gate" across both senses — sense A is perch at `:78`, sense B is the mechanical pre-check in the producer table.
- **Q:** Should the confusing names be changed? **A:** Yes — rename `Plan-Review-Gate` → `Plan-Validate` and name the new one `Discussion-Validate`. Verified blast radius: 8 occurrences on 8 lines, 4 markdown files, zero Go — see Decision `rename-gate-producers-to-validate`.
- **Q:** `loom.md:78`'s `## The gate` section still uses "gate" for perch — repair here? **A:** No. Hand off to task E, which owns that section and must re-read `loom.md` end to end anyway. Recorded here and in the commit message so E finds it.
