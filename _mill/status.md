# Status

```yaml
phase: approved-sandbox-resolve-core
slug: dev-test-binary
branch: dev-test-binary
plan: _mill/plan
parent: main
module_verify_baseline: clean
task: dev/test lyx.exe separated from production deploy
task_description: |
  dev/test lyx.exe separated from production deploy
```

## Timeline

```text
discussing  '2026-07-25T07:04:32Z'
discussion-fix-r3  '2026-07-25T07:39:53Z'
discussed  '2026-07-25T07:40:25Z'
planning  '2026-07-25T07:55:49Z'
plan-review-r1  '2026-07-25T08:02:04Z'
plan-fix-r1  '2026-07-25T08:02:04Z'
plan-review-r2  '2026-07-25T08:07:42Z'
plan-fix-r2  '2026-07-25T08:07:42Z'
plan-review-r3  '2026-07-25T08:12:49Z'
plan-fix-r3  '2026-07-25T08:12:49Z'
plan-fix-r4  '2026-07-25T08:17:55Z'
planned  '2026-07-25T08:18:27Z'
implementing  '2026-07-25T08:19:34Z'
approved-devbin-and-deploy  '2026-07-25T08:23:13Z'
approved-sandbox-resolve-core  '2026-07-25T08:26:14Z'
```

## Batches

```yaml
batches:
  - name: devbin-and-deploy
    state: approved
    implementer_session: 4775aea4-5b47-494c-a9f8-e4ab5e7a7d54
    start_sha: dcc1c9a47952d7b10fd715c457144485e0c5aaea
    commit_sha: 78db2cfd181013f79602ac7a7bf7168bc73b0a2a
  - name: sandbox-resolve-core
    state: approved
    implementer_session: 2bd59fa8-cb88-4fbc-9551-fc6cbd8b2fa3
    start_sha: 1ca64e81f1b8a3f52c0e880b21b5a71ae029e45d
    commit_sha: 11a7e9e248ea70d73ae37b0f8339d8166342f48c
  - name: sandbox-wire-and-guard
    state: running
    implementer_session: 9adc0d8e-7ba1-4ece-8764-13c8c209c64b
    start_sha: c681fea82b39d1e40929711e2748d73fb04a6f20
  - name: launchers-and-lifecycle
    state: pending
  - name: crucible-sweep
    state: pending
  - name: suite-docs-sweep
    state: pending
```
