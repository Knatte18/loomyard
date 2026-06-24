MILL_REVIEW_BEGIN
# Review: ly-git-clone hub-creator (host, weft, board) — holistic

```yaml
verdict: APPROVE
reviewer_model: opushigh
reviewed_file: plan/
date: 2026-06-24
```

## Findings

### [NIT] cloneRepo runFromDir param is superfluous
**Location:** Batch 1 / Card 3
**Issue:** `cloneRepo(url, dest, runFromDir string)` passes `runFromDir` as cwd to `git.RunGit`, but `dest` is always an absolute path (`filepath.Join(hubPath, …)`), so the clone cwd is irrelevant.
**Fix:** Either drop the `runFromDir` parameter or document that it exists only to satisfy `RunGit`'s signature; not blocking.

### [NIT] MkdirAll failure path unspecified
**Location:** Batch 1 / Card 3 step (4)
**Issue:** The card specifies teardown for clone failures but does not say what `cloneHub` returns if `os.MkdirAll(hubPath, …)` itself fails (a partial/empty dir may be left).
**Fix:** State that a MkdirAll error returns the wrapped error directly (no teardown needed since no clone ran).

### [NIT] deriveHostName trailing-slash edge not covered
**Location:** Batch 1 / Card 2 `TestDeriveHostName`
**Issue:** The four table cases omit a trailing-slash URL (`…/repo/`), whose final segment is empty and would yield `""`.
**Fix:** Add a trailing-slash case (or explicitly note it as out-of-contract) so the empty-result branch is pinned.

### [NIT] overview.md module-dispatch block not updated for git-clone
**Location:** Batch 2 (docs) vs Card 6
**Issue:** Card 6 adds `git-clone` to `main.go`'s package-doc Modules list, but `docs/overview.md`'s "Module dispatch" switch snippet (lines 182-199) and Modules list still omit it; batch 2 only touches the roadmap and the weft model.
**Fix:** Optionally extend Card 8 (or a new card) to add the `git-clone` case to the overview dispatch snippet; the snippet is illustrative, so non-blocking.

## Verdict

APPROVE
Plan is constraint-clean, source-grounded, and well-sequenced; only minor NITs remain.
MILL_REVIEW_END
