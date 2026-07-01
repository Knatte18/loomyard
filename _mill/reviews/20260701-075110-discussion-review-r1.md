MILL_REVIEW_BEGIN
# Review: CLI ergonomics from the sandbox run: config editor + warp error wrapping

```yaml
verdict: APPROVE
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-07-01
```

The discussion is well-scoped and source-accurate. Verified: all three checkout.go sites (88/134/165), worktreelist.go:40, weftengine/sync.go:160 match the cited current messages; `yamlengine.Reconcile`/`collectLeafPaths` is the node-preserving technique claimed; `configcli.dispatch`/`editOne`/`buildConfigLong` and `configengine.Edit` scaffold behavior match; `configreg.Names/Template` and the flat templates back the no-nested-keys claim; docs/overview.md:214 config bullet matches the text the discussion says must change. Decisions carry rationale + rejected alternatives; constraints (CLI/Cobra, Hub Geometry, lyxtest leaf, docs lifecycle) are addressed; failure modes (unknown key, no partial write, scaffold-on-missing, `=`/spaces) covered.

## Findings

### [NOTE] `--set` with no module positional is unspecified
**Section:** Scope / Technical context (dispatch)
**Issue:** `Args` is `MaximumNArgs(1)`, so `lyx config --set key=value` with zero args is syntactically valid but the intended behavior (error vs. apply-to-what) is never stated.
**Fix:** State that `--set` requires a module positional and errors clearly when absent.

### [NOTE] `--print` + `--set` precedence undefined
**Section:** Technical context (dispatch ordering)
**Issue:** Both flags are described as "carved out first," but combining `--print` and `--set` in one invocation has no defined precedence.
**Fix:** Note that `--set` and `--print` are mutually exclusive (or which wins).

## Verdict

APPROVE
Scope, decisions, and constraints are complete; only two minor edge-case ambiguities remain as non-blocking NOTEs.
MILL_REVIEW_END
