MILL_REVIEW_BEGIN
# Review: Optimise and slim the test suite

```yaml
verdict: APPROVE
reviewer_model: opushigh
reviewed_file: C:\Code\loomyard\wts\optimize-test-suite\_mill\discussion.md
date: 2026-06-20
```

## Findings

### [NOTE] paths helpers_test.go untagged-but-spawns risk
**Section:** Decisions › build-tag gating
**Issue:** `internal/paths/helpers_test.go` exists (carries `newTestRepo`) but is not named in the explicit classification list, only covered by the blanket "helpers migrate to lyxtest" clause; if any case-bearing test remains in paths after migration, the untagged/offline guarantee could leak.
**Fix:** Plan should confirm paths `helpers_test.go` is fully drained into lyxtest (no residual `func Test...` body) so the default loop stays subprocess-free.

### [NOTE] Equivalence guardrail diff is manual
**Section:** Testing › Equivalence guardrail
**Issue:** Pre/post snapshot-and-diff of `-list` and `=== RUN` leaves is described but the comparison method (eyeball vs scripted diff, and where snapshots live given the read-only/no-temp-files constraints elsewhere) is unstated.
**Fix:** Name the diff mechanism (e.g. saved text files diffed locally, not committed) so coverage-loss detection is reproducible.

### [NOTE] post-copy origin url rewrite assumes single-line config
**Section:** Decisions › fixture-amortisation
**Issue:** Rewriting the `[remote "origin"] url` line as a text edit (no `git remote set-url`) is sound, but the discussion doesn't note the assumption that the template's `.git/config` has exactly one origin url line and stable formatting.
**Fix:** Add a one-line invariant (template config shape is fixed/known) so the text-edit rewrite is unambiguous for the implementer.

## Verdict

APPROVE
Scope, decisions, and source claims all verify; only minor non-blocking clarifications noted.
MILL_REVIEW_END
