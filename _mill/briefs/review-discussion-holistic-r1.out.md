I have enough to assess the discussion. The key issue is a conflict between registering `models.yaml` in configreg (reconcile) and the pinned open-ended-alias extension requirement.

MILL_REVIEW_BEGIN
# Review: Build modelspec - the model-spec parser + registry

```yaml
verdict: GAPS_FOUND
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-07-11
```

## Findings

### [GAP] Reconcile drops operator-added alias entries
**Section:** Decisions §"configreg registration"; Technical context "Reconcile behaviour to verify"
**Issue:** `yamlengine.Reconcile` marshals the template tree and reports every existing leaf-path absent from the template as `removed`, and `configsync.ReconcileAll` writes that pruned result (proven by `TestReconcileAll_DropsStaleMuxClaudeKey`, which drops a top-level `claude:` key) — so an operator-added alias (`zephyr: {engine, model}`) in models.yaml would be silently deleted by `lyx config reconcile --apply`, directly breaking the pinned "new-model-without-recompile" requirement whose property #1 is that models.yaml EXTENDS the registry. The discussion mischaracterizes reconcile as merely "add-missing/idempotent" (line 283) and its proposed test only checks a hand-edited *value* of an existing template key survives, never an operator-*added* alias.
**Fix:** Resolve how open-ended alias entries survive reconcile before planning — e.g. exclude models from `ReconcileAll`'s drop-stale pass, register it read-only for discoverability, or specify an add-only reconcile path — and require a test asserting an operator-added alias entry survives a re-reconcile, not just an edited value.

### [NOTE] Reason to avoid configengine.Load applies equally to configreg reconcile
**Section:** Decisions §"Registry loading — direct read"
**Issue:** The discussion rejects `configengine.Load` because models.yaml "has open-ended alias keys" and a fixed template key set is wrong — yet the configreg/reconcile path it does adopt has the same fixed-template-key model, making the two decisions internally inconsistent.
**Fix:** State explicitly why the open-ended-keys objection that rules out configengine.Load does not also rule out (or must be mitigated in) the configreg reconcile registration.

## Verdict

GAPS_FOUND
Registering models.yaml for reconcile conflicts with the pinned open-ended-alias extension requirement.
MILL_REVIEW_END
