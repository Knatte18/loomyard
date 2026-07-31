MILL_REVIEW_BEGIN
# Review: fabric: clone-does-everything + subpath-in-weft + init dissolution

```yaml
verdict: GAPS_FOUND
reviewer_model: opus
reviewer_self_id: Claude Opus 4.8 (claude-opus-4-8)
reviewed_file: _mill/discussion.md
date: 2026-07-31
```

## Findings

### [GAP] "fixing Resolve fixes them all" omits SiblingLayout
**Section:** Technical context (hubgeometry) / relpath-record-wins-cwd-is-a-hard-gate
**Issue:** `Layout.SiblingLayout` (hubgeometry.go:186) hardcodes `RelPath: "."` and never reads the marker; it is the spawn-free RelPath deriver used by `hostLayoutFor` (Status/Reconcile hot path) and is machine-pinned equal to `Resolve` by `TestSiblingLayout_EquivalentToResolve`. Patching only `Resolve` makes Reconcile/Status compute `RelPath="."` (not `backend`) for every worktree of a subpath-anchored hub — wiring/committing at the wrong subpath — and breaks the equivalence test.
**Fix:** State that `SiblingLayout` (and its equivalence test) must also consult the recorded anchor, and that `hostLayoutFor`'s non-sibling `Resolve(worktreeRoot)` fallback must not spuriously trip the at-or-below gate (worktreeRoot is above a subpath anchor).

### [GAP] Sandbox Suite Coverage invariant unaddressed for init deletion
**Section:** Constraints / Task-completion / Testing
**Issue:** `init` is a sandbox-covered module — SANDBOX-CORE-SUITE.md S6 "Subfolder init" carries `**Covers:** init` and drives `lyx init`/`lyx init --undo`; five other suites list "`lyx init` first." preconditions. Deleting `init` obsoletes S6 and makes those preconditions false, but the Sandbox Suite Coverage invariant is not in the Constraints list and no suite-file update is named in Task-completion/Testing.
**Fix:** Acknowledge Sandbox Suite Coverage: retarget/replace S6 and the "`lyx init` first" preconditions to clone-does-everything, and cover the new `lyx fabric unwire` verb.

### [NOTE] Weft Git Invariant not cross-referenced for the new weft:main writes
**Section:** Constraints / Technical context (gotchas)
**Issue:** Clone now commits two new files (`.fabric-anchor`, repo-wide `fabric.yaml`) onto `weft:main`; the gotcha says they go "through fabric's commit/push path (like board's Sync)", but the Weft Git Invariant (incl. its anchored/cross-module exclusion bullets) is not listed among directly-touched constraints.
**Fix:** Add the Weft Git Invariant to Constraints and confirm the new weft:main commit pathspec is exclusion-safe.

## Verdict

GAPS_FOUND
Two mechanisms (SiblingLayout, sandbox coverage) fall outside the discussion's stated scope.
MILL_REVIEW_END
