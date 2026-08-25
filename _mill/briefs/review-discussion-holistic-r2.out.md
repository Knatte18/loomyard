MILL_REVIEW_BEGIN
# Review: loom: Plan-Review producer

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude Opus 4.x-class model (Anthropic); exact build not self-verifiable
reviewed_file: _mill/discussion.md
date: 2026-08-25
```

## Findings

### [BLOCKING:decision] `Plan-Burler` profile omits `fix-scope`/`tool-use`
**Section:** Decisions — "The fasit is `decision-record.md`" / Technical context
**Issue:** The discussion decides `target`, `fasit`, `rubric_stencil` and `run_subdir` but never states `fix-scope` or `tool-use`, and `fix-scope` is safety-critical: `burlerengine/doc.go` names the plan/discussion/review class as `FixScopeOverlay` (agent runs no git, loop owner commits) while the shipped Discussion row uses `source`; an empty value is not caught by `burlerRoundProfile` and fails only inside `Profile.validate` at the first round, mid-run.
**Fix:** Add a decision fixing both keys for `Plan-Burler`, with rationale for `overlay` vs `source` over `_lyx/plan` against the Fabric Git Invariant, rather than leaving it to copy-paste from the Discussion row.

### [BLOCKING:design] Judge never sees the fasit rubric item 4 measures against
**Section:** Decisions — fasit / rubric shape (item 4 "Fidelity to the decision record")
**Issue:** One rubric stencil feeds both rows, but `decision-record.md` reaches only the Burler's `fasit`; `bouncer-template-judge.md` gives the judge exactly `{{.artifacts}}` (`_lyx/plan` only), the round report and the prior ledger — so the row that actually emits APPROVED/BLOCKING is told to check decision-record fidelity with no path to that file.
**Fix:** Decide how the judge reaches the decision record (a second `artifact_paths` entry, an explicit worktree-relative path named in the rubric text, or scoping item 4 to the Burler) and record the rejected options.

### [BLOCKING:scope] Stale-reference sweep enumerates only one of several sites
**Section:** Scope — Docs / Out
**Issue:** The method used to find text invalidated by this change found `designs/loom.md`'s stuck-routing sentence but missed at least: `designs/loom.md` lines 49–54 ("Both count fourteen" — the recipe becomes fifteen), `designs/shed.md` lines 91 and 148 (same `Plan-Review` → `Plan-Write` routing example), `designs/review-finding-classification.md` line 47 ("Plan-Review's own future rubric"), `internal/loomcli/smoke_test.go` lines 20–21 ("two of its fourteen rows -- Plan-Review and Webster-Review -- with stub producers"), and `contracts/recipes/loom-recipe.yaml`'s own header ("The fourteen row names below").
**Fix:** State the enumeration method (a repo-wide scan for `Plan-Review`, `NamePlanReview`, and the fourteen-row count claim) and let scope follow from it, instead of a hand-listed doc set.

### [NIT:consistency] Sequence-test assertions understated as "still reaches Publish"
**Section:** Testing — `internal/loomrecipe`
**Issue:** `sequence_test.go` pins a 14-entry `wantSequenceOrder` including `NamePlanReview`/Done plus exact counters `loomBurler.calls == 1` and `bouncerJudgeCalls == 1`; a second segment makes these 17 entries and 2/2, which the one-line "traversing two review segments" does not convey.
**Fix:** Name the three added history entries and the two counter increments explicitly in the testing section.

## Verdict

REQUEST_CHANGES
Two undecided profile/fasit questions and an incomplete stale-reference sweep.
MILL_REVIEW_END
