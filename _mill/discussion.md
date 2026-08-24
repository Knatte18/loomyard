# Discussion: loom: redesign the Discussion format

```yaml
task: loom: redesign the Discussion format
slug: loom-redesign-discussion-format
status: discussing
parent: main
```

## Problem

loom's `_lyx/discussion/` two-file split already exists and works: `decision-record.md` (lean, Plan-facing — `Plan-Write` reads only this file) and `support-log.md` (audit trail — rejected alternatives, question ledger — `Plan-Write` never reads it).
A 2026-08-23 comparison against millhouse's `mill-start` skill found the growth problem isn't the file format — it's two structural biases the current stencil (`contracts/stencils/loom/loom-template-discussion.md`) inherited by being modeled closely on `mill-start`'s own Phase: Discuss:

1. `Discussion-Write`'s interview bakes in deep architecture/interface/dependency gathering as a mandatory category, spending tokens/time on content that's now Quarry's (or manual grep's) job, computed fresh at Plan time — regardless of which of the two files it would land in.
2. The eventual `Discussion-Review` rubric (a separate, later roadmap item) needs a sanctioned way to flag misplaced or over-gathered content for removal/relocation, not only gap-filling — a purely-additive review loop (no mechanism to remove content, only add it) is the concrete driver behind discussion files that only grow across review rounds and never converge.
   `manifest/designs/review-finding-classification.md` independently documents the same non-convergence failure mode on a different task (a 6-round loop whose blocking-finding count never reached zero) and proposes a related, broader fix — that doc stays its own Someday proposal, only cross-referenced here.

This task designs the fix for both gaps as a standalone companion doc to `manifest/designs/plan-card-format.md`.
It does not implement either fix — those are separate, already-scoped roadmap items (`loom: Discussion-Write producer`, `loom: Discussion-Review producer`).

## Scope

**In:**

- Write `manifest/designs/discussion-format.md` — new standalone design doc, "designed, not implemented" status header matching `plan-card-format.md`'s own pattern.
- Doc states explicitly that the file split and section shapes are unchanged: `decision-record.md`'s 7 required H2 sections, `support-log.md`'s 4 H2 sections, `internal/discussionparser/validate.go`'s mechanical checks — all correct as-is, not touched by this task.
- Doc specifies **Fix 1** (exploration scope): what replaces "Architecture — modules, interfaces, dependencies" as its own mandatory interview category — a bounded positive/negative instruction pair (see Decisions below).
- Doc specifies **Fix 2** (review-loop principle, not rubric text): Discussion-Review must accept "belongs in `support-log.md` instead" and "doesn't belong in Discussion at all" as legitimate findings; states the completeness-before-leanness relocation test; states the writer/reviewer symmetry framing.
- Doc has an "Open, not decided here" closing section (matching `plan-card-format.md`'s own convention) naming what's explicitly deferred: the stencil rewrite itself, the actual Discussion-Review rubric text, and `review-finding-classification.md`'s finding-class vocabulary.
- Fix two now-stale "Supersedes" claims (confirmed via repo-wide grep, no others found): `manifest/designs/plan-card-format.md` line 3 and `manifest/roadmap.md` line 14 (the Wave-group intro) both currently claim `plan-card-format.md` supersedes `contracts/stencils/loom/loom-template-discussion.md` — drop that clause from both, since the new `discussion-format.md` doc now owns that supersession claim instead.
- Add lightweight cross-reference pointers (link only, no restated content) from `manifest/designs/loom.md`'s two mentions of the stencil (lines 35 and 75, in "Discussion producer detail") and from `manifest/designs/review-finding-classification.md`'s intro (line 7) to the new `discussion-format.md` doc.
- Once the doc and manifest cleanup land, move the "loom: redesign the Discussion format" item from `## Planned` (currently nested under "loom: rewrite for the new Plan Card format" → Wave 1) to `## Done`, following the existing Done-entry format/detail level (see e.g. the "Shed recipe: engine registry" entry), linking the new doc.

**Out:**

- No code changes anywhere — this task is markdown-only (no Go, no stencil file edits).
- No rewrite of `contracts/stencils/loom/loom-template-discussion.md` itself — that's the separate `loom: Discussion-Write producer` roadmap item (Wave 2), which will use this task's doc as its source.
- No Discussion-Review rubric text — that's the separate `loom: Discussion-Review producer` roadmap item.
- No change to `decision-record.md`'s or `support-log.md`'s section shapes, and no change to `internal/discussionparser/validate.go` or its tests.
- No adoption of `review-finding-classification.md`'s finding-class vocabulary (`design`/`scope`/`decision`/`consistency`) — it stays its own Someday, not-yet-designed proposal; this task only cross-references it.
- No change to `manifest/designs/scout-plan-symbol-fields.md` or `webster-parallel-execution.md` — both are already named stale elsewhere (`roadmap.md:14`, `plan-card-format.md:3`) but reconciling/deleting them belongs to the Card-format work that named them, not this task.

## Decisions

### deliverable-is-a-new-doc

- Decision: Write a new standalone doc, `manifest/designs/discussion-format.md`, mirroring `plan-card-format.md`'s "designed, not implemented" header pattern. Do not edit `contracts/stencils/loom/loom-template-discussion.md` in this task.
- Rationale: The roadmap brief calls this "a companion design doc to `plan-card-format.md`" in its own literal wording; matches the sibling doc's own pattern; keeps the prompt rewrite inside its own already-scoped roadmap item (with its own review) instead of conflating a design decision with prompt engineering.
- Rejected: Editing the stencil directly now, since (unlike the plan stencil) it's already fully built and wired to `DiscussionSpec`. Rejected because the stencil rewrite is explicitly a separate, already-existing roadmap item; conflating them would make both tasks' own scope harder to review holistically.

### file-split-and-sections-unchanged

- Decision: `decision-record.md`'s 7 required H2 sections and `support-log.md`'s 4 H2 sections stay exactly as they are today; `internal/discussionparser/validate.go`'s mechanical checks are unaffected. The new doc states this explicitly as a "what's not changing" note.
- Rationale: The roadmap item itself frames this task as narrower than a from-scratch format redesign specifically because the two-file split already keeps `decision-record.md` lean by construction — the two actual problems are exploration-behavior and review-additivity bias, neither of which requires touching the section shapes.
- Rejected: Revisiting section shapes as part of this task — no evidence surfaced that the shapes themselves are a problem, and doing so would require touching `discussionparser.Validate` and its tests, outside this task's markdown-only scope.

### fix1-bounded-exploration

- Decision: The new doc's Fix 1 replaces "Architecture — modules, interfaces, dependencies" as its own mandatory interview category with a matched positive/negative instruction pair: `Discussion-Write` may still ask, at a coarse level, whether the design conflicts with an existing pattern or which module boundary it falls under; it must NOT enumerate exact signatures, file:line citations, interface shapes, or dependency lists, or do exhaustive existing-pattern research — that's Quarry's/manual-grep's job, computed fresh at Plan time. This applies regardless of which file the content would land in — stated once, at the exploration-behavior level, not duplicated per-file, since fixing what gets explored means there's nothing to accidentally over-log downstream either (in `decision-record.md`'s Decisions/Constraints or in `support-log.md`'s Interview section).
- Rationale: Directly implements the roadmap item's gap (1). A purely negative instruction ("don't gather architecture detail") under-specifies how much is too much, risking either continued over-gathering at the margin or an overcorrection that refuses to note something genuinely decision-relevant (e.g., a fact needed to justify choosing between two approaches). A bounded positive/negative pair fixes both failure directions and gives the later Discussion-Review rubric task a matching, symmetric line to check findings against.
- Rejected: Dropping the category with no replacement — under-specifies the boundary the same way a pure negative does; "does this conflict with an existing pattern" is a genuinely useful coarse-level question that Scope/Constraints/Edge cases/Testing don't naturally absorb.

### fix2-principle-only

- Decision: The new doc states Fix 2 as a principle for the future Discussion-Review rubric to implement, not as rubric text itself: (a) "belongs in `support-log.md` instead" and "doesn't belong in Discussion at all" must be accepted as legitimate review findings, on equal footing with gap-filling findings; (b) before any relocation, check whether the content carries a requirement/constraint Planner needs — if so, extract it into `decision-record.md`'s own Decisions/Constraints first, then move only the surrounding deliberation narrative (the completeness-before-leanness test, worded this way in the roadmap item itself); (c) a symmetry note — whatever `Discussion-Write`'s stencil says not to gather, `Discussion-Review`'s eventual rubric must say not to flag as missing, or the same purely-additive, non-convergent bias reappears even with the writer-side fix in place. This echoes (cross-references, does not adopt) `review-finding-classification.md`'s own item 5 principle.
- Rationale: Directly implements the roadmap item's gap (2), at the scope this task actually owns — the rubric's full text is a separate, already-scoped roadmap item (`loom: Discussion-Review producer`, roadmap.md lines 43-47), explicitly named "a later, dependent task" in this task's own roadmap entry.
- Rejected: Writing the actual rubric text now — it's already its own roadmap item with its own scope description; folding it in here would duplicate/preempt that task's own review.

### manifest-cleanup-bundled-in

- Decision: This task also fixes the two confirmed-stale "Supersedes" claims (`plan-card-format.md:3`, `roadmap.md:14`) and adds cross-reference pointers from `loom.md`'s two stencil mentions and from `review-finding-classification.md`'s intro to the new doc.
- Rationale: User authorized removing/fixing now-outdated manifest content encountered during this task, beyond the one line originally flagged ("you can freely remove things in the manifest that are now outdated, I think there's several"); a repo-wide grep for `loom-template-discussion` confirmed exactly these two stale supersession claims exist (no others), plus two files whose stencil mentions would benefit from a pointer to the new doc, per the Producer Pointer-Rule Invariant's point-don't-restate convention already used throughout `loom.md`.
- Rejected: Leaving the stale claims in place — two docs both claiming to supersede the same stencil is an active inconsistency, and the user granted explicit permission to clean it up now.

### roadmap-item-moves-to-done

- Decision: Once `discussion-format.md` is written and the manifest cleanup lands, move the "loom: redesign the Discussion format" item from `## Planned` to `## Done`, following this repo's existing Done-entry format and detail level (see e.g. the "Shed recipe: engine registry" entry), linking the new doc.
- Rationale: Matches this repo's own stated convention (project CLAUDE.md: "`manifest/roadmap.md` moves only on completing or adding a planned item") — this task completes exactly one planned item.
- Rejected: none considered — this is existing repo convention, not a new choice.

## Technical context

- `manifest/designs/plan-card-format.md` (84 lines) is the sibling doc and stylistic model: terse, decision-oriented prose, no meta-commentary, "Status: designed, not implemented" header, an "Open, not decided here" closing section.
- `manifest/designs/loom.md` lines 73-111 ("Discussion producer detail") already documents `Discussion-Validate`'s mechanical checks (matches `internal/discussionparser/validate.go` exactly — 7 required H2 headings) and a partial Discussion-Review "what not to flag" rubric (3 items: missing optional "Notes for the plan writer" subsection; missing rejected-alternatives in `decision-record.md`; incomplete call-site enumeration). That existing content stays valid and unchanged — this task's doc supplements it with the relocation/exclusion-as-legitimate-finding principle, it does not replace it.
- `contracts/stencils/loom/loom-template-discussion.md` (already fully built, ~90 lines) is the current stencil: its Step 2 ("Explore before asking") plus Step 3's six-category interview list (Scope/Constraints/Architecture/Edge cases/Security/Testing) are a near-verbatim copy of `mill-start`'s own Phase: Discuss categories. This is what the separate `loom: Discussion-Write producer` roadmap item will rewrite, using this task's new doc as its source.
- `internal/discussionparser/validate.go` mechanically checks the 7 required H2 sections in `decision-record.md` and that both files exist; it checks no content quality, so it is entirely unaffected by this task.
- `manifest/designs/review-finding-classification.md` (82 lines, "Status: Someday, not yet designed in implementation detail") independently documents the same purely-additive non-convergence problem (a 6-round example where scope findings never converged) and proposes a finding-class vocabulary (`design`/`scope`/`decision`/`consistency`) plus the exact symmetry principle this task's Fix 2 needs (its own item 5). This task cross-references it but does not adopt its vocabulary or change its status.
- Exactly two stale "Supersedes" claims exist (confirmed via `grep -rn "loom-template-discussion" --include="*.md" manifest/ docs/ CONSTRAINTS.md`): `manifest/designs/plan-card-format.md:3` and `manifest/roadmap.md:14`. No other file makes a supersession claim about this stencil — `loom.md`'s two mentions and `review-finding-classification.md`'s mention are plain factual references, not supersession claims, and stay accurate as-is (only gain a pointer).
- `manifest/roadmap.md`'s `## Done` section (starting line 170) shows the expected entry format and detail level for the eventual "move to Done" step — see e.g. its "Shed recipe: engine registry" entry.

## Constraints

- Markdown semantic line breaks (project CLAUDE.md): one sentence per line, breaking at internal clause boundaries — applies to every line written or edited in `discussion-format.md`, `plan-card-format.md`, `roadmap.md`, `loom.md`, and `review-finding-classification.md`.
- Producer Pointer-Rule Invariant (`CONSTRAINTS.md`, referenced in `loom.md` line 61): cross-references between producer docs and stencils are pointers into a format-contract file, never a restated copy — applies to how `loom.md` and `review-finding-classification.md` reference the new doc (link, not paraphrase), and to how the new doc itself should be written so the eventual rubric can point at it rather than copy it.
- Documentation Lifecycle (`CONSTRAINTS.md` → `docs/overview.md#documentation-lifecycle`): the new doc is a draft ("designed, not implemented") until the dependent producer-rewrite tasks land, at which point its durable parts fold into `loom.md`/`overview.md` per existing convention — the same lifecycle `plan-card-format.md` and `loom.md` are already under.
- No code changes in this task — everything above is markdown-only, so no Go build/test constraints apply.

## Auto-mode assumptions

Not applicable — this discussion ran interactively; all decisions above were reached with direct user input (see Q&A log), not autonomous self-picks.

## Open risks

- The new doc's Fix 1 wording (the positive/negative instruction pair) is a judgment call about where exactly the "coarse level vs. deferred" line sits; the later `loom: Discussion-Write producer` task, when it actually rewrites the stencil, may find the line needs adjusting once tested against a real interview. That's expected iteration for that task, not a defect in this one.
- This task bundles a small amount of unrelated-but-adjacent manifest cleanup (the two stale Supersedes lines, two cross-reference pointers) alongside the core design-doc deliverable. If a reviewer considers that scope creep despite the user's explicit authorization, the fallback is to split the cleanup into its own trivial follow-up commit — but per the Q&A log below, the user already authorized bundling it.

## Acceptance criteria

- `manifest/designs/discussion-format.md` exists, with a "designed, not implemented" status header, a "what's not changing" note (file split/section shapes unchanged), Fix 1 (bounded exploration-scope instruction pair), Fix 2 (relocation/exclusion-as-legitimate-finding principle, completeness-before-leanness test, writer/reviewer symmetry note), and an "Open, not decided here" section naming the stencil rewrite, the rubric text, and `review-finding-classification.md`'s vocabulary as deferred.
- `grep -rn "loom-template-discussion" --include="*.md" manifest/ docs/ CONSTRAINTS.md` shows zero remaining lines where `plan-card-format.md` claims to supersede `contracts/stencils/loom/loom-template-discussion.md`.
- `manifest/designs/loom.md` (lines 35, 75) and `manifest/designs/review-finding-classification.md` (line 7) each carry a pointer to the new `discussion-format.md` doc.
- `manifest/roadmap.md`'s `## Done` section contains a new entry for "loom: redesign the Discussion format" linking the new doc, and the item no longer appears under `## Planned`.
- All edited/new markdown lines follow the repo's semantic-line-break convention.

## Testing

No code changes, so no `go test` scenarios apply.
Acceptance is structural/textual — see Acceptance criteria above.
Recommend mill-plan include a lightweight verify step: after writing all files, re-run `grep -rn "loom-template-discussion" --include="*.md" manifest/ docs/ CONSTRAINTS.md` and confirm the two previously-stale lines no longer claim supersession, and confirm `manifest/roadmap.md`'s `## Done` section contains the new entry while `## Planned` no longer does.

## Q&A log

- **Q:** Should this task edit the stencil (`loom-template-discussion.md`) directly, or produce a standalone design doc with the stencil rewrite deferred to a separate roadmap item? **A:** Standalone doc; stencil rewrite is its own already-existing roadmap item ("loom: Discussion-Write producer").
- **Q:** `plan-card-format.md`'s preamble claims to supersede `loom-template-discussion.md` too — fix that now? **A:** Yes.
- **Q:** Do the file split / section shapes (`decision-record.md`'s 7 headings, `support-log.md`'s 4 headings) change as part of this task? **A:** Left to the assistant's judgment; decided no change — the roadmap item already frames this task as narrower than a from-scratch redesign because the split already works.
- **Q:** Fold `review-finding-classification.md`'s Someday finding-class vocabulary into this task's doc, or keep it deferred and only cross-reference it? **A:** User granted broad permission to remove/fix other now-stale manifest content encountered during the task ("you can freely remove things in the manifest that are now outdated, I think there's several") rather than picking narrowly between the two original options; interpreted as: keep the vocabulary itself deferred/Someday (don't adopt it), and use the permission to fix the two confirmed stale Supersedes claims plus add the two cross-reference pointers.
- **Q:** What replaces "Architecture — modules, interfaces, dependencies" as a mandatory interview category? **A:** A bounded positive/negative pair — coarse-level pattern/module-boundary questions stay in scope, exact signatures/citations/interface enumeration are explicitly deferred to Plan time.
