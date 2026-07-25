MILL_REVIEW_BEGIN
# Review: dev/test lyx.exe separated from production deploy

```yaml
verdict: APPROVE
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-07-25
```

## Findings

### [NOTE] Guard test self-exclusion unspecified
**Section:** Testing — "Guard test for the new invariant"
**Issue:** The scan across `tools/sandbox/*.go` for `lookPath("lyx")` / `exec.Command("lyx"` / `exec.CommandContext("lyx"` will trip on its own guard-test source (and any future test that documents the forbidden literals), yet the discussion names no self-exclusion — unlike every analogous CONSTRAINTS guard (tierpurity, hermeticenv, sandbox-coverage) which carries an explicit allowlist for its own file.
**Fix:** State that the guard excludes its own file (per-file scan exclusion, matching the tierpurity/hermeticenv precedent) so the new invariant's test data does not self-trip.

### [NOTE] devbin vs deploy repoRoot derivation depth
**Section:** Decisions — shared-devbin-helper / derived-dev-path-never-hardcoded
**Issue:** `tools/deploy/main.go`'s existing `repoRoot()` uses `runtime.Caller(0)` at depth `../..`; the shared helper lives at `tools/internal/devbin`, so its own `runtime.Caller` derivation needs a different depth, and deploy also needs repo root for the build itself (`build.Dir = root`, `go build ./cmd/lyx`) — the discussion leaves whether deploy delegates its build-root to devbin or keeps a second derivation, which is exactly the drift the decision warns against.
**Fix:** Note that deploy should source repo root from the single `devbin` derivation (or state it keeps its own) so the two do not diverge; API detail otherwise correctly deferred to mill-plan.

## Verdict

APPROVE
Thorough, source-grounded, prior gaps resolved; two non-blocking implementation notes only.
MILL_REVIEW_END
