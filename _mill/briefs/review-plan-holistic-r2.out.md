MILL_REVIEW_BEGIN
# Review: Formalize the Tier 1/2 substrate rule and re-tier mis-tagged tests — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnetxhigh
reviewer_self_id: Claude Sonnet 4.5 (Sonnet 5 per system context)
reviewed_file: plan/
date: 2026-08-01
```

## Findings

### [BLOCKING] Card 4's Sleep-guard wiring instructions leave the check unreachable
**Location:** batch `tierpurity-guard-generalization`, Card 4
**Issue:** The current `TestTierPurity_UntaggedTestsSpawnNothing` walk callback returns `nil` on every branch of the existing banned-token check (`!bad` → return; `spawnerAllowed`/`pathAllowlisted` true → return; append-then-return on the flagged path). Card 4 says to add the new Sleep check "immediately after the existing banned-token check block" so it "runs regardless of whether the banned-token check already flagged this file" — but literally placing new statements after a block whose every path already returns makes that new code unreachable, meaning `findLongLiteralSleep` would never fire against the real repo tree for ANY file, not just banned-token-flagged ones. Batch 1's own `verify:` (three `-run` filters, no `go vet`) would not catch this: `TestFindLongLiteralSleep` unit-tests the helper directly, and `TestTierPurity` would simply pass trivially (never having reached the new branch), silently defeating the batch's own claim that this run "proves the reedengine/reedcli allowlist entries necessary."
**Fix:** Add an explicit instruction to Card 4 to remove/restructure the banned-token check's three early `return nil` statements (turn the `!bad`/`spawnerAllowed` guards into simple conditionals with no return, and drop the `return nil` after the `failures` append) so both checks execute unconditionally per file before a single trailing `return nil`.

### [NIT] Batch 2's dependency rationale cites the wrong card number
**Location:** batch `scoutengine-scout-tag`, Batch Scope
**Issue:** "...because this batch's Card 6 edits the same `allowedSpawners` map batch 1 leaves in place" — Card 6 only retags `ensureserver_integration_test.go` and never touches `allowedSpawners`; the card that edits `cmd/lyx/tierpurity_test.go`'s `allowedSpawners` map is Card 11.
**Fix:** Change "Card 6" to "Card 11" in the Batch Scope prose.

### [NIT] Card 11's updated allowlist reason understates post-split usage
**Location:** batch `scoutengine-scout-tag`, Card 11
**Issue:** After Card 10's split, `spawnAndHoldSubprocess` is still called by all three remaining subtests in `supervised_test.go` (`TestEnsureSupervised_RetryExhaustionReturnsErrServerSpawnTimeout`, `TestEnsureSupervised_UncontendedLockWithUndialableHealthyStateReturnsErrServerSpawnTimeout`, `TestEnsureSupervised_WedgedEscalationReuseReleasesLock`), not only the retry-exhaustion one — but Card 11's new reason string names only "the retry-exhaustion PID-liveness fixture," inconsistent with Card 10's own header-comment rewrite ("each remaining subtest spawns one real short-lived child process via `spawnAndHoldSubprocess`...").
**Fix:** Broaden Card 11's reason string to cover all three remaining subtests' shared use of the fixture, not just the retry-exhaustion one.

## Verdict

REQUEST_CHANGES
Card 4's wiring instructions risk silently unreachable code for the new Sleep guard; two card-numbering/wording nits in batch 2.
MILL_REVIEW_END
