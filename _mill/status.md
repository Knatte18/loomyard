# Status

```yaml
phase: approved-merge-drops-weft
slug: weft-local-only-files
branch: weft-local-only-files
plan: _mill/plan
parent: main
task: Add a local-only file category to weft
task_description: |
  Add a local-only file category to weft
```

## Timeline

```text
discussing  '2026-08-27T05:35:03Z'
discussed  '2026-08-27T07:24:47Z'
planning  '2026-08-27T07:40:33Z'
plan-review-r1  '2026-08-27T07:49:19Z'
plan-fix-r1  '2026-08-27T07:50:08Z'
plan-review-r2  '2026-08-27T07:57:03Z'
planned  '2026-08-27T07:57:25Z'
implementing  '2026-08-27T07:57:56Z'
approved-merge-drops-weft  '2026-08-27T08:30:09Z'
```

## Batches

```yaml
batches:
  - name: merge-drops-weft
    state: approved
    implementer_session: c71b0080-aebc-4d32-ae8d-51b91cf9ad40
    start_sha: 89d72540692795868bde40fb0c612db9ba5f4932
    commit_sha: f23616d0a60fa83a81ec082007ff4637bb2d5f19
    verify_baseline_failures: []
  - name: cleanup-raddle-gate
    state: running
    implementer_session: b2882614-b190-4d09-ab62-d67c393cbec4
    start_sha: 555f354595ef9d46db1d54919600c7ecd5334b23
    verify_baseline_failures: []
  - name: shed-commitstatus-seam
    state: pending
    verify_baseline_failures: []
  - name: weft-guards-drop
    state: pending
    verify_baseline_failures: []
  - name: pull-non-fatal-weft
    state: pending
    verify_baseline_failures: []
  - name: push-and-mergestate-probe
    state: pending
    verify_baseline_failures: []
  - name: loom-transition-commit
    state: pending
    verify_baseline_failures: []
```
