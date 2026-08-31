MILL_REVIEW_BEGIN
# Review: Reconsider the collapsed strand strip default size — holistic

```yaml
verdict: APPROVE
reviewer_model: sonnetxhigh
reviewer_self_id: claude-sonnet-5 (Sonnet 5, per system self-report)
reviewed_file: plan/
date: 2026-08-31
```

## Findings

None.

Verified against source: `config_test.go` lines 61/138 are exactly the two `CollapsedStripRows != 3` assertions cited, with the sibling assertions (`Width`, `Height`, `MinFullRows`, `StrandName`, `DebugLog`, `Mouse`, `Header.HeightRows`) matching what the card says must stay untouched.
`template_posix.yaml`/`template_windows.yaml` line 5 is byte-identical today, confirming the card's "replace with the byte-identical line" instruction is achievable as written.
`doc.go`'s "Silent layout rescale" bullet contains the exact phrase `turned a 3-row collapsed strip into 1 row` (lines 371-372) and it is the only occurrence of that measurement in the package, confirming both the reword target and the "no other occurrence" claim.
`attachgeometry_integration_test.go` asserts pane height against `e.cfg.CollapsedStripRows`/`e.cfg.Header.HeightRows`, never a literal, confirming the "value-agnostic, no edit needed" disposition.
`render` package (`height.go`, `rules.go`, `types.go`) treats `CollapsedStripRows` as an opaque budget with no coupling to `MinFullRows`/`clampHeaderHeight`, confirming the `no-render-or-configsync-change` decision's rationale.
`mill-config.yaml`'s `pipeline.done_gate` is confirmed `go test ./... && go test -tags integration ./...`, backing the `integration-tier-is-a-landing-gate` decision without needing a plan-side gate change.
All 8 named decisions in `_mill/discussion.md`'s `## Decisions` section (strip-default-six, both-templates-lockstep, no-value-migration, no-synthetic-indicator, clamp-path-unchanged, rationale-lives-in-the-template-comment, doc-anecdote-marked-as-then-default, integration-run-is-a-landing-gate) are faithfully carried into Card 1's Requirements or the overview's Shared Decisions.
Both templates carry `text eol=lf` in `.gitattributes`, so no CRLF/LF risk from the Windows template edit.
Single batch, single card, no Moves, no cycles, `All Files Touched` matches the union of Edits exactly, Context is sufficient for every function/constant the Requirements name.

## Verdict

APPROVE
Every specific claim (line numbers, exact quoted text, decision coverage) checks out against source; no constraint violations found.
MILL_REVIEW_END
