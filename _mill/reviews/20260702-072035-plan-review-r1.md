I have verified all plan claims against the source files. Let me compile the review.

MILL_REVIEW_BEGIN
# Review: Expand the sandbox suite: subfolder init, weft, warp, config reconcile + coverage invariant — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opushigh
reviewed_file: plan/
date: 2026-07-02
```

## Findings

### [BLOCKING] Wrong filepath.Dir walk-up count for repo root
**Location:** Batch 2 / Card 4, step (b)
**Issue:** Card 4(b) says resolve repo root via "two `filepath.Dir` walk-ups ... the same technique `cmd/lyx/registration_test.go` uses", but a file at `cmd/lyx/X.go` needs **three** `filepath.Dir` calls, and `registration_test.go:71` in fact uses three (`filepath.Dir(filepath.Dir(filepath.Dir(testFile)))`) — its own comment saying "two" is stale; the code is authoritative. Following the literal "two" yields `.../cmd`, so `os.ReadFile(<root>/tools/sandbox/SANDBOX-SUITE.md)` fails and the new test cannot find the doc.
**Fix:** State three `filepath.Dir` walk-ups (matching registration_test.go's code), not two.

## Verdict

REQUEST_CHANGES
One concrete path-resolution error would break the only test the task ships; otherwise the plan is sound.
MILL_REVIEW_END