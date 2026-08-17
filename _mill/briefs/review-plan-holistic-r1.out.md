MILL_REVIEW_BEGIN
# Review: shuttleengine + reedengine + tokenvocab told-geometry — holistic

```yaml
verdict: APPROVE
reviewer_model: sonnetxhigh
reviewer_self_id: claude-sonnet-5 (Sonnet 5)
reviewed_file: plan/
date: 2026-08-17
```

## Summary

Reviewed the overview and all four batches (12 cards) against the referenced source files: `fabricengine/junctionnames.go`+`hubscratch_test.go`, `reedengine/{lifecycle,lock,server,strand,doc,state,name}.go` and their tests, `reedcli/smoke_debuglog_test.go`, `cmd/lyx/constructoranchoring_test.go`, `tokenvocab/*.go`, `reedengine/header*.go`, all five `reedengine.New`/`shuttleengine.NewRunner` CLI construction sites (burlercli, perchcli, webstercli, shuttlecli, reedcli), `shuttleengine/{run,rundir,wait,doc,spec}.go` and their tests, `fakes_test.go`, the `websterengine.FindRun` call sites (`recoverbatch.go`, `runlevel.go`), `docs/overview.md`'s package tree, and `manifest/designs/producers-standalone.md`'s T3/T6/T7 sections.

Every load-bearing textual claim checked against source was accurate: line/row citations (`constructoranchoring_test.go` rows 96/144, the two `HubLogsDir` call sites in `smoke_debuglog_test.go`, the five `reedengine.New`/four `NewRunner` CLI sites, the seven `NewRunner` test call sites, the two `websterengine.FindRun` sites), the `AnchorPath`/`anchorRoot` naming conflict with `producers-standalone.md:291-300` (correctly identified and overridden with a stated rationale), and the `internal/hubgeom` scope-creep concern the orchestrator's own prior review (`_mill/orch-review.md`) raised — the plan's card 10 adds the exact T6/T7 breadcrumb lines that review asked for.

Batch DAG is acyclic and every `file:`/`depends-on` entry resolves (batch 1 and 2 are independent roots sharing no file; batch 3 correctly depends on both — card 8's `header.go` edit builds on batch 2's `tokenvocab.Ctx` shape, `hubgeom` on batch 1's `fabricengine.HubLogsDir`; batch 4 correctly depends on batch 3 for `Geometry.AnchorPath`/`WorktreeRoot`). Global step numbering (1–12) is sequential with no gaps. Every card carries all six required fields; `Moves: none` throughout, so the rename-mechanic section is correctly absent. The overview's `## All Files Touched` list is a complete, accurate union of every card's `Edits:`/`Creates:` targets — cross-checked file by file. Requirements consistently name concrete identifiers (struct fields in declared order, exact call-site replacements, exact doc-comment obligations) rather than vague prose, and Context lists cover what Requirements reference. The "told geometry, never validated," "no additive twins," "one teller" (`hubgeom.ReedGeometry`), and "byte-identical behaviour" shared decisions are consistently honored card-to-card, including the explicit swap-detection fixture discipline (distinct anchor/worktree values) carried through reed's and shuttle's test conversions alike. Integration-tagged test files touched by batch 3/4 (`contract_integration_test.go`, `mouse_boot_integration_test.go`, `recoverbatch_test.go`, `verbs_test.go`) are correctly run for real by each batch's `verify:`, not merely vet-checked, matching their `//go:build integration` tags.

No constraint violations, decision misalignments, or context-completeness gaps found.

## Verdict

APPROVE
Plan is accurate, internally consistent, and fully grounded in the cited source across all four batches.
MILL_REVIEW_END
