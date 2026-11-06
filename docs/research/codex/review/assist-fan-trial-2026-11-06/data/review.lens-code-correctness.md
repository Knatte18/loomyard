```yaml
lens: code-correctness
outcome: reported
```

- `BeginBatch`, empty or nil findings: `orderOnly` remains true, but `len(blocking) == 0` skips refusal; normal dispatch continues. Correct.
- `BeginBatch`, informational delete-before-reference alone: finding enters advisories, never blocking; normal dispatch continues. Correct.
- `BeginBatch`, blocking delete-before-reference with no begun cards: blocking is nonempty and `orderOnly` stays true; cause is `plan_order`. Correct.
- `BeginBatch`, blocking delete-before-reference after a batch began: the order-only branch takes precedence over begun state; cause remains `plan_order`. Correct.
- `BeginBatch`, multiple blocking delete-before-reference findings: each preserves `orderOnly`; all blocking details are retained. Correct.
- `BeginBatch`, blocking delete-before-reference plus an unrelated informational finding: informational findings do not change `orderOnly`; cause remains `plan_order`. Correct.
- `BeginBatch`, mixed blocking checks in either order: the first non-order check makes `orderOnly` false permanently; cause is `plan_defect` without begun cards and `tree_moved` otherwise. Correct.
- `BeginBatch`, informational delete-before-reference plus a different blocking check: only the other check participates in classification; the existing cause is preserved. Correct.
- `BeginBatch`, a blocking finding with an empty check: it is not recognized as delete-before-reference and preserves the existing cause selection. Correct.
- `BeginBatch`, `ValidateDispatch` failure: the existing restamp attempt runs, then the resolution error returns before classification; no plan-order error masks infrastructure failure. Correct.
- `BeginBatch`, restamp failure with successful resolution: the restamp error returns before classification; no batch is recorded. When both calls fail, the resolution error retains precedence. Correct.
- `PlanConflictError.Error`, `plan_order`: substitutes the move-delete instruction while preserving the class, findings, rebaseline instruction, and requested batch number. Correct.
- `PlanConflictError.Error`, either existing cause or an empty cause: retains the previous match-tree wording. Empty findings join to an empty string, and batch zero renders as `00`, preserving existing formatting behavior. Correct.
- `deleteOrderFindings`, resolved reference: the new constant has exactly the previous literal value, so its blocking finding is unchanged. Correct.
- `deleteOrderFindings`, name-only method reference: the same constant preserves the informational finding. Empty subjects or no matching references still produce no findings. Correct.
- `laterImportFindings`, matching package import: the constant preserves the previous blocking check value. Empty deleting cards, missing files, and unreadable-file errors follow unchanged paths. Correct.
- `dispatchFindingsIndex.ValidateDispatch`: returns its supplied findings, including nil, with nil error; its use does not alter the wrapped index’s other operations. Correct.
- `deleteOrderSites` and `TestLaterDeleteReferences`: replacing the identical check literal with the shared constant preserves their filtering and assertions. Correct.

No findings.

Tool calls: none; the inherited diff, definitions, callers, and failure paths supplied the evidence required for this lens.
