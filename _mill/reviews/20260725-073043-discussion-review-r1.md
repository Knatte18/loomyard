MILL_REVIEW_BEGIN
# Review: dev/test lyx.exe separated from production deploy

```yaml
verdict: GAPS_FOUND
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-07-25
```

## Findings

### [GAP] muxDown PATH-prepend appears redundant
**Section:** Decisions › agent-path-prepend-child-only; Scope
**Issue:** `muxDown` already execs the absolute resolved `lyxPath` (suite.go:208), and `lyx mux` re-invokes lyx via `os.Executable()` not bare `lyx` (muxengine/lifecycle.go:524, headerpane.go), so prepending `.dev-bin` to its PATH resolves no lyx lookup — unlike `launchAgent`, whose agent types bare `lyx`.
**Fix:** State why `mux down` needs the PATH prepend (does it shell bare `lyx`?), or drop the muxDown env-threading + its test and keep only the absolute-path pass-through.

### [NOTE] cloneRun path threading and stale error text unspecified
**Section:** Technical context (cloneRun); Decisions › resolution-with-path-fallback
**Issue:** Current `cloneRun` seam is `func(parentDir string) error` and hardcodes `exec.Command("lyx", ...)` with a "lyx not found on PATH" startup message (main.go:33-45); the discussion says exec the `resolveLyx()` result but not how the resolved path reaches the seam (signature change vs internal resolveLyx call) nor that the message goes stale.
**Fix:** Name the intended seam shape (e.g. `cloneRun(parentDir, lyxPath)`) and note the startup-error string needs updating.

### [NOTE] resolveLyx existence-only check reintroduces the footgun for dev
**Section:** Decisions › resolution-with-path-fallback
**Issue:** `resolveLyx` selects `.dev-bin/lyx` on `os.Stat` existence alone, so a partial/aborted `go build -o .dev-bin/lyx` (the exact clobber risk the task targets for prod) yields a broken dev binary silently chosen.
**Fix:** Note that dev staleness is accepted-and-surfaced-by-fingerprint (deliberate), or state any freshness guard is out of scope.

## Verdict

GAPS_FOUND
One gap: muxDown's PATH-prepend rationale is unjustified given it already execs the absolute resolved path.
MILL_REVIEW_END
