I have completed my verification. All plan claims check out against source: the exact error-string call sites (idecli:59, initcli:84, configcli:185, muxpoccli:100), the hubgeometry `exitCode != 0` branch (line 96), the warp struct literals, the build tags on test files, and the S2/S6 Watch blocks. I also confirmed boardcli already uses bare `rerr.Error()` (correctly excluded from the plan), so the four-call-site coverage is complete.

MILL_REVIEW_BEGIN
# Review: Fix lyx CLI defects + host-commit gap from the sandbox run — holistic

```yaml
verdict: APPROVE
reviewer_model: opushigh
reviewed_file: plan/
date: 2026-07-01
```

## Findings

### [NIT] Card 5 references TestReconcile_DryRun but its file is not in Context
**Location:** Batch 1 / Card 5
**Issue:** Requirements point the implementer at `TestReconcile_DryRun` in `internal/configcli/reconcile_test.go` for the chdir pattern, but Card 5's `Context:` is `none` and that file is not in its `Edits:` — unlike Cards 2/3 which list it in `Context:`. The "chdir pattern used elsewhere in this package" is also slightly inaccurate: muxpoccli's existing `cli_test.go` has no chdir helper.
**Fix:** Add `internal/configcli/reconcile_test.go` to Card 5 `Context:` for parity with Cards 2/3, or drop the cross-package reference since `t.Chdir(t.TempDir())` is idiomatic and the edited `cli_test.go` already demonstrates RunCLI + JSON-envelope parsing.

## Verdict

APPROVE
Plan is accurate, well-grounded, and constraint-clean; one minor context-consistency nit only.
MILL_REVIEW_END