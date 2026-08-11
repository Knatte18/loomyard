MILL_REVIEW_BEGIN
# Review: format docs: name their producers and contracts in producer-model terms, add Discussion-Review-Gate — holistic

```yaml
verdict: APPROVE
reviewer_model: sonnetxhigh
reviewer_self_id: claude-sonnet-5
reviewed_file: plan/
date: 2026-08-11
```

## Findings

### [NIT:consistency] Card 4's quoted anchor block is pre-edit text, not the quoted-text anchor the plan's own convention calls for
**Location:** batch 1, card 4 **Issue:** The Shared Decision `line-numbers-in-this-plan-are-pre-edit-anchors` says a later card locates its target "by the quoted text, not by the stale number," but card 4's fenced quote of `:14` reproduces the pre-card-2 wording ("is what the Plan producer should ever see") even though card 2 (which runs first) will already have substituted `Plan-Write` there. **Fix:** none required — the card's own parenthetical already flags the discrepancy ("Card 2 has already substituted `Plan-Write`..."), so an implementer is not misled; note only for polish.

I re-verified every `:NN` line citation across both batches against the live file contents (`discussion-format.md`, `plan-format.md`, `loom.md`, `shed.md`, `roadmap.md`, `shed-followups.md`) via direct reads and targeted greps, including the full "the Plan producer" / "the Discussion producer" / "discussion-review gate" / "the planner" enumeration, the `Plan-Review-Gate` (8 occurrences, 4 files) and `Discussion-Review-Gate` (9 occurrences, `shed-followups.md` only) blast radii, the `Discussion-Validate` producer-table cell content, and the `websterengine/doc.go` + `recordbatch.go` + `docs/overview.md:60` citations backing card 4. All line numbers, quoted text blocks, and grep-based acceptance criteria in both batches match the actual source content exactly, and every `### Decision:` in `_mill/discussion.md` is faithfully carried into the corresponding card. The Batch Index DAG (1 → 2, no cycle), `verify:` command (`TestEnforcement_MarkdownLinks`, confirmed present in `internal/lyxcwd/docslink_test.go`), and `## All Files Touched` union are all correct. No `Moves:` anywhere, so no `## Rename mechanic` section is required or missing.

## Verdict

APPROVE
Plan is precise, fully grounded against source files, and faithfully implements every discussion Decision; only a cosmetic nit found.
MILL_REVIEW_END
