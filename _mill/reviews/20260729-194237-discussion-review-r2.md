MILL_REVIEW_BEGIN
# Review: fabric: Fabric.Commit classify+dispatch + unified diff/status

```yaml
verdict: GAPS_FOUND
reviewer_model: sonnet
reviewer_self_id: claude-sonnet-5 (per runtime environment info; best-effort self-assessment)
reviewed_file: /home/knatte/Code/loomyard/wts/fabric-commit-api/_mill/discussion.md
date: 2026-07-29
```

## Findings

### [GAP] Weft-lock scope contradicts for a warp-only degenerate commit
**Section:** `warp-first-ordering` vs `warp-only-commit-is-plain-git`
**Issue:** `warp-first-ordering` states unconditionally "`Fabric.Commit` acquires the weft write lock before the warp commit and holds it across both commits"; `warp-only-commit-is-plain-git` states the warp side has "no fabric write-lock" and that an all-warp-classified input "degenerates to a single warp-only commit" that is "legitimate." Neither reconciles whether a warp-only call acquires the weft lock at all (pure overhead/inconsistent with "no fabric write-lock") or skips it (making the first decision's "holds it across both commits" phrasing conditional on a weft side actually existing).
**Fix:** State explicitly that the weft write lock is acquired only when the classifier finds at least one weft-side path (i.e., skipped entirely for a warp-only degenerate commit), and cross-reference this from both decisions.

### [GAP] Partial-failure framing omits the RecordCorrespondence-only-failure case
**Section:** `partial-failure-report-not-rollback` / `commit-result-and-message`
**Issue:** `CommitWeft` (weftgit.go) already distinguishes a third outcome beyond "weft commit lands" / "weft commit fails": the commit lands (`committed=true`, `sha` set) but `RecordCorrespondence` then errors, returning `(sha, true, err)` — the commit is not lost, only the index update. The partial-failure decision and `CommitResult{WarpSHA,WarpCommitted,WeftSHA,WeftCommitted}` shape only describe binary "weft failed" vs "weft succeeded," and the Testing section's partial-failure case only covers "weft commit fails after a successful warp commit," not this landed-but-uncorded sub-case, which has a materially different recovery story (self-heals via `RebuildIndex`, no data lost).
**Fix:** Have mill-plan's `CommitResult`/`PartialCommitError` shape explicitly cover this third case (e.g. `WeftCommitted=true` with a distinct correspondence-recording error), and add an integration test for it.

### [GAP] WEFT_SKIP_GIT/WEFT_SKIP_PUSH scope is stated inconsistently across commit vs. push
**Section:** `skip-git-weft-scoped` vs `async-push-both-sides-detached`
**Issue:** `skip-git-weft-scoped` says the bypass is "weft-scoped with no warp analogue" (warp always commits regardless of the env vars). `async-push-both-sides-detached` says the same env vars gate the entire async-push child, "which also skips the warp push, since the whole async-push child is gated on it." So `WEFT_SKIP_GIT`/`WEFT_SKIP_PUSH` do have a warp effect — just for push, not commit — but this is never stated as one coherent rule; a reader following only the commit-side decision would conclude warp is unaffected.
**Fix:** Add one sentence in either decision stating plainly: "the WEFT_* bypass never affects the warp commit, but does gate the warp push (since push is a single combined async child)."

### [NOTE] No stale-pathspec tolerance on the warp side, unlike weft's filter
**Section:** `warp-only-commit-is-plain-git` / `weftPathspecFilter` (weftgit.go)
**Issue:** `CommitWeft` runs `weftPathspecFilter` specifically because `git add --` fails its entire invocation if any one positive pathspec entry matches nothing on disk or in the index. The warp side goes "straight through `f.Warp.StageAndCommit(msg, files)`" with no equivalent filtering, so one stale/already-removed path among many legitimate warp files aborts the whole warp commit (and, per warp-first ordering, the weft side too, since nothing is attempted on weft if warp fails).
**Fix:** Either note this is an accepted risk (caller-trusted explicit file list, unlike weft's broader directory pathspecs) or flag it as an open item for mill-plan to decide.

### [NOTE] snapshotTags has no destination on a warp-only degenerate commit
**Section:** `snapshot-trailer-written-now`
**Issue:** The `Snapshot:` trailer is written only on the weft commit. If a caller passes non-empty `snapshotTags` but the file list classifies entirely to warp (degenerate warp-only commit), there is no weft commit to carry the trailer, and the discussion doesn't say whether this silently drops the tags or should surface an error/warning.
**Fix:** State the intended behavior (silent drop is probably fine and matches "trusts its caller," but should be one explicit sentence, not an inference).

## Verdict

GAPS_FOUND
Three GAPs: weft-lock scope for warp-only commits, the RecordCorrespondence-only-failure case, and WEFT_SKIP_* scope consistency.
MILL_REVIEW_END
