```yaml
lens: code-tests
outcome: reported
```

Behavior-to-test list:

- Delete-only blocking findings become `plan_order` before any batch begins: `TestBeginBatch_ConflictCauseNamesAPlanOrderConflict/DeleteOrderWithNoBatchBegun` (`internal/websterengine/beginbatch_test.go:838`).
- Delete-only blocking findings become `plan_order` after a batch begins: the `DeleteOrderAfterABatchIsBegun` row (`:837`).
- Mixed blocking findings retain `tree_moved` or `plan_defect`: the two `MixedFindings` rows (`:839–840`), covering both finding orders.
- An informational delete-order finding does not change another blocking finding’s cause: `InformationalDeleteOrderBesideABlockingFinding` (`:841`).
- An unrelated informational finding does not prevent otherwise delete-only blocking findings from becoming `plan_order`: **none**.
- The `plan_order` message names moving the delete and retains the rebaseline step: both delete-order rows assert `moveDelete` through the error message (`:823`, `:862–864`).
- Other causes retain the match-tree remedy: mixed and informational rows assert `matchTree` (`:824`, `:839–841`, `:862–864`).
- Empty and informational-only findings allow dispatch: existing clean-plan and informational-only subtests in `TestBeginBatch_ReResolvesPlanAtDispatch` (`:705`, `:778`).
- Replacing the check literal with `CheckDeleteBeforeReference` preserves resolved, method-name-only, and package-import findings: `TestLaterDeleteReferences`, including its check/card assertion (`internal/planglyph/deleteorder_test.go:38`, `:241`); typed plan-gate and scan-only dispatch findings are also checked by `TestGlyphChain_CallerCoverageTyped` (`internal/planglyph/glyphchain_integration_test.go:308`).
- Master’s immediate stuck outcome for `plan_order`, and the revised stencil recovery wording: static prose, exempt from test requirements.
- Current-tree timing documentation: describes existing validation behavior; adds no executable branch.

Finding:

- **MEDIUM — scope — `internal/websterengine/beginbatch_test.go:837` — No test pins that unrelated informational findings preserve `plan_order`.** The contract classifies the whole **blocking** set, but all successful `plan_order` rows contain only a blocking delete-order finding. The informational row combines an informational delete-order finding with an unrelated blocker, so it cannot detect incorrectly including unrelated advisories in `orderOnly`. Moving the `orderOnly` update outside the severity guard would break this behavior while every added row still passes. Add a row containing a blocking `CheckDeleteBeforeReference` finding and an informational finding with another check, expecting `ConflictCausePlanOrder` and `moveDelete`. Evidence: severity-scoped accumulation at `internal/websterengine/beginbatch.go:500–506`; complete new row set at `internal/websterengine/beginbatch_test.go:837–841`.

Tool calls:

1. Searched engine and CLI tests for `plan_order`, its constant, and the injected-findings fixture; read the new classification table and additional delete-reference test rows to map changed behavior to assertions and confirm the missing advisory case.
