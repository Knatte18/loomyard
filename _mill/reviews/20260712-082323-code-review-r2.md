MILL_REVIEW_BEGIN
# Review: Add typed file-ops to lyx's plan-format — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-07-12
```

## Findings

### [BLOCKING] Sandbox suite scenario S9 documents a now-broken plan-format v1 walkthrough
**Location:** `tools/sandbox/SANDBOX-CORE-SUITE.md:297-362` (Scenario S9 — Builder plan validate/status)
**Issue:** S9's hand-written example plan uses `format: 1`, a Batch Index entry with no `(C cards)` segment, `### Card 1` (not `NN.C`), and a `**Where:**` field. With `recognizedFormat=2` and the new mandatory grammar, this plan now fails to parse (unparseable index line / unrecognized card heading) instead of producing the documented `{"ok":true,"valid":true,"batches":1}` result the scenario tells an operator to expect — the walkthrough is actively wrong, not merely stale prose.
**Fix:** Rewrite S9's example plan to valid plan-format v2 (format: 2, `(1 card)`, `### Card 01.1`, all five typed fields) so the documented expected output still holds.

## Notes (non-blocking, apply anyway per repo convention)

### [NOTE] docs/reference/model-spec.md still links "plan-format v1"
**Location:** `docs/reference/model-spec.md:5`
**Issue:** "Pinned alongside [plan-format v1](../modules/plan-format.md)" — the doc it links to is now v2; this cross-reference was missed (file isn't in this task's touched-file list, but the content is now inaccurate).
**Fix:** Update the label to "plan-format v2".

### [NIT] Batch 4's `depends-on: [1]` understates its real dependency on batch 2
**Location:** `_mill/plan/00-overview.md:32-36`, `_mill/plan/04-templates-help.md:9`
**Issue:** Card 15 exists specifically to fix a `buildercli` test regression introduced by batch 2's commit `71b6242`; batch 4 therefore depends on batch 2 (and 3, since batch 3 also edits validate.go) having already landed, not just batch 1. Harmless in practice since lyx executes batches strictly in number order (no DAG), but the annotation is misleading as documentation.
**Fix:** List `depends-on: [1, 2, 3]` (or note the true dependency in Batch Scope prose) for accuracy.

## Verdict

REQUEST_CHANGES
Sandbox suite Scenario S9's plan-format v1 walkthrough now produces incorrect documented results under v2.
MILL_REVIEW_END
