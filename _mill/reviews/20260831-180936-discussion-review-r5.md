MILL_REVIEW_BEGIN
# Review: Surface merge-in-progress in fabric status

```yaml
duration_s: 104.0
verdict: APPROVE
reviewer_model: opus
reviewer_self_id: Claude (Opus-class), Anthropic
reviewed_file: _mill/discussion.md
date: 2026-08-31
```

## Findings

### [NIT:consistency] F18 edit text unqualified on vantage point
**Demoted-from:** BLOCKING
**Section:** `docs-in-same-commit` disposition table, `SANDBOX-FABRIC-SUITE.md:404` row
**Issue:** The prescribed F18 sentence ("`status` … must report `merge_in_progress: true` for the whole live window") is written unqualified, and sits directly beside F18's existing line listing `remove` of the merge's own source pair among the refusing verbs — the one refusal driven by the hub-wide `mergeSourceInFlight` predicate, where `merge_in_progress` is legitimately `false`; this is exactly the unqualified phrasing the `field-is-this-pair-only` decision forbids for the `Long` text.
**Fix:** State that the F18 edit must name the vantage point (the prime worktree holding the record) and must say the field does not predict `remove`'s hub-wide refusal, mirroring the `Long`-text qualification requirement.

### [NIT:scope] Parked-merge fixture shape underspecified
**Section:** Technical context → "Test scaffolding that already exists"
**Issue:** The bullet pairs `hubforge.NewHub` with `hubforge.SeedFabricConfig`, but the cited parked-merge exemplar `TestRunCLI_MergeStageRejectsAPathThatIsNotConflicted` (`merge_cli_integration_test.go:432-436`) calls `NewHub` alone with no `SeedFabricConfig`, while `TestRunCLI_ReadOnlyVerbsOmitMutationsKey` (`cli_test.go:900-901`) does seed; the discussion does not say which shape scenario 2 takes.
**Fix:** Say which fixture shape each of the two scenarios uses, or state that either is acceptable.

### [NIT:consistency] Reflow disposition for `docs/overview.md:210` unstated
**Section:** Constraints (semantic line breaks) vs `docs-in-same-commit`
**Issue:** `docs/overview.md:210` is today one physical line carrying four sentences, already contrary to the one-sentence-per-line rule the Constraints section restates; the discussion requires the edit but does not say whether the surrounding line is reflowed or the new clause simply appended.
**Fix:** State the disposition — append in place, or reflow the whole bullet — so the diff size is not left to the plan writer.

## Verdict

APPROVE
One unqualified sandbox-doc edit instruction contradicts the task's own this-pair-only decision.
_Note: 1 finding(s) demoted from BLOCKING to NIT by the stage's blocking-class ceiling; current blocking_count is 0._
MILL_REVIEW_END
