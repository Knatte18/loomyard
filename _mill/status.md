# Status

```yaml
phase: approved-probe and clone flip
slug: fabric-warp-binding-in-weft
branch: fabric-warp-binding-in-weft
plan: _mill/plan
parent: main
module_verify_baseline: clean
task: 'fabric: store the warp-URL binding in weft:main; fold bootstrap into clone (slice 10)'
task_description: |
  fabric: store the warp-URL binding in weft:main; fold bootstrap into clone (slice 10)
```

## Timeline

```text
discussing  '2026-08-09T07:56:53Z'
discussion-fix-r5  '2026-08-09T08:43:44Z'
discussed  '2026-08-09T08:43:44Z'
planning  '2026-08-09T09:02:12Z'
plan-review-r1  '2026-08-09T09:11:54Z'
plan-fix-r1  '2026-08-09T09:11:54Z'
plan-review-r2  '2026-08-09T09:23:13Z'
plan-fix-r2  '2026-08-09T09:23:13Z'
plan-review-r3  '2026-08-09T09:34:14Z'
plan-fix-r3  '2026-08-09T09:34:14Z'
plan-review-r4  '2026-08-09T09:44:08Z'
plan-fix-r4  '2026-08-09T09:44:08Z'
plan-fix-r5  '2026-08-09T09:52:55Z'
planned  '2026-08-09T09:53:05Z'
implementing  '2026-08-09T09:53:35Z'
approved-warp-binding core  '2026-08-09T09:59:33Z'
approved-probe and clone flip  '2026-08-09T10:09:22Z'
```

## Batches

```yaml
batches:
  - name: warp-binding core
    state: approved
    implementer_session: ca2ab51a-6492-4b38-ab93-add5f9bd9065
    start_sha: ab0a6449e9c944e416882f2687b765b0fc737eb3
    commit_sha: 1c3a842b2687d0d50a465f2ba27f84fc51316e5a
    verify_baseline_failures: []
  - name: probe and clone flip
    state: approved
    implementer_session: 78df627b-9b79-45fa-84dc-c80ac42b76ac
    start_sha: cd607f18e26347d9af0ab0bd57083aef13bc1e75
    commit_sha: d62efbd204a2d8f8270a4ea4f824307552446750
    verify_baseline_failures: []
  - name: cli surface
    state: running
    implementer_session: a6c66ff5-3d5b-4229-8d62-c8fd075e4dc3
    start_sha: 243f4eaeca7615dd8f23b51809b1f3b778cbded1
    verify_baseline_failures: []
  - name: clone integration tests
    state: pending
    verify_baseline_failures: []
  - name: reconcile backfill
    state: pending
    verify_baseline_failures: []
  - name: docs and sandbox suites
    state: pending
    verify_baseline_failures: []
```
