MILL_REVIEW_BEGIN
# Review: loom: Plan-Review producer

```yaml
duration_s: 172.0
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-25
```

## Findings

### [BLOCKING:design] `commit_seam` naming a nil Env closure has no disposition
**Section:** Scope (line 35) + Testing → `internal/shedrecipe/entries_bouncer_test.go`
**Issue:** The four enumerated `commit_seam` cases (`plan`, `discussion`, absent, unrecognised) never say what happens when the key names a seam `Env` does not carry — `env.CommitPlan == nil` would assign a nil `BouncerConfig.Commit`, which the same discussion defines as "commit nothing", silently reproducing the exact no-seam failure the `overlay` decision rejects; `internal/shedrecipe/entries_planwrite.go:41` and `entries_discussionwrite.go:33` already handle this class with `requireSeam` (which also catches typed nils, per `env_test.go:68`).
**Fix:** Decide and record whether `commit_seam: plan`/`discussion` calls `requireSeam("Bouncer", "CommitPlan"/"CommitDiscussion", …)` at construction, and add the corresponding test case.

### [NIT:design] Fixes are uncommitted on every non-approving segment exit
**Section:** "`Plan-Burler`'s `fix-scope` is `overlay`, and the segment commits through the Bouncer"
**Issue:** `settle` commits only on `verdictApproved` (`internal/shedadapters/bouncer.go:284`), so a budget-exhausted or otherwise blocked segment leaves the overlay round's edits to `_lyx/plan` uncommitted in the weft working tree — the same "next unrelated `CommitAnchoredPaths` caller sweeps it up under someone else's commit message" condition the decision cites when rejecting the no-seam option.
**Fix:** State that the blocked-run path deliberately leaves `_lyx/plan` dirty for the human who is already in the loop, so a plan writer does not read it as an oversight or add a second commit site.

## Verdict

REQUEST_CHANGES
One undecided failure mode in `commit_seam` resolution; everything else verified against source.
MILL_REVIEW_END
