MILL_REVIEW_BEGIN
# Review: landing: Publish + Finalize producers

```yaml
duration_s: 200.0
verdict: REQUEST_CHANGES
reviewer_model: opus
reviewer_self_id: Claude (Anthropic) — Opus-class model, exact version not self-verifiable
reviewed_file: _mill/discussion.md
date: 2026-08-19
```

## Findings

### [NIT:scope] Injecting layer for the Fabric closures does not exist
**Demoted-from:** BLOCKING
**Section:** `fabric-handles-are-injected-closures` / Scope
**Issue:** The closures are assigned to "the CLI/orchestrator layer", but no production package calls `loomshed.New` anywhere in the tree (only `internal/loomshed/*_test.go` reference it), there is no `loomcli`, and Scope explicitly excludes any new `lyx` command — so the parent resolution chain (`fabricengine.List(sourceDir)` → branch match → `lyxcwd.ResolveWorktree` → `fabricengine.Open`) has no home, no owner, and no test in the Testing section.
**Fix:** Name the concrete file that fills `loomshed.Deps.Landing` (including both openers and the parent-branch match) and either bring it into Scope or state explicitly that both closures ship nil-able and unexercised until a loom orchestrator lands.

### [BLOCKING:design] Conflict report path is relative and unanchored
**Section:** `mergeresolve-drives-shuttle-directly`
**Issue:** `.lyx/landing/conflict-resolution-r<attempt>.md` is given as a relative path, but `Spec.validate` resolves relative `OutputFiles` against `worktreeRoot` (`spec.go:132`, called at `run.go:143`), not `AnchorPath()` — so on any anchored hub (`AnchorRel != "."`) the report lands at `<worktreeRoot>/.lyx/...`, violating the Durable-vs-Ephemeral rule that `.lyx` is `_lyx`'s sibling under the anchor; the discussion also names no `_lyx/landing` twin and no scratch accessor, against "no engine derives its own `.lyx` path".
**Fix:** State that the report directory is a told absolute path carried in `landingshed.Deps` (anchored scratch dir), and say what the mirrored `_lyx` subpath is, or justify the deviation.

### [NIT:consistency] `MergeStageResolved` signature stated two ways
**Section:** `merge-stage-resolved-verb`
**Issue:** The Decision bullet says `MergeStageResolved(paths []string) error`; the Signature bullet eleven lines later, and Scope, say `(StageResult, error)`.
**Fix:** Delete the stale `error`-only spelling from the Decision bullet.

### [NIT:decision] `*ErrUnmergeableState` has no stated disposition
**Section:** Decision table / Testing (`internal/mergeresolve`)
**Issue:** `MergeIn` returns `*ErrUnmergeableState` when `unifyConflictPaths` reports an unmappable path (`mergepaths.go:79-87`) and can return `*ErrMergeIncomplete`; neither appears in the decision table or the test list, which enumerate only `ErrForeignMergeState` and the crash-recovery probe.
**Fix:** State the catch-all rule for unlisted typed merge errors (presumably `Stuck`, no abort, since fabric already self-aborted) and add one test row.

### [NIT:decision] CONSTRAINTS.md reference to `landing.md` left standing
**Section:** `docs-lifecycle-landing-md-deletes` / Constraints
**Issue:** `CONSTRAINTS.md`'s Markdown Link Integrity bullet cites `landing.md`'s own outgoing `../../CONSTRAINTS.md#...` link as a live worked example; the docs list only updates the Told-Geometry list, so that prose goes stale on deletion (prose, so no test catches it).
**Fix:** Add that bullet to the same-commit doc edits, or state that it is deliberately left as a historical example.

## Verdict

REQUEST_CHANGES
Two blockers: no owner for the injected openers, and an unanchored `.lyx` report path.
_Note: 1 finding(s) demoted from BLOCKING to NIT by the stage's blocking-class ceiling; current blocking_count is 1._
MILL_REVIEW_END
