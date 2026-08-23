MILL_REVIEW_BEGIN
# Review: landing: parent-fabric resolution chain

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic), Opus-class; exact build not self-verifiable from inside the session
reviewed_file: /home/knatte/Code/loomyard/wts/landing-parent-fabric-resolution-chain/_mill/discussion.md
date: 2026-08-23
```

## Findings

### [BLOCKING:design] Stale/prunable parent entry has no disposition
**Section:** "The four steps, concretely" + Testing → `OpenParent` end to end
**Issue:** `parseWorktreePorcelain` (`internal/fabricengine/worktreelist.go:38-79`) reads only `worktree`/`HEAD`/`branch`/`detached`/`bare` — it drops `prunable`, so a deleted-but-unpruned parent worktree still matches on branch; step 3 then runs `ResolveWorktree` at a nonexistent path, and `gitWorktreeRoot` (`internal/lyxcwd/lyxcwd.go:148-160`) returns `ErrNotAGitRepo`, which names neither the branch nor the real remedy. The discussion enumerates exactly three edge cases (detached, unique-match, missing weft sibling) and states "these two failure modes must stay distinguishable", but a gone/stale warp side is a third with no stated disposition.
**Fix:** Decide and record whether the matcher skips prunable entries, whether `OpenParent` maps a step-3 resolve failure onto its own branch-naming error, and add the case to the integration-test scenario list.

### [NIT:consistency] `Deps.PushBranch` field-doc correction unlisted
**Section:** `push-verb-gets-a-neutral-fabric-method` / "Files that will be touched"
**Issue:** `internal/landingshed/deps.go:62-64` says "the layer that names it is the caller", which the decision itself concedes now resolves to `fabricengine`, not the caller — yet the `deps.go` entry in the file list names only the `OpenFabric`/`OpenParentFabric` deferral and laziness wording.
**Fix:** State explicitly whether that phrase is corrected in the same commit or deliberately left as-is.

### [NIT:consistency] Package-doc list omits `Fabric.PushBranch`
**Section:** Technical context → Docs
**Issue:** The docs list names `internal/fabricengine` (`OpenParent`, `Fabric.OriginURL`) but not `Fabric.PushBranch`, the third new exported symbol in that package and the one carrying the load-bearing vocabulary rationale.
**Fix:** Add `Fabric.PushBranch` to the `internal/fabricengine` package-doc line.

### [NIT:consistency] "Why now" understates the current breakage
**Section:** Problem
**Issue:** "the moment a real run reaches them, `shedbuild.Build` fails at construction" — `publishEntry`/`finalizeEntry` (`internal/shedrecipe/entries_simple.go:39,60`) call `NewPublish`/`NewFinalize` at build time and both rows are in `contracts/recipes/loom-recipe.yaml`, so `lyx loom drive` fails at `loomrecipe.New` (`drive.go:45`) on every invocation today, never reaching a row. The discussion's own "Eager construction is confined to `drive`" section says this correctly.
**Fix:** Reword the Problem sentence to match.

## Verdict

REQUEST_CHANGES
One unaddressed resolution-chain failure mode; the rest is sound and source-accurate.
MILL_REVIEW_END
