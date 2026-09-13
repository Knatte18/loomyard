# Status

```yaml
phase: approved-production-prose
slug: hub-suffix-lyxhub
branch: hub-suffix-lyxhub
plan: _mill/plan
parent: main
task: Rename hub container suffix from -HUB to -LYXHUB
task_description: |
  Rename hub container suffix from -HUB to -LYXHUB
```

## Timeline

```text
discussing  '2026-09-13T10:47:44Z'
discussion-fix-r1  '2026-09-13T10:56:43Z'
discussion-fix-r2  '2026-09-13T11:01:32Z'
discussion-fix-r3  '2026-09-13T11:05:29Z'
discussed  '2026-09-13T11:05:29Z'
planning  '2026-09-13T11:18:23Z'
plan-review-r1  '2026-09-13T11:23:48Z'
planned  '2026-09-13T11:24:09Z'
implementing  '2026-09-13T11:24:40Z'
approved-core-constants  '2026-09-13T11:30:10Z'
approved-production-prose  '2026-09-13T11:32:55Z'
```

## Batches

```yaml
batches:
  - name: core-constants
    state: approved
    implementer_session: e1c7ef24-bb7a-419a-9a46-5b3cba70c439
    start_sha: 5e959db65ce02e771efe77b5c94a4dcecbfc70d3
    commit_sha: 420d415a2926ef4a02ddcfcce156dd863e5eeb42
    verify_baseline_failures: []
  - name: production-prose
    state: approved
    implementer_session: 620c4c9c-d5bf-46e8-aae2-97a09be6de63
    start_sha: 05b3776718ba61b28e2f47f3cd5a5f7a84d314be
    commit_sha: f44db2c3c9951942d1e2d8078ce2cfe969366382
    verify_baseline_failures: []
  - name: fabricengine-tests
    state: running
    implementer_session: e97239b7-ef6d-4f3a-aa62-296429215c40
    start_sha: 9c4d5f01e5767584b044cbc020415fc225d3dfc0
    verify_baseline_failures: []
  - name: peripheral-tests
    state: pending
    verify_baseline_failures: []
  - name: sandbox-fixture
    state: pending
    verify_baseline_failures: []
  - name: docs-prose
    state: pending
    verify_baseline_failures: []
  - name: final-sweep-gate
    state: pending
    verify_baseline_failures: []
```
