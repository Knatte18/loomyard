The plan is faithful to the source files and decisions.

MILL_REVIEW_BEGIN
# Review: internal/paths: subpath init + mirrored system dirs — holistic

```yaml
verdict: APPROVE
reviewer_model: opushigh
reviewed_file: plan/
date: 2026-06-15
```

## Findings

### [NIT] Untouched consumer tests rely on root-collapse invariant
**Location:** Batch 2 (cards 7-8) / overview "All Files Touched"
**Issue:** `internal/worktree/add_test.go` and `remove_test.go` call `LauncherDir(slug)`/`PortalsDir()`+slug directly (add_test.go:150,188; remove_test.go:116,120,186); they resolve only from hub root, so they stay green via the `RelPath == "."` byte-identity, but neither the plan nor any card acknowledges them.
**Fix:** Add a one-line note in batch 2 scope that these root-level tests are unaffected by the mirror, so an implementer doesn't assume they need editing or panic if they pass unchanged.

### [NIT] Card 6 prune guard wording risks an off-by-one read
**Location:** Batch 2 / Card 6
**Issue:** The stop conditions list both "`cur` equals `stop`" and "`filepath.Rel(stop, cur)` returns `.`"; these are the same condition stated twice, and the card never says whether the equality/Rel check is evaluated before or after the `os.Remove`, which determines whether `stop` itself can ever be removed.
**Fix:** State that the guard (Rel/equality check) is evaluated at loop top before any `os.Remove`, so `stop` is never a remove candidate.

### [NIT] Card 8 menu MkdirAll vs. never-clobber ordering left implicit
**Location:** Batch 2 / Card 8
**Issue:** Original `launchers.go` does the `os.Stat` never-clobber check, returns early if present, and only `MkdirAll`s when absent; card 8 says "keep the never-clobber `os.Stat` check" and "`MkdirAll(filepath.Dir(menuCmdPath))` before writing" without pinning that MkdirAll stays inside the absent-branch (after the early return).
**Fix:** Specify MkdirAll runs only in the menu-absent branch, after the stat early-return, to avoid an MkdirAll on every writeLaunchers call.

## Verdict

APPROVE — climb math, DAG, decisions, and method-split all verified against source; findings are clarifications only.
MILL_REVIEW_END