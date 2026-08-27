MILL_REVIEW_BEGIN
# Review: Add a local-only file category to weft

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude Opus 5 (claude-opus-5)
reviewed_file: _mill/discussion.md
date: 2026-08-27
```

## Findings

### [BLOCKING:design] Diverged weft strands `lyx fabric pull`
**Section:** `weft-guards-drop-with-it` / `commit-hard-errors-push-warns` / Technical context "The pull side is an operator step"
**Issue:** The design makes a locally-diverged weft "routine, expected", but `Fabric.Pull` pulls weft FIRST via `PullWeft` → `gitrepo.Repo.Pull` = `git pull --ff-only` (`pull.go:229-247`, `weftgit.go:316-322`, `gitrepo/pull.go:18-27`) and returns immediately on weft failure — so after one rejected transition push, the operator's own resume verb refuses for the whole pair, warp included, and no fabric verb reconciles a diverged weft any more (weft merging is being removed).
**Fix:** State the disposition for a diverged weft: either how `Fabric.Pull`'s weft arm behaves (skip/force/reset) or the explicit named manual recovery, and put it in Scope.

### [BLOCKING:design] `resetMergeSides` weft arm has no disposition
**Section:** Scope / `mergestate-weft-fields-stay` / Technical context "MergeAbort/resetMergeSides restore both sides"
**Issue:** `resetMergeSides` hard-resets the weft to `st.WeftStart` with `force: true` (`destroy.go:1196-1218`), reached from `MergeAbort` (`mergelifecycle.go:397`) and three self-abort sites (`merge.go:288,520,657`); with the weft now advancing independently during a merge attempt, an abort silently discards already-pushed status commits, and no decision says whether the weft arm stays.
**Fix:** Add a decision covering the weft arm of `resetMergeSides` (drop it, or re-read the weft SHA at reset time) with the same rejected-alternatives treatment the other guards get.

### [BLOCKING:design] `MergeStateActive`'s warp arm rests on a false premise
**Section:** `skip-while-mid-merge`
**Issue:** The stated rationale is "git refuses a path-scoped commit while `MERGE_HEAD` is live", but warp and weft are separate clones with separate `.git` (`clone.go`), and the commit runs in the weft worktree — warp merge state cannot block it; inheriting `foreignMergeStatePresent`'s two-sidedness (`mergestate.go:257-276`) therefore freezes every status commit for the whole duration of a warp conflict-resolution session, exactly the Stuck the feature should stay observable through.
**Fix:** Either state a separate justification for skipping on warp-side state, or scope `MergeStateActive` to the weft side and say so.

### [NIT:consistency] Guard doc comments assert removed semantics
**Section:** Constraints → Documentation lifecycle
**Issue:** The same-commit doc list names `doc.go` and `cleanup.go` but not `mergeguards.go`, whose comments assert two-sided guard semantics and specifically that "an up_to_date side is never concluded and cannot move" (`sideConcludeMayHaveLanded`, `:424-437`) — false for a weft that commits per transition.
**Fix:** Add `internal/fabricengine/mergeguards.go`'s comments to the same-commit doc list.

## Verdict

REQUEST_CHANGES
Three unresolved dispositions: diverged-weft pull path, merge-abort weft reset, and the probe's warp arm.
MILL_REVIEW_END
