MILL_REVIEW_BEGIN
# Review: fabric: warp-rebase / remote-reconcile recovery

```yaml
verdict: GAPS_FOUND
reviewer_model: opushigh
reviewer_self_id: Claude Opus 4.8 (claude-opus-4-8)
reviewed_file: _mill/discussion.md
date: 2026-08-01
```

## Findings

### [GAP] Warp-pull refresh primitive is not inventoried
**Section:** Technical context / Scope "In" (unified pull)
**Issue:** `internal/gitrepo` has no fetch and no ff-vs-non-ff classifier; `Repo.Pull()` is `git pull --ff-only` and deliberately hides stderr (pull.go:18), so a diverged/rebased warp surfaces as an opaque "git exited N" — the design cannot tell "non-fast-forward rebase" from "no remote" via it, and `hasUnpushed` (push.go:233) is unexported.
**Fix:** Name the new warp-side primitives this slice must add (fetch-without-merge, non-ff/divergence classification, an exported unpushed-commit check) rather than listing only already-present blocks as "confirmed present."

### [GAP] gitrepo Client Boundary Invariant not in constraint coverage
**Section:** Constraints
**Issue:** The warp half adds gitexec CLI-bound operations (a warp `fetch`, plus reset already present); CONSTRAINTS' gitrepo Client Boundary Invariant requires any new `gitexec` call inside `internal/gitrepo` to be justified in that list in the same commit, but the discussion's Constraints omit this invariant entirely.
**Fix:** Add the gitrepo Client Boundary Invariant to the constraint list and note that any new CLI-bound method must update its pinned set and `TestGitrepoBoundary_PinnedRunCallSites`.

### [GAP] Written report: existence, location and geometry undecided
**Section:** Scope "In" vs Decision pattern-conflict-reporting
**Issue:** Scope states a report is definitively "written," but the Decision says "result (and/or a written report)" — undecided whether it is a return field, an on-disk file, or both; no path or format is pinned, and a file path would need to resolve through `hubgeometry` (Hub Geometry Invariant).
**Fix:** Decide return-struct vs written-file (or both); if written, pin the `_lyx`-relative location constructed via a `hubgeometry` helper.

### [GAP] Two-sided Fabric.Pull partial-failure contract unstated
**Section:** Decisions safe-vs-unsafe-reconcile / unified-pull-dispatch
**Issue:** `Fabric.Commit` (commit.go:139) and `RevertWithWeft` (revert.go:150) both define warp-first ordering with explicit rollback/`*PartialCommitError`; `Fabric.Pull` runs warp-first then weft ff-pull, but the doc never says what happens if the weft ff-pull fails after warp was already reset/reconciled — the only atomicity statement covers the unpushed+diverged abort case.
**Fix:** Specify Pull's partial-failure semantics (rollback or typed partial-result) when the weft half fails after the warp half has mutated.

### [NOTE] Rebase detection conflates any warp history rewrite
**Section:** Decision rebase-detection-scope
**Issue:** "Latest `Warp-SHA` no longer exists" flags any warp history rewrite (a force-push of unrelated history, not only a rebase); the clean-local auto-reset then applies to those too.
**Fix:** State that the safe reset intentionally covers any non-ff rewrite, or narrow the wording away from "rebase" specifically.

## Verdict

GAPS_FOUND
Warp-pull primitive, gitrepo-boundary constraint, report location, and two-sided partial-failure contract need resolving.
MILL_REVIEW_END
