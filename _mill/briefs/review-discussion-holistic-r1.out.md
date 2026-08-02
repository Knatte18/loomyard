MILL_REVIEW_BEGIN
# Review: fabric: collapse external API surface onto Commit — stop leaking warp/weft

```yaml
verdict: GAPS_FOUND
reviewer_model: opushigh
reviewer_self_id: Claude Opus 4.8 (claude-opus-4-8)
reviewed_file: _mill/discussion.md
date: 2026-08-02
```

## Findings

### [GAP] `lyx fabric status` verb name already taken
**Section:** Decisions › dead-methods-diff-status-kept; Technical context (dead methods)
**Issue:** A `status` subcommand already exists on the flat `lyx fabric` tree (`internal/fabriccli/weft_verbs.go:143`, backed by `StatusWeft`); the decision only cautions about the Go-level `Topology.Status` clash, missing this hard cobra verb-name collision.
**Fix:** Decide explicitly whether `Fabric.Status` replaces/merges the existing weft `status` verb or takes a different verb name, and say which — you cannot register two `status` subcommands under one parent.

### [NOTE] Perch exclude-magic: existing git-exclude backstop unmentioned
**Section:** Technical context › KEY IMPLEMENTATION RISK; Testing
**Issue:** The (correct) biggest-hazard framing omits the already-present `seedWeftArtifactExcludes`/`crossModuleMachineLocalExcludes` backstop (`weftgit.go:97`) as a candidate resolution; note perch's locks live at `_lyx/perch/<block>/run.lock` (two-deep, per `run_integration_test.go:112`), which the current `**/_lyx/*/*.lock` pattern does NOT reach.
**Fix:** Add the exclude-file route to the option list, and record that it requires deepening the pattern (e.g. `**/_lyx/*/**/*.lock`) since perch locks sit one level below what the pattern covers today.

### [NOTE] Dropping SyncWeft undercuts kept Diff/Status test setup
**Section:** Decisions › dead-methods-diff-status-kept; Testing (dropped methods)
**Issue:** `SyncWeft` has no production callers but is the correspondence-recording setup path in `diff_integration_test.go` for the very `Diff` verb being kept; "delete or fold the orphaned tests" glosses over this cross-dependency.
**Fix:** State that Diff/Status test setup migrates onto `Fabric.Commit` (which records correspondence) when SyncWeft is removed, so the kept verbs keep integration coverage.

## Verdict

GAPS_FOUND
The new `lyx fabric status` verb collides with an existing verb of the same name; resolve before planning.
MILL_REVIEW_END
