# Status

```yaml
phase: holistic-reviewing
slug: test-suite-regression
branch: test-suite-regression
plan: _mill/plan
parent: main
module_verify_baseline: clean
task: 'Fix test-suite regression: slow Tier 1 + 2 red packages + stale benchmarks'
task_description: |
  Fix test-suite regression: slow Tier 1 + 2 red packages + stale benchmarks
```

## Timeline

```text
discussing  '2026-07-12T06:15:33Z'
discussion-fix-r1  '2026-07-12T07:00:45Z'
discussed  '2026-07-12T07:01:03Z'
planning  '2026-07-12T07:06:53Z'
plan-fix-r1  '2026-07-12T07:16:34Z'
planned  '2026-07-12T07:16:49Z'
implementing  '2026-07-12T07:17:48Z'
approved-fix-red-packages  '2026-07-12T07:24:10Z'
approved-retier-offline-loop  '2026-07-12T07:43:47Z'
approved-tier-purity-guard  '2026-07-12T07:54:59Z'
approved-rebaseline-docs  '2026-07-12T08:18:47Z'
holistic-reviewing  '2026-07-12T08:19:03Z'
```

## Batches

```yaml
batches:
  - name: fix-red-packages
    state: approved
    implementer_session: ea0af731-2170-45fd-9104-0d267d10a0f2
    start_sha: e82d98084e4a529c9ccaa5d2a1d46a41663adafc
    commit_sha: 8d9e9122693a9ba6e4e7093b9423e5854969a529
  - name: retier-offline-loop
    state: approved
    implementer_session: 9d14fbc4-2936-44fc-9ae1-85be3d059200
    start_sha: e06400c124a16f5fdcac713971b062c7dea41306
    commit_sha: db9543d953f144f758a36ecc806bf5980a2048f2
  - name: tier-purity-guard
    state: approved
    implementer_session: a022da72-e4d2-4c9f-98ff-42a8732ed677
    start_sha: c2df0d1a457641719e1dd0c1af050083b8012cc8
    commit_sha: 4e03204
  - name: rebaseline-docs
    state: approved
    implementer_session: 75428762-e83f-4064-86c6-853eb063a356
    start_sha: fdd0b40a501b8b89adc3651e0833ffaa2878eedf
    commit_sha: d4716bd
```
