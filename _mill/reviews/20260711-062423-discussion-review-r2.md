I have verified the key claims against source. The discussion is well-grounded: `shuttleengine.Spec` validate leaves Effort untouched (parallel for Version holds), claudeengine `Prepare`/`buildLaunchCmd` seam matches the described insertion point, `configsync.ReconcileAll` does prune-and-write (confirming the seed-only need), and `configreg.Modules()` matches the pinned list. Let me finalize.

MILL_REVIEW_BEGIN
# Review: Build modelspec - the model-spec parser + registry

```yaml
verdict: APPROVE
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-07-11
```

## Findings

### [NOTE] `lyx config models --set`/`--edit` interaction unaddressed
**Section:** Decisions › configreg registration; New-model-without-recompile
**Issue:** Registering `models` exposes the config CLI, but `configengine.Set` hard-errors on any key absent from the template (`set_test.go:TestSet_UnknownKeyRemovesScaffoldedFile`), so `lyx config models --set zephyr.engine=claude` cannot add a new alias — partially at odds with the "extend via config alone" narrative; only `--edit`/hand-edit works (existing top-level keys are preserved, not deleted, so no data loss).
**Fix:** State that operator alias additions go through `--edit`/direct file edit (the file is operator-owned after materialization), and that `--set` rejects unknown alias keys by design.

### [NOTE] Leaf enforcement "mirrors lyxtest" but needs an allowlist, not a banned-list
**Section:** Decisions › Leaf discipline
**Issue:** `internal/lyxtest/leaf_enforcement_test.go` checks a fixed banned-import list; modelspec's invariant is an allowlist (only stdlib + hubgeometry + yaml.v3), which is a different check shape — "mirroring" could be misread as copying the banned-list pattern.
**Fix:** Note the modelspec test asserts imports are a subset of the allowed set (reject anything outside), not a banned-list scan.

## Verdict
APPROVE
Round-1 reconcile-pruning gap resolved via seed-only; remaining items are non-blocking clarifications.
MILL_REVIEW_END
