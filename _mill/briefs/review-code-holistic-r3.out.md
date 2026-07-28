MILL_REVIEW_BEGIN
# Review: native clients: migrate gitrepo to go-git + selfreport gh-CLI to go-github — holistic

```yaml
verdict: APPROVE
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-07-28
```

## Findings

### [NIT] Stale "unused/untested" comments in gogit.go
**Location:** `internal/gitrepo/gogit.go:99-104,181,233`
**Issue:** `goGit`, `lookupObjectRetrying`, and `packFingerprint` still carry batch-1-era doc text and `//nolint:unused` pragmas claiming "no migrated read calls it yet in this batch" and "only exercised by gogit_test.go." That was true after batch 1 but is now false: `gitrepo.go`'s `CurrentSHA`/`SHAExists`/`CurrentBranch`/`ChangedFilesSince` and `snapshot.go`'s `remoteName`/`SnapshotSHA`/`isStrictDescendant`/`SetSnapshotSHA` all call these unconditionally from non-test, untagged production code as of batches 3-4.
**Fix:** Update the comments (and drop the now-unnecessary `nolint:unused` pragmas) to reflect that these are live production call paths, not just integration-test-only helpers.

### [NIT] Dead `oracleHasUnpushed` left behind after the reversal
**Location:** `internal/gitrepo/oracle_test.go:180-196`
**Issue:** Card 21 (batch 5) reverted `hasUnpushed` to the CLI and deleted its parity test cases from `gogit_test.go`/`parity_test.go` per the reversal criterion, but `oracleHasUnpushed` — the oracle function those cases called — was not itself removed (it wasn't on card 21's `Edits:` list) and now has zero callers anywhere in the package.
**Fix:** Delete `oracleHasUnpushed` along with its `errOracleNoCommits`-adjacent sentinel usage, consistent with card 21's "delete them rather than keeping them as self-checks" instruction for the rest of the hasUnpushed parity surface.

## Verdict

APPROVE
Full plan realized correctly end-to-end; only two cosmetic dead-code/stale-comment NITs found, no functional or invariant issues.
MILL_REVIEW_END
