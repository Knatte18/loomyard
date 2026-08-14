MILL_REVIEW_BEGIN
# Review: Move <hub>/.lyx into <hub>/_board — holistic

```yaml
verdict: APPROVE
reviewer_model: sonnetxhigh
reviewer_self_id: claude-sonnet-4.5 (Claude Sonnet 5, per system self-identification)
reviewed_file: plan/
date: 2026-08-14
```

## Method

Read both batches (12 + 13 cards) and the overview in full, then cross-checked every card's factual claims
against the actual source: `junctionnames.go`, `clone.go`, `clone_test.go`, `reedengine/lifecycle.go`,
`serverlog.go`, `reedcli/up.go`, `smoke_debuglog_test.go`, `structuraldirs_test.go`, `junctionnames_test.go`,
`weftgit.go`, `bolt.go`, `fabriccli/clone.go`, `constructoranchoring_test.go`, `uncontainedwrite_test.go`,
`boardjunction_integration_test.go`, `reconcile.go`, `add.go`, `remove.go`, `junction.go`, `unwire.go`
(both packages), `portals.go`, `cli_test.go`, `remove_junctions_integration_test.go`,
`livestate_manifest_test.go`, `doc.go`, `fabric.go`, `docs/overview.md`, `manifest/designs/*.md`, and
`tools/sandbox/SANDBOX-FABRIC-SUITE.md`. Also verified: the `## All Files Touched` union against every
card's Edits/Creates/Move-target set (exact match, 35/35), the Batch Index DAG (2 batches, no cycle, both
`file:` entries present), global card numbering (1–25, no gaps, no dupes), the one `Moves:` pair's format
and its `## Rename mechanic` section, and every card whose file carries `//go:build integration` against
both batches' `verify:` lines (all covered).

Every quoted string, function signature, line-order claim, and cross-file dependency ordering (card 3
before card 4, cards 13→14, cards 15+16 before 17) checked against source matched exactly — including
verbatim doc-comment quotes in `reconcile.go` (`repairPairWiring`, `appendPrDetail`), the
`uncontainedWriteAllowlist` reason string in `cmd/lyx/uncontainedwrite_test.go`, and the sandbox suite's
F8/F9/F15 sections (F13/F10/F14's own `_board` mentions correctly left untouched, since they describe the
hub-level directory rather than the deleted junction).

No constraint violation, decision-implementation gap, sequencing defect, context-completeness gap, or
requirements-vagueness found across either batch.

## Verdict

APPROVE. Plan is fully grounded against source; no findings.
MILL_REVIEW_END
