# Status

```yaml
phase: approved-fix-red-packages
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
    state: pending
  - name: tier-purity-guard
    state: pending
  - name: rebaseline-docs
    state: pending
```
