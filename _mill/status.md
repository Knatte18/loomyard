# Status

```yaml
phase: approved-dirtiness-probe
slug: fabric-destructive-chokepoint
branch: fabric-destructive-chokepoint
plan: _mill/plan
parent: main
module_verify_baseline: clean
task: 'fabric: one ownership-and-dirtiness gate for all destruction (slice 12)'
task_description: |
  fabric: one ownership-and-dirtiness gate for all destruction (slice 12)
```

## Timeline

```text
discussing  '2026-08-10T10:16:14Z'
discussed  '2026-08-10T12:41:06Z'
planning  '2026-08-10T12:59:08Z'
plan-review-r1  '2026-08-10T13:11:42Z'
plan-fix-r1  '2026-08-10T13:11:42Z'
plan-review-r2  '2026-08-10T13:22:31Z'
plan-fix-r2  '2026-08-10T13:22:31Z'
plan-review-r3  '2026-08-10T17:06:58Z'
plan-fix-r3  '2026-08-10T17:06:58Z'
plan-review-r4  '2026-08-10T17:18:00Z'
plan-fix-r4  '2026-08-10T17:18:00Z'
plan-fix-r5  '2026-08-10T17:31:51Z'
planned  '2026-08-10T17:32:03Z'
implementing  '2026-08-10T17:43:28Z'
approved-dirtiness-probe  '2026-08-10T17:53:36Z'
```

## Batches

```yaml
batches:
  - name: dirtiness-probe
    state: approved
    implementer_session: 2c58d66a-c805-4f7d-b687-de9606e95776
    start_sha: bb9aa21012d0c08cc19d94e5d03eadefe53335ef
    commit_sha: c248d402fa9a0ab171a97a52bfeef8ac3a325469
    verify_baseline_failures: []
  - name: the-gate
    state: pending
    verify_baseline_failures: []
  - name: path-callsites
    state: pending
    verify_baseline_failures: []
  - name: clone-callsites
    state: pending
    verify_baseline_failures: []
  - name: branch-callsites
    state: pending
    verify_baseline_failures: []
  - name: guard-and-docs
    state: pending
    verify_baseline_failures: []
  - name: gap-integration-tests
    state: pending
    verify_baseline_failures: []
```
