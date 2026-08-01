MILL_REVIEW_BEGIN
# Review: Formalize the Tier 1/2 substrate rule and re-tier mis-tagged tests

```yaml
verdict: APPROVE
reviewer_model: sonnethigh
reviewer_self_id: claude-sonnet-4.5 (self-assessed; exact minor version not independently verifiable)
reviewed_file: _mill/discussion.md
date: 2026-08-01
```

## Findings

### [NOTE] supervised_test.go test-func count off by one
**Section:** Technical context, bullet 4 (`supervised_test.go`'s split)
**Issue:** States the file "already has four test funcs, not two" to justify the stale doc comment claim — source shows five (`TestEnsureSupervised_RetryExhaustionReturnsErrServerSpawnTimeout` L68, `...UncontendedLockWithUndialableHealthyStateReturnsErrServerSpawnTimeout` L136, `...WedgedEscalationReuseReleasesLock` L210, `...StaleSocketCleanupAllowsRebind` L369, `...DaemonLogsToOwnFileNotCallersStderr` L438).
**Fix:** Say "five," not "four" — doesn't change the required action (full doc-comment rewrite), but the implementer should not anchor the rewrite on a miscounted number when re-describing the file's contents.

## Verdict

APPROVE
All load-bearing claims verified against source; prior rounds' GAPs stayed fixed; only a trivial count nit remains.
MILL_REVIEW_END
