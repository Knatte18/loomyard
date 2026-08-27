MILL_REVIEW_BEGIN
# Review: Add a local-only file category to weft

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-27
```

## Findings

### [BLOCKING:scope] internal/gitrepo absent from scope but needs four new verbs
**Section:** § Scope / In **Issue:** Every mechanic named needs a `gitrepo` primitive that does not exist: `MergeStart(ref string, squash bool)` hardcodes `--ff` (`internal/gitrepo/merge.go:78`) with no no-ff option; there is no index-only `git rm --cached` verb; there is no path-scoped reset or path-scoped checkout (only `ResetHard(sha)`, `CheckoutDetached`). `internal/gitrepo` is named nowhere in Scope or Constraints, and `merge_integration_test.go:791-796` currently asserts "fabric pins --ff so the operator's config cannot reach it". **Fix:** Add `internal/gitrepo` to Scope with the primitives it gains, and acknowledge the gitrepo Client Boundary Invariant (pinned method list updated same commit) and the gitexec Checked-Call pinned counts.

### [BLOCKING:design] MergeIn has no options parameter to be told the set through
**Section:** § mergein-protects-symmetrically **Issue:** `Fabric.MergeIn(source string)` (`merge.go:116`) takes no `MergeOptions`, and `mergeresolve.MergeSurface` pins that exact signature (`internal/mergeresolve/deps.go:24`); "unconditional" protection also contradicts `local-only-set-is-told-not-configured`, since an unconditional MergeIn has no caller to be told by. **Fix:** State how the set reaches `MergeIn` — signature change plus the interface and CLI call sites it ripples to — or restate the protection as told-and-conditional.

### [BLOCKING:design] The archive-tag durability premise does not exist in this repo
**Section:** § scaffolding-never-reaches-parent **Issue:** The rationale rests on "the task branch and its archive tag" as the durable record, but loomyard creates no git tags anywhere; `internal/fabricengine/doc.go:1193-1196` states archive tagging "needs a source outside git". **Fix:** Either drop the archive-tag claim and state plainly that the branch alone is the record, or make tagging an explicit out-of-scope prerequisite.

### [BLOCKING:design] Index-only delete strips all of _lyx from the branch tip, permanently
**Section:** § child-side-delete-is-index-only / § Gotchas **Issue:** The gotcha covers only `status.json` returning on the next persist; `_lyx/discussion.md`, `_lyx/plan/`, and review artifacts are removed from the task branch tree by the delete-commit and nothing re-adds them, so a second machine pulling the branch after landing gets no plan — undercutting the cross-machine-resume premise the task exists for. **Fix:** State the disposition of the non-status `_lyx` content after the delete-commit (accepted loss, or re-commit), rather than only status.json's.

### [BLOCKING:design] publish-needs-nothing addresses only the PR diff, not Publish's merge-in
**Section:** § publish-needs-nothing **Issue:** `Publish` runs a full merge-in from the parent at step 4 (`internal/landingshed/publish.go:120-130`, via the same `mergeresolve.Resolver`) and commits status at step 3b; the decision argues only that `_lyx` cannot appear in a warp-side PR diff, never whether that parent→child merge needs the `MergeIn` protection. **Fix:** Extend the decision to say whether Publish's merge-in is covered, and by what.

### [BLOCKING:design] Forced --no-ff is unscoped across sides and across the squash arm
**Section:** § force-no-ff-when-set-is-non-empty **Issue:** `Fabric.Merge` calls `MergeStart` on both warp (`merge.go:491`) and weft (`merge.go:503`), and today's sole member routes weft-side only; the decision does not say whether warp is forced too — which would change warp landing history from FF to a merge commit for every loom landing — nor that the squash arm never fast-forwards and needs nothing. **Fix:** Scope the forcing to the classified side(s) that actually carry local-only paths, and state the squash arm explicitly.

### [BLOCKING:design] Persist-time commit failure disposition ignores in-merge state
**Section:** § commit-hard-errors-push-warns **Issue:** A path-scoped commit is impossible while MERGE_HEAD is live (git refuses a partial commit during a merge), which is reachable via the foreign-merge-state path `mergeresolve.mergeInErrorResult` deliberately leaves untouched (`mergeresolve.go:72-73`); every subsequent persist would then hard-error and kill a run that today only goes Stuck. **Fix:** State how `CommitStatus` behaves when the pair is mid-merge — skip, or hard-error by design.

## Verdict

REQUEST_CHANGES
Missing gitrepo scope, an unreachable MergeIn seam, and a false archive-tag premise.
MILL_REVIEW_END
