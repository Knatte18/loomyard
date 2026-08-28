MILL_REVIEW_BEGIN
# Review: Audit internal/logger coverage across spawn/hard-error paths — holistic

```yaml
verdict: APPROVE
reviewer_model: sonnetxhigh
reviewer_self_id: Claude Sonnet 4.5 (Anthropic), self-assessed
reviewed_file: plan/
date: 2026-08-28
```

## Notes (non-blocking, informational only)

Verified the overview, all five batch files, and every source file the plan cites (line numbers,
function names, existing import lists, existing allowlist maps, existing test names) against the
actual worktree contents. Every specific claim checked out exactly, including several that are easy
to get wrong:

- `treadleengine/judge.go` outcome comparisons at lines 130 and 192, and `treadleengine/targeting.go`
  at line 61 — the plan's corrected line numbers (post `selector-reruns-are-the-authority`) match the
  file exactly, not the discussion's original 131/193/62.
- `shedadapters/bouncer.go` outcome comparisons at lines 473 and 593 match exactly.
- `githubclient/doc.go:82`, `reedengine/doc.go:315`, `reedengine/attach.go:35` are the exact lines
  carrying the literal `exec.Command` substring in prose, confirmed via grep.
- Spawn-call counts per file (`gitexec.go` 1, `gitkit.go` 3, `githubclient/token.go` 1,
  `hubforge/hub.go` 1, `cmd/testtiming/main.go` 1, `tools/deploy/main.go` 3 + `tools/sandbox/*` 4 = 7)
  all match a direct grep.
- `cmd/lyx/tierpurity_test.go`'s `allowedSpawners` map has exactly 13 entries today, matching the
  plan's "fourteenth entry" framing for card 15; `checkedCallMinScannedFiles = 200` is real and cited
  correctly as the vacuous-scan-floor precedent.
- `internal/mergeresolve/seam_enforcement_test.go`'s allowlist does NOT yet admit `logger` (card 4
  correctly adds it); `internal/landingshed/seam_enforcement_test.go`'s allowlist DOES already admit
  `logger` (card 14 correctly says no edit needed there) — the plan gets this asymmetry right in both
  directions.
- Every cited existing test (`TestSingleLLMProducer_OutcomeDiedAndTimeout`,
  `TestResolve_ShuttleOutcomes_MapToStuckNoConclude`, `gitwrap_test.go`'s tagged-internal shape) exists
  as described.
- The "All Files Touched" union in the overview matches the derived union of every card's
  `Edits:`/`Creates:` exactly (25 entries, no drift in either direction).
- Card numbering is sequential 1–15 with no gaps across the five batches; the Batch Index DAG has no
  cycle, references only declared batch names, and every `file:` exists in the plan directory.
- No card contains a non-empty `Moves:`, so the absence of a `## Rename mechanic` section in every
  batch is correct, not a gap.
- Batch 5's card correctly depends on both batch 1 (audit doc + guard's own documentation obligations)
  and batch 3 (every `add`-verdict site must already log before the guard can pass), and batch 3's
  integration-tagged test is reachable via the batch's own `verify:` second clause.

No constraint violations, scope gaps, decision-alignment issues, or requirements-specificity problems
found. This is a very tightly cross-referenced plan; nothing warrants even a NIT.

## Verdict

APPROVE
Every checked claim (line numbers, counts, existing allowlists, existing tests) verified exactly against source.
MILL_REVIEW_END
