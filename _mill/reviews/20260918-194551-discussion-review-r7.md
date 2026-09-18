MILL_REVIEW_BEGIN
# Review: reed: AddStrand and attach self-heal a cold worktree

```yaml
duration_s: 176.6
verdict: APPROVE
reviewer_model: opusmedium
reviewer_self_id: claude-opus-5
reviewed_file: _mill/discussion.md
date: 2026-09-18
```

## Findings

### [NIT:decision] loadOrInitStateLocked's route enumeration goes stale
**Demoted-from:** BLOCKING
**Section:** Scope → "Doc updates in the same commit" / Technical context
**Issue:** `internal/reedengine/spawn.go:210-212`'s doc comment enumerates the routes into it verbatim — "Every call site reaches here with the told session already up — Up/Resume via `ensureServerAndSessionLocked`, every other op via `requireSessionLocked`" — and `AddStrand` reaching it via the new `ensureSessionLocked` falsifies that list; the site appears in no doc-update list and no grep instruction (the doc.go grep is scoped to `doc.go` alone).
**Fix:** Name `spawn.go`'s `loadOrInitStateLocked` doc comment as an in-scope comment edit, or widen the grep instruction to `requireSessionLocked` across all of `internal/reedengine`, not just `doc.go`.

### [NIT:decision] Existing foreign-session smoke test not dispositioned
**Demoted-from:** BLOCKING
**Section:** Testing → "Foreign-session refusal survives" / `foreign-session-refusal-preserved`
**Issue:** `internal/reedcli/smoke_staterecovery_test.go:299` (`TestSmokeDiagnosticVerbsNameTheOrphanSessionRatherThanPointingAtResume`) already drives `status`, `attach` and `add` against the R5-F5 renamed-worktree fixture and asserts the orphan-session refusal, under a comment framing all three as "Every verb that goes through `requireSessionLocked` rather than through a boot" — a premise this task falsifies for two of the three; the discussion proposes new R5-F4/R5-F5 tests without saying whether this one is extended, re-commented, or duplicated, and it falls outside both stated sweep rules (it asserts the foreign-session text, not `no reed session`, and it lives inside `reedcli`, which the repo-wide `AddStrand` grep excludes).
**Fix:** State the disposition for this test explicitly — comment rewrite plus whether the proposed "Foreign-session refusal survives" case extends it or is a new sibling.

### [NIT:scope] Third sandbox hit exists today, not just as drift
**Section:** Constraints → Sandbox Suite Coverage
**Issue:** The section says "This is not a 'check whether anything needs updating' item; the hits are known" and lists two, but `tools/sandbox/SANDBOX-SHUTTLE-SUITE.md:178` ("it will fail at `add strand: no reed session` -- that is expected and costs no tokens") is a third present-day hit falsified by `shuttle-inherits-the-self-heal`, and its "costs no tokens" claim inverts — the second run would now boot a session and spawn an agent.
**Fix:** Add SHUTTLE-SUITE S5 to the known-hit list so the token-cost claim is dispositioned rather than left to the sweep grep.

## Verdict

APPROVE
Two named artifacts carry falsified premises with no stated disposition.
_Note: 2 finding(s) demoted from BLOCKING to NIT by the stage's blocking-class ceiling; current blocking_count is 0._
MILL_REVIEW_END
