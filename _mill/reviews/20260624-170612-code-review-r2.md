MILL_REVIEW_BEGIN
# Review: ly-git-clone hub-creator (host, weft, board) — holistic

```yaml
verdict: APPROVE
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-06-24
```

## Findings

### [NIT] `cloneRepo` wraps nil error in IsDir branch
**Location:** `C:\Code\loomyard\wts\ly-git-clone\internal\gitclone\clone.go:96-99`
**Issue:** When `os.Stat` succeeds but `!info.IsDir()`, the return formats `%w` around a nil error, producing `"parent directory does not exist: <nil>"` — misleading and not required by the plan.
**Fix:** Separate the two conditions: return a plain non-wrapped error string when `!info.IsDir()`, and `%w` the actual `err` only when `err != nil`.

### [NIT] `cloneRepo` deviates from plan's exact `RunGit` call
**Location:** `C:\Code\loomyard\wts\ly-git-clone\internal\gitclone\clone.go:88-105`
**Issue:** Plan Card 3 specifies `git.RunGit([]string{"clone", url, dest}, filepath.Dir(dest))` passing the full absolute `dest`; the implementation passes only the basename (`gitDest = filepath.ToSlash(filepath.Base(dest))`).
**Fix:** Functionally equivalent so no correctness risk; adding `// dest is split into parent+basename so git resolves cleanly on Windows` comment would clarify the intentional divergence.

### [NIT] Board URL re-derived in `cli.go` independently of `cloneHub`
**Location:** `C:\Code\loomyard\wts\ly-git-clone\internal\gitclone\cli.go:49-51`
**Issue:** `cli.go` calls `deriveBoardURL(weftURL)` to populate the JSON response after `cloneHub` already did the same internally; if `cloneHub`'s resolution logic ever diverges, the JSON `board` field would not reflect what was actually cloned.
**Fix:** Consider returning `(hubPath, resolvedBoardURL string, err error)` from `cloneHub` so the CLI uses exactly the URL that was cloned; or document that both calls are intentionally identical within the same package.

## Verdict

APPROVE
All plan cards are fully realised; path-invariant, dormant-hub, and JSON-output decisions are consistently applied; no blocking issues found.
MILL_REVIEW_END
