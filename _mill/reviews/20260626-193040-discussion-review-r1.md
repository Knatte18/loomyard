MILL_REVIEW_BEGIN
# Review: Local lyx sandbox for manual experimentation

```yaml
verdict: APPROVE
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-06-26
```

## Findings

### [NOTE] removeAll seam misattributed to tools/deploy
**Section:** Testing → Command-dispatch seam
**Issue:** It says to "mirror `tools/deploy`'s `var removeAll = os.RemoveAll` seam," but `tools/deploy/main.go` has no such seam — the `var removeAll = os.RemoveAll` testability seam lives in `internal/warp/clone.go:30`.
**Fix:** Repoint the reference to `internal/warp/clone.go` (or just describe the package-level-var seam pattern generically); deploy's actual seams are `-dest` and `runtime.Caller`.

### [NOTE] Relative --parent resolution left open
**Section:** Decisions → hub-location ("Open for the plan")
**Issue:** Base for resolving a relative `--parent` (process cwd vs. repo root) is deferred; verified `paths.Getwd()` is plain `os.Getwd()`, so a relative `exec.Command.Dir` resolves from the sandbox process cwd (repo root after `pushd`), matching the recommendation.
**Fix:** Have the plan adopt the stated recommendation (launcher passes absolute `C:\Code`) and make the relative-path semantics explicit so it is not re-litigated.

### [NOTE] Error surface when lyx missing / board unreachable unspecified
**Section:** Scope (Out: deploying lyx) + Decisions → board-precondition
**Issue:** "lyx not on PATH" and "board unreachable → warp tears down Hub" are correctly out-of-scope as auto-fixes, but the discussion does not say whether the tool surfaces the subprocess's stderr/exit so the operator sees a clear cause.
**Fix:** State that the tool propagates `lyx warp clone` exit code and stderr verbatim (no papering over), so these by-design failures are legible.

## Verdict

APPROVE
Well-grounded and decisive; only minor reference/clarity NOTEs, no blocking gaps.
MILL_REVIEW_END