MILL_REVIEW_BEGIN
# Review: Launch ly-supervise and orchestrator via lyx reed add

```yaml
duration_s: 153.0
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude Opus 4.x-class model (claude-opus-5 per session metadata; self-assessment uncertain)
reviewed_file: _mill/discussion.md
date: 2026-09-18
```

## Findings

### [BLOCKING:design] `lyx` binary resolution in the generated task
**Section:** §Decisions `vscode-task-is-the-launch-surface` / `fail-loud-on-no-hub`
**Issue:** The chain replaces a bare `claude` (one PATH dependency the operator already has in their GUI shell) with three bare `lyx` invocations, and the discussion never states how `lyx` is spelled in the generated `tasks.json` — a VS Code `folderOpen` task shell on macOS/Windows does not inherit a login shell's PATH, so `lyx: command not found` yields three red tasks and no Claude at all.
**Fix:** Decide and record the disposition of the `lyx` reference (bare PATH name, `os.Executable()` absolute path stamped at `ide spawn` time, or a documented PATH prerequisite), and extend `fail-loud-on-no-hub` to cover binary-not-found as distinct from `reed up` failing.

### [NIT:consistency] Candidate-set rule contradicts branch 1
**Demoted-from:** BLOCKING
**Section:** §Decisions `add-if-absent-flag`, "Candidate set, stated once…"
**Issue:** The text says the branches are evaluated "against that set alone, and the hidden branch fires only when the set is empty", but branch **no strand of that name at all** (ordinary add) and branch **every strand of that name is hidden** (no-op) both have an empty candidate set and opposite outcomes — disambiguating them needs a second, never-named hidden-inclusive name-match set.
**Fix:** Name both sets explicitly (name-matched-all vs non-hidden candidates) and restate the four branches as a decision table keyed on both, since getting this wrong silently adds a duplicate strand on every reopen of a worktree with a hidden `claude`.

### [NIT:design] Empty `PaneID` not covered by the liveness rule
**Section:** §Decisions `add-if-absent-flag`, "Liveness predicate"
**Issue:** The rule is stated purely as `aliveIDSet` membership, but `planResumeLaunches` (`lifecycle.go:148`) guards `s.PaneID != "" && liveIDs[s.PaneID]`; after a reboot `Up` calls `clearAllPaneBindings` (`lifecycle.go:675-679`), so the persisted orchestrator arrives with an empty `PaneID` — the single most common relaunch path.
**Fix:** State the predicate as `PaneID != "" && aliveIDSet[PaneID]`, matching `planResumeLaunches` verbatim, and add the empty-`PaneID` case to the §Testing decision table.

### [NIT:scope] Scope "In" omits artefacts Testing mandates
**Section:** §Scope **In** vs §Testing / §Constraints
**Issue:** Testing requires tagged smoke/integration coverage and a `tools/sandbox/SANDBOX-REED-SUITE.md` scenario, but the Scope "In" inventory lists only `internal/vscode/config_test.go`, `internal/reedcli`, `internal/reedengine` tests and the doc set.
**Fix:** Add the tagged test file(s) and the sandbox suite file to the Scope "In" list so the work inventory matches the test plan.

### [NIT:design] Two VS Code windows on one worktree
**Section:** §Decisions `vscode-task-is-the-launch-surface`
**Issue:** Idempotency is reasoned only across sequential reopens; two concurrent windows on the same worktree each run the chain, `--if-absent` correctly no-ops, and both then `attach`, giving two tmux clients whose differing sizes clamp the layout.
**Fix:** State this as accepted (no worse than today's two side terminals) or out of scope, so a plan writer does not invent a guard for it.

## Verdict

REQUEST_CHANGES
Binary-resolution decision missing; candidate-set rule self-contradicts on the empty-set case.
_Note: 1 finding(s) demoted from BLOCKING to NIT by the stage's blocking-class ceiling; current blocking_count is 1._
MILL_REVIEW_END
