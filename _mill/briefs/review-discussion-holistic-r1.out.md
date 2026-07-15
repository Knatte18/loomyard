MILL_REVIEW_BEGIN
# Review: Reconsider whether lyx mux needs anchor:top at all

```yaml
verdict: APPROVE
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-07-15
```

## Findings

### [NOTE] Stale-comment/help sweep narrower than actual surface
**Section:** Technical context / staging-must-preserve-green-build (step 2)
**Issue:** The doc-sweep is enumerated only as "four values / lists `top_band_rows`", but top-band prose also lives in `policy.go`'s file doc and `partitionByAnchor` doc ("the fixed top-band set"), `rules.go`'s `Rules`/paneOrder comment ("top bands first"), plus the user-facing `add.go` invalid-anchor error string (`add.go:66`, `want top|below-parent|hidden`) and `--anchor` flag usage (`add.go:113`, `placement: top|below-parent|hidden`) — the last two only pinned indirectly via the Testing/Constraints sections, not the file-by-file list.
**Fix:** State that the plan sweeps every `top`/`top-band` reference in retained code comments and CLI help, not just the two enumerated spots, so a plan writer reading Technical context alone does not leave dead references on a leaf whose motivation is "no dead surface".

## Verdict

APPROVE
Complete, source-accurate, and decided; only a doc/help-sweep scoping clarification is worth recording.
MILL_REVIEW_END
