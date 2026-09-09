MILL_REVIEW_BEGIN
# Review: Bump quarry to v0.2.0 and adopt Status.Known()/Rejected()

```yaml
verdict: APPROVE
reviewer_model: opusmedium
reviewer_self_id: Claude Opus 4.x-class model (exact build unknown to me)
reviewed_file: _mill/discussion.md
date: 2026-09-09
```

## Findings

### [NIT:decision] Follow-up record has no durable destination
**Section:** Decisions → scope-planglyph-only **Issue:** The `quarrycli` `describeRejectedResolve` deviation is "recorded as a follow-up", but the only place it is recorded is `_mill/discussion.md`, and `no-doc-changes` rules out roadmap/docs — per CLAUDE.md, worktree-local notes vanish at merge. **Fix:** Name the durable destination (wiki task via mill's wiki module, or a code comment at `internal/quarrycli/resolve.go:80`) or state explicitly that the record is intentionally ephemeral.

### [NIT:consistency] Table column labels are checkIDs, not emitted Check values
**Section:** Testing → drift-guard table **Issue:** The columns are the four `checkID` arms, but the emitted `Finding.Check` for `rename-not-done-old`/`rename-not-done-new` is `"rename-not-done"` for both (`donecheck.go:205,214`), so a plan writer reading the table as an assertion over `Check` strings would write the wrong expectation. **Fix:** State in the table caption that the columns name `doneCheckEntry.checkID` arms and that the two rename arms share one `Check` value distinguished by detail string.

## Verdict

APPROVE
Scope, decisions, and the completeness test's failing mechanism are all concrete and source-accurate.
MILL_REVIEW_END
