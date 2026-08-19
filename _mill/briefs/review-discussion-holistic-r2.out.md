MILL_REVIEW_BEGIN
# Review: landing: Publish + Finalize producers

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-19
```

## Findings

### [BLOCKING:design] OutputFiles is a Runner rule, not an adapter rule
**Section:** `mergeresolve-drives-shuttle-directly`
**Issue:** The rejection rests on a false premise — `shuttleengine/spec.go:115` `validate` (called from `run.go:143`, inside `Runner.Start`) hard-errors on `len(OutputFiles)==0` ("a run's output file IS its return value") and rejects any entry that already exists on disk, so bypassing `SingleLLMProducer` does not escape the constraint; a conflict session editing existing files in place can neither satisfy nor omit it, and `Done` is classified by file existence.
**Fix:** State what the conflict spec's `OutputFiles` holds (a fresh resolution-report artifact under `.lyx`, or an explicit `shuttleengine` change to allow an empty list) and re-justify the rejection against the real constraint.

### [BLOCKING:design] Parent pair handle has no stated provenance
**Section:** `finalize-merge-geometry` / `fabric-handles-are-injected-closures`
**Issue:** `OpenParentFabric` needs a `*lyxcwd.Location` for the parent pair, but the parent is known only as a branch name (`loomengine.Status.Parent`) and no branch→worktree lookup exists in the tree (`fabricengine` exposes only `PrimeName`); the case where the parent branch has no checked-out fabric pair — and the case where the parent worktree is dirty, which `Merge`'s guard refuses with `mergeReasonWorktreeDirty` — is unaddressed.
**Fix:** Name where the CLI layer derives the parent worktree path from the parent branch, and state the verdict for "parent branch has no live pair" and "parent worktree dirty".

### [BLOCKING:decision] MergeStageResolved vs the Mutation Record Invariant
**Section:** `merge-stage-resolved-verb`
**Issue:** The proposed signature `MergeStageResolved(paths []string) error` returns no result type, so it carries no embedded `MutationRecord` and no `Kind`, yet it mutates the index; the discussion's Constraints section cites the Mutation Record Invariant only for passing `MergeResult` through and never states a disposition for the new verb (the guard's pinned table would not catch the omission).
**Fix:** State explicitly whether staging counts as a mutating verb — and if so, give it a result type embedding `MutationRecord` plus its `Kind` and guard-table row in the same commit.

### [NIT:consistency] Inbound-link count is off by one
**Section:** Constraints → Markdown Link Integrity
**Issue:** "eleven inbound references across six files" contradicts the decision's own enumeration, which totals twelve (loom.md 4, shed.md 4, raddle.md 1, fabric-unified-view.md 1, roadmap.md 1, loomshed.go 1) — verified by grep.
**Fix:** Correct the total to twelve, or state that "eleven" counts markdown links only and excludes the prose reference.

### [NIT:consistency] Fake merge interface omits MergeStageResolved
**Section:** Testing → `internal/mergeresolve`
**Issue:** The faked Fabric surface is listed as `MergeIn`, `MergeContinue`, `MergeAbort`, `MergeInProgress`, yet two bullets below assert `MergeStageResolved` is called and ordered before `MergeContinue`.
**Fix:** Add `MergeStageResolved` to the named seam method list.

## Verdict

REQUEST_CHANGES
Shuttle OutputFiles premise, parent-pair provenance, and the new verb's record disposition need resolving.
MILL_REVIEW_END
