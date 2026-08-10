# Status

```yaml
phase: approved-path-callsites
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
approved-the-gate  '2026-08-10T18:06:18Z'
self-resolved-verify-logic  '2026-08-10T18:36:09Z'
approved-path-callsites  '2026-08-10T18:40:06Z'
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
    state: approved
    implementer_session: 57f1b8c0-fb0a-4ad3-925a-fcd3190557e4
    start_sha: 926039d1f60bee80af3448f572a021803e903308
    commit_sha: ba49173d15ac92961578bc2a683c8bf3133447bf
    verify_baseline_failures: []
  - name: path-callsites
    state: approved
    implementer_session: 887460a1-bf7c-4879-84a9-c4b13bf87dec
    start_sha: 44a55010ad28ca8198d781b96900f231453d6b6d
    commit_sha: 4f5b147fa36c28e207494ca2801c0180a2209902
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
