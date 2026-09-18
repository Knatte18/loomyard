# Review: Replace reed's header pane with a status-line and Selvage

```yaml
verdict: APPROVE
reviewer_model: orchestrator
reviewed_file: _mill/discussion.md
date: 2026-09-18
```

## Findings

### [NIT:consistency] config-reconcile gotcha contradicts actual reconcile behavior
**Demoted-from:** BLOCKING
**Section:** Decision `config-block-replaces-header-with-status_line-and-selvage`, "Gotcha for the plan"
**Issue:** The discussion states `lyx config reconcile` "is key-based and additive — it adds the new keys but does not remove a stale `header:` block from an already-materialized `reed.yaml`," and tells the plan "do not write removal logic for it." This is false: `internal/configsync/configsync.go`'s `ReconcileAll` calls `yamlengine.Reconcile(template, existing)`, which returns both `added` **and** `removed` key-paths and merges only what the template still declares — `configsync_test.go`'s `TestReconcileAll_DropsStaleReedClaudeKey` pins exactly this behavior for a prior stale top-level key (`claude:`) that the template stopped declaring. A stale `header:` block is structurally identical and would be stripped the same way once the template drops it, whenever reconcile runs with `apply=true` (`lyx config reconcile --apply`; not automatic on every `reed up`).
**Suggested fix:** Correct the gotcha to state reconcile *does* remove a stale `header:` block on explicit `lyx config reconcile --apply`, note it is not automatic on `up`/`down`, and add a reconcile-removal case to the config test list in Testing (currently only "unmarshalling cleanly and being ignored" is planned, which assumes the wrong mechanism).

## Verdict

APPROVE
One grounded factual error about `lyx config reconcile`'s removal behavior needs correcting before planning.
_Note: 1 finding(s) demoted from BLOCKING to NIT by the stage's blocking-class ceiling; current blocking_count is 0._
