MILL_REVIEW_BEGIN
# Review: ly-git-clone hub-creator (host, weft, board) — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-06-24
```

## Findings

### [BLOCKING] cloneHub returns non-empty hubPath on error

**Location:** `internal/gitclone/clone.go:60,65,76`

**Issue:** On clone failure, `cloneHub` returns `(hubPath, teardownHub(...))` — it returns the now-removed (or removal-attempted) hub path instead of `""`. The plan specifies "teardown and return" without preserving hubPath on error. Any future caller checking `hubPath != ""` as a success signal would observe a path that has already been deleted, creating a correctness trap. The current CLI caller (`cli.go`) ignores `hubPath` on error so no functional regression exists today, but the API is misleading.

**Fix:** Change the three failure returns to `return "", teardownHub(hubPath, err)`.

### [NIT] TeardownFailure test does not assert hub path in error message

**Location:** `internal/gitclone/clone_integration_test.go:318-322`

**Issue:** The plan (Card 5) requires `TestCloneHub_TeardownFailure` to "assert the returned error mentions the residual hub path". The test checks only `errMsg != ""` — it does not verify that the error string contains `hubPath`.

**Fix:** Add `if !strings.Contains(errMsg, hubPath) { t.Errorf(...) }` after line 319.

### [NIT] `overview.md` module-tree diagram omits `internal/gitclone/`

**Location:** `docs/overview.md:158-169`

**Issue:** The ASCII tree listing the `github.com/Knatte18/loomyard/` package layout does not include `internal/gitclone/` alongside `internal/board/`, `internal/weft/`, etc., even though the new package is live and the Modules section and dispatch snippet were correctly updated.

**Fix:** Add `├── internal/gitclone/           the git-clone hub-creator` to the tree.

## Verdict

REQUEST_CHANGES
The blocking issue (hubPath returned on error) needs fixing; two nits improve test fidelity and doc completeness.
MILL_REVIEW_END
